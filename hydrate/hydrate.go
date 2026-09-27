package hydrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/layer"
)

// Hydrator 填充器(spec §4.2):渲染前的独立纯结构步骤。
//
// 数据集裁决(spec 未定义态的裁定,PHP 权威实现同款):dataset == nil = 未绑
// 数据集,全画布恒等直通——含模板表的画布不填充,模板表以声明态进入渲染、
// 按零行空壳呈现(与 §4.4「空数组 = 合法零行、模板空数据不特殊」同精神);
// 传具体数据 = 已绑,填充语义全量生效(含 rows_path_invalid 等结构性报错)。
//
// 断行/度量不做注入:TextLayer 自携带 LineBreaker/Measurer 策略,高度定稿经
// canvas.FromGraph 的解码路径完成(V1 高度耦合重放,spec §4.2 字面偏离但
// 功能等价,与 PHP 填充器同款裁决)
type Hydrator struct {
	evaluator ExpressionEvaluator
}

// NewHydrator 构造填充器;evaluator 为 nil 时用默认受限插值求值器
func NewHydrator(evaluator ExpressionEvaluator) *Hydrator {
	if evaluator == nil {
		evaluator = NewInterpolationEvaluator()
	}
	return &Hydrator{evaluator: evaluator}
}

// Hydrate 填充:全画布标记字段求值 + 模板表实例化。填充走 graph 层改写 +
// canvas.FromGraph 重建,求值结果永不回写源 graph;identity 场景返回原画布
func (h *Hydrator) Hydrate(c *canvas.Canvas, dataset any) (*canvas.Canvas, error) {
	// 未绑数据集:不跑求值器,全字面(标记字段按 value 镜像显示原文),存量行为零变化
	if dataset == nil {
		return c, nil
	}

	graph := c.Graph()
	if !graphHasBindings(graph.Layers) {
		return c, nil // 无标记无模板:恒等直通
	}

	root := normalizeDataset(dataset)
	if err := assertReservedRootKeys(root); err != nil {
		return nil, err
	}

	for i := range graph.Layers {
		if err := h.hydrateNode(&graph.Layers[i], root, nil); err != nil {
			return nil, err
		}
	}

	return canvas.FromGraph(graph)
}

// hasTemplate 模板键在场判定:"template": null 与缺键同判(PHP isset 同款)
func hasTemplate(raw json.RawMessage) bool {
	return raw != nil && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// graphHasBindings 图上是否存在表达式标记或模板表(决定是否进入填充流程)
func graphHasBindings(nodes []layer.Node) bool {
	for i := range nodes {
		node := &nodes[i]
		if node.Data != nil && node.Data.ValueType == layer.ValueTypeExpression {
			return true
		}
		if hasTemplate(node.Template) {
			return true
		}
		if node.Rows != nil && graphHasBindings(*node.Rows) {
			return true
		}
		if node.Cells != nil && graphHasBindings(*node.Cells) {
			return true
		}
		if node.Content != nil && !bytes.Equal(bytes.TrimSpace(node.Content), []byte("null")) {
			var contentNode layer.Node
			if err := json.Unmarshal(node.Content, &contentNode); err == nil &&
				graphHasBindings([]layer.Node{contentNode}) {
				return true
			}
		}
	}
	return false
}

// hydrateNode 填充单个 graph 节点:求值标记字段 + 结构子树递归(行/格/内容同作用域传递)。
// rowNamespace 为 nil = 非行上下文,非 nil = 行上下文("row" + "$index")
func (h *Hydrator) hydrateNode(node *layer.Node, root map[string]any, rowNamespace map[string]any) error {
	if node.Type == layer.TypeTable && hasTemplate(node.Template) {
		return h.hydrateTemplateNode(node, root, rowNamespace)
	}

	if err := h.evaluateNode(node, root, rowNamespace); err != nil {
		return err
	}

	if node.Rows != nil {
		for i := range *node.Rows {
			if err := h.hydrateNode(&(*node.Rows)[i], root, rowNamespace); err != nil {
				return err
			}
		}
	}
	if node.Cells != nil {
		for i := range *node.Cells {
			if err := h.hydrateNode(&(*node.Cells)[i], root, rowNamespace); err != nil {
				return err
			}
		}
	}
	if node.Content != nil && !bytes.Equal(bytes.TrimSpace(node.Content), []byte("null")) {
		var contentNode layer.Node
		if err := json.Unmarshal(node.Content, &contentNode); err != nil {
			return fmt.Errorf("解码 content: %w", err)
		}
		if err := h.hydrateNode(&contentNode, root, rowNamespace); err != nil {
			return err
		}
		node.Content = marshalNode(contentNode)
	}
	return nil
}

// hydrateTemplateNode 模板表实例化:rowsPath 定位行数组 → 每行求值 → 模板行改写
// 为具体行。行高定稿由 canvas.FromGraph 的 V1 耦合重放完成(声明态豁免在此闭环);
// 嵌套模板表(模板格内容含 TableLayer)的 rowsPath 以当前行数据为取数范围(行相对)
func (h *Hydrator) hydrateTemplateNode(node *layer.Node, root map[string]any, rowNamespace map[string]any) error {
	rowsPath := ""
	if node.Data != nil {
		rowsPath = node.Data.RowsPath
	}
	if rowsPath == "" {
		return fmt.Errorf("%w: 模板表缺少有效 data.rowsPath", ErrRowsPathInvalid)
	}

	// 取数范围:顶层 = 数据集根;嵌套模板表 = 当前行数据(行相对)
	scope := any(root)
	if rowNamespace != nil {
		scope = rowNamespace["row"]
	}
	rows, err := navigateRows(scope, rowsPath)
	if err != nil {
		return err
	}

	instances := make([]layer.Node, 0, len(rows))
	serial := 0
	for _, item := range rows {
		// 每行独立解码模板载荷(PHP 数组值拷贝同款,避免指针字段跨行别名)
		var rowNode layer.Node
		if err := json.Unmarshal(node.Template, &rowNode); err != nil {
			return fmt.Errorf("解码模板行: %w", err)
		}
		serial++
		if err := h.hydrateNode(&rowNode, root, map[string]any{"row": item, "$index": serial}); err != nil {
			return err
		}
		rowNode.Type = layer.TypeTableRow
		instances = append(instances, rowNode)
	}

	node.Rows = &instances
	node.Template = nil
	node.Data = nil
	return nil
}

// navigateRows 点路径取行数组:任一段缺键/中途非对象/最终非数组 → rows_path_invalid。
// 注:PHP 侧关联数组也算「数组」(foreach 迭代值、顺序即插入序),Go map 无稳定
// 迭代序不可复现,故对象形状按 rows_path_invalid 处理(共享 fixture 不含该形状)
func navigateRows(scope any, rowsPath string) ([]any, error) {
	value := scope
	for _, segment := range strings.Split(rowsPath, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrRowsPathInvalid, rowsPath)
		}
		next, exists := object[segment]
		if !exists {
			return nil, fmt.Errorf("%w: %s", ErrRowsPathInvalid, rowsPath)
		}
		value = next
	}
	rows, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRowsPathInvalid, rowsPath)
	}
	return rows, nil
}

// evaluateNode 标记字段求值:求值结果替换为字面值并解除标记(填充产物无表达式)。
// 缺字段信号(空串)由本层按字段类施加策略:文案兜底空串、资源类报错(spec §3.4);
// 门控三条件 = valueType 标记 + expression 在场 + 绑定面三类内容层(spec §3.1)
func (h *Hydrator) evaluateNode(node *layer.Node, root map[string]any, rowNamespace map[string]any) error {
	if node.Data == nil || node.Data.ValueType != layer.ValueTypeExpression || node.Data.Expression == nil {
		return nil
	}

	isResource := false
	switch node.Type {
	case layer.TypeText:
	case layer.TypeImage, layer.TypeQrCode:
		isResource = true
	default:
		return nil // 绑定面仅三类内容层(spec §3.1)
	}

	// 行上下文 = 行命名空间 + 根键合并(命名空间分离,根级 row/$ 键已由
	// assertReservedRootKeys 拦截,无键冲突;PHP $rowNamespace + $root 同款)
	context := root
	if rowNamespace != nil {
		context = make(map[string]any, len(root)+len(rowNamespace))
		for k, v := range root {
			context[k] = v
		}
		for k, v := range rowNamespace {
			context[k] = v
		}
	}

	result, err := h.evaluator.Evaluate(*node.Data.Expression, context)
	if err != nil {
		return err
	}

	if isResource && result == "" {
		return fmt.Errorf("%w: %s", ErrExpressionEmptyResource, *node.Data.Expression)
	}

	value := result
	node.Data.Value = &value
	node.Data.ValueType = layer.ValueTypeStatic
	if node.Type == layer.TypeText {
		empty := ""
		node.Data.Expression = &empty // TextLayer 恒写 expression 键(现状形态)
	} else {
		node.Data.Expression = nil // Image/Qr 条件写键
	}
	return nil
}

// normalizeDataset 数据集规范化:统一经 JSON 往返归一为可导航对象——
// map 原样可用,Go 形状(struct 等)顺带归一,JSON 列表/标量形状归一失败
// 按空数据集处理(PHP 同精神:查找全部落空,rowsPath 报 rows_path_invalid)
func normalizeDataset(dataset any) map[string]any {
	if dataset == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(dataset)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

// assertReservedRootKeys 数据集根级键名不得为 row 或 $ 前缀,否则 {{row.x}} 的
// row 段歧义(spec §3.2)
func assertReservedRootKeys(root map[string]any) error {
	for key := range root {
		if key == "row" || strings.HasPrefix(key, "$") {
			return fmt.Errorf("%w: 数据集根级键名不得为 row 或 $ 前缀（%s）", ErrReservedRootKey, key)
		}
	}
	return nil
}

// marshalNode 序列化嵌套 content 载荷;Node 仅含 JSON 基本类型,失败不可达
func marshalNode(n layer.Node) json.RawMessage {
	raw, err := json.Marshal(n)
	if err != nil {
		panic(fmt.Errorf("图层节点序列化失败: %w", err))
	}
	return raw
}

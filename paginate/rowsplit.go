package paginate

// 逐行拆分纯函数(提案 003,paginate-semantics v2.1;PHP Compiler\RowSplitter 逐镜像):
// 模板表行在页界按内容行切开的可拆判定 / 拆分求界 / 片段构造三件套;装箱决策表在
// Paginator.split。零 I/O、零渲染依赖,与 hydrate 同源消费定稿行 graph(前置不变:
// 高度已定稿);断行只读 layer.TextLayer 既有面(Lines/LineHeightPx),text 包零改动。
//
// 语义权威 = submodule 语料 corpus/paginate-semantics/v2.1/ + spec/pipeline.md §3/§7;
// 适用域 = 默认启发式度量栈(行数与片段内容由定稿图给定,语料以显式 \n 硬断行钉面,
// 不依赖断行度量);反例域 = 增强度量注入与字素切分缝(编译期行数漂移由渲染面位图
// 裁剪兜底,不 fail-fast)。格间 lineHeightPx 异质行、非零声明余量(P ≠ 0)为未钉
// 不承诺区,公式照提案逐字实现、不顺手扩展。

import (
	"encoding/json"
	"strings"

	"github.com/HankChenCH/go-canvas/layer"
)

// rowLineStats 行的可拆统计:行内所有格 content 均为 TextLayer 时返回最大内容行数
// 与行级行高(M_i = TextFromGraph 重建取 Lines() 长度,\n 为硬断行、不新增度量注入面;
// L 取首格内容层 LineHeightPx(),v1 语料只钉全行同质 L);任一格 content 缺失或非
// TextLayer(图片/QR 单体格)→ ok = false = 不可拆
func rowLineStats(rowGraph *layer.Node) (maxLines, lineHeightPx int, ok bool) {
	if rowGraph.Cells == nil || len(*rowGraph.Cells) == 0 {
		return 0, 0, false
	}

	for i := range *rowGraph.Cells {
		cell := &(*rowGraph.Cells)[i]
		text, isText := cellTextContent(cell)
		if !isText {
			return 0, 0, false
		}
		if n := len(text.Lines()); n > maxLines {
			maxLines = n
		}
		if i == 0 {
			lineHeightPx = text.LineHeightPx()
		}
	}

	return maxLines, lineHeightPx, true
}

// splitBound 拆分界 K = min(floor(剩余空间 / L), M_max − 1)(剩余空间 = available −
// currentHeight,由调用方传入);K < 1(含非全文本格、M_max ≤ 1、P < 0 的不一致 graph、
// L ≤ 0 非法字号)返回 0 = 不拆,整行走原子路径。防线与 PHP 侧 RowSplitter::splitBound
// 成对平移(守卫是几何契约的一部分)
func splitBound(rowGraph *layer.Node, remaining int) int {
	maxLines, lineHeightPx, ok := rowLineStats(rowGraph)
	if !ok {
		return 0
	}
	if lineHeightPx <= 0 {
		return 0
	}

	// 声明余量 P = 行高 − M_max × L(padding/声明差;v1 语料恒 0):
	// P < 0 = 行高连内容行数都装不下的不一致 graph,视为不可拆(防线用例钉)
	if declared := rowHeight(rowGraph) - maxLines*lineHeightPx; declared < 0 {
		return 0
	}

	bound := min(remaining/lineHeightPx, maxLines-1)
	if bound < 1 {
		return 0
	}
	return bound
}

// splitRowGraph 行片段构造(Σ 高恒等 = 原行高,装箱总量守恒):前段行高 = K × L + P、
// 续段行高 = 原行高 − 前段行高(≤ 0 即不可拆,防线);两片段所有格盒高与内容层
// spec.height = 所在片段行高(独立边框盒);格内容值按 \n 硬断行切片——前段取前
// min(M_i, K) 段、续段承余段(无余段 = 空串,格盒仍画满续段高);其余字段逐字段
// 克隆源节点,不新增不删除键(wire 键形纪律)。ok = false = 不可拆(k < 1 / 非全文
// 本格 / P < 0 / 续段 ≤ 0)。
//
// 注:hydrate 产物 TextLayer data 恒 StaticValue(表达式已在 hydrate 定稿求值),
// 内容值切片写 data.value;直调 Paginate 传入未定稿 ExpressionValue 属未钉域。
// 深拷贝纪律与 hydrate 同源:Cells 切片与 content 载荷改写前重建,共享指针字段
// (Visible/FontFamily/Border 等)分片只读不写,源行 graph 不被改写
func splitRowGraph(rowGraph *layer.Node, k int) (front, cont layer.Node, ok bool) {
	if k < 1 {
		return layer.Node{}, layer.Node{}, false
	}

	maxLines, lineHeightPx, statsOk := rowLineStats(rowGraph)
	if !statsOk {
		return layer.Node{}, layer.Node{}, false
	}

	rowH := rowHeight(rowGraph)
	declared := rowH - maxLines*lineHeightPx
	if declared < 0 {
		return layer.Node{}, layer.Node{}, false
	}

	frontHeight := k*lineHeightPx + declared
	contHeight := rowH - frontHeight
	if contHeight <= 0 {
		return layer.Node{}, layer.Node{}, false
	}

	front = *rowGraph
	cont = *rowGraph
	front.Spec.Shape.Height = frontHeight
	cont.Spec.Shape.Height = contHeight

	frontCells := make([]layer.Node, 0, len(*rowGraph.Cells))
	contCells := make([]layer.Node, 0, len(*rowGraph.Cells))
	for i := range *rowGraph.Cells {
		cell := &(*rowGraph.Cells)[i]

		// 格独立分片:所有格盒高 = 所在片段行高(独立边框盒,css-break 先例)
		frontCell := *cell
		frontCell.Spec.Shape.Height = frontHeight
		contCell := *cell
		contCell.Spec.Shape.Height = contHeight

		// 内容层分片:仅 TextLayer 改写(盒高同步所在片段行高 + 值按 \n 切片),
		// 其余载荷两片段原样承入(null/单体格至此不可达,lineStats 已挡)
		frontContent, contContent := splitCellContent(cell, frontHeight, contHeight, k)
		if frontContent != nil {
			frontCell.Content = frontContent
			contCell.Content = contContent
		}

		frontCells = append(frontCells, frontCell)
		contCells = append(contCells, contCell)
	}
	front.Cells = &frontCells
	cont.Cells = &contCells

	return front, cont, true
}

// splitCellContent 单元格内容层分片(PHP splitRowGraph 内容分支同款):解码 content
// 键载荷,TextLayer 载荷改写盒高与 data.value 后重编码;非文本内容原样返回 nil nil
// (两片段共享源载荷)。返回 nil 时片段格直接沿用源 content 键
func splitCellContent(cell *layer.Node, frontHeight, contHeight, k int) (frontRaw, contRaw json.RawMessage) {
	if len(cell.Content) == 0 {
		return nil, nil
	}
	var content layer.Node
	if err := json.Unmarshal(cell.Content, &content); err != nil || content.Type != layer.TypeText {
		return nil, nil
	}

	text := layer.TextFromGraph(content)
	take := min(len(text.Lines()), k)
	segments := strings.Split(contentValue(&content), "\n")
	// 内容行数可超过 \n 段数(度量折行不新增硬段):取满全部段、续段为空串
	take = min(take, len(segments))

	frontNode := content
	frontNode.Spec.Shape.Height = frontHeight
	frontValue := strings.Join(segments[:take], "\n")
	frontNode.Data = withValue(content.Data, frontValue)

	contNode := content
	contNode.Spec.Shape.Height = contHeight
	contValue := strings.Join(segments[take:], "\n")
	contNode.Data = withValue(content.Data, contValue)

	return mustMarshalNode(frontNode), mustMarshalNode(contNode)
}

// contentValue content 载荷的现值(PHP $content['data']['value'] ?? '' 同款:
// data 缺席或 value 为 null 归空串)
func contentValue(content *layer.Node) string {
	if content.Data == nil || content.Data.Value == nil {
		return ""
	}
	return *content.Data.Value
}

// withValue 值切片写回 data 键(值载体拷贝替换,源 Data 不被改写;data 键缺席时
// 最小构造——页产物经 fromGraph 重建归一,中间键形不进 wire 断言面)
func withValue(d *layer.Data, v string) *layer.Data {
	if d == nil {
		return &layer.Data{Value: &v}
	}
	copied := *d
	copied.Value = &v
	return &copied
}

// mustMarshalNode 分片载荷重编码;Node 仅含 JSON 基本类型,失败不可达
func mustMarshalNode(n layer.Node) json.RawMessage {
	raw, err := json.Marshal(n)
	if err != nil {
		panic("分片内容层重编码失败: " + err.Error())
	}
	return raw
}

// cellTextContent 解码单元格 content 键并重建文本层(rowLineStats 内容检查同款):
// 键缺省 / null / 非 TextLayer 类型 = 非文本内容
func cellTextContent(cell *layer.Node) (*layer.TextLayer, bool) {
	if len(cell.Content) == 0 {
		return nil, false
	}
	var content layer.Node
	if err := json.Unmarshal(cell.Content, &content); err != nil || content.Type != layer.TypeText {
		return nil, false
	}
	return layer.TextFromGraph(content), true
}

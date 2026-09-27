package layer

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// 表格容器三层:表(TableLayer)→行(TableRowLayer)→单元格(TableCellLayer)→内容层。
// 与 PHP 相同的构建期副作用,一次性同步、"先 add 后改尺寸不重算":
// 表 addRow 同步行宽为表宽并累加内容盒高;行 addCell 行高取最高单元格;
// 单元格 addContentLayer 同步内容层宽、压平/采纳其高度。批量选项
// (WithRows/WithCells/WithContent)与 graph 解码复现同一条 add 路径。

// TableLayer 表格图层(行容器)。V2 模板态(spec §2):声明 TableRowTemplate +
// data.rowsPath,与 rows XOR 互斥——wire 上 template/data 为条件写键(模板态不写
// rows,V1 态不写 template/data,字节面保三端 parity);填充在渲染前的独立纯结构
// 步骤完成(hydrate 包),填充产物不回写本图层(往返恒等对象 = 声明态)
type TableLayer struct {
	base
	rows []*TableRowLayer

	// contentBoxHeight 已收纳行的累计高度,isOverHeight 的判定基准(PHP 私有字段同款)
	contentBoxHeight int

	// template 行模板声明(V2):非 nil = 模板态,与 rows 互斥
	template *TableRowTemplate
	// rowsPath 取行路径(点路径字符串):仅模板态参与填充与 wire 写键(spec §3.3)
	rowsPath string
}

// NewTableLayer 构造表格图层
func NewTableLayer(opts ...tableLayerOpt) *TableLayer {
	l := &TableLayer{base: newBase()}
	for _, o := range opts {
		o.applyTable(l)
	}
	return l
}

// AddRow 收纳行:行宽同步为表宽,内容盒高累加行高(构建期副作用,与 PHP 一致)。
// 模板态禁止追加具体行(template ⊕ rows 互斥,API 误用报错——PHP 同款,
// 调用期错误不占解码期异常类)
func (l *TableLayer) AddRow(row *TableRowLayer) error {
	if l.template != nil {
		return fmt.Errorf("template_rows_conflict: 模板态表格不得追加具体行,请先 SetTemplate(nil)")
	}
	row.setWidth(l.Width())
	l.rows = append(l.rows, row)
	l.contentBoxHeight += row.Height()
	return nil
}

// IsOverHeight 累计行高加上新行是否超出表高
func (l *TableLayer) IsOverHeight(row *TableRowLayer) bool {
	return l.contentBoxHeight+row.Height() > l.Height()
}

// Rows 已收纳行(副本,外部改写不影响表)
func (l *TableLayer) Rows() []*TableRowLayer {
	out := make([]*TableRowLayer, len(l.rows))
	copy(out, l.rows)
	return out
}

// SetTemplate 声明模板行(V2,spec §2.2/§2.3):与 rows 互斥,清空切换语义——
// 设置模板即清空既有行;宽度耦合沿用(行宽 = 表宽,关 autoWidth),高度耦合全豁免。
// 传 nil 清除模板与 rowsPath 回 V1 形态
func (l *TableLayer) SetTemplate(template *TableRowTemplate) {
	if template == nil {
		l.template = nil
		l.rowsPath = ""
		return
	}
	template.setWidth(l.Width())
	l.rows = nil
	l.contentBoxHeight = 0
	l.template = template
}

// Template 声明的行模板;未声明为 nil
func (l *TableLayer) Template() *TableRowTemplate { return l.template }

// SetRowsPath 取行路径(点路径字符串,spec §3.3):填充时从数据集(嵌套模板表
// 则从当前行数据)定位行数组;仅模板态参与填充与 wire 写键
func (l *TableLayer) SetRowsPath(rowsPath string) { l.rowsPath = rowsPath }

// RowsPath 取行路径
func (l *TableLayer) RowsPath() string { return l.rowsPath }

// TypeName implements Layer
func (l *TableLayer) TypeName() string { return TypeTable }

// Graph 序列化为 wire 节点。模板态:条件写键 data(已设 rowsPath)/template,
// 不写 rows(XOR 互斥);V1 态:rows 键恒写(空表为 [],PHP 同款),无损全量行结构
func (l *TableLayer) Graph() Node {
	n := l.base.wireNode(TypeTable)
	if l.template != nil {
		if l.rowsPath != "" {
			n.Data = &Data{RowsPath: l.rowsPath}
		}
		n.Template = marshalNode(l.template.Graph())
		return n
	}
	rows := make([]Node, 0, len(l.rows))
	for _, r := range l.rows {
		rows = append(rows, r.Graph())
	}
	n.Rows = &rows
	return n
}

// TableFromGraph 由 wire 节点重建表格图层。模板态:校验 XOR 与 rowsPath 后经
// SetTemplate/SetRowsPath 装配;V1 态:经 AddRow 复现行宽同步与高度累加。
// 注:rowsPath 非字符串的病态 wire 在 JSON 解码层即报错(Node.Data 强类型),
// 与 PHP 的 rows_path_missing 分类略有差异,均为解码期 fail-fast
func TableFromGraph(n Node) (*TableLayer, error) {
	l := NewTableLayer()
	l.applyNode(n)

	// 模板态判定:"template": null 与缺键同判(PHP array_key_exists + !== null 同款)
	templateState := n.Template != nil && !bytes.Equal(bytes.TrimSpace(n.Template), []byte("null"))
	if templateState {
		if n.Rows != nil {
			return nil, ErrTemplateRowsConflict
		}
		rowsPath := ""
		if n.Data != nil {
			rowsPath = n.Data.RowsPath
		}
		if rowsPath == "" {
			return nil, ErrRowsPathMissing
		}
		var templateNode Node
		if err := json.Unmarshal(n.Template, &templateNode); err != nil {
			return nil, fmt.Errorf("解码模板行: %w", err)
		}
		template, err := TableRowTemplateFromGraph(templateNode)
		if err != nil {
			return nil, err
		}
		l.SetTemplate(template)
		l.SetRowsPath(rowsPath)
		return l, nil
	}

	if n.Rows != nil {
		for _, rn := range *n.Rows {
			row, err := TableRowFromGraph(rn)
			if err != nil {
				return nil, err
			}
			if err := l.AddRow(row); err != nil {
				return nil, err
			}
		}
	}
	return l, nil
}

// TableRowLayer 表格行图层(单元格容器)
type TableRowLayer struct {
	base
	cells []*TableCellLayer
}

// NewTableRowLayer 构造表格行图层
func NewTableRowLayer(opts ...tableRowLayerOpt) *TableRowLayer {
	l := &TableRowLayer{base: newBase()}
	for _, o := range opts {
		o.applyTableRow(l)
	}
	return l
}

// AddCell 收纳单元格:行高取最高单元格,setHeight 语义随之清除行 auto 标志
func (l *TableRowLayer) AddCell(cell *TableCellLayer) {
	if h := cell.Height(); l.Height() < h {
		l.setHeight(h)
	}
	l.cells = append(l.cells, cell)
}

// Cells 已收纳单元格(副本,外部改写不影响行)
func (l *TableRowLayer) Cells() []*TableCellLayer {
	out := make([]*TableCellLayer, len(l.cells))
	copy(out, l.cells)
	return out
}

// TypeName implements Layer
func (l *TableRowLayer) TypeName() string { return TypeTableRow }

// Graph 序列化为 wire 节点;cells 键恒出现(空行为 [],PHP 同款)
func (l *TableRowLayer) Graph() Node {
	n := l.base.wireNode(TypeTableRow)
	cells := make([]Node, 0, len(l.cells))
	for _, c := range l.cells {
		cells = append(cells, c.Graph())
	}
	n.Cells = &cells
	return n
}

// TableRowFromGraph 由 wire 节点重建表格行图层:经 AddCell 复现行高取最高
func TableRowFromGraph(n Node) (*TableRowLayer, error) {
	l := NewTableRowLayer()
	l.applyNode(n)
	if n.Cells != nil {
		for _, cn := range *n.Cells {
			cell, err := TableCellFromGraph(cn)
			if err != nil {
				return nil, err
			}
			l.AddCell(cell)
		}
	}
	return l, nil
}

// contentLayer 单元格内容层约束:addContentLayer 的构建期副作用需回写内容层尺寸。
// 方法集含未导出方法,仅库内图层类型(嵌入 base)可实现——对齐 PHP 只接受
// AbstractLayer 子类,外部实现传不进来(编译期报错)
type contentLayer interface {
	Layer
	Width() int
	Height() int
	setWidth(int)
	setHeight(int)
}

// TableCellLayer 表格单元格图层:包装一个内容层
type TableCellLayer struct {
	base
	content Layer
}

// NewTableCellLayer 构造表格单元格图层
func NewTableCellLayer(opts ...tableCellLayerOpt) *TableCellLayer {
	l := &TableCellLayer{base: newBase()}
	for _, o := range opts {
		o.applyTableCell(l)
	}
	return l
}

// AddContentLayer 包装内容层并同步尺寸(构建期副作用,与 PHP 一致,一次性):
// 内容层宽同步为单元格宽;auto 单元格采纳内容层高度(setHeight 随之清除自身
// auto 标志);固定单元格把内容层高度压平为单元格高——setHeight 已清内容层
// auto 标志,等价 PHP setHeight(...)->setAutoHeight(false)
func (l *TableCellLayer) AddContentLayer(content contentLayer) {
	content.setWidth(l.Width())
	if l.autoHeight {
		l.setHeight(content.Height())
	} else {
		content.setHeight(l.Height())
	}
	l.content = content
}

// ContentLayer 包装的内容层;未包装为 nil
func (l *TableCellLayer) ContentLayer() Layer { return l.content }

// AddTemplateContentLayer 模板上下文装配(TableLayer V2,spec §2.3):只同步内容层宽
// = 格宽,高度耦合全豁免——声明高与 autoHeight 标志原样保留,实例高度由填充定稿
func (l *TableCellLayer) AddTemplateContentLayer(content contentLayer) {
	content.setWidth(l.Width())
	l.content = content
}

// TypeName implements Layer
func (l *TableCellLayer) TypeName() string { return TypeTableCell }

// Graph 序列化为 wire 节点;content 键恒出现:无内容层为 null(PHP ?->graph() 同款)。
// Content 以 RawMessage 承载,往返按字节原样保留
func (l *TableCellLayer) Graph() Node {
	n := l.base.wireNode(TypeTableCell)
	if l.content != nil {
		n.Content = marshalNode(l.content.Graph())
	} else {
		n.Content = json.RawMessage("null")
	}
	return n
}

// TableCellFromGraph 由 wire 节点重建表格单元格图层:经 AddContentLayer 复现尺寸同步;
// content 键缺省或为 null 视为无内容层(PHP !empty 守卫同款)
func TableCellFromGraph(n Node) (*TableCellLayer, error) {
	return cellFromGraph(n, false)
}

// cellFromGraph 单元格解码共用路径:template = true 走模板装配（高度豁免）,
// false 走 V1 高度耦合装配
func cellFromGraph(n Node, template bool) (*TableCellLayer, error) {
	l := NewTableCellLayer()
	l.applyNode(n)
	content, err := contentFromGraph(n.Content)
	if err != nil {
		return nil, err
	}
	if content != nil {
		if template {
			l.AddTemplateContentLayer(content)
		} else {
			l.AddContentLayer(content)
		}
	}
	return l, nil
}

// contentFromGraph 解码单元格 content 键载荷:键缺省或 null 归 nil
func contentFromGraph(raw json.RawMessage) (contentLayer, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var child Node
	if err := json.Unmarshal(raw, &child); err != nil {
		return nil, fmt.Errorf("解码单元格 content: %w", err)
	}
	l, err := FromGraph(child)
	if err != nil {
		return nil, err
	}
	cl, ok := l.(contentLayer)
	if !ok {
		return nil, fmt.Errorf("单元格 content 不支持容器尺寸同步: %s", child.Type)
	}
	return cl, nil
}

// marshalNode 序列化嵌套 content 载荷;Node 仅含 JSON 基本类型,失败不可达
func marshalNode(n Node) json.RawMessage {
	raw, err := json.Marshal(n)
	if err != nil {
		panic(fmt.Errorf("图层节点序列化失败: %w", err))
	}
	return raw
}

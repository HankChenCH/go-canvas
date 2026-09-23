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

// TableLayer 表格图层(行容器)
type TableLayer struct {
	base
	rows []*TableRowLayer

	// contentBoxHeight 已收纳行的累计高度,isOverHeight 的判定基准(PHP 私有字段同款)
	contentBoxHeight int
}

// NewTableLayer 构造表格图层
func NewTableLayer(opts ...tableLayerOpt) *TableLayer {
	l := &TableLayer{base: newBase()}
	for _, o := range opts {
		o.applyTable(l)
	}
	return l
}

// AddRow 收纳行:行宽同步为表宽,内容盒高累加行高(构建期副作用,与 PHP 一致)
func (l *TableLayer) AddRow(row *TableRowLayer) {
	row.setWidth(l.Width())
	l.rows = append(l.rows, row)
	l.contentBoxHeight += row.Height()
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

// TypeName implements Layer
func (l *TableLayer) TypeName() string { return TypeTable }

// Graph 序列化为 wire 节点;无损:全量行结构,rows 键恒出现(空表为 [],PHP 同款)
func (l *TableLayer) Graph() Node {
	n := l.base.wireNode(TypeTable)
	rows := make([]Node, 0, len(l.rows))
	for _, r := range l.rows {
		rows = append(rows, r.Graph())
	}
	n.Rows = &rows
	return n
}

// TableFromGraph 由 wire 节点重建表格图层:经 AddRow 复现行宽同步与高度累加
func TableFromGraph(n Node) (*TableLayer, error) {
	l := NewTableLayer()
	l.applyNode(n)
	if n.Rows != nil {
		for _, rn := range *n.Rows {
			row, err := TableRowFromGraph(rn)
			if err != nil {
				return nil, err
			}
			l.AddRow(row)
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
	l := NewTableCellLayer()
	l.applyNode(n)
	content, err := contentFromGraph(n.Content)
	if err != nil {
		return nil, err
	}
	if content != nil {
		l.AddContentLayer(content)
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

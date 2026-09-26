package layer

// TableRowTemplate 表格行模板(TableLayer V2,spec §2):单行循环体声明,
// 仅合法出现在 TableLayer 的 template 字段内——出现在画布图层序列/rows/cells/
// content 属非法 wire,工厂虽能解码,契约禁止(对齐 PHP 不拒绝)。
//
// 装配语义(spec §2.3):宽度耦合沿用(行宽 = 表宽、内容宽 = 格宽,列版式与数据
// 无关)、高度耦合全豁免(行/格/内容的声明高与 autoHeight 标志原样保留——占位
// 内容不参与 V1 高度耦合,实例高度由展开阶段按真实数据定稿)。
// wire 上模板行与具体行同构(区别只在外层 TableLayer 用 template 键包它)

// TableRowTemplate 表格行模板
type TableRowTemplate struct {
	base
	cells []*TableCellLayer
}

// NewTableRowTemplate 构造表格行模板
func NewTableRowTemplate(opts ...tableRowTemplateLayerOpt) *TableRowTemplate {
	l := &TableRowTemplate{base: newBase()}
	for _, o := range opts {
		o.applyTableRowTemplate(l)
	}
	return l
}

// AddCell 挂载单元格:只挂格,不做行高耦合(区别于 TableRowLayer.AddCell 的行高取最高)
func (l *TableRowTemplate) AddCell(cell *TableCellLayer) {
	l.cells = append(l.cells, cell)
}

// Cells 已挂载单元格(副本,外部改写不影响模板行)
func (l *TableRowTemplate) Cells() []*TableCellLayer {
	out := make([]*TableCellLayer, len(l.cells))
	copy(out, l.cells)
	return out
}

// TypeName implements Layer
func (l *TableRowTemplate) TypeName() string { return TypeTableRowTemplate }

// Graph 序列化为 wire 节点;cells 键恒出现(空模板行为 [],与 TableRowLayer 同形)
func (l *TableRowTemplate) Graph() Node {
	n := l.base.wireNode(TypeTableRowTemplate)
	cells := make([]Node, 0, len(l.cells))
	for _, c := range l.cells {
		cells = append(cells, c.Graph())
	}
	n.Cells = &cells
	return n
}

// TableRowTemplateFromGraph 由 wire 节点重建表格行模板:格解码走高度豁免装配
// (templateFromGraph,区别于具体行的 V1 高度耦合)
func TableRowTemplateFromGraph(n Node) (*TableRowTemplate, error) {
	l := NewTableRowTemplate()
	l.applyNode(n)
	if n.Cells != nil {
		for _, cn := range *n.Cells {
			cell, err := cellFromGraph(cn, true)
			if err != nil {
				return nil, err
			}
			l.AddCell(cell)
		}
	}
	return l, nil
}

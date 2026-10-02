package layer

// base 各图层共享的盒模型/对齐/定位/priority 设定(对应 PHP AbstractLayer 字段面)。
// Go 无继承,经结构体嵌入复用;默认值逐字段对齐 PHP,隐藏行为(width=0 清除边框、
// auto 清零尺寸等)收敛在 setter 与 options 里,不散落字段赋值。

// 对齐取值(对齐 PHP AbstractLayer 注释面)
const (
	AlignLeft   = "left"
	AlignCenter = "center"
	AlignRight  = "right"
	AlignTop    = "top"
	AlignBottom = "bottom"
)

// 九锚点取值
const (
	AnchorTopLeft     = "top-left"
	AnchorTop         = "top"
	AnchorTopRight    = "top-right"
	AnchorLeft        = "left"
	AnchorCenter      = "center"
	AnchorRight       = "right"
	AnchorBottomLeft  = "bottom-left"
	AnchorBottom      = "bottom"
	AnchorBottomRight = "bottom-right"
)

// DefaultBorderColor 边框缺省色(PHP setBorder* 默认参数)
const DefaultBorderColor = "#000"

type base struct {
	width, height   int
	autoWidth       bool
	autoHeight      bool
	lineHeight      float64
	padding         Padding
	border          Border
	bgColor         *string
	horizontalAlign string
	verticalAlign   string
	position        string
	x, y            int
	priority        int
	// displayName 用户命名(wire 键 name,layer-panel-ux 工单 01):与类型标识无关,
	// 缺省空串且序列化键省略(仅非空写键)
	// visible 显隐设定(wire 键 visible):缺省 true 且序列化键省略(仅 false 写键),
	// 渲染循环跳过 false 的根图层
	displayName string
	visible     bool
}

// newBase 默认态:0 尺寸、无背景、left/top、top-left、priority 0、行高 1、可见、未命名
func newBase() base {
	return base{
		lineHeight:      1,
		horizontalAlign: AlignLeft,
		verticalAlign:   AlignTop,
		position:        AnchorTopLeft,
		visible:         true,
	}
}

// ---- setter 语义(对齐 PHP 同名 setter,含隐藏行为) ----

func (b *base) setWidth(w int)  { b.autoWidth = false; b.width = w }
func (b *base) setHeight(h int) { b.autoHeight = false; b.height = h }

// setAutoWidth 置 auto 标志并清零宽度(PHP setWidth('auto') 语义)
func (b *base) setAutoWidth()  { b.autoWidth = true; b.width = 0 }
func (b *base) setAutoHeight() { b.autoHeight = true; b.height = 0 }

func (b *base) setBackground(color string) {
	if color == "" {
		b.bgColor = nil
		return
	}
	b.bgColor = &color
}

func (b *base) setPosition(x, y int, anchor string) {
	b.x, b.y, b.position = x, y, anchor
}

// borderSide width=0 即清除(PHP setBorder* 隐藏语义)
func borderSide(width int, color string) *BorderSide {
	if width == 0 {
		return nil
	}
	return &BorderSide{Width: width, Color: color}
}

func (b *base) setBorderAll(width int, color string) {
	side := borderSide(width, color)
	b.border = Border{Top: side, Bottom: side, Left: side, Right: side}
}

func (b *base) setBorderTop(width int, color string)    { b.border.Top = borderSide(width, color) }
func (b *base) setBorderBottom(width int, color string) { b.border.Bottom = borderSide(width, color) }
func (b *base) setBorderLeft(width int, color string)   { b.border.Left = borderSide(width, color) }
func (b *base) setBorderRight(width int, color string)  { b.border.Right = borderSide(width, color) }

// setPaddingAll CSS 1 值:全边
func (b *base) setPaddingAll(v float64) {
	b.padding = Padding{Top: v, Bottom: v, Left: v, Right: v}
}

// setPaddingHV CSS 2 值:上下 vertical / 左右 horizontal
func (b *base) setPaddingHV(horizontal, vertical float64) {
	b.padding = Padding{Top: vertical, Bottom: vertical, Left: horizontal, Right: horizontal}
}

// setPaddingTHB CSS 3 值:上 / 左右 / 下
func (b *base) setPaddingTHB(top, horizontal, bottom float64) {
	b.padding = Padding{Top: top, Bottom: bottom, Left: horizontal, Right: horizontal}
}

// setPaddingTRBL CSS 4 值:上右下左
func (b *base) setPaddingTRBL(top, right, bottom, left float64) {
	b.padding = Padding{Top: top, Right: right, Bottom: bottom, Left: left}
}

// ---- getter(对齐 PHP 公共 getter) ----

func (b *base) Width() int  { return b.width }
func (b *base) Height() int { return b.height }

// ContentWidth 内容区宽度 = 宽度 - 左右 padding(向零截断,对齐 PHP intval)
func (b *base) ContentWidth() int {
	return contentWidthOf(b.width, b.padding)
}

// ContentHeight 内容区高度 = 高度 - 上下 padding
func (b *base) ContentHeight() int {
	return contentHeightOf(b.height, b.padding)
}

// contentWidthOf 任意动态宽 - 左右 padding(向零截断,对齐 PHP intval)。
// Width() 被覆写的图层(Text)经它派生 ContentWidth,恢复 PHP getContentWidth
// 经 $this->getWidth() 的动态分派语义
func contentWidthOf(width int, padding Padding) int {
	return int(float64(width) - padding.Left - padding.Right)
}

// contentHeightOf 任意动态高 - 上下 padding(向零截断,对齐 PHP intval)。
// Height() 被覆写的图层(Text/QR)经它派生 ContentHeight,恢复 PHP
// getContentHeight 经 $this->getHeight() 的动态分派语义
func contentHeightOf(height int, padding Padding) int {
	return int(float64(height) - padding.Top - padding.Bottom)
}

func (b *base) Background() *string     { return b.bgColor }
func (b *base) Padding() Padding        { return b.padding }
func (b *base) Border() Border          { return b.border }
func (b *base) HorizontalAlign() string { return b.horizontalAlign }
func (b *base) VerticalAlign() string   { return b.verticalAlign }
func (b *base) Priority() int           { return b.priority }

// SetDisplayName 用户命名(wire 键 name,layer-panel-ux 工单 01);空串 = 未命名,序列化键省略
func (b *base) SetDisplayName(name string) { b.displayName = name }

// DisplayName 用户命名;缺省空串
func (b *base) DisplayName() string { return b.displayName }

// SetVisible 显隐设定(wire 键 visible);false 的根图层被渲染循环跳过(最终输出排除)
func (b *base) SetVisible(visible bool) { b.visible = visible }

// Visible 显隐设定;缺省 true
func (b *base) Visible() bool { return b.visible }

// Position 返回 (锚点, x, y)
func (b *base) Position() (string, int, int) {
	return b.position, b.x, b.y
}

// ---- graph 构造与回放 ----

// wireNode 以 typ 为类型标识生成 wire 节点的公共部分(对应 PHP AbstractLayer::graph)。
// 键省略契约(name/visible,layer-panel-ux 工单 01):name 仅非空写、visible 仅 false 写
// ——缺省态字节面与无字段版本一致
func (b *base) wireNode(typ string) Node {
	node := Node{
		Type:     typ,
		Priority: b.priority,
		Spec: Spec{
			Shape: Shape{
				Width:           b.width,
				Height:          b.height,
				AutoWidth:       b.autoWidth,
				AutoHeight:      b.autoHeight,
				LineHeight:      b.lineHeight,
				Padding:         b.padding,
				Border:          b.border,
				BackgroundColor: b.bgColor,
			},
			Align:    Align{Horizontal: b.horizontalAlign, Vertical: b.verticalAlign},
			Position: Position{X: b.x, Y: b.y, Position: b.position},
		},
	}
	if b.displayName != "" {
		node.Name = b.displayName
	}
	if !b.visible {
		hidden := false
		node.Visible = &hidden
	}
	return node
}

// applyNode 把 wire 节点回放到 base(对齐 PHP applyGraph)。
// 除 name/visible 条件键外 wire 是无缺键的定长结构,空串仅视作"锚点/对齐缺省",
// 保留图层构造默认;name/visible 在场才生效,缺省值形态(空串、true)由序列化侧归一省略。
func (b *base) applyNode(n Node) {
	shape := n.Spec.Shape
	if shape.AutoWidth {
		b.setAutoWidth()
	} else {
		b.setWidth(shape.Width)
	}
	if shape.AutoHeight {
		b.setAutoHeight()
	} else {
		b.setHeight(shape.Height)
	}
	b.lineHeight = shape.LineHeight
	b.padding = shape.Padding
	b.border = shape.Border
	b.bgColor = shape.BackgroundColor

	if h := n.Spec.Align.Horizontal; h != "" {
		b.horizontalAlign = h
	}
	if v := n.Spec.Align.Vertical; v != "" {
		b.verticalAlign = v
	}
	if p := n.Spec.Position; p.Position != "" || p.X != 0 || p.Y != 0 {
		anchor := p.Position
		if anchor == "" {
			anchor = AnchorTopLeft
		}
		b.setPosition(p.X, p.Y, anchor)
	}
	b.priority = n.Priority
	if n.Name != "" {
		b.displayName = n.Name
	}
	if n.Visible != nil {
		b.visible = *n.Visible
	}
}

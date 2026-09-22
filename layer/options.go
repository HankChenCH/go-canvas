package layer

// 选项机制:Go 无继承,公共设定与图层专属设定经"接口分隔"达成编译期类型安全——
// 公共选项施加到嵌入的 base,图层专属选项施加到具体图层;错配的选项(如给文本图层
// 传 WithImage)过不了编译。新增图层类型时补一行 baseOpt 的 apply 适配即可。

// baseOpt 公共图层选项:作用于任意图层共享的盒模型/对齐/定位设定
type baseOpt func(*base)

func (f baseOpt) applyImage(l *ImageLayer) { f(&l.base) }

// imageOpt 图片图层专属选项
type imageOpt func(*ImageLayer)

func (f imageOpt) applyImage(l *ImageLayer) { f(l) }

// imageLayerOpt 图片图层构造选项(公共 + 图片专属)
type imageLayerOpt interface{ applyImage(*ImageLayer) }

// WithSize 声明尺寸;会清除 auto 标志
func WithSize(width, height int) baseOpt {
	return func(b *base) { b.setWidth(width); b.setHeight(height) }
}

// WithAutoWidth 自动宽度。auto 用独立选项表达(graph 面本就是 autoWidth 布尔标志,
// 不复刻 PHP setter 的 'auto' 字符串糖)
func WithAutoWidth() baseOpt {
	return func(b *base) { b.setAutoWidth() }
}

// WithAutoHeight 自动高度
func WithAutoHeight() baseOpt {
	return func(b *base) { b.setAutoHeight() }
}

// WithLineHeight 行高倍数(行高像素 = ceil(字号 × 行高倍数),文本布局消费)
func WithLineHeight(lineHeight float64) baseOpt {
	return func(b *base) { b.lineHeight = lineHeight }
}

// WithBackground 背景色;空串表示无背景
func WithBackground(color string) baseOpt {
	return func(b *base) { b.setBackground(color) }
}

// WithPosition 定位:九锚点 + 父盒内偏移,不做边界钳位(负值即溢出摆放)。
// 省略锚点即 top-left(PHP setPosition 默认参数)
func WithPosition(x, y int, anchor ...string) baseOpt {
	return func(b *base) {
		a := AnchorTopLeft
		if len(anchor) > 0 {
			a = anchor[0]
		}
		b.setPosition(x, y, a)
	}
}

// WithHorizontalAlign 水平对齐:left / center / right
func WithHorizontalAlign(align string) baseOpt {
	return func(b *base) { b.horizontalAlign = align }
}

// WithVerticalAlign 垂直对齐:top / center / bottom
func WithVerticalAlign(align string) baseOpt {
	return func(b *base) { b.verticalAlign = align }
}

// WithPriority 绘制次序:越大越先渲染(视觉上越垫底)
func WithPriority(priority int) baseOpt {
	return func(b *base) { b.priority = priority }
}

// WithBorder 四边同设;width=0 即清除全部边框,缺省色 #000
func WithBorder(width int, color ...string) baseOpt {
	return func(b *base) { b.setBorderAll(width, borderColor(color)) }
}

// WithBorderTop 仅上边框;width=0 即清除,缺省色 #000
func WithBorderTop(width int, color ...string) baseOpt {
	return func(b *base) { b.setBorderTop(width, borderColor(color)) }
}

// WithBorderBottom 仅下边框;width=0 即清除,缺省色 #000
func WithBorderBottom(width int, color ...string) baseOpt {
	return func(b *base) { b.setBorderBottom(width, borderColor(color)) }
}

// WithBorderLeft 仅左边框;width=0 即清除,缺省色 #000
func WithBorderLeft(width int, color ...string) baseOpt {
	return func(b *base) { b.setBorderLeft(width, borderColor(color)) }
}

// WithBorderRight 仅右边框;width=0 即清除,缺省色 #000
func WithBorderRight(width int, color ...string) baseOpt {
	return func(b *base) { b.setBorderRight(width, borderColor(color)) }
}

// WithPadding 内边距,CSS 1 值语义:全边
func WithPadding(v float64) baseOpt {
	return func(b *base) { b.setPaddingAll(v) }
}

// WithPaddingVH 内边距,CSS 2 值语义:垂直(上下)在前 / 水平(左右)在后,
// 与 PHP setPadding(1,2) 的首参垂直一致
func WithPaddingVH(vertical, horizontal float64) baseOpt {
	return func(b *base) { b.setPaddingHV(horizontal, vertical) }
}

// WithPaddingTHB 内边距,CSS 3 值语义:上 / 左右 / 下
func WithPaddingTHB(top, horizontal, bottom float64) baseOpt {
	return func(b *base) { b.setPaddingTHB(top, horizontal, bottom) }
}

// WithPaddingTRBL 内边距,CSS 4 值语义:上右下左
func WithPaddingTRBL(top, right, bottom, left float64) baseOpt {
	return func(b *base) { b.setPaddingTRBL(top, right, bottom, left) }
}

func borderColor(color []string) string {
	if len(color) > 0 {
		return color[0]
	}
	return DefaultBorderColor
}

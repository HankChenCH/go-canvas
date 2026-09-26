package layer

import (
	"github.com/hankchen/go-canvas/text"
)

// 选项机制:Go 无继承,公共设定与图层专属设定经"接口分隔"达成编译期类型安全——
// 公共选项施加到嵌入的 base,图层专属选项施加到具体图层;错配的选项(如给文本图层
// 传 WithImage)过不了编译。新增图层类型时补一行 baseOpt 的 apply 适配即可。

// baseOpt 公共图层选项:作用于任意图层共享的盒模型/对齐/定位设定
type baseOpt func(*base)

func (f baseOpt) applyImage(l *ImageLayer)         { f(&l.base) }
func (f baseOpt) applyText(l *TextLayer)           { f(&l.base) }
func (f baseOpt) applyQrCode(l *QrCodeLayer)       { f(&l.base) }
func (f baseOpt) applyTable(l *TableLayer)         { f(&l.base) }
func (f baseOpt) applyTableRow(l *TableRowLayer)   { f(&l.base) }
func (f baseOpt) applyTableCell(l *TableCellLayer) { f(&l.base) }
func (f baseOpt) applyTableRowTemplate(l *TableRowTemplate) {
	f(&l.base)
}

// imageOpt 图片图层专属选项
type imageOpt func(*ImageLayer)

func (f imageOpt) applyImage(l *ImageLayer) { f(l) }

// imageLayerOpt 图片图层构造选项(公共 + 图片专属)
type imageLayerOpt interface{ applyImage(*ImageLayer) }

// textOpt 文本图层专属选项
type textOpt func(*TextLayer)

func (f textOpt) applyText(l *TextLayer) { f(l) }

// textLayerOpt 文本图层构造选项(公共 + 文本专属)
type textLayerOpt interface{ applyText(*TextLayer) }

// qrCodeOpt 二维码图层专属选项
type qrCodeOpt func(*QrCodeLayer)

func (f qrCodeOpt) applyQrCode(l *QrCodeLayer) { f(l) }

// qrCodeLayerOpt 二维码图层构造选项(公共 + 二维码专属)
type qrCodeLayerOpt interface{ applyQrCode(*QrCodeLayer) }

// tableOpt 表格图层专属选项
type tableOpt func(*TableLayer)

func (f tableOpt) applyTable(l *TableLayer) { f(l) }

// tableLayerOpt 表格图层构造选项(公共 + 表格专属)
type tableLayerOpt interface{ applyTable(*TableLayer) }

// tableRowOpt 表格行图层专属选项
type tableRowOpt func(*TableRowLayer)

func (f tableRowOpt) applyTableRow(l *TableRowLayer) { f(l) }

// tableRowLayerOpt 表格行图层构造选项(公共 + 行专属)
type tableRowLayerOpt interface{ applyTableRow(*TableRowLayer) }

// tableCellOpt 表格单元格图层专属选项
type tableCellOpt func(*TableCellLayer)

func (f tableCellOpt) applyTableCell(l *TableCellLayer) { f(l) }

// tableCellLayerOpt 表格单元格图层构造选项(公共 + 单元格专属)
type tableCellLayerOpt interface{ applyTableCell(*TableCellLayer) }

// tableRowTemplateLayerOpt 表格行模板构造选项(公共 + 模板行专属;模板行暂无专属选项)
type tableRowTemplateLayerOpt interface{ applyTableRowTemplate(*TableRowTemplate) }

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

// WithText 文本内容(PHP setText 的选项形态):字面 setter,解除表达式标记
func WithText(text string) textOpt {
	return func(l *TextLayer) { l.SetText(text) }
}

// WithFont 字体设定:字体文件路径/URL + 字号 + 颜色(PHP setFont 的选项形态);
// 空串或纯数字字体 = 渲染端内置默认字体语义,并清空已物化结果
func WithFont(font string, fontSize int, fontColor string) textOpt {
	return func(l *TextLayer) { l.setFont(font, fontSize, fontColor) }
}

// WithAutowrap 是否按内容盒宽自动断行
func WithAutowrap(autowrap bool) textOpt {
	return func(l *TextLayer) { l.autowrap = autowrap }
}

// WithAngle 文本旋转角度
func WithAngle(angle int) textOpt {
	return func(l *TextLayer) { l.textAngle = angle }
}

// WithLineBreaker 覆写默认断行器(Canvas 级统一切换断行策略时使用,
// 保证多个渲染端拿到同一排版结果)
func WithLineBreaker(breaker text.LineBreaker) textOpt {
	return func(l *TextLayer) { l.lineBreaker = breaker }
}

// WithMeasurerFactory 覆写默认度量器工厂(Canvas 级统一切换度量策略时使用)
func WithMeasurerFactory(factory text.MeasurerFactory) textOpt {
	return func(l *TextLayer) { l.measurerFactory = factory }
}

// WithQrText 二维码内容(PHP setText 的选项形态)
func WithQrText(text string) qrCodeOpt {
	return func(l *QrCodeLayer) { l.SetText(text) }
}

// WithRows 批量收纳行:内部走同一条 AddRow 路径(行宽同步、内容盒高累加)。
// 选项无错误通道;模板态误用属编程错误,与 marshalNode 同款 fail-fast
func WithRows(rows ...*TableRowLayer) tableOpt {
	return func(l *TableLayer) {
		for _, r := range rows {
			if err := l.AddRow(r); err != nil {
				panic(err)
			}
		}
	}
}

// WithCells 批量收纳单元格:内部走同一条 AddCell 路径(行高取最高)
func WithCells(cells ...*TableCellLayer) tableRowOpt {
	return func(l *TableRowLayer) {
		for _, c := range cells {
			l.AddCell(c)
		}
	}
}

// WithContent 包装内容层:内部走同一条 AddContentLayer 路径(尺寸同步/压平/采纳)
func WithContent(content contentLayer) tableCellOpt {
	return func(l *TableCellLayer) { l.AddContentLayer(content) }
}

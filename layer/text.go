package layer

import (
	"math"

	"github.com/hankchen/go-canvas/text"
)

// TextLayer 文本图层:断行与度量经可注入策略完成(布局纯函数,渲染端共享同一结果)。
// 默认策略对齐 PHP:启发式度量 + 照搬版贪心断行器(ADR-0003)
type TextLayer struct {
	base
	text      string
	font      string
	fontSize  int
	fontColor string
	textAngle int
	autowrap  bool

	lineBreaker     text.LineBreaker
	measurerFactory text.MeasurerFactory

	// Resolver 物化后的本地字体路径,渲染端优先使用
	resolvedFont *string
}

// NewTextLayer 构造文本图层;默认值逐字段对齐 PHP:垂直 bottom(覆写基类 top)、
// 字号 12、色 #000000、水平对齐沿用基类 left
func NewTextLayer(opts ...textLayerOpt) *TextLayer {
	l := &TextLayer{
		base:            newBase(),
		fontSize:        12,
		fontColor:       "#000000",
		lineBreaker:     text.NewUax14LineBreaker(),
		measurerFactory: text.HeuristicMeasurerFactory,
	}
	l.verticalAlign = AlignBottom
	for _, o := range opts {
		o.applyText(l)
	}
	return l
}

func (l *TextLayer) setFont(font string, fontSize int, fontColor string) {
	l.font = font
	l.fontSize = fontSize
	l.fontColor = fontColor
	l.resolvedFont = nil
}

// Text 文本内容
func (l *TextLayer) Text() string { return l.text }

// Font 字体文件路径/URL;空串或纯数字表示使用渲染端内置默认字体
func (l *TextLayer) Font() string { return l.font }

// FontSize 字号
func (l *TextLayer) FontSize() int { return l.fontSize }

// FontColor 字体颜色
func (l *TextLayer) FontColor() string { return l.fontColor }

// Angle 文本旋转角度
func (l *TextLayer) Angle() int { return l.textAngle }

// Autowrap 是否按内容盒宽自动断行
func (l *TextLayer) Autowrap() bool { return l.autowrap }

// SetResolvedFont 回写物化结果。仅供 ResourceResolver 使用(对应 PHP @internal),业务方勿调。
func (l *TextLayer) SetResolvedFont(path string) {
	l.resolvedFont = &path
}

// ResolvedFont 渲染实际使用的字体文件:优先物化结果;空串/纯数字表示渲染端内置默认字体
func (l *TextLayer) ResolvedFont() string {
	if l.resolvedFont != nil {
		return *l.resolvedFont
	}
	return l.font
}

// Height 文本高度(覆写基类):auto 时 = 行数×行高像素 + padding 高
// (非 autowrap 无文本时仅 padding);否则返回声明高度
func (l *TextLayer) Height() int {
	if !l.autoHeight {
		return l.height
	}

	paddingHeight := int(l.padding.Top + l.padding.Bottom)

	if l.autowrap {
		return l.LineHeightPx()*len(l.Lines()) + paddingHeight
	}

	if l.text != "" {
		return l.LineHeightPx() + paddingHeight
	}

	return paddingHeight
}

// Lines 断行结果(纯布局函数):autowrap 开启时按内容盒宽断行,否则整段单行(含空文本)。
// 度量工厂的字体值传 ResolvedFont():真实字体度量需要物化后的本地路径才能加载,
// 未物化时回落原始值(启发式工厂忽略字体值,两种口径无观察差异)
func (l *TextLayer) Lines() []string {
	if !l.autowrap {
		return []string{l.text}
	}

	return l.lineBreaker.BreakText(
		l.text,
		float64(l.ContentWidth()),
		l.measurerFactory(l.ResolvedFont(), float64(l.fontSize)),
	)
}

// LineHeightPx 单行高度(像素)= 字号 × 行高倍数,向上取整
func (l *TextLayer) LineHeightPx() int {
	return int(math.Ceil(float64(l.fontSize) * l.lineHeight))
}

// TextOrigin 文本在内容盒内的绘制基准点(对齐 + 行数),纯布局计算,渲染端共用。
// 返回纯对齐锚点——PHP 的 GD 基线魔数 - round(fontSize*0.1) 不移植(ADR-0003),
// 基线差由渲染后端 drawText 用字体 metrics 消化;其余分支结构与 PHP 逐条对应
func (l *TextLayer) TextOrigin() (int, int) {
	lineCount := len(l.Lines())
	if lineCount < 1 {
		lineCount = 1
	}

	// 取值 left/center/right,未知取值归 0(PHP match default 臂)
	posx := 0
	switch l.horizontalAlign {
	case AlignCenter:
		posx = l.ContentWidth() / 2
	case AlignRight:
		posx = l.ContentWidth()
	}

	// 取值 top/center/bottom,未知取值归 0(PHP match default 臂)
	posy := 0
	switch l.verticalAlign {
	case AlignCenter:
		if l.autoHeight {
			posy = l.LineHeightPx() / 2
		} else {
			posy = (l.ContentHeight() - l.LineHeightPx()*(lineCount-1)) / 2
		}
	case AlignBottom:
		if l.autowrap {
			posy = l.ContentHeight() - l.LineHeightPx()*(lineCount-1)
		} else {
			posy = l.ContentHeight()
		}
	}

	return posx, posy
}

// TypeName implements Layer
func (l *TextLayer) TypeName() string { return TypeText }

// Graph 序列化为 wire 节点:spec.fontFamily 保留完整 font 原始值(路径/URL,不做
// basename 截断,对齐 PHP);data 恒含 expression 空串占位与文本 value
func (l *TextLayer) Graph() Node {
	n := l.base.wireNode(TypeText)
	n.Spec.FontFamily = &FontFamily{
		Font:      l.font,
		FontSize:  l.fontSize,
		FontColor: l.fontColor,
		Angle:     l.textAngle,
		Autowrap:  l.autowrap,
	}
	value := l.text
	expression := ""
	n.Data = &Data{ValueType: ValueTypeStatic, Expression: &expression, Value: &value}
	return n
}

// TextFromGraph 由 wire 节点重建文本图层(工厂 FromGraph 的文本分支)。
// wire 缺 fontFamily/data 键时保留构造默认(对齐 PHP fromGraph 的 isset 容错)
func TextFromGraph(n Node) *TextLayer {
	l := NewTextLayer()
	l.applyNode(n)
	if ff := n.Spec.FontFamily; ff != nil {
		l.setFont(ff.Font, ff.FontSize, ff.FontColor)
		l.textAngle = ff.Angle
		l.autowrap = ff.Autowrap
	}
	if n.Data != nil {
		l.text = derefOrEmpty(n.Data.Value)
	}
	return l
}

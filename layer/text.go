package layer

import (
	"math"
	"strings"

	"github.com/hankchen/go-canvas/text"
)

// TextLayer 文本图层:断行与度量经可注入策略完成(布局纯函数,渲染端共享同一结果)。
// 默认策略对齐 PHP:启发式度量 + 照搬版贪心断行器(ADR-0003)
type TextLayer struct {
	base
	text string
	// expression 数据表达式标记(TableLayer V2,spec §3.1):非 nil = 已标记,
	// value 载体(text)恒镜像表达式原文;字面 setter 解除标记
	expression *string
	font       string
	fontSize   int
	fontColor  string
	textAngle  int
	autowrap   bool

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

// SetText 设置字面文本,并解除表达式标记(标记与字面互斥,PHP setText 同款)
func (l *TextLayer) SetText(text string) {
	l.text = text
	l.expression = nil
}

// SetExpression 标记数据表达式(spec §3.1):value 载体恒镜像表达式原文
// (旧端降级可见、审计可读的求值源记录;求值结果永不落图层)
func (l *TextLayer) SetExpression(expression string) {
	l.expression = &expression
	l.text = expression
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

// Width 宽度(覆写基类,ADR 0014):autoWidth 时 = 未断行自然宽——按显式换行拆段
// 取最大度量宽(ceil)+ 横向 padding(向零截断求和,镜像 Height() 纵向 padding);
// 空文本 = 0 + 横向 padding(空串拆段得 [""],度量宽 0 自然落到该分支)。求值忽略
// autowrap——内容盒宽即自然宽,断行不再切分(除显式换行),组合退化为不折行,
// autowrap 标志留在结构不动;拆段只按 \n 硬拆、不经 LineBreaker 缝(断行策略属
// autowrap 渲染路径)。度量经层内度量缝(缺省启发式,可注入增强),字体值口径同
// Lines()(工单 09),布局不触发物化
func (l *TextLayer) Width() int {
	if !l.autoWidth {
		return l.width
	}

	measurer := l.measurerFactory(l.ResolvedFont(), float64(l.fontSize))

	natural := 0.0
	for _, segment := range strings.Split(l.text, "\n") {
		natural = math.Max(natural, measurer.Measure(segment))
	}

	return int(math.Ceil(natural)) + int(l.padding.Left+l.padding.Right)
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

// ContentHeight 内容区高度 = 动态高 - 上下 padding。覆写:Height() 有 auto 语义
// (行数×行高+padding),须按多态结果计算——PHP getContentHeight 经
// $this->getHeight() 动态分派,基类版读原始字段对 auto 图层是错的
// (工单10 布局快照 fixture 抓出的移植偏差)
func (l *TextLayer) ContentHeight() int {
	return contentHeightOf(l.Height(), l.padding)
}

// ContentWidth 内容区宽度 = 动态宽 - 左右 padding。覆写:Width() 有 auto 语义
// (自然宽,ADR 0014),须按多态结果计算——对齐 PHP getContentWidth 经
// $this->getWidth() 动态分派;autoWidth + autowrap 组合的内容盒宽即自然宽,
// 断行不再折行(基类版读原始字段,autoWidth 层会得到 0-padding 的负盒)
func (l *TextLayer) ContentWidth() int {
	return contentWidthOf(l.Width(), l.padding)
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
	if l.expression != nil {
		n.Data = &Data{ValueType: ValueTypeExpression, Expression: l.expression, Value: &value}
	} else {
		expression := ""
		n.Data = &Data{ValueType: ValueTypeStatic, Expression: &expression, Value: &value}
	}
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
		if n.Data.ValueType == ValueTypeExpression && n.Data.Expression != nil {
			l.SetExpression(*n.Data.Expression)
		} else {
			l.text = derefOrEmpty(n.Data.Value)
		}
	}
	return l
}

package layer

import (
	"image"
	"math"
)

// ImageLayer 图片图层:只记录资源原始引用(URL 或本地路径),
// 下载与物化由渲染前的 ResourceResolver 统一完成——构造与 setter 不做任何 I/O。

// 适配模式取值(ADR 0023/0024):cover 铺满裁切(缺省)= 等比缩放铺满内容盒、
// 溢出居中裁切;contain 完整显示 = 等比缩放至恰好完整落入内容盒、按对齐放置,
// 空隙透明;fill 拉伸铺满 = 非等比缩放至恰好铺满内容盒,不裁切不留空隙(不保源图
// 宽高比);tile 平铺 = 按内在尺寸 1:1 原尺寸重复铺放,首块按对齐锚定、向右下
// 无限重复、止于网格 ∩ 内容盒(末行末列半块截断,见根 CONTEXT.md「平铺(tile)」词条)。
// 未知值宽容保存(渲染面按 cover 降级,白名单在渲染臂)
const (
	FitCover   = "cover"
	FitContain = "contain"
	FitFill    = "fill"
	FitTile    = "tile"
)

// 裁剪形状取值(ADR 0026):两档——矩形(rect,缺省不裁)与圆形(circle,内切圆)。
// 圆形档是完整圆形框:背景色填充进圆、内容按 fit 绘出后遮罩进圆、边框沿圆周描成环。
// 未知值宽容保存(渲染面按矩形降级,白名单在渲染臂)
const (
	ClipRect   = "rect"
	ClipCircle = "circle"
)

// ImageLayer 图片图层
type ImageLayer struct {
	base
	rawImg *string
	// expression 数据表达式标记(TableLayer V2,spec §3.1):非 nil = 已标记,
	// value 载体(rawImg)恒镜像表达式原文;字面 setter 解除标记
	expression *string
	// fit 适配模式声明(ADR 0023):静态声明不参与数据表达式,只影响绘制、
	// 不影响盒子布局;空串 = cover 缺省,未知值原样透传
	fit string
	// clip 裁剪形状声明(ADR 0026):静态声明不参与数据表达式,只影响绘制、
	// 不影响盒子布局;空串 = rect 缺省,未知值原样透传
	clip        string
	resolvedSrc *string
}

// NewImageLayer 构造图片图层;默认对齐 center/center(PHP 字段覆写),其余同基类默认态
func NewImageLayer(opts ...imageLayerOpt) *ImageLayer {
	l := &ImageLayer{base: newBase()}
	l.horizontalAlign = AlignCenter
	l.verticalAlign = AlignCenter
	for _, o := range opts {
		o.applyImage(l)
	}
	return l
}

// WithImage 图片引用(PHP setImage 的选项形态)
func WithImage(src string) imageOpt {
	return func(l *ImageLayer) { l.SetImage(src) }
}

// SetImage 设置引用:空串归 null,并清空已物化结果
func (l *ImageLayer) SetImage(src string) {
	if src == "" {
		l.rawImg = nil
	} else {
		l.rawImg = &src
	}
	l.expression = nil
	l.resolvedSrc = nil
}

// SetExpression 标记数据表达式(spec §3.1):value 载体恒镜像表达式原文
// (旧端降级可见、审计可读的求值源记录;求值结果永不落图层)
func (l *ImageLayer) SetExpression(expression string) {
	l.expression = &expression
	l.rawImg = &expression
	l.resolvedSrc = nil
}

// Image 原始引用(URL 或本地路径)
func (l *ImageLayer) Image() *string { return l.rawImg }

// SetResolvedSrc 回写物化结果。仅供 ResourceResolver 使用(对应 PHP @internal),业务方勿调。
func (l *ImageLayer) SetResolvedSrc(src string) {
	l.resolvedSrc = &src
}

// SetFit 声明适配模式(ADR 0023):与 src 正交(改图/打标/解标均不扰动);
// cover 显式声明归一空串缺省态,保条件写键字节面(name="" 先例)
func (l *ImageLayer) SetFit(fit string) {
	if fit == FitCover {
		fit = ""
	}
	l.fit = fit
}

// Fit 适配模式声明原文;空串 = cover 缺省(渲染面未知值按 cover 降级,白名单在渲染臂)
func (l *ImageLayer) Fit() string { return l.fit }

// SetClip 声明裁剪形状(ADR 0026):与 src/fit 正交(改图/打标/解标均不扰动);
// rect 显式声明归一空串缺省态,保条件写键字节面(fit 显式 cover 归一先例)
func (l *ImageLayer) SetClip(clip string) {
	if clip == ClipRect {
		clip = ""
	}
	l.clip = clip
}

// Clip 裁剪形状声明原文;空串 = rect 缺省(渲染面未知值按矩形降级,白名单在渲染臂)
func (l *ImageLayer) Clip() string { return l.clip }

// ResolvedSrc 渲染实际可用的图片来源:优先物化结果,回退原始引用
func (l *ImageLayer) ResolvedSrc() *string {
	if l.resolvedSrc != nil {
		return l.resolvedSrc
	}
	return l.rawImg
}

// ImageOrigin 图片的绘制起点(对齐 + padding,PHP getImageOrigin 同款参照系:
// left/top 臂取 padding,center/贴边臂按宽高差)。纯布局计算,渲染端共用;
// 未知取值归 0(PHP match default 臂);整除向零截断,对齐 PHP intval
func (l *ImageLayer) ImageOrigin() (int, int) {
	return l.alignedX(l.ContentWidth()), l.alignedY(l.ContentHeight())
}

// ContainRect contain 收缩矩形(ADR 0023):按源图内在比例把内容盒收缩成等比矩形——
// 两维同一缩放系数取 min(内容宽比, 内容高比),长边贴内容盒;放置按对齐 + padding
// 与 ImageOrigin 同门公式。纯布局计算,渲染端共用;内在尺寸由调用方读取
// (读取失败统一退化 fill 形态,ADR 0024),非法尺寸(≤0)不进本函数,
// 收缩结果 ≤0 由后端 DrawImage 防护只画盒
func (l *ImageLayer) ContainRect(imgWidth, imgHeight int) (int, int, int, int) {
	scale := math.Min(float64(l.ContentWidth())/float64(imgWidth), float64(l.ContentHeight())/float64(imgHeight))
	width := int(float64(imgWidth) * scale)
	height := int(float64(imgHeight) * scale)
	return l.alignedX(width), l.alignedY(height), width, height
}

// CoverRect cover 等比居中裁窗(ADR 0024):源图坐标系里与内容盒同宽高比的窗口,
// 窗外部分被裁切不绘制——与 ContainRect 同门的纯布局函数(box/scale 公式互为
// max/min 镜像),cover 窗公式唯一实例落包级 CoverSourceWindow。对齐不进源窗
// (cover 恒居中裁),padding 经内容盒收缩间接收窄窗口;内在尺寸由调用方读取,
// 非法尺寸(≤0)不进本函数
func (l *ImageLayer) CoverRect(imgWidth, imgHeight int) image.Rectangle {
	return CoverSourceWindow(l.ContentWidth(), l.ContentHeight(), imgWidth, imgHeight)
}

// CoverSourceWindow cover 裁窗公式唯一落点(ADR 0024):图片臂(内容盒)与画布
// 背景臂(画布矩形)共用同一公式防数学漂移;boxWidth×boxHeight 为目标矩形
// (无 padding 语义时传整盒)。整除向零截断对齐 PHP intval
func CoverSourceWindow(boxWidth, boxHeight, imgWidth, imgHeight int) image.Rectangle {
	scale := math.Max(float64(boxWidth)/float64(imgWidth), float64(boxHeight)/float64(imgHeight))
	windowWidth := int(float64(boxWidth) / scale)
	windowHeight := int(float64(boxHeight) / scale)

	return image.Rect(
		int(float64(imgWidth-windowWidth)/2),
		int(float64(imgHeight-windowHeight)/2),
		int(float64(imgWidth-windowWidth)/2)+windowWidth,
		int(float64(imgHeight-windowHeight)/2)+windowHeight,
	)
}

// InscribedCircleRect 内切圆矩形(ADR 0026):边长 = min(内容区宽高),按盒内对齐
// 定位(alignedX/alignedY 同门公式——center 取整盒中点、left/top 贴 padding 原点侧),
// 内边距即框外留白;非方盒恒正方几何(渲染面即正圆),不变形为椭圆。
// 纯布局计算,与 ContainRect/QrOrigin 同门;边长 ≤0(内容区被 padding 吃满或
// 超吃)归零矩形——Go image.Rect 会规范化交换 Min/Max,负边长的原始形态
// (PHP 返回 [x, y, -side, -side]、渲染臂 $side <= 0 拦截)在 Go 不可表达,
// 防线前移到本函数;渲染臂空矩形防护照旧(PHP 同位双门)
func (l *ImageLayer) InscribedCircleRect() image.Rectangle {
	side := min(l.ContentWidth(), l.ContentHeight())
	if side <= 0 {
		return image.Rectangle{}
	}
	x := l.alignedX(side)
	y := l.alignedY(side)
	return image.Rect(x, y, x+side, y+side)
}

// CircleStroke 圆周描边参数解析(ADR 0026):宽取四边已声明宽的最小值、颜色按
// top→right→bottom→left 取首个非空边色、四边全未声明不描边(规则明文在根
// CONTEXT.md「裁剪形状(clip shape)」词条)。纯函数,返回边框边目同款形态;
// 未声明或 width≤0(wire 解码可注入)的边不计——与渲染端 borderBands 同纪律
func (l *ImageLayer) CircleStroke() *BorderSide {
	declared := make([]*BorderSide, 0, 4)
	for _, side := range []*BorderSide{l.border.Top, l.border.Right, l.border.Bottom, l.border.Left} {
		if side != nil && side.Width > 0 {
			declared = append(declared, side)
		}
	}
	if len(declared) == 0 {
		return nil
	}

	width := declared[0].Width
	for _, side := range declared[1:] {
		width = min(width, side.Width)
	}
	return &BorderSide{Width: width, Color: declared[0].Color}
}

// TileBlock tile 可见块(ADR 0024):src 为源图坐标的可见子矩形、dst 为图层盒
// 坐标的落位矩形,两矩形同尺寸(可见子矩形 1:1 映射)
type TileBlock struct {
	Src image.Rectangle
	Dst image.Rectangle
}

// TileRects tile 可见块矩形组(ADR 0024):单元 = 内在尺寸 1:1 零参数,首块左上
// 锚定于对齐 + padding(与 ImageOrigin/ContainRect 同一 aligned 公式——小图 center
// 网格整体居中、大图 center 锚点为负露出图片中部),网格自首块向右向下无限延伸,
// 绘制面 = 网格 ∩ 内容盒:末行末列半块截断、锚点未覆盖处空隙透明露下层。
// 行主序返回;可见块数不设上限(静默截断是语义意外)。纯布局计算;
// 内在尺寸由调用方读取,非法尺寸(≤0)不进本函数(Go 侧防御 0 步进死循环返回 nil)
func (l *ImageLayer) TileRects(imgWidth, imgHeight int) []TileBlock {
	if imgWidth <= 0 || imgHeight <= 0 {
		return nil
	}

	anchorX := l.alignedX(imgWidth)
	anchorY := l.alignedY(imgHeight)
	left := int(l.padding.Left)
	top := int(l.padding.Top)
	right := left + l.ContentWidth()
	bottom := top + l.ContentHeight()

	blocks := []TileBlock{}
	for y := anchorY; y < bottom; y += imgHeight {
		dstY := max(y, top)
		dstHeight := min(y+imgHeight, bottom) - dstY
		if dstHeight <= 0 {
			continue
		}
		for x := anchorX; x < right; x += imgWidth {
			dstX := max(x, left)
			dstWidth := min(x+imgWidth, right) - dstX
			if dstWidth <= 0 {
				continue
			}
			blocks = append(blocks, TileBlock{
				Src: image.Rect(dstX-x, dstY-y, dstX-x+dstWidth, dstY-y+dstHeight),
				Dst: image.Rect(dstX, dstY, dstX+dstWidth, dstY+dstHeight),
			})
		}
	}

	return blocks
}

// alignedX 对齐放置横向公式(align + padding 同门):placedWidth 尺寸的内容在
// 整盒内的起点——left 臂取 padding、center/贴边臂按整盒宽差(不取内容盒,
// PHP alignedX 同款);未知取值归 0,整除向零截断对齐 PHP intval
func (l *ImageLayer) alignedX(placedWidth int) int {
	switch l.horizontalAlign {
	case AlignLeft:
		return int(l.padding.Left)
	case AlignCenter:
		return (l.Width() - placedWidth) / 2
	case AlignRight:
		return l.Width() - placedWidth
	default:
		return 0
	}
}

// alignedY 对齐放置纵向公式:与 alignedX 同门镜像
func (l *ImageLayer) alignedY(placedHeight int) int {
	switch l.verticalAlign {
	case AlignTop:
		return int(l.padding.Top)
	case AlignCenter:
		return (l.Height() - placedHeight) / 2
	case AlignBottom:
		return l.Height() - placedHeight
	default:
		return 0
	}
}

// TypeName implements Layer
func (l *ImageLayer) TypeName() string { return TypeImage }

// Graph 序列化为 wire 节点;data 条件写键:标记态三键(ExpressionValue),
// 未标记两键(StaticValue,无 expression 键);fit 条件写键(ADR 0023):
// 缺省(cover)整键不落、缺省态字节面与无字段版本一致,落键殿后 spec 末位
// (shape/align/position/fit);clip 条件写键(ADR 0026):缺省(rect)整键不落、
// 殿后 spec 末位 fit 之后(键序 shape/align/position/fit/clip);未知值原样透传
// 保往返恒等——保 Go↔PHP 字节 parity
func (l *ImageLayer) Graph() Node {
	n := l.base.wireNode(TypeImage)
	if l.expression != nil {
		n.Data = &Data{ValueType: ValueTypeExpression, Expression: l.expression, Value: l.rawImg}
	} else {
		n.Data = &Data{ValueType: ValueTypeStatic, Value: l.rawImg}
	}
	if l.fit != "" {
		fit := l.fit
		n.Spec.Fit = &fit
	}
	if l.clip != "" {
		clip := l.clip
		n.Spec.Clip = &clip
	}
	return n
}

// ImageFromGraph 由 wire 节点重建图片图层(工厂 FromGraph 的图片分支)。
// fit 宽容读(ADR 0023):无键回填缺省,显式 cover 归一空串,未知值原样透传;
// clip 宽容读(ADR 0026):无键回填缺省,显式 rect 归一空串,未知值原样透传
func ImageFromGraph(n Node) *ImageLayer {
	l := NewImageLayer()
	l.applyNode(n)
	if n.Spec.Fit != nil {
		l.SetFit(*n.Spec.Fit)
	}
	if n.Spec.Clip != nil {
		l.SetClip(*n.Spec.Clip)
	}
	if n.Data != nil {
		if n.Data.ValueType == ValueTypeExpression && n.Data.Expression != nil {
			l.SetExpression(*n.Data.Expression)
		} else {
			l.SetImage(derefOrEmpty(n.Data.Value))
		}
	}
	return l
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

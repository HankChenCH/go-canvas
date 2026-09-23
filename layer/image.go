package layer

// ImageLayer 图片图层:只记录资源原始引用(URL 或本地路径),
// 下载与物化由渲染前的 ResourceResolver 统一完成——构造与 setter 不做任何 I/O。

// ImageLayer 图片图层
type ImageLayer struct {
	base
	rawImg      *string
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
	l.resolvedSrc = nil
}

// Image 原始引用(URL 或本地路径)
func (l *ImageLayer) Image() *string { return l.rawImg }

// SetResolvedSrc 回写物化结果。仅供 ResourceResolver 使用(对应 PHP @internal),业务方勿调。
func (l *ImageLayer) SetResolvedSrc(src string) {
	l.resolvedSrc = &src
}

// ResolvedSrc 渲染实际可用的图片来源:优先物化结果,回退原始引用
func (l *ImageLayer) ResolvedSrc() *string {
	if l.resolvedSrc != nil {
		return l.resolvedSrc
	}
	return l.rawImg
}

// ImageOrigin 图片在内容盒内的放置起点(对齐 + padding),纯布局计算,渲染端共用。
// 未知取值归 0(PHP match default 臂);整除向零截断,对齐 PHP intval
func (l *ImageLayer) ImageOrigin() (int, int) {
	posx := 0
	switch l.horizontalAlign {
	case AlignLeft:
		posx = int(l.padding.Left)
	case AlignCenter:
		posx = int(float64(l.Width()-l.ContentWidth()) / 2)
	case AlignRight:
		posx = l.Width() - l.ContentWidth()
	}

	posy := 0
	switch l.verticalAlign {
	case AlignTop:
		posy = int(l.padding.Top)
	case AlignCenter:
		posy = int(float64(l.Height()-l.ContentHeight()) / 2)
	case AlignBottom:
		posy = l.Height() - l.ContentHeight()
	}

	return posx, posy
}

// TypeName implements Layer
func (l *ImageLayer) TypeName() string { return TypeImage }

// Graph 序列化为 wire 节点;data 仅 valueType/value 两键(无 expression 预留键)
func (l *ImageLayer) Graph() Node {
	n := l.base.wireNode(TypeImage)
	n.Data = &Data{ValueType: ValueTypeStatic, Value: l.rawImg}
	return n
}

// ImageFromGraph 由 wire 节点重建图片图层(工厂 FromGraph 的图片分支)
func ImageFromGraph(n Node) *ImageLayer {
	l := NewImageLayer()
	l.applyNode(n)
	if n.Data != nil {
		l.SetImage(derefOrEmpty(n.Data.Value))
	}
	return l
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

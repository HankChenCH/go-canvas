package layer

// QrCodeLayer 二维码图层:只存二维码内容文本,图像由渲染前的 ResourceResolver
// 经 QRMaterializer 物化为本地 PNG(ADR-0002)——构造与 setter 不做任何 I/O。

// QrCodeLayer 二维码图层
type QrCodeLayer struct {
	base
	qrText string
	// expression 数据表达式标记(TableLayer V2,spec §3.1):非 nil = 已标记,
	// value 载体(qrText)恒镜像表达式原文;字面 setter 解除标记
	expression  *string
	resolvedSrc *string
}

// NewQrCodeLayer 构造二维码图层;对齐/定位沿用基类默认(left/top,PHP 无覆写)
func NewQrCodeLayer(opts ...qrCodeLayerOpt) *QrCodeLayer {
	l := &QrCodeLayer{base: newBase()}
	for _, o := range opts {
		o.applyQrCode(l)
	}
	return l
}

// SetText 设置二维码内容,并解除表达式标记、清空已物化结果
func (l *QrCodeLayer) SetText(content string) {
	l.qrText = content
	l.expression = nil
	l.resolvedSrc = nil
}

// SetExpression 标记数据表达式(spec §3.1):value 载体恒镜像表达式原文
// (旧端降级可见、审计可读的求值源记录;求值结果永不落图层)
func (l *QrCodeLayer) SetExpression(expression string) {
	l.expression = &expression
	l.qrText = expression
	l.resolvedSrc = nil
}

// Text 二维码内容
func (l *QrCodeLayer) Text() string { return l.qrText }

// SetResolvedSrc 回写物化结果。仅供 ResourceResolver 使用(对应 PHP @internal),业务方勿调。
func (l *QrCodeLayer) SetResolvedSrc(src string) {
	l.resolvedSrc = &src
}

// ResolvedSrc 已物化的二维码图片,未物化为 nil
// (不回退内容文本:内容经 Text() 与 data.value 获取,PHP getResolvedSrc 同为可空)
func (l *QrCodeLayer) ResolvedSrc() *string { return l.resolvedSrc }

// Height 高度(覆写基类):未声明高度(auto 或 0)时按宽度兜底成正方形,
// 避免 0 高盒子导致渲染报错(PHP QrCodeLayer::getHeight 同款)
func (l *QrCodeLayer) Height() int {
	if !l.autoHeight && l.height > 0 {
		return l.height
	}
	return l.Width()
}

// ContentHeight 内容区高度 = 动态高 - 上下 padding。覆写理由同 TextLayer:
// Height() 有宽度兜底语义,须按多态结果计算(PHP getContentHeight 动态分派)
func (l *QrCodeLayer) ContentHeight() int {
	return contentHeightOf(l.Height(), l.padding)
}

// ContentSide 二维码内容边长 = min(内容区宽, 内容区高):内切于内容盒的正方形
// (quiet zone 语义——padding 留白即码外静区),≤0 时绘制端 DrawImage 防护只画盒;
// 内容区负值直通(PHP intval 同门,不钳零)
func (l *QrCodeLayer) ContentSide() int {
	return min(l.ContentWidth(), l.ContentHeight())
}

// QrOrigin 二维码在内容盒内的放置起点(对齐 + padding,PHP getQrOrigin 同款参照系:
// 镜像 ImageLayer.ImageOrigin,内容尺寸换成内切边长)。纯布局计算,渲染端共用;
// 未知取值归 0(PHP match default 臂);整除向零截断,对齐 PHP intval
func (l *QrCodeLayer) QrOrigin() (int, int) {
	side := l.ContentSide()

	posx := 0
	switch l.horizontalAlign {
	case AlignLeft:
		posx = int(l.padding.Left)
	case AlignCenter:
		posx = (l.Width() - side) / 2
	case AlignRight:
		posx = l.Width() - side
	}

	posy := 0
	switch l.verticalAlign {
	case AlignTop:
		posy = int(l.padding.Top)
	case AlignCenter:
		posy = (l.Height() - side) / 2
	case AlignBottom:
		posy = l.Height() - side
	}

	return posx, posy
}

// TypeName implements Layer
func (l *QrCodeLayer) TypeName() string { return TypeQrCode }

// Graph 序列化为 wire 节点;data 条件写键:标记态三键(ExpressionValue),
// 未标记两键(StaticValue,无 expression 键)——保 Go↔PHP 字节 parity;
// value 始终携带内容,与是否已物化无关(无损)
func (l *QrCodeLayer) Graph() Node {
	n := l.base.wireNode(TypeQrCode)
	value := l.qrText
	if l.expression != nil {
		n.Data = &Data{ValueType: ValueTypeExpression, Expression: l.expression, Value: &value}
	} else {
		n.Data = &Data{ValueType: ValueTypeStatic, Value: &value}
	}
	return n
}

// QrCodeFromGraph 由 wire 节点重建二维码图层(工厂 FromGraph 的二维码分支)
func QrCodeFromGraph(n Node) *QrCodeLayer {
	l := NewQrCodeLayer()
	l.applyNode(n)
	if n.Data != nil {
		if n.Data.ValueType == ValueTypeExpression && n.Data.Expression != nil {
			l.SetExpression(*n.Data.Expression)
		} else {
			l.SetText(derefOrEmpty(n.Data.Value))
		}
	}
	return l
}

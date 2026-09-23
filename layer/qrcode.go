package layer

// QrCodeLayer 二维码图层:只存二维码内容文本,图像由渲染前的 ResourceResolver
// 经 QRMaterializer 物化为本地 PNG(ADR-0002)——构造与 setter 不做任何 I/O。

// QrCodeLayer 二维码图层
type QrCodeLayer struct {
	base
	qrText      string
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

// SetText 设置二维码内容,并清空已物化结果
func (l *QrCodeLayer) SetText(content string) {
	l.qrText = content
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

// TypeName implements Layer
func (l *QrCodeLayer) TypeName() string { return TypeQrCode }

// Graph 序列化为 wire 节点;data 仅 valueType/value 两键(无 expression 预留键),
// value 始终携带内容,与是否已物化无关(无损)
func (l *QrCodeLayer) Graph() Node {
	n := l.base.wireNode(TypeQrCode)
	value := l.qrText
	n.Data = &Data{ValueType: ValueTypeStatic, Value: &value}
	return n
}

// QrCodeFromGraph 由 wire 节点重建二维码图层(工厂 FromGraph 的二维码分支)
func QrCodeFromGraph(n Node) *QrCodeLayer {
	l := NewQrCodeLayer()
	l.applyNode(n)
	if n.Data != nil {
		l.SetText(derefOrEmpty(n.Data.Value))
	}
	return l
}

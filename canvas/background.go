package canvas

import "encoding/json"

// 画布背景(ADR 0022 / canvas-background spec §3):画布级声明式填充(背景色 +
// 背景图),纯值对象。对拍基准 = php-canvas-next CanvasBackground(v1.3.0 权威端)。
//
// - 背景是渲染语义、不属图层树:无显隐、无优先级、不进命中面
// - 静态:不进 hydrate 改写面与表达式白名单,hydrate/expand 仅透传
// - wire 条件写键:两分量皆空 → Graph() 整键不落(name/visible 先例);
//   落键时仅非空分量落键,键序钉死 color, image
// - image 为裸 string 资源引用(URL 或本地路径,与 ImageLayer src 同构);适配固定 cover

// CanvasBackground 画布背景值对象(不可变:无 setter,分页/文档各页共享同实例安全)。
// 空串与 nil 同归缺省(与 ImageLayer 同精神):条件写键的空分量
type CanvasBackground struct {
	// Color 背景色(与图层 shape.backgroundColor 同格式域);nil = 未设
	Color *string `json:"color,omitempty"`
	// Image 背景图资源引用(远程 URL 或本地路径);nil = 未设
	Image *string `json:"image,omitempty"`
}

// NewCanvasBackground 构造画布背景;空串分量归 nil(PHP 构造器空串同归缺省同款)。
// 两参数按 wire 键序 color, image 排列,未设分量传 nil
func NewCanvasBackground(color, image *string) *CanvasBackground {
	if color != nil && *color == "" {
		color = nil
	}
	if image != nil && *image == "" {
		image = nil
	}
	return &CanvasBackground{Color: color, Image: image}
}

// IsEmpty 两分量皆空:与无背景同态,wire 整键不落
func (b *CanvasBackground) IsEmpty() bool {
	return b == nil || (b.Color == nil && b.Image == nil)
}

// UnmarshalJSON 宽容读归一(spec §3):空串分量与缺键同归缺省(PHP fromGraph 经
// 构造器空串归 null 同款),任意来源 wire 的解码 → 再编码往返恒等
func (b *CanvasBackground) UnmarshalJSON(data []byte) error {
	type alias CanvasBackground
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*b = *NewCanvasBackground(a.Color, a.Image)
	return nil
}

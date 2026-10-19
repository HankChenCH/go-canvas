package imagerenderer

import (
	"bytes"
	"image"

	// 图片解码按格式注册:wire 面图片源以 PNG/JPEG 为主,GIF 顺带覆盖
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// decode 解码图片字节到内在坐标系(未按 EXIF 转正)。blit 的 srcRect 源窗与
// 分派层 image.DecodeConfig 同处内在坐标系(ADR 0024),转正由 DrawImage 在
// 裁窗后执行(裁窗 → 转正的次序理由见其 docblock)
func decode(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

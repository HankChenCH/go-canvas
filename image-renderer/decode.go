package imagerenderer

import (
	"bytes"
	"image"

	// 图片解码按格式注册:wire 面图片源以 PNG/JPEG 为主,GIF 顺带覆盖
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// decodeOriented 解码图片字节并按 EXIF orientation 转正到观察方向。
// 仅 JPEG 携带 EXIF,其余格式原样返回
func decodeOriented(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if o := exifOrientation(data); o != 1 {
		return applyOrientation(img, o), nil
	}
	return img, nil
}

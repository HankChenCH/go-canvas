package pdfrenderer

import (
	"bytes"
	"fmt"
	"image"

	// 内在尺寸探测按格式注册:wire 面图片源以 PNG/JPEG 为主,GIF 顺带覆盖
	// (与核心模板/位图后端解码集一致,三格式直嵌)
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// decodeConfig 解码图片头块取内在尺寸(未按 EXIF 转正)。blit 的 srcRect 源窗
// 与分派层 image.DecodeConfig 同处内在坐标系(ADR 0024),转正由 DrawImage 以
// 放置矩阵消化——像素不被搬动,直嵌字节原样进文档
func decodeConfig(data []byte) (image.Rectangle, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return image.Rectangle{}, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return image.Rectangle{}, fmt.Errorf("非法图片尺寸 %dx%d", cfg.Width, cfg.Height)
	}
	return image.Rect(0, 0, cfg.Width, cfg.Height), nil
}

// sniffType 按魔数嗅探直嵌格式(fpdf 的 parsejpg/parsepng/parsegif 只认
// ImageType 标签,而物化缓存键的扩展名来自 URL、不可信)。非三格式报错——
// 对齐位图后端「垃圾字节报错中止」的结局 parity
func sniffType(data []byte) (string, error) {
	switch {
	case len(data) >= 8 && bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")):
		return "png", nil
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8:
		return "jpg", nil
	case len(data) >= 6 && (bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a"))):
		return "gif", nil
	default:
		return "", fmt.Errorf("无法识别的图片格式:仅支持 PNG/JPEG/GIF 直嵌")
	}
}

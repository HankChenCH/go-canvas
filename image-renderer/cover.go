package imagerenderer

import (
	"image"
	"math"

	xdraw "golang.org/x/image/draw"
)

// cover 缩放裁切:等比缩放至铺满目标盒,溢出居中裁掉(PHP intervention cover 同款)。
// 明确放弃逐像素复刻 GD/Imagick(spec Out of Scope),插值选 CatmullRom 质量优先

// coverImage 把 src 等比缩放并居中裁切为 width×height 位图。
// width/height 为正由调用方保证;裁切窗与居中偏移整型化时向零截断
func coverImage(src image.Image, width, height int) image.Image {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()

	// 铺满所需缩放系数取较大者;裁切窗 = 目标盒 / 系数(≤源尺寸)
	scale := math.Max(float64(width)/float64(sw), float64(height)/float64(sh))
	cropW := int(float64(width) / scale)
	cropH := int(float64(height) / scale)
	// 极端宽高比(如 100×4 → 20×100)下向零截断可得 0 宽窗:钳到 ≥1,
	// 宁可纵横比略让也不输出空图
	if cropW < 1 {
		cropW = 1
	}
	if cropH < 1 {
		cropH = 1
	}
	cx := (sw - cropW) / 2
	cy := (sh - cropH) / 2

	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src,
		image.Rect(b.Min.X+cx, b.Min.Y+cy, b.Min.X+cx+cropW, b.Min.Y+cy+cropH),
		xdraw.Over, nil)
	return dst
}

package imagerenderer

import (
	"image"

	xdraw "golang.org/x/image/draw"
)

// blit 拉伸搬运(ADR 0024):源窗子图缩放至目标尺寸——等比与否由分派层几何决定
// (cover 裁窗 / fill 整图的差别只在分派层给出的矩形形状),原语只做像素搬运。
// 明确放弃逐像素复刻 GD/Imagick(spec Out of Scope),插值选 CatmullRom 质量优先
// (沿旧 cover 缩放路线)

// scaleImage 把 src 整幅缩放为 width×height 位图。width/height 为正由调用方保证
func scaleImage(src image.Image, width, height int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	return dst
}

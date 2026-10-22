package imagerenderer

// 形状能力口绘制面(ADR 0026):椭圆绘制原语 + 椭圆掩码合成。stdlib 无椭圆
// 原语,按 stdlib draw 掩码面自绘——二值覆盖无抗锯齿(对齐 PHP GD 驱动形态),
// 像素采样点位距缘留过渡带余量,采样纪律见 clip_test.go

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/HankChenCH/go-canvas/layer"
)

// DrawEllipse implements renderer.ShapeCapableBackend(椭圆绘制原语,ADR 0026):
// 在 (x, y) 起的 width×height 外接矩形内画内切椭圆,fillColor 填充 + stroke 圆周
// 描边一体承接(圆形档的背景填充与描边环同原语两调)。
// width/height ≤ 0 防护不画;stroke.Width ≤ 0 不描(wire 解码可注入,与
// borderBands 同纪律)。描边为外缘内缩带(与 DrawRect 盒内嵌边带同门):
// 内缩 ≥ 半短边时环带吃满整个椭圆内部(几何自然退化)
func (r *Renderer) DrawEllipse(x, y, width, height int, fillColor *string, stroke *layer.BorderSide) error {
	surface, err := r.face()
	if err != nil {
		return err
	}
	if width <= 0 || height <= 0 {
		return nil
	}
	bounds := image.Rect(x, y, x+width, y+height)

	if fillColor != nil && *fillColor != "" {
		c, err := parseColor(*fillColor)
		if err != nil {
			return err
		}
		draw.DrawMask(surface, bounds, &image.Uniform{C: c}, image.Point{}, ellipseMask(bounds, 0), bounds.Min, draw.Over)
	}

	if stroke != nil && stroke.Width > 0 {
		c, err := parseColor(stroke.Color)
		if err != nil {
			return err
		}
		draw.DrawMask(surface, bounds, &image.Uniform{C: c}, image.Point{}, ellipseMask(bounds, stroke.Width), bounds.Min, draw.Over)
	}
	return nil
}

// ellipseMask 外接矩形 bounds 的内切椭圆二值掩码:椭圆内 alpha 255、外 0;
// inset > 0 时排除外缘内缩 inset 的内椭圆(圆周描边环带)。
// 像素中心采样(像素 (x,y) 取 (x+0.5, y+0.5) 判定)
func ellipseMask(bounds image.Rectangle, inset int) *image.Alpha {
	mask := image.NewAlpha(bounds)
	cx := float64(bounds.Min.X) + float64(bounds.Dx())/2
	cy := float64(bounds.Min.Y) + float64(bounds.Dy())/2
	rx := float64(bounds.Dx()) / 2
	ry := float64(bounds.Dy()) / 2

	inner := inset > 0 && rx-float64(inset) > 0 && ry-float64(inset) > 0
	irx, iry := rx-float64(inset), ry-float64(inset)

	opaque := color.Alpha{A: 0xFF}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			if !insideEllipse(px, py, cx, cy, rx, ry) {
				continue
			}
			if inner && insideEllipse(px, py, cx, cy, irx, iry) {
				continue
			}
			mask.SetAlpha(x, y, opaque)
		}
	}
	return mask
}

// applyEllipseMask 以椭圆掩码裁切位图(原位改写):掩码 = ellipse 外接矩形的
// 内切椭圆,掩码外像素置全透明、掩码内源 alpha 不动(二值掩码,对齐 PHP GD
// 驱动逐像素二值 alpha 同门)。ellipse 为 img 局部坐标
func applyEllipseMask(img *image.NRGBA, ellipse image.Rectangle) {
	cx := float64(ellipse.Min.X) + float64(ellipse.Dx())/2
	cy := float64(ellipse.Min.Y) + float64(ellipse.Dy())/2
	rx := float64(ellipse.Dx()) / 2
	ry := float64(ellipse.Dy()) / 2

	transparent := color.NRGBA{}
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			if insideEllipse(float64(x)+0.5, float64(y)+0.5, cx, cy, rx, ry) {
				continue
			}
			img.SetNRGBA(x, y, transparent)
		}
	}
}

// insideEllipse 点是否落在椭圆内(半轴 rx/ry,圆心 cx/cy)
func insideEllipse(px, py, cx, cy, rx, ry float64) bool {
	dx := (px - cx) / rx
	dy := (py - cy) / ry
	return dx*dx+dy*dy <= 1
}

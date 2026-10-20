package pdfrenderer

// 形状能力口绘制面(ADR 0026):椭圆绘制原语。矢量承接 = fpdf 原生 Ellipse,
// 填充("F")/描边("D")/填充加描边("FD")三风格一体;描边带宽为驱动原生的
// 缘上居中带 [r-w/2, r+w/2]——与位图后端的外缘内缩带 [r-w, r] 的环位分歧由
// 采样验收取公共带吸收(PHP GD/Imagick 双驱同款纪律,spec 未钉对齐方式)

import (
	"github.com/HankChenCH/go-canvas/layer"
)

// DrawEllipse implements renderer.ShapeCapableBackend(椭圆绘制原语,ADR 0026):
// 在 (x, y) 起的 width×height 外接矩形内画内切椭圆,fillColor 填充 + stroke 圆周
// 描边一体承接(圆形档的背景填充与描边环同原语两调,双非空即 "FD" 一发)。
// width/height ≤ 0 防护不画;stroke.Width ≤ 0 不描(wire 解码可注入,与
// borderBands 同纪律)。pxToPt 口径:外接矩形 → 圆心/半轴、线宽换算后统一
// ×0.75,fpdf Ellipse 入参恒用户空间(内部自换算,与 ClipRect/Transform 家族
// 同一双空间纪律)。颜色先解析后挂载——错误路径不残留任何文档状态。
// 已知限制:fpdf 的 ExtGState alpha 对单次 "FD" 的填充与描边一体生效,两色
// alpha 不同时后挂载者同时作用于两者(当前分派层两调各只带一色,不触发)
func (r *Renderer) DrawEllipse(x, y, width, height int, fillColor *string, stroke *layer.BorderSide) error {
	doc, err := r.docf()
	if err != nil {
		return err
	}
	if width <= 0 || height <= 0 {
		return nil
	}

	// 颜色先全量解析(错误在改任何文档状态前返回),再按风格挂载
	var fill, draw *rgba
	if fillColor != nil && *fillColor != "" {
		c, err := parseColor(*fillColor)
		if err != nil {
			return err
		}
		fill = &c
	}
	if stroke != nil && stroke.Width > 0 {
		c, err := parseColor(stroke.Color)
		if err != nil {
			return err
		}
		draw = &c
	}

	style := ""
	if fill != nil {
		doc.SetAlpha(float64(fill.A)/255, "Normal")
		doc.SetFillColor(int(fill.R), int(fill.G), int(fill.B))
		style = "F"
	}
	if draw != nil {
		doc.SetAlpha(float64(draw.A)/255, "Normal")
		doc.SetDrawColor(int(draw.R), int(draw.G), int(draw.B))
		doc.SetLineWidth(float64(stroke.Width) * pxToPt)
		style += "D"
	}
	if style == "" {
		return nil
	}

	cx := (float64(x) + float64(width)/2) * pxToPt
	cy := (float64(y) + float64(height)/2) * pxToPt
	doc.Ellipse(cx, cy, float64(width)/2*pxToPt, float64(height)/2*pxToPt, 0, style)
	doc.SetAlpha(1, "Normal")
	return doc.Error()
}

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
// 同一双空间纪律)。半透明色经 ExtGState 挂 alpha(paintRect 同款),画完恢复
// 不透明——后续元素不吃残留
func (r *Renderer) DrawEllipse(x, y, width, height int, fillColor *string, stroke *layer.BorderSide) error {
	doc, err := r.docf()
	if err != nil {
		return err
	}
	if width <= 0 || height <= 0 {
		return nil
	}

	style := ""
	if fillColor != nil && *fillColor != "" {
		c, err := parseColor(*fillColor)
		if err != nil {
			return err
		}
		doc.SetAlpha(float64(c.A)/255, "Normal")
		doc.SetFillColor(int(c.R), int(c.G), int(c.B))
		style = "F"
	}
	if stroke != nil && stroke.Width > 0 {
		c, err := parseColor(stroke.Color)
		if err != nil {
			return err
		}
		doc.SetAlpha(float64(c.A)/255, "Normal")
		doc.SetDrawColor(int(c.R), int(c.G), int(c.B))
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

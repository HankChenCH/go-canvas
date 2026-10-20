package pdfrenderer

// 结构与矩形原语测试:pdfinfo 页数/页尺寸、背景色中心点、四边边框色带内侧
// 采样(避开抗锯齿边缘)。经标准管线驱动(模板 → 后端),验收即打开可见。

import (
	"image"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/resolver"
)

func TestRenderStructureSinglePagePxToPt(t *testing.T) {
	// 空画布:pdfinfo 页数 1、页尺寸 = px × 0.75 pt(200×100 px → 150×75 pt)
	c := canvas.New(200, 100)
	pdf := renderPdfCanvas(t, nil, c)
	assertPDFStructure(t, pdf, 1, 200, 100)
}

func TestDrawRectBackgroundCenterSample(t *testing.T) {
	// 背景填充:实心区中心点精确取样;透明底画布上其余区域保持无物
	l := layer.NewImageLayer(layer.WithSize(20, 10), layer.WithBackground("#ff0000"))
	pdf := renderPdfLayer(t, l)
	img := rasterize(t, pdf, 20, 10)
	assertPixel(t, img, 10, 5, red)
}

func TestDrawRectBorderBandsInnerSample(t *testing.T) {
	// 四边异色边框:每边色带**内侧**采样(线宽 4,取第 2 行/列,避开矢量
	// 抗锯齿边缘);边框序 top/bottom/left/right 后画覆盖先画由异色互证
	l := layer.NewImageLayer(layer.WithSize(30, 20), layer.WithBackground("#ffffff"),
		layer.WithBorderTop(4, "#0000ff"),
		layer.WithBorderBottom(4, "#0000ff"),
		layer.WithBorderLeft(4, "#ff0000"),
		layer.WithBorderRight(4, "#ff0000"))

	pdf := renderPdfLayer(t, l)
	img := rasterize(t, pdf, 30, 20)
	// 白底中心
	assertPixel(t, img, 15, 10, white)
	// top 色带内侧(y=1)与 bottom 色带内侧(y=18):蓝
	assertPixelNear(t, img, 15, 1, blue, 8)
	assertPixelNear(t, img, 15, 18, blue, 8)
	// left 色带内侧(x=1,行中段避开角部覆盖序)与 right 色带内侧(x=28):红
	assertPixelNear(t, img, 1, 10, red, 8)
	assertPixelNear(t, img, 28, 10, red, 8)
}

func TestDrawRectNilAndEmptyBackgroundSkipped(t *testing.T) {
	// nil 背景(不传 WithBackground)跳过填充:渲染成功、结构合法
	l := layer.NewImageLayer(layer.WithSize(8, 8))
	pdf := renderPdfLayer(t, l)
	assertPDFStructure(t, pdf, 1, 8, 8)
}

func TestCanvasBackgroundPaintedFirst(t *testing.T) {
	// 画布背景铺装(ADR 0022):先色后图层——色块被后绘图层盒覆盖
	redBg := "#ff0000"
	c := canvas.New(20, 20, layer.NewImageLayer(
		layer.WithSize(10, 10), layer.WithPosition(5, 5), layer.WithBackground("#ffffff")))
	c.SetBackground(canvas.NewCanvasBackground(&redBg, nil))
	pdf := renderPdfCanvas(t, resolver.New(resolver.WithCacheRoot(t.TempDir())), c)
	img := rasterize(t, pdf, 20, 20)
	assertPixel(t, img, 2, 2, red)
	assertPixel(t, img, 10, 10, white)
}

func TestBeginInvalidSizeFails(t *testing.T) {
	// 尺寸防线:非正尺寸报错,渲染面不建立
	r := New()
	if err := r.Begin(0, 10); err == nil {
		t.Error("Begin(0, 10) 应报错")
	}
	if err := r.Begin(10, -1); err == nil {
		t.Error("Begin(10, -1) 应报错")
	}
}

func TestDrawPrimitiveBeforeBeginFails(t *testing.T) {
	// 用法错误防线:未 Begin 即绘制原语报错(模板保证先 Begin,防御直调)
	r := New()
	if err := r.DrawRect(0, 0, 1, 1, nil, layer.Border{}); err == nil {
		t.Error("未 Begin 调 DrawRect 应报错")
	}
	if err := r.DrawImage("/dev/null", image.Rectangle{}, image.Rect(0, 0, 1, 1)); err == nil {
		t.Error("未 Begin 调 DrawImage 应报错")
	}
	if err := r.DrawText("x", 0, 0, "", 12, "#000", "left", "top", 0); err == nil {
		t.Error("未 Begin 调 DrawText 应报错")
	}
	if product := r.End(); product != nil {
		t.Errorf("未 Begin 的 End 产物 = %v, want nil", product)
	}
}

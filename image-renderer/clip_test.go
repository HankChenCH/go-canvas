package imagerenderer

// 裁剪形状圆形档像素采样(ADR 0026,平移 php-canvas-image-renderer
// ImageClipRenderingTest,assertPixel 无黄金文件):圆形档 = 完整圆形框——
// 背景色填进内切圆、内容按 fit 绘出后遮罩进圆、边框沿圆周描成环;
// 四角透明露下层、padding 区留白、非方盒恒正圆留透明边;
// circle × fit 四档正交组合;未知 clip 值降级矩形。
// 描边带采样点取缘内缩留过渡带余量(Go 内描带 [r-w, r] 与 PHP 居中带
// [r-w/2, r+w/2] 的公共区),经公开渲染管线(模板 → 位图后端)驱动

import (
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
)

// bandPng 上下两色带 PNG 落盘(遮罩采样源图,对齐 PHP bandPng)
func bandPng(t *testing.T, width, height int, top, bottom color.NRGBA) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	half := height / 2
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if y < half {
				img.SetNRGBA(x, y, top)
			} else {
				img.SetNRGBA(x, y, bottom)
			}
		}
	}
	return writePngFixture(t, img)
}

func TestClipCircleBackgroundFillsInscribedCircleCornersRevealLowerLayer(t *testing.T) {
	// 背景绿填进内切圆;四角在圆外透明露出垫底红层(整层渲染面裁进圆)
	lower := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#f00"), layer.WithPriority(10))
	circle := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#0f0"), layer.WithPriority(1))
	circle.SetClip(layer.ClipCircle)

	product, err := newRenderer(t).Render(context.Background(), canvas.New(100, 100, lower, circle))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := productAsImage(t, product)

	assertPixel(t, img, 2, 2, red)
	assertPixel(t, img, 97, 2, red)
	assertPixel(t, img, 2, 97, red)
	assertPixel(t, img, 97, 97, red)
	assertPixel(t, img, 50, 50, green)
	// 圆内四向中点(r=50,采样点距缘 ≥14px 留过渡带余量)
	assertPixel(t, img, 15, 50, green)
	assertPixel(t, img, 85, 50, green)
	assertPixel(t, img, 50, 15, green)
	assertPixel(t, img, 50, 85, green)
}

func TestNonSquareBoxKeepsPerfectCircleWithTransparentFlanks(t *testing.T) {
	// 非方盒 100×50:内切圆 center (50,25) r=25,左右留透明边、不变形为椭圆
	lower := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithBackground("#f00"), layer.WithPriority(10))
	circle := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithBackground("#ff0"), layer.WithPriority(1))
	circle.SetClip(layer.ClipCircle)

	product, err := newRenderer(t).Render(context.Background(), canvas.New(100, 50, lower, circle))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := productAsImage(t, product)

	yellow := color.NRGBA{R: 0xFF, G: 0xFF, B: 0x00, A: 0xFF}
	assertPixel(t, img, 50, 25, yellow)
	assertPixel(t, img, 32, 25, yellow)
	assertPixel(t, img, 50, 10, yellow)
	assertPixel(t, img, 10, 25, red)
	assertPixel(t, img, 90, 25, red)
	assertPixel(t, img, 2, 2, red)
}

func TestClipCircleImageMaskedIntoCircleCoverFit(t *testing.T) {
	// cover(缺省):方图进方盒整图绘出后遮罩进圆——圆内露图、圆外露下层
	src := bandPng(t, 100, 100, red, blue)
	lower := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#0f0"), layer.WithPriority(10))
	circle := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage(src), layer.WithPriority(1))
	circle.SetClip(layer.ClipCircle)

	product, err := newRenderer(t).Render(context.Background(), canvas.New(100, 100, lower, circle))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := productAsImage(t, product)

	assertPixel(t, img, 50, 25, red)
	assertPixel(t, img, 50, 75, blue)
	assertPixel(t, img, 2, 2, green)
	assertPixel(t, img, 97, 97, green)
}

func TestClipCircleBorderStrokesRingAlongCircumference(t *testing.T) {
	// 环带描边:padding 12 → 内切圆 side 76 center (50,50) r=38,border 宽 8 蓝——
	// Go 内描带 [30,38](与 DrawRect 盒内嵌边带同门),采样半径 ≈36(r - w/4)取
	// GD 内描带与 Imagick 居中带公共区跨驱可平移;环内露背景白、环外与四角露下层红
	// (padding 即环外留白)
	src := solidPng(t, 40, 40, color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xFF})
	lower := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#f00"), layer.WithPriority(10))
	circle := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#ffffff"),
		layer.WithImage(src), layer.WithPadding(12), layer.WithBorder(8, "#0000ff"), layer.WithPriority(1))
	circle.SetClip(layer.ClipCircle)

	product, err := newRenderer(t).Render(context.Background(), canvas.New(100, 100, lower, circle))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := productAsImage(t, product)

	assertPixel(t, img, 14, 50, blue)
	assertPixel(t, img, 86, 50, blue)
	assertPixel(t, img, 50, 14, blue)
	assertPixel(t, img, 50, 86, blue)
	// 环内被图覆盖:圆心 = 源图色(先背景填充后内容绘出,图遮住同圆的背景填充)
	assertPixel(t, img, 50, 50, color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xFF})
	assertPixel(t, img, 2, 2, red)
	assertPixel(t, img, 50, 5, red)
}

func TestClipCircleWithFillFitStretchThenMask(t *testing.T) {
	// fill:40×40 红图非等比拉伸进内容盒 100×50 后遮罩进内切圆 (25,0)-(75,50)
	src := solidPng(t, 40, 40, red)
	lower := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithBackground("#0f0"), layer.WithPriority(10))
	circle := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithImage(src), layer.WithPriority(1))
	circle.SetFit(layer.FitFill)
	circle.SetClip(layer.ClipCircle)

	product, err := newRenderer(t).Render(context.Background(), canvas.New(100, 50, lower, circle))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := productAsImage(t, product)

	assertPixel(t, img, 50, 25, red)
	assertPixel(t, img, 35, 25, red)
	assertPixel(t, img, 10, 25, green)
}

func TestClipCircleWithContainFitGapInsideCircleStaysTransparent(t *testing.T) {
	// contain:40×10 上下两色图进 100×100 盒收缩 100×25(y=37 起,半带界 y≈50);
	// 圆内但收缩矩形外的空隙透明露下层——先绘后裁不改变 contain 几何
	src := bandPng(t, 40, 10, red, blue)
	lower := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#0f0"), layer.WithPriority(10))
	circle := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage(src), layer.WithPriority(1))
	circle.SetFit(layer.FitContain)
	circle.SetClip(layer.ClipCircle)

	product, err := newRenderer(t).Render(context.Background(), canvas.New(100, 100, lower, circle))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := productAsImage(t, product)

	assertPixel(t, img, 30, 40, red)
	assertPixel(t, img, 30, 55, blue)
	// 圆内(dist 40 < 50)但 y<37 收缩矩形外:透明露下层
	assertPixel(t, img, 50, 10, green)
	// 圆外:透明露下层
	assertPixel(t, img, 2, 2, green)
}

func TestClipCircleWithTileFitBlocksOutsideCircleMaskedAway(t *testing.T) {
	// tile:40×40 单元 center 锚定 (30,30),可见块铺满 [30,100)²——
	// 圆内块区露图、圆内锚点未覆盖区(x<30)与圆外透明露下层
	src := solidPng(t, 40, 40, red)
	lower := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#0f0"), layer.WithPriority(10))
	circle := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage(src), layer.WithPriority(1))
	circle.SetFit(layer.FitTile)
	circle.SetClip(layer.ClipCircle)

	product, err := newRenderer(t).Render(context.Background(), canvas.New(100, 100, lower, circle))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := productAsImage(t, product)

	assertPixel(t, img, 35, 35, red)
	assertPixel(t, img, 50, 50, red)
	assertPixel(t, img, 15, 50, green)
	assertPixel(t, img, 5, 5, green)
}

func TestUnknownClipValueRendersRectangleWithoutMask(t *testing.T) {
	// 未知 clip 值渲染面按矩形降级(wire 透传保存):整盒铺图无遮罩、四角是图
	src := bandPng(t, 100, 100, red, blue)
	graph := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage(src)).Graph()
	star := "star"
	graph.Spec.Clip = &star
	circle := layer.ImageFromGraph(graph)

	img := renderLayerProduct(t, circle)

	assertPixel(t, img, 2, 2, red)
	assertPixel(t, img, 97, 97, blue)
	assertPixel(t, img, 50, 25, red)
}

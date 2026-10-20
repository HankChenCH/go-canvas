package pdfrenderer

// 裁剪形状圆形档 poppler 点位采样(ADR 0026,与位图腿 image-renderer/clip_test.go
// 同一矩阵):掩码 blit 走 ClipEllipse 矢量裁剪路径(直嵌零重编码与放置矩阵全复用),
// 椭圆经 fpdf 原生 Ellipse 绘制。位图/矢量等价域 = 单页放置面(ADR 0024 延伸至
// 椭圆面,采样差异不在契约内);圆缘采样点自缘内缩 ≥3px 留矢量抗锯齿过渡带余量。
// 描边带采样取居中带与内缩带公共区:fpdf 描边为缘上居中带 [r-w/2, r+w/2](驱动
// 原生行为,与位图后端内缩带 [r-w, r] 的分歧由采样半径 r-w/4 吸收,PHP 双驱同款
// 纪律),经公开渲染管线(模板 → PDF 后端)驱动

import (
	"image"
	"image/color"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
)

// bandPng 上下两色带 PNG 落盘(遮罩采样源图,与位图腿同名夹具同式)
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
	// 背景绿填进内切圆;四角在圆外露出垫底红层(整层渲染面裁进圆)
	lower := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#f00"), layer.WithPriority(10))
	circle := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#0f0"), layer.WithPriority(1))
	circle.SetClip(layer.ClipCircle)

	img := rasterize(t, renderPdfCanvas(t, nil, canvas.New(100, 100, lower, circle)), 100, 100)

	assertPixel(t, img, 2, 2, red)
	assertPixel(t, img, 97, 2, red)
	assertPixel(t, img, 2, 97, red)
	assertPixel(t, img, 97, 97, red)
	assertPixel(t, img, 50, 50, green)
	// 圆内四向中点(r=50,采样点距缘 ≥15px 留矢量过渡带余量)
	assertPixel(t, img, 15, 50, green)
	assertPixel(t, img, 85, 50, green)
	assertPixel(t, img, 50, 15, green)
	assertPixel(t, img, 50, 85, green)
}

func TestNonSquareBoxKeepsPerfectCircleWithTransparentFlanks(t *testing.T) {
	// 非方盒 100×50:内切圆 center (50,25) r=25,左右露出下层、不变形为椭圆
	lower := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithBackground("#f00"), layer.WithPriority(10))
	circle := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithBackground("#ff0"), layer.WithPriority(1))
	circle.SetClip(layer.ClipCircle)

	img := rasterize(t, renderPdfCanvas(t, nil, canvas.New(100, 50, lower, circle)), 100, 50)

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

	img := rasterize(t, renderPdfCanvas(t, nil, canvas.New(100, 100, lower, circle)), 100, 100)

	assertPixel(t, img, 50, 25, red)
	assertPixel(t, img, 50, 75, blue)
	assertPixel(t, img, 2, 2, green)
	assertPixel(t, img, 97, 97, green)
}

func TestClipCircleBorderStrokesRingAlongCircumference(t *testing.T) {
	// 环带描边:padding 12 → 内切圆 side 76 center (50,50) r=38,border 宽 8 蓝——
	// fpdf 居中带 [34,42],采样半径 ≈36(r - w/4)与位图内缩带 [30,38] 取公共区
	// 跨后端同矩阵;环内露图、环外与四角露下层红(padding 即环外留白)
	src := solidPng(t, 40, 40, color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xFF})
	lower := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#f00"), layer.WithPriority(10))
	circle := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#ffffff"),
		layer.WithImage(src), layer.WithPadding(12), layer.WithBorder(8, "#0000ff"), layer.WithPriority(1))
	circle.SetClip(layer.ClipCircle)

	img := rasterize(t, renderPdfCanvas(t, nil, canvas.New(100, 100, lower, circle)), 100, 100)

	// 采样点半径 36 距居中带内缘 2px,矢量抗锯齿下用容差取样
	assertPixelNear(t, img, 14, 50, blue, 8)
	assertPixelNear(t, img, 86, 50, blue, 8)
	assertPixelNear(t, img, 50, 14, blue, 8)
	assertPixelNear(t, img, 50, 86, blue, 8)
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

	img := rasterize(t, renderPdfCanvas(t, nil, canvas.New(100, 50, lower, circle)), 100, 50)

	assertPixel(t, img, 50, 25, red)
	assertPixel(t, img, 35, 25, red)
	assertPixel(t, img, 10, 25, green)
}

func TestClipCircleWithContainFitGapInsideCircleStaysTransparent(t *testing.T) {
	// contain:40×10 上下两色图进 100×100 盒收缩 100×25(y=37 起,半带界 y≈50);
	// 圆内但收缩矩形外的空隙露下层——先绘后裁不改变 contain 几何
	src := bandPng(t, 40, 10, red, blue)
	lower := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#0f0"), layer.WithPriority(10))
	circle := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage(src), layer.WithPriority(1))
	circle.SetFit(layer.FitContain)
	circle.SetClip(layer.ClipCircle)

	img := rasterize(t, renderPdfCanvas(t, nil, canvas.New(100, 100, lower, circle)), 100, 100)

	assertPixel(t, img, 30, 40, red)
	assertPixel(t, img, 30, 55, blue)
	// 圆内(dist 40 < 50)但 y<37 收缩矩形外:露下层
	assertPixel(t, img, 50, 10, green)
	// 圆外:露下层
	assertPixel(t, img, 2, 2, green)
}

func TestClipCircleWithTileFitBlocksOutsideCircleMaskedAway(t *testing.T) {
	// tile:40×40 单元 center 锚定 (30,30),可见块铺满 [30,100)²——
	// 圆内块区露图、圆内锚点未覆盖区(x<30)与圆外露下层
	src := solidPng(t, 40, 40, red)
	lower := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#0f0"), layer.WithPriority(10))
	circle := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage(src), layer.WithPriority(1))
	circle.SetFit(layer.FitTile)
	circle.SetClip(layer.ClipCircle)

	img := rasterize(t, renderPdfCanvas(t, nil, canvas.New(100, 100, lower, circle)), 100, 100)

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

	img := rasterize(t, renderPdfLayer(t, circle), 100, 100)

	assertPixel(t, img, 2, 2, red)
	assertPixel(t, img, 97, 97, blue)
	assertPixel(t, img, 50, 25, red)
}

func TestDrawEllipseFillStrokeAndCombinedStyles(t *testing.T) {
	// 原语级三风格:fill 单独("F")/ stroke 单独("D")/ 填充加描边一发("FD");
	// 白底画布直驱后端,描边为缘上居中带。防护腿:非正尺寸与 Width≤0 静默不画
	t.Run("fill", func(t *testing.T) {
		r := New()
		if err := r.Begin(40, 40); err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if err := r.DrawRect(0, 0, 40, 40, ptr("#ffffff"), layer.Border{}); err != nil {
			t.Fatalf("DrawRect: %v", err)
		}
		if err := r.DrawEllipse(10, 10, 20, 20, ptr("#ff0000"), nil); err != nil {
			t.Fatalf("DrawEllipse: %v", err)
		}
		img := rasterize(t, endPdfBytes(t, r), 40, 40)
		assertPixel(t, img, 20, 20, red)
		// 外接矩形角落椭圆外(距缘 ≥2px 留矢量过渡带余量):白底不被波及
		assertPixel(t, img, 11, 11, white)
	})

	t.Run("stroke", func(t *testing.T) {
		r := New()
		if err := r.Begin(40, 40); err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if err := r.DrawRect(0, 0, 40, 40, ptr("#ffffff"), layer.Border{}); err != nil {
			t.Fatalf("DrawRect: %v", err)
		}
		if err := r.DrawEllipse(10, 10, 20, 20, nil,
			&layer.BorderSide{Width: 4, Color: "#0000ff"}); err != nil {
			t.Fatalf("DrawEllipse: %v", err)
		}
		img := rasterize(t, endPdfBytes(t, r), 40, 40)
		// 居中带 [8,12],采样半径 10 距带缘 2px 容差取样
		assertPixelNear(t, img, 20, 10, blue, 8)
		assertPixelNear(t, img, 20, 30, blue, 8)
		assertPixelNear(t, img, 10, 20, blue, 8)
		assertPixelNear(t, img, 30, 20, blue, 8)
		// 环内未被填充波及:白
		assertPixel(t, img, 20, 20, white)
	})

	t.Run("fill and stroke", func(t *testing.T) {
		r := New()
		if err := r.Begin(40, 40); err != nil {
			t.Fatalf("Begin: %v", err)
		}
		if err := r.DrawRect(0, 0, 40, 40, ptr("#ffffff"), layer.Border{}); err != nil {
			t.Fatalf("DrawRect: %v", err)
		}
		if err := r.DrawEllipse(10, 10, 20, 20, ptr("#ff0000"),
			&layer.BorderSide{Width: 4, Color: "#0000ff"}); err != nil {
			t.Fatalf("DrawEllipse: %v", err)
		}
		img := rasterize(t, endPdfBytes(t, r), 40, 40)
		assertPixel(t, img, 20, 20, red)
		assertPixelNear(t, img, 20, 10, blue, 8)
	})

	t.Run("guards", func(t *testing.T) {
		r := New()
		if err := r.Begin(10, 10); err != nil {
			t.Fatalf("Begin: %v", err)
		}
		// 非正外接矩形与 Width≤0 描边:静默不画,渲染成功
		if err := r.DrawEllipse(0, 0, 0, 10, ptr("#ff0000"), nil); err != nil {
			t.Errorf("非正尺寸应静默不画: %v", err)
		}
		if err := r.DrawEllipse(0, 0, 10, -1, nil,
			&layer.BorderSide{Width: 2, Color: "#0000ff"}); err != nil {
			t.Errorf("非正尺寸应静默不画: %v", err)
		}
		if err := r.DrawEllipse(0, 0, 10, 10, nil,
			&layer.BorderSide{Width: 0, Color: "#0000ff"}); err != nil {
			t.Errorf("Width≤0 描边应静默不描: %v", err)
		}
		endPdfBytes(t, r)
	})
}

func TestDrawImageMaskedInscribedEllipseClip(t *testing.T) {
	// 原语级掩码 = 椭圆内切于给定矩形(非方 maskRect 即椭圆,分派层传正方形才圆):
	// 40×30 掩码 center (20,15) rx=20 ry=15——中心与轴上近缘点露图,角落掩码外不画
	r := New()
	if err := r.Begin(40, 30); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	src := solidPng(t, 40, 30, red)
	if err := r.DrawImageMasked(src, image.Rectangle{}, image.Rect(0, 0, 40, 30), image.Rect(0, 0, 40, 30)); err != nil {
		t.Fatalf("DrawImageMasked: %v", err)
	}
	img := rasterize(t, endPdfBytes(t, r), 40, 30)

	assertPixel(t, img, 20, 15, red)
	assertPixel(t, img, 20, 3, red)
	assertPixel(t, img, 4, 15, red)
	// 掩码外不画:页底白
	assertPixel(t, img, 2, 2, white)
	assertPixel(t, img, 37, 27, white)
}

func TestDrawImageMaskedEmptyRectGuards(t *testing.T) {
	// dstRect/maskRect 空矩形防护不画(原语级,位图后端同款):渲染成功
	r := New()
	if err := r.Begin(10, 10); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	src := solidPng(t, 4, 4, red)
	if err := r.DrawImageMasked(src, image.Rectangle{}, image.Rectangle{}, image.Rect(0, 0, 5, 5)); err != nil {
		t.Errorf("空 dstRect 应静默不画: %v", err)
	}
	if err := r.DrawImageMasked(src, image.Rectangle{}, image.Rect(0, 0, 5, 5), image.Rectangle{}); err != nil {
		t.Errorf("空 maskRect 应静默不画: %v", err)
	}
	endPdfBytes(t, r)
}

// endPdfBytes 终结会话并断言产物为非空 PDF 字节(直驱后端的原语级测试收尾)
func endPdfBytes(t *testing.T, r *Renderer) []byte {
	t.Helper()
	product := r.End()
	pdf, ok := product.([]byte)
	if !ok || len(pdf) == 0 {
		t.Fatalf("产物 = %T, want 非空 []byte", product)
	}
	return pdf
}

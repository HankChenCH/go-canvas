package pdfrenderer

// 二维码物化与默认接线测试:QRMaterializer 固定选项(对齐 endroid)、QR 图层
// 管线渲染角点暗像素、NewRenderer 默认物化器接线(ADR-0002 缝)

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/resolver"
)

func TestQRMaterializerGeneratesPNG(t *testing.T) {
	// 物化产物 = 可解码 PNG(显式 PNG 编码器,库默认 JPEG);纯黑前景纯白背景
	pngBytes, err := QRMaterializer{}.Materialize(t.Context(), "https://github.com/HankChenCH/go-canvas", 90)
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("产物不是合法 PNG: %v", err)
	}
	if img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		t.Fatalf("PNG 尺寸异常: %v", img.Bounds())
	}
}

func TestQrCodeLayerRendersDarkFinderCorners(t *testing.T) {
	// QR 图层管线渲染:三个 finder pattern 角区含暗像素簇、整体暗像素占比
	// 落 QR 典型区间(码图内切于内容盒,ADR 0015)。块量化缩放(yeqown 出图
	// 尺寸向下量化后 cover 拉伸)使单点采样对齐敏感,故用角区计数 + 占比区间
	c := canvas.New(60, 60, layer.NewQrCodeLayer(
		layer.WithSize(60, 60), layer.WithQrText("go-canvas"),
		layer.WithPosition(0, 0),
	))
	img := rasterize(t, renderPdfCanvas(t, nil, c), 60, 60)

	darkIn := func(box image.Rectangle) int {
		n := 0
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				if c := samplePixel(img, x, y); c.R < 100 && c.G < 100 && c.B < 100 {
					n++
				}
			}
		}
		return n
	}

	// finder 角区(12×12):至少 1/3 暗像素(finder 外环 + 中心块的主体)
	for _, box := range []image.Rectangle{
		image.Rect(0, 0, 12, 12), image.Rect(48, 0, 60, 12), image.Rect(0, 48, 12, 60),
	} {
		if n := darkIn(box); n < 48 {
			t.Errorf("QR finder 角区 %v 暗像素 = %d, want >= 48", box, n)
		}
	}

	// 整体占比:QR 码典型 ~50%,容 [15%, 70%](纠错级别与内容相关)
	total := darkIn(image.Rect(0, 0, 60, 60))
	if total < 60*60*15/100 || total > 60*60*70/100 {
		t.Errorf("QR 暗像素占比 = %d/%d, 超出 QR 典型区间", total, 60*60)
	}
}

func TestNewRendererDefaultResolverWiring(t *testing.T) {
	// NewRenderer(nil) 接线默认物化器(QR 缝默认接线);New() 不带——
	// 核心 nil-resolver 组装路径经 DefaultResolver() 发现并优先采用
	if New().DefaultResolver() != nil {
		t.Error("New() 不应自带默认物化器")
	}
	r := NewRenderer(nil)
	if r.DefaultResolver() == nil {
		t.Error("NewRenderer(nil) 应接线默认物化器")
	}
	// 传入自建解析器按原样使用
	custom := resolver.New(resolver.WithCacheRoot(t.TempDir()))
	if NewRenderer(custom).DefaultResolver() != custom {
		t.Error("NewRenderer(custom) 应原样使用传入解析器")
	}
}

// TestDownloadFailureAbortsRenderWithoutProduct 见 cancel_test.go(下载失败
// 注入属失败/取消面,QR 图层管线同面消费)

package imagerenderer

// fit 像素点位断言(ADR 0023,平移 php-canvas-image-renderer ImageRendererTest
// fit 组用例):contain 等比收缩后空隙透明露下层与盒背景、收缩矩形按 align + padding
// 放置(非恒居中)、cover 缺省与未知 fit 值降级零回归、损坏图按 cover 走结局等价。
// 无黄金文件约定,点位取样即验收面;经公开渲染管线(模板 → 位图后端)驱动

import (
	"context"
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
)

// magenta 品红(下层透出用例的源图左半色)
var magenta = color.NRGBA{R: 0xFF, G: 0x00, B: 0xFF, A: 0xFF}

// fitViaWire 经 wire 往返给图片图层注入 spec.fit(模拟第三方手写 graph:显式 cover /
// 未知枚举值的透传形态,不经 SetFit 归一门)
func fitViaWire(t *testing.T, l *layer.ImageLayer, fit string) *layer.ImageLayer {
	t.Helper()
	raw, err := json.Marshal(l.Graph())
	if err != nil {
		t.Fatalf("序列化 graph: %v", err)
	}
	var node layer.Node
	if err := json.Unmarshal(raw, &node); err != nil {
		t.Fatalf("解码 graph: %v", err)
	}
	node.Spec.Fit = &fit
	return layer.ImageFromGraph(node)
}

func TestContainLeavesGapShowingLayerBoxBackground(t *testing.T) {
	// 40×10 双色图 contain 进 40×20 白底盒:收缩 40×10 垂直居中(y∈[5,15)),
	// 上下空隙露盒背景白;cover 降级时 (20,2) 是红(放大裁切铺满)
	wide := twoColorPng(t, 40, 10, red, blue, 20)

	l := layer.NewImageLayer(layer.WithSize(40, 20), layer.WithBackground("#ffffff"), layer.WithImage(wide))
	l.SetFit(layer.FitContain)
	img := renderLayerProduct(t, l)

	assertPixel(t, img, 10, 10, red)
	assertPixel(t, img, 30, 10, blue)
	assertPixel(t, img, 20, 2, white)
	assertPixel(t, img, 20, 18, white)
	// 收缩矩形边界精确贴 y∈[5,15):行 5/14 是图、行 4/15 是空隙
	assertPixel(t, img, 10, 5, red)
	assertPixel(t, img, 10, 14, red)
	assertPixel(t, img, 10, 4, white)
	assertPixel(t, img, 10, 15, white)
}

func TestContainGapTransparencyRevealsLowerLayer(t *testing.T) {
	// 空隙透明(非填充色):上下空隙透出垫底红层,收缩矩形 y∈[5,15) 内是源图(左品红右蓝)
	redLayer := layer.NewImageLayer(layer.WithSize(40, 20), layer.WithBackground("#f00"), layer.WithPriority(5))
	wide := twoColorPng(t, 40, 10, magenta, blue, 20)
	contain := layer.NewImageLayer(layer.WithSize(40, 20), layer.WithImage(wide), layer.WithPriority(1))
	contain.SetFit(layer.FitContain)

	product, err := newRenderer(t).Render(context.Background(), canvas.New(40, 20, redLayer, contain))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := productAsImage(t, product)

	assertPixel(t, img, 20, 2, red)
	assertPixel(t, img, 20, 18, red)
	assertPixel(t, img, 10, 10, magenta)
	assertPixel(t, img, 30, 10, blue)
	// 矩形边界:行 5/14 是图、行 4/15 是垫底红
	assertPixel(t, img, 10, 5, magenta)
	assertPixel(t, img, 10, 14, magenta)
	assertPixel(t, img, 10, 4, red)
	assertPixel(t, img, 10, 15, red)
}

func TestContainPlacesShrunkRectByAlignAndPadding(t *testing.T) {
	// contain 收缩矩形按 align + padding 放置(非居中组合):100×60 盒 padding
	// 上10/右30/下20/左20 + left/top,内容盒 50×30,40×10 源图收缩 50×12
	// (scale = min(50/40, 30/10) = 1.25)贴内容盒左上原点 (20,10)
	wide := twoColorPng(t, 40, 10, red, blue, 20)

	l := layer.NewImageLayer(
		layer.WithSize(100, 60),
		layer.WithBackground("#ffffff"),
		layer.WithImage(wide),
		layer.WithPaddingTRBL(10, 30, 20, 20),
		layer.WithHorizontalAlign(layer.AlignLeft),
		layer.WithVerticalAlign(layer.AlignTop),
	)
	l.SetFit(layer.FitContain)
	img := renderLayerProduct(t, l)

	// 收缩矩形 [20,70)×[10,22):贴 padding 原点,分界线 x=45(本地 25 = 源 20)
	assertPixel(t, img, 20, 15, red)
	assertPixel(t, img, 40, 15, red)
	assertPixel(t, img, 60, 15, blue)
	assertPixel(t, img, 69, 15, blue)
	// 放置精度:首行贴 y=10(padding top),上方露盒背景
	assertPixel(t, img, 40, 10, red)
	assertPixel(t, img, 40, 9, white)
	// 左侧 padding 空隙露盒背景
	assertPixel(t, img, 19, 15, white)
	// 矩形下方至内容盒底(y<40)的空隙露盒背景;cover 会铺满内容盒此处是蓝
	assertPixel(t, img, 40, 22, white)
	assertPixel(t, img, 40, 39, white)
}

func TestUnknownFitValueAndExplicitCoverRenderSamePixelsAsCover(t *testing.T) {
	// 渲染面降级零回归(点位与 TestImageIsCoverCroppedIntoContentBox 一致):
	// 未知 fit 值(白名单在渲染臂,wire 透传保存)与显式 cover(归一缺省态)
	// 都按 cover 原路径渲染
	wide := twoColorPng(t, 40, 10, red, blue, 20)

	// 未知值经 wire 透传(往返恒等),渲染按 cover 降级
	l := fitViaWire(t,
		layer.NewImageLayer(layer.WithSize(20, 20), layer.WithBackground("#ffffff"), layer.WithImage(wide)),
		"fill")
	if got := l.Fit(); got != "fill" {
		t.Fatalf("解码后 Fit() = %q, want fill(透传)", got)
	}
	img := renderLayerProduct(t, l)
	assertPixel(t, img, 5, 10, red)
	assertPixel(t, img, 15, 10, blue)

	// 显式 cover 归一为缺省态,走同一 cover 路径
	explicit := layer.NewImageLayer(layer.WithSize(20, 20), layer.WithBackground("#ffffff"), layer.WithImage(wide))
	explicit.SetFit(layer.FitCover)
	img = renderLayerProduct(t, explicit)
	assertPixel(t, img, 5, 10, red)
	assertPixel(t, img, 15, 10, blue)
}

func TestCorruptedImageWithContainFollowsCoverPathExactly(t *testing.T) {
	// 损坏图 + contain 降级走 cover 路径(ADR 0023):渲染结局与无 fit 的 cover
	// 完全一致——fit 臂 DecodeConfig 读取失败不新增异常面(两端不可解码文件的
	// 结局都收敛到 DrawImage 的解码失败,故断言 outcome 等价而非具体错误文案)
	src := filepath.Join(t.TempDir(), "broken.png")
	if err := os.WriteFile(src, []byte("definitely-not-an-image"), 0o644); err != nil {
		t.Fatalf("写损坏测试图: %v", err)
	}

	outcome := func(contain bool) string {
		l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithBackground("#ffffff"), layer.WithImage(src))
		if contain {
			l.SetFit(layer.FitContain)
		}
		product, err := newRenderer(t).RenderLayer(context.Background(), l)
		if err != nil {
			return "error: " + err.Error()
		}
		img := productAsImage(t, product)
		return "image: bounds " + img.Bounds().String()
	}

	if got, want := outcome(true), outcome(false); got != want {
		t.Errorf("损坏图 contain 渲染结局必须与 cover 完全一致:\n contain = %s\n cover   = %s", got, want)
	}
}

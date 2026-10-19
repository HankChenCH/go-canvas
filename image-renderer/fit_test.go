package imagerenderer

// fit 像素点位断言(ADR 0023/0024,平移 php-canvas-image-renderer ImageRendererTest
// fit 组用例与 spec §8 T2 点位清单):contain 等比收缩后空隙透明露下层与盒背景、
// 收缩矩形按 align + padding 放置(非恒居中)、cover/contain 回归、fill 拉伸形变、
// tile 网格锚定/截断/空隙、未知 fit 值降级与损坏图渲染结局等价。
// 无黄金文件约定,点位取样即验收面;经公开渲染管线(模板 → 位图后端)驱动

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
)

// magenta 品红(下层透出用例的源图左半色)
var magenta = color.NRGBA{R: 0xFF, G: 0x00, B: 0xFF, A: 0xFF}

// cyan 青(四象限标定图右下象限)
var cyan = color.NRGBA{R: 0x00, G: 0xFF, B: 0xFF, A: 0xFF}

// fitViaWire 经 wire 往返给图片图层注入 spec.fit(模拟第三方手写 graph:显式 cover /
// 白名单外枚举串的透传形态,不经 SetFit 归一门)
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

// quadPng 四象限标定图:TL/TR/BL/BR 各占 1/4(tile 网格点位用色块定位)
func quadPng(t *testing.T, width, height int, tl, tr, bl, br color.NRGBA) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			switch {
			case y < height/2 && x < width/2:
				img.SetNRGBA(x, y, tl)
			case y < height/2:
				img.SetNRGBA(x, y, tr)
			case x < width/2:
				img.SetNRGBA(x, y, bl)
			default:
				img.SetNRGBA(x, y, br)
			}
		}
	}
	return writePngFixture(t, img)
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

func TestFillStretchesIntoContentBoxWithoutCropOrGap(t *testing.T) {
	// fill 拉伸形变(ADR 0024):40×10 上下分带图(上红下蓝,行切分)非等比拉伸进
	// 20×20 白底盒——不裁切不留空隙,分界线压中(y=10);cover 会裁中部横带(全红)、
	// contain 会留空隙,均与此点位不同(取样点远离分界,避开插值过渡带)
	banded := image.NewNRGBA(image.Rect(0, 0, 40, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 40; x++ {
			if y < 5 {
				banded.SetNRGBA(x, y, red)
			} else {
				banded.SetNRGBA(x, y, blue)
			}
		}
	}

	l := layer.NewImageLayer(
		layer.WithSize(20, 20),
		layer.WithBackground("#ffffff"),
		layer.WithImage(writePngFixture(t, banded)),
	)
	l.SetFit(layer.FitFill)
	img := renderLayerProduct(t, l)

	assertPixel(t, img, 10, 3, red)
	assertPixel(t, img, 0, 7, red)
	assertPixel(t, img, 19, 0, red)
	assertPixel(t, img, 10, 13, blue)
	assertPixel(t, img, 0, 12, blue)
	assertPixel(t, img, 19, 19, blue)
}

func TestTileAnchorsGridAndTruncatesAtContentBox(t *testing.T) {
	// tile 网格(ADR 0024):40×40 四象限单元进 100×50 透明盒(center)→ 首块锚定
	// (30,5) 网格整体居中,右/下溢出半块截断(末行末列半块),锚点前空隙透明露下层
	unit := quadPng(t, 40, 40, red, green, blue, cyan)

	l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithImage(unit))
	l.SetFit(layer.FitTile)
	img := renderLayerProduct(t, l)

	// 首块 (30,5)-(70,45):四象限按 1:1 落位
	assertPixel(t, img, 31, 6, red)
	assertPixel(t, img, 69, 6, green)
	assertPixel(t, img, 31, 44, blue)
	assertPixel(t, img, 69, 44, cyan)
	// 末列半块 (70,5)-(100,45):源窗 x∈[0,30),图案重新从源左缘开始
	assertPixel(t, img, 70, 6, red)
	assertPixel(t, img, 99, 44, cyan)
	// 末行半块 (30,45)-(70,50):源窗 y∈[0,5)
	assertPixel(t, img, 31, 45, red)
	assertPixel(t, img, 69, 49, green)
	// 角块 (70,45)-(100,50):源窗 x∈[0,30) y∈[0,5)
	assertPixel(t, img, 70, 45, red)
	assertPixel(t, img, 99, 49, green)
	// 空隙透明露下层:锚点左侧、锚线上方、末行左段
	assertPixel(t, img, 29, 10, clear)
	assertPixel(t, img, 50, 4, clear)
	assertPixel(t, img, 10, 49, clear)
}

func TestTileRightAlignLeavesGapOnLeft(t *testing.T) {
	// align=right/top:首块贴右、锚点前空隙透明露下层,下方照常补行
	unit := quadPng(t, 40, 40, red, green, blue, cyan)

	l := layer.NewImageLayer(
		layer.WithSize(100, 50),
		layer.WithImage(unit),
		layer.WithHorizontalAlign(layer.AlignRight),
		layer.WithVerticalAlign(layer.AlignTop),
	)
	l.SetFit(layer.FitTile)
	img := renderLayerProduct(t, l)

	assertPixel(t, img, 60, 0, red)
	assertPixel(t, img, 99, 9, green)
	// 锚点前空隙(左侧整条)
	assertPixel(t, img, 59, 10, clear)
	assertPixel(t, img, 0, 0, clear)
	// 下方补行(末行半块,源窗 y∈[0,10))
	assertPixel(t, img, 60, 40, red)
	assertPixel(t, img, 99, 49, green)
}

func TestTileLargeImageNegativeAnchorRevealsMiddle(t *testing.T) {
	// 大图 center:锚点为负露出图片中部,可见块只有网格首块的中部子矩形,
	// 无空隙(网格 ∩ 内容盒 = 内容盒)
	wide := twoColorPng(t, 120, 80, red, green, 60)

	l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithImage(wide))
	l.SetFit(layer.FitTile)
	img := renderLayerProduct(t, l)

	// src 窗 (10,15)-(110,65):分界线 src x=60 → dst x=50,四角满铺
	assertPixel(t, img, 0, 0, red)
	assertPixel(t, img, 49, 25, red)
	assertPixel(t, img, 50, 25, green)
	assertPixel(t, img, 99, 49, green)
}

func TestCoverExtremeAspectRatioWindowDegradesToWholeImage(t *testing.T) {
	// 极端宽高比:20×100 竖图进 100×4 横带,cover 窗向零截断得零高(位移公式
	// 不钳制,对齐 PHP intval)——零矩形落整图哨兵,后端按解码实际尺寸整图拉伸
	// (旧 coverImage 的 ≥1 钳制语义不移植,ADR 0024 哨兵契约接管)
	tall := twoColorPng(t, 20, 100, red, blue, 10)

	l := layer.NewImageLayer(layer.WithSize(100, 4), layer.WithBackground("#ffffff"), layer.WithImage(tall))
	img := renderLayerProduct(t, l)

	// 整图拉伸:左右分界压中(x=50)、纵向满铺(取样点远离分界,避开插值过渡带)
	assertPixel(t, img, 25, 2, red)
	assertPixel(t, img, 75, 2, blue)
	assertPixel(t, img, 25, 0, red)
	assertPixel(t, img, 75, 3, blue)
}

func TestUnknownFitValueAndExplicitCoverRenderSamePixelsAsCover(t *testing.T) {
	// 渲染面降级零回归(点位与 TestImageIsCoverCroppedIntoContentBox 一致):
	// 未知 fit 值(白名单在渲染臂,wire 透传保存)与显式 cover(归一缺省态)
	// 都按 cover 原路径渲染。fill/tile 已转正(ADR 0024),白名单外示例改用
	// 从未入册的值
	wide := twoColorPng(t, 40, 10, red, blue, 20)

	// 未知值经 wire 透传(往返恒等),渲染按 cover 降级
	l := fitViaWire(t,
		layer.NewImageLayer(layer.WithSize(20, 20), layer.WithBackground("#ffffff"), layer.WithImage(wide)),
		"flip")
	if got := l.Fit(); got != "flip" {
		t.Fatalf("解码后 Fit() = %q, want flip(透传)", got)
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

func TestCorruptedImageRendersSameOutcomeAcrossAllFits(t *testing.T) {
	// 损坏图统一退化 fill 形态(ADR 0024):内在尺寸读取失败后白名单四档与未知值
	// 全部走同一零哨兵分派,渲染结局与无 fit 的 cover 完全一致——两端不可解码文件
	// 的结局都收敛到 DrawImage 的解码失败,故断言 outcome 等价而非具体错误文案
	src := filepath.Join(t.TempDir(), "broken.png")
	if err := os.WriteFile(src, []byte("definitely-not-an-image"), 0o644); err != nil {
		t.Fatalf("写损坏测试图: %v", err)
	}

	outcome := func(fit string) string {
		l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithBackground("#ffffff"), layer.WithImage(src))
		if fit != "" {
			l.SetFit(fit)
		}
		product, err := newRenderer(t).RenderLayer(context.Background(), l)
		if err != nil {
			return "error: " + err.Error()
		}
		img := productAsImage(t, product)
		return "image: bounds " + img.Bounds().String()
	}

	baseline := outcome("")
	for _, fit := range []string{layer.FitCover, layer.FitContain, layer.FitFill, layer.FitTile, "flip"} {
		if got := outcome(fit); got != baseline {
			t.Errorf("损坏图 fit=%s 渲染结局必须与 cover 完全一致:\n fit = %s\n cover = %s", fit, got, baseline)
		}
	}
}

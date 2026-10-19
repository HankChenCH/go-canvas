package renderer_test

// paintImage 全 fit 分派契约(ADR 0024,平移 php-canvas-next ImageFitPaintingTest):
// 绘制原语是双矩形 blit(srcRect → dstRect 拉伸搬运,stdlib image.Rectangle),
// 四档 fit 的几何全部在分派层算定——cover(缺省/未知值降级)等比居中裁窗、
// fill 整图(与 cover 同门 dstRect,唯一差别在 srcRect)、contain 收缩(ADR 0023
// 语义逐字节不动)、tile 可见块逐块 blit;内在尺寸读取为全 fit 前置(绘制分派点
// 一次),读取失败统一退化 fill 形态(srcRect 零哨兵 = 后端按解码实际尺寸取整图)。
// 核心包只断言分派几何(假后端,DrawImage 不真解码),像素点位在 image-renderer module

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/HankChenCH/go-canvas/layer"
)

// writeSolidPng 生成已知内在尺寸的纯色 PNG 落盘,返回路径(模拟物化后的本地图片;
// PHP ImageFitPaintingTest::writePng 同款场景,直接走 png.Encode 保证真实可解码)
func writeSolidPng(t *testing.T, width, height int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 0x7f, G: 0x80, B: 0x81, A: 0xff})
		}
	}
	path := filepath.Join(t.TempDir(), "fixture.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("建测试图文件: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("编码测试图: %v", err)
	}
	return path
}

// imageWithWireFit 经 wire 往返给图片图层注入 spec.fit(模拟第三方手写 graph:
// 显式 cover / 白名单外枚举串的表达形态,不经 SetFit 归一门)
func imageWithWireFit(t *testing.T, l *layer.ImageLayer, fit string) *layer.ImageLayer {
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

// assertImageCalls 断言 DrawImage 调用序列(src/dst 矩形逐对比较)
func assertImageCalls(t *testing.T, got []imageCall, want []imageCall) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("image 调用数 = %d, want %d:\n got  %v\n want %v", len(got), len(want), got, want)
	}
	for i, w := range want {
		if got[i].src != w.src || got[i].srcRect != w.srcRect || got[i].dstRect != w.dstRect {
			t.Errorf("image[%d] = {src %s, srcRect %s, dstRect %s}, want {src %s, srcRect %s, dstRect %s}",
				i, got[i].src, got[i].srcRect, got[i].dstRect, w.src, w.srcRect, w.dstRect)
		}
	}
}

// wholeImage 整图 srcRect(内在尺寸坐标系)
func wholeImage(w, h int) image.Rectangle { return image.Rect(0, 0, w, h) }

func TestCoverDispatchesProportionalCenterCropWindow(t *testing.T) {
	// 缺省 cover:分派点读内在尺寸算等比居中裁窗(40×40 方图进 100×50 横盒 → 中部横带),
	// dstRect = getImageOrigin + 内容宽高;图层盒背景 rect 照旧先画
	src := writeSolidPng(t, 40, 40)
	backend := &fakeBackend{}
	l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithImage(src))

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertOps(t, backend.ops, []string{"begin", "rect", "image", "end"})
	assertImageCalls(t, backend.images, []imageCall{
		{src, image.Rect(0, 10, 40, 30), image.Rect(0, 0, 100, 50)},
	})
}

func TestExplicitCoverWireAlsoCovers(t *testing.T) {
	// wire 显式 cover(第三方手写 graph):解码归一缺省,渲染走 cover 裁窗
	src := writeSolidPng(t, 40, 40)
	backend := &fakeBackend{}
	l := imageWithWireFit(t,
		layer.NewImageLayer(layer.WithSize(100, 50), layer.WithImage(src)),
		layer.FitCover)

	if got := l.Fit(); got != "" {
		t.Fatalf("解码后 Fit() = %q, want 空串(cover 归一)", got)
	}
	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertImageCalls(t, backend.images, []imageCall{
		{src, image.Rect(0, 10, 40, 30), image.Rect(0, 0, 100, 50)},
	})
}

func TestUnknownFitValueDegradesToCover(t *testing.T) {
	// 未知 fit 值渲染面按 cover 降级(白名单在渲染臂,不在解码门);
	// fill/tile 已转正(ADR 0024),白名单外示例改用从未入册的值
	src := writeSolidPng(t, 40, 40)
	backend := &fakeBackend{}
	l := imageWithWireFit(t,
		layer.NewImageLayer(layer.WithSize(100, 50), layer.WithImage(src)),
		"flip")

	if got := l.Fit(); got != "flip" {
		t.Fatalf("解码后 Fit() = %q, want flip(透传)", got)
	}
	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertImageCalls(t, backend.images, []imageCall{
		{src, image.Rect(0, 10, 40, 30), image.Rect(0, 0, 100, 50)},
	})
}

func TestContainBlitsWholeImageIntoShrunkRect(t *testing.T) {
	// contain(ADR 0023 语义不动):srcRect = 整图,dstRect = 收缩矩形(盒内居中)
	src := writeSolidPng(t, 40, 40)
	backend := &fakeBackend{}
	l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithImage(src))
	l.SetFit(layer.FitContain)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertImageCalls(t, backend.images, []imageCall{
		{src, wholeImage(40, 40), image.Rect(25, 0, 75, 50)},
	})
}

func TestContainPlacesByAlignAndPadding(t *testing.T) {
	// 左上对齐 + padding(VH 6/8):收缩矩形贴内容盒原点(同门 align + padding 语义)。
	// 内容盒 84×38 → 等比收缩 84×21(scale = min(84/200, 38/50) = 0.42)
	src := writeSolidPng(t, 200, 50)
	backend := &fakeBackend{}
	l := layer.NewImageLayer(
		layer.WithSize(100, 50),
		layer.WithImage(src),
		layer.WithPaddingVH(6, 8),
		layer.WithHorizontalAlign(layer.AlignLeft),
		layer.WithVerticalAlign(layer.AlignTop),
	)
	l.SetFit(layer.FitContain)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertImageCalls(t, backend.images, []imageCall{
		{src, wholeImage(200, 50), image.Rect(8, 6, 92, 27)},
	})
}

func TestFillBlitsWholeImageIntoSameDstAsCover(t *testing.T) {
	// fill:整图 srcRect + 与 cover 同门 dstRect(唯一差别在源矩形,不裁切不保比)
	src := writeSolidPng(t, 40, 40)
	backend := &fakeBackend{}
	l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithImage(src))
	l.SetFit(layer.FitFill)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertImageCalls(t, backend.images, []imageCall{
		{src, wholeImage(40, 40), image.Rect(0, 0, 100, 50)},
	})
}

func TestFillDstFollowsAlignAndPadding(t *testing.T) {
	// fill 的 dst 与 cover 同门:getImageOrigin + 内容宽高(padding 内缩、对齐定 origin)
	src := writeSolidPng(t, 40, 40)
	backend := &fakeBackend{}
	l := layer.NewImageLayer(
		layer.WithSize(100, 50),
		layer.WithImage(src),
		layer.WithPadding(10),
	)
	l.SetFit(layer.FitFill)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertImageCalls(t, backend.images, []imageCall{
		{src, wholeImage(40, 40), image.Rect(10, 10, 90, 40)},
	})
}

func TestTileDispatchesVisibleBlocksInRowMajorOrder(t *testing.T) {
	// tile:40×40 单元进 100×50 盒(center)→ 网格整体居中、右/下溢出半块截断,
	// 逐块以可见子矩形 blit(src/dst 同尺寸 1:1),裁切纯几何完成、原语零裁剪参数
	src := writeSolidPng(t, 40, 40)
	backend := &fakeBackend{}
	l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithImage(src))
	l.SetFit(layer.FitTile)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertImageCalls(t, backend.images, []imageCall{
		{src, image.Rect(0, 0, 40, 40), image.Rect(30, 5, 70, 45)},
		{src, image.Rect(0, 0, 30, 40), image.Rect(70, 5, 100, 45)},
		{src, image.Rect(0, 0, 40, 5), image.Rect(30, 45, 70, 50)},
		{src, image.Rect(0, 0, 30, 5), image.Rect(70, 45, 100, 50)},
	})
}

func TestTileDstFollowsLayerPosition(t *testing.T) {
	// dst 是绝对坐标(图层锚点偏移入账),src 不随位移;40×40 图铺满 40×40 盒恰一块
	src := writeSolidPng(t, 40, 40)
	backend := &fakeBackend{}
	l := layer.NewImageLayer(
		layer.WithSize(40, 40),
		layer.WithImage(src),
		layer.WithPosition(5, 7),
	)
	l.SetFit(layer.FitTile)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertImageCalls(t, backend.images, []imageCall{
		{src, wholeImage(40, 40), image.Rect(5, 7, 45, 47)},
	})
}

func TestIntrinsicSizeFailureDegradesAllFitsToFillForm(t *testing.T) {
	// 内在尺寸读取失败(损坏/非图文件):白名单四档统一退化整图拉伸(fill 形态),
	// srcRect 走零哨兵(后端按解码实际尺寸取整图,真损坏图在后端解码抛、结局 parity);
	// tile 也退化为单次 blit,不产生 N 块循环
	src := filepath.Join(t.TempDir(), "broken.png")
	if err := os.WriteFile(src, []byte("definitely-not-an-image"), 0o644); err != nil {
		t.Fatalf("写损坏测试图: %v", err)
	}

	for _, fit := range []string{"", layer.FitCover, layer.FitContain, layer.FitFill, layer.FitTile, "flip"} {
		backend := &fakeBackend{}
		l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithImage(src))
		if fit != "" {
			l.SetFit(fit)
		}

		if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
			t.Fatalf("RenderLayer(fit=%q): %v", fit, err)
		}

		assertImageCalls(t, backend.images, []imageCall{
			{src, image.Rectangle{}, image.Rect(0, 0, 100, 50)},
		})
	}
}

func TestQrCodeBlitsWholeImageIntoInscribedSquare(t *testing.T) {
	// QR 臂迁移(ADR 0015 内切正方形 + ADR 0024 blit):产物恒正方形、目标恒正方形,
	// cover 窗 = 整图——srcRect 零哨兵整图 blit、零内在尺寸读取,quiet zone 语义不变
	qr := layer.NewQrCodeLayer(layer.WithSize(100, 50), layer.WithPadding(5), layer.WithQrText("payload"))
	qr.SetResolvedSrc("/tmp/qr.png")
	backend := &fakeBackend{}

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), qr); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	if len(backend.images) != 1 {
		t.Fatalf("image 调用数 = %d, want 1", len(backend.images))
	}
	img := backend.images[0]
	if img.src != "/tmp/qr.png" {
		t.Errorf("src = %q, want 物化引用", img.src)
	}
	if !img.srcRect.Empty() {
		t.Errorf("QR 整图 blit:srcRect = %s, want 零哨兵", img.srcRect)
	}
	// 内容盒 90×40 → 内切边长 40,left/top 缺省锚定 padding 原点
	if img.dstRect != (image.Rect(5, 5, 45, 45)) {
		t.Errorf("dstRect = %s, want (5,5)-(45,45)", img.dstRect)
	}
}

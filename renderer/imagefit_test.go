package renderer_test

// paintImage contain 分派契约(ADR 0023 / spec §4):绘制分派层按物化后源图内在比例
// 把目标矩形收缩成等比矩形再调 DrawImage——原语签名与 cover 语义一字不动;
// cover 缺省(含未知值降级、显式 cover、内在尺寸读取失败)路径零回归。
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
// 显式 cover / 未知值不的表达式形态,不经 SetFit 归一门)
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

func TestContainShrinksRectToIntrinsicRatio(t *testing.T) {
	// 方图(1:1)进 100×50 横盒:收缩为 50×50、盒内居中;图层盒背景 rect 照旧先画
	src := writeSolidPng(t, 40, 40)
	backend := &fakeBackend{}
	l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithImage(src))
	l.SetFit(layer.FitContain)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertOps(t, backend.ops, []string{"begin", "rect", "image", "end"})
	if len(backend.images) != 1 {
		t.Fatalf("image 调用数 = %d, want 1", len(backend.images))
	}
	if got := backend.images[0]; got.src != src || got.x != 25 || got.y != 0 || got.w != 50 || got.h != 50 {
		t.Errorf("image 调用 = %+v, want {src 25 0 50 50}", got)
	}
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

	if len(backend.images) != 1 {
		t.Fatalf("image 调用数 = %d, want 1", len(backend.images))
	}
	if got := backend.images[0]; got.src != src || got.x != 8 || got.y != 6 || got.w != 84 || got.h != 21 {
		t.Errorf("image 调用 = %+v, want {src 8 6 84 21}", got)
	}
}

func TestCoverPathUnchangedByDefault(t *testing.T) {
	// 缺省 cover:内容盒整铺,分派几何与无 fit 版本逐字节一致
	src := writeSolidPng(t, 40, 40)
	backend := &fakeBackend{}
	l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithImage(src))

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	if len(backend.images) != 1 {
		t.Fatalf("image 调用数 = %d, want 1", len(backend.images))
	}
	if got := backend.images[0]; got.src != src || got.x != 0 || got.y != 0 || got.w != 100 || got.h != 50 {
		t.Errorf("image 调用 = %+v, want {src 0 0 100 50}", got)
	}
}

func TestExplicitCoverWireAlsoCovers(t *testing.T) {
	// wire 显式 cover(第三方手写 graph):解码归一缺省,渲染走 cover
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

	if len(backend.images) != 1 {
		t.Fatalf("image 调用数 = %d, want 1", len(backend.images))
	}
	if got := backend.images[0]; got.x != 0 || got.y != 0 || got.w != 100 || got.h != 50 {
		t.Errorf("image 调用 = %+v, want {src 0 0 100 50}", got)
	}
}

func TestUnknownFitValueDegradesToCover(t *testing.T) {
	// 未知 fit 值渲染面按 cover 降级(白名单在渲染臂,不在解码门)
	src := writeSolidPng(t, 40, 40)
	backend := &fakeBackend{}
	l := imageWithWireFit(t,
		layer.NewImageLayer(layer.WithSize(100, 50), layer.WithImage(src)),
		"fill")

	if got := l.Fit(); got != "fill" {
		t.Fatalf("解码后 Fit() = %q, want fill(透传)", got)
	}
	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	if len(backend.images) != 1 {
		t.Fatalf("image 调用数 = %d, want 1", len(backend.images))
	}
	if got := backend.images[0]; got.x != 0 || got.y != 0 || got.w != 100 || got.h != 50 {
		t.Errorf("image 调用 = %+v, want {src 0 0 100 50}", got)
	}
}

func TestIntrinsicSizeReadFailureFallsBackToCover(t *testing.T) {
	// 内在尺寸读取失败(损坏/非图文件)按 cover 走:损坏图不因 fit 崩分派
	// (真后端对损坏图的 DrawImage 结局属 image-renderer module 的断言面)
	src := filepath.Join(t.TempDir(), "broken.png")
	if err := os.WriteFile(src, []byte("definitely-not-an-image"), 0o644); err != nil {
		t.Fatalf("写损坏测试图: %v", err)
	}
	backend := &fakeBackend{}
	l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithImage(src))
	l.SetFit(layer.FitContain)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	if len(backend.images) != 1 {
		t.Fatalf("image 调用数 = %d, want 1", len(backend.images))
	}
	if got := backend.images[0]; got.src != src || got.x != 0 || got.y != 0 || got.w != 100 || got.h != 50 {
		t.Errorf("image 调用 = %+v, want {src 0 0 100 50}(cover 几何)", got)
	}
}

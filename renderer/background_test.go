package renderer_test

// 画布背景铺装(ADR 0022 / canvas-background spec §4):插入点 = 根层循环之前,
// 先色后图——背景之下无物(画布初始透明保留),背景图透明区露背景色;背景图
// cover 铺满画布矩形(DrawImage 原语自带 cover),画布级惰性物化(绘制分派前、
// 失败即抛、产物整体丢弃);renderLayer 单图层入口不画背景(无画布矩形语义)。
// 对拍基准 = php-canvas-next AbstractRenderer::paintBackground(v1.3.0)。

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/renderer"
	"github.com/HankChenCH/go-canvas/resolver"
)

func strp(s string) *string { return &s }

func backgroundedCanvas(t *testing.T, color, image *string, layers ...layer.Layer) *canvas.Canvas {
	t.Helper()
	c := canvas.New(320, 240, layers...)
	c.SetBackground(canvas.NewCanvasBackground(color, image))
	return c
}

func TestRenderPaintsBackgroundColorBeforeLayers(t *testing.T) {
	// 先铺色后图层:首个 rect = 整幅画布 + 背景色(无边框)
	content := layer.NewImageLayer(layer.WithSize(20, 10))
	backend := &fakeBackend{}
	r := newRenderer(t, backend)

	if _, err := r.Render(context.Background(),
		backgroundedCanvas(t, strp("#ffffff"), nil, content)); err != nil {
		t.Fatalf("Render: %v", err)
	}

	assertOps(t, backend.ops, []string{"begin", "rect", "rect", "end"})
	bg := backend.rects[0]
	if bg.x != 0 || bg.y != 0 || bg.w != 320 || bg.h != 240 {
		t.Errorf("背景色 rect = (%d,%d,%d,%d), want 整幅画布 (0,0,320,240)", bg.x, bg.y, bg.w, bg.h)
	}
	if bg.bg == nil || *bg.bg != "#ffffff" {
		t.Errorf("背景色 = %v, want #ffffff", bg.bg)
	}
	if bg.border != (layer.Border{}) {
		t.Errorf("背景铺色不得带边框: %+v", bg.border)
	}
}

func TestRenderPaintsBackgroundImageCoverOnTopOfColor(t *testing.T) {
	// 先色后图:背景图整幅 cover(透明区露色),均在根层之前
	content := layer.NewImageLayer(layer.WithSize(20, 10))
	backend := &fakeBackend{}
	r := newRenderer(t, backend)

	if _, err := r.Render(context.Background(),
		backgroundedCanvas(t, strp("#ffffff"), strp("/tmp/local.png"), content)); err != nil {
		t.Fatalf("Render: %v", err)
	}

	assertOps(t, backend.ops, []string{"begin", "rect", "image", "rect", "end"})
	if img := backend.images[0]; img.src != "/tmp/local.png" ||
		img.x != 0 || img.y != 0 || img.w != 320 || img.h != 240 {
		t.Errorf("背景图绘制 = %+v, want 本地路径整幅 cover (0,0,320,240)", img)
	}
}

func TestRenderBackgroundImageMaterializesAtPaintTime(t *testing.T) {
	// 背景图画布级惰性物化:事件序 = begin → download → image → end(无预遍历)
	var events []string
	backend := &loggingBackend{fakeBackend: &fakeBackend{}, events: &events}
	downloader := &spyDownloader{events: &events, content: []byte("png-bytes")}
	r := renderer.New(backend, resolver.New(
		resolver.WithDownloader(downloader),
		resolver.WithCacheRoot(t.TempDir()),
	))

	if _, err := r.Render(context.Background(),
		backgroundedCanvas(t, nil, strp("https://cdn.example.com/bg.png"))); err != nil {
		t.Fatalf("Render: %v", err)
	}

	want := []string{"begin", "download:https://cdn.example.com/bg.png", "image", "end"}
	if fmt.Sprint(events) != fmt.Sprint(want) {
		t.Errorf("事件序不符(物化时机漂移):\n got  %v\n want %v", events, want)
	}
}

func TestRenderBackgroundMaterializationFailureDropsSurface(t *testing.T) {
	// 失败即抛:渲染面丢弃(end 不调用)、无产物逃逸,稳定 code 上冒
	var events []string
	backend := &loggingBackend{fakeBackend: &fakeBackend{}, events: &events}
	downloader := &spyDownloader{events: &events, err: errors.New("boom")}
	r := renderer.New(backend, resolver.New(
		resolver.WithDownloader(downloader),
		resolver.WithCacheRoot(t.TempDir()),
	))

	product, err := r.Render(context.Background(),
		backgroundedCanvas(t, nil, strp("https://cdn.example.com/missing.png")))
	if err == nil {
		t.Fatal("背景物化失败必须报错")
	}
	if !errors.Is(err, resolver.ErrResourceDownloadFailed) {
		t.Errorf("错误 = %v, want ErrResourceDownloadFailed", err)
	}
	if product != nil {
		t.Errorf("失败渲染必须无产物, got %v", product)
	}
	for _, op := range backend.ops {
		if op == "end" {
			t.Error("渲染面必须丢弃(end 不得调用)")
		}
	}
}

func TestRenderLayerSkipsBackground(t *testing.T) {
	// 单图层便捷入口不画背景(spec §4 Q9):无画布矩形语义,背景只随 Render 走
	content := layer.NewImageLayer(layer.WithSize(30, 20))
	backend := &fakeBackend{}
	r := newRenderer(t, backend)

	if _, err := r.RenderLayer(context.Background(), content); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}
	assertOps(t, backend.ops, []string{"begin", "rect", "end"})
}

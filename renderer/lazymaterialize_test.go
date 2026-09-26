package renderer_test

// 渲染期惰性物化(TableLayer V2 spec §4.3 / go-canvas 工票 12):
// 渲染器无 resolve 预遍历,绘制分派前按需物化——失败即抛、渲染面丢弃、
// 无产物逃逸(begin/end 包裹丢弃渲染面);对存量图产物字节等价,仅失败时机
// 从预遍历后移到首笔绘制。对拍基准 = php-canvas-next AbstractRenderer 与
// image-renderer 包的惰性物化 spy 测试。

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/layer"
	"github.com/hankchen/go-canvas/renderer"
	"github.com/hankchen/go-canvas/resolver"
)

// spyDownloader 记录下载事件(与绘制原语共享同一事件序,锁物化时机)
type spyDownloader struct {
	events  *[]string
	content []byte
	err     error
}

func (d *spyDownloader) Download(ctx context.Context, url string) ([]byte, error) {
	*d.events = append(*d.events, "download:"+url)
	if d.err != nil {
		return nil, d.err
	}
	return d.content, nil
}

// loggingBackend 包裹 fakeBackend,把五个原语调用追加进共享事件序
type loggingBackend struct {
	*fakeBackend
	events *[]string
}

func (b *loggingBackend) Begin(w, h int) error {
	*b.events = append(*b.events, "begin")
	return b.fakeBackend.Begin(w, h)
}

func (b *loggingBackend) DrawRect(x, y, w, h int, bg *string, border layer.Border) error {
	*b.events = append(*b.events, "rect")
	return b.fakeBackend.DrawRect(x, y, w, h, bg, border)
}

func (b *loggingBackend) DrawImage(src string, x, y, w, h int) error {
	*b.events = append(*b.events, "image")
	return b.fakeBackend.DrawImage(src, x, y, w, h)
}

func (b *loggingBackend) DrawText(line string, x, y int, fontFile string, fontSize int, fontColor, horizontalAlign, verticalAlign string, angle int) error {
	*b.events = append(*b.events, "text")
	return b.fakeBackend.DrawText(line, x, y, fontFile, fontSize, fontColor, horizontalAlign, verticalAlign, angle)
}

func (b *loggingBackend) End() any {
	*b.events = append(*b.events, "end")
	return b.fakeBackend.End()
}

func TestLazyMaterializationHappensAtPaintTime(t *testing.T) {
	// 物化发生在绘制分派时(首笔绘制前)而非渲染前预遍历:
	// 事件序 = begin → download → rect → image → end
	var events []string
	image := layer.NewImageLayer(layer.WithSize(40, 24))
	image.SetImage("https://example.com/a.png")
	backend := &loggingBackend{fakeBackend: &fakeBackend{}, events: &events}
	downloader := &spyDownloader{events: &events, content: []byte("png-bytes")}
	r := renderer.New(backend, resolver.New(
		resolver.WithDownloader(downloader),
		resolver.WithCacheRoot(t.TempDir()),
	))

	if _, err := r.Render(context.Background(), canvas.New(100, 100, image)); err != nil {
		t.Fatalf("Render: %v", err)
	}

	want := []string{"begin", "download:https://example.com/a.png", "rect", "image", "end"}
	if fmt.Sprint(events) != fmt.Sprint(want) {
		t.Errorf("事件序不符(物化时机漂移):\n got  %v\n want %v", events, want)
	}
}

func TestMaterializationFailureDropsSurface(t *testing.T) {
	// 失败即抛:渲染面丢弃(end 不调用)、无产物逃逸,稳定 code 上冒
	var events []string
	image := layer.NewImageLayer(layer.WithSize(40, 24))
	image.SetImage("https://example.com/missing.png")
	backend := &loggingBackend{fakeBackend: &fakeBackend{}, events: &events}
	downloader := &spyDownloader{events: &events, err: errors.New("boom")}
	r := renderer.New(backend, resolver.New(
		resolver.WithDownloader(downloader),
		resolver.WithCacheRoot(t.TempDir()),
	))

	product, err := r.Render(context.Background(), canvas.New(100, 100, image))
	if err == nil {
		t.Fatal("物化失败必须报错")
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

func TestRenderLayerLazyMaterialization(t *testing.T) {
	// 便捷入口同语义:无预遍历,绘制分派前按需物化
	var events []string
	font := layer.NewTextLayer(layer.WithSize(60, 24), layer.WithFont("https://example.com/font.ttf", 12, "#000"))
	font.SetText("物化时机")
	backend := &loggingBackend{fakeBackend: &fakeBackend{}, events: &events}
	downloader := &spyDownloader{events: &events, content: []byte("ttf-bytes")}
	r := renderer.New(backend, resolver.New(
		resolver.WithDownloader(downloader),
		resolver.WithCacheRoot(t.TempDir()),
	))

	if _, err := r.RenderLayer(context.Background(), font); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	want := []string{"begin", "download:https://example.com/font.ttf", "rect", "text", "end"}
	if fmt.Sprint(events) != fmt.Sprint(want) {
		t.Errorf("事件序不符:\n got  %v\n want %v", events, want)
	}
}

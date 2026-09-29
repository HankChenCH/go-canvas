package renderer_test

// 渲染期控制面检查点(工票 render-cancellation 03,ADR 0008):ctx 错误映射三端
// 稳定 code(spec §5.2,php MaterializeException 为语义权威)——检查点与 php 四
// 检查点同位(根层循环每图层前/表行循环每行前;物化前见 resolver 包;下载前
// 检查点由 net/http ctx 穿线免费获得)。失败即抛、渲染面丢弃、无产物逃逸。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/layer"
	"github.com/hankchen/go-canvas/renderer"
	"github.com/hankchen/go-canvas/resolver"
)

// cancelAfterRectBackend 第 n 次 DrawRect 落笔后武装取消:模拟检查点之间的
// 长阻塞段被外部打断,驱动下一个检查点掐断渲染(不依赖真实定时,确定性测试)
type cancelAfterRectBackend struct {
	*fakeBackend
	cancel context.CancelFunc
	afterN int
}

func (b *cancelAfterRectBackend) DrawRect(x, y, w, h int, bg *string, border layer.Border) error {
	err := b.fakeBackend.DrawRect(x, y, w, h, bg, border)
	if b.cancel != nil && len(b.fakeBackend.rects) >= b.afterN {
		b.cancel()
		b.cancel = nil
	}
	return err
}

func TestPreCancelledContextAbortsRender(t *testing.T) {
	// 预取消 ctx:根层循环检查点先于任何绘制/物化掐断——零下载、无产物逃逸
	var events []string
	image := layer.NewImageLayer(layer.WithSize(40, 24))
	image.SetImage("https://example.com/a.png")
	backend := &loggingBackend{fakeBackend: &fakeBackend{}, events: &events}
	downloader := &spyDownloader{events: &events, content: []byte("png-bytes")}
	r := renderer.New(backend, resolver.New(
		resolver.WithDownloader(downloader),
		resolver.WithCacheRoot(t.TempDir()),
	))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	product, err := r.Render(ctx, canvas.New(100, 100, image))
	if err == nil {
		t.Fatal("预取消 ctx 的渲染必须报错")
	}
	if !errors.Is(err, resolver.ErrRenderCancelled) {
		t.Errorf("错误 = %v, want ErrRenderCancelled", err)
	}
	if product != nil {
		t.Errorf("失败渲染必须无产物, got %v", product)
	}
	for _, op := range backend.ops {
		if op == "end" {
			t.Error("渲染面必须丢弃(end 不得调用)")
		}
	}
	for _, ev := range events {
		if strings.HasPrefix(ev, "download:") {
			t.Errorf("检查点必须先于物化,下载不应发生: %v", events)
		}
	}
}

func TestExpiredDeadlineAbortsRender(t *testing.T) {
	// 已过期 deadline(WithTimeout 0 = 立即超时):ctx.Err() = DeadlineExceeded
	// → 映射 render_deadline_exceeded(与人为取消两码分立)
	image := layer.NewImageLayer(layer.WithSize(40, 24))
	image.SetImage("https://example.com/a.png")
	backend := &fakeBackend{}
	r := renderer.New(backend, resolver.New(resolver.WithCacheRoot(t.TempDir())))

	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	product, err := r.Render(ctx, canvas.New(100, 100, image))
	if err == nil {
		t.Fatal("deadline 已到的渲染必须报错")
	}
	if !errors.Is(err, resolver.ErrRenderDeadlineExceeded) {
		t.Errorf("错误 = %v, want ErrRenderDeadlineExceeded", err)
	}
	if product != nil {
		t.Errorf("失败渲染必须无产物, got %v", product)
	}
}

func TestTableRowLoopCheckpoint(t *testing.T) {
	// 行循环检查点:首行绘制中武装取消,第二行绘制前被行循环检查点掐断——
	// 根层循环检查点早已通过,无行循环检查点时本测试必失败(整表画完且无错)
	table := layer.NewTableLayer(layer.WithSize(100, 40))
	for range 2 {
		row := layer.NewTableRowLayer(layer.WithSize(100, 20))
		if err := table.AddRow(row); err != nil {
			t.Fatalf("AddRow: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend := &cancelAfterRectBackend{fakeBackend: &fakeBackend{}, cancel: cancel, afterN: 2}
	r := renderer.New(backend, resolver.New(resolver.WithCacheRoot(t.TempDir())))

	_, err := r.Render(ctx, canvas.New(100, 100, table))
	if err == nil {
		t.Fatal("首行绘制后取消,渲染必须报错")
	}
	if !errors.Is(err, resolver.ErrRenderCancelled) {
		t.Errorf("错误 = %v, want ErrRenderCancelled", err)
	}
	if len(backend.rects) != 2 {
		t.Errorf("第二行绘制前必须被行循环检查点掐断,rect 次数 = %d, want 2(表盒+首行)", len(backend.rects))
	}
	for _, op := range backend.ops {
		if op == "end" {
			t.Error("渲染面必须丢弃(end 不得调用)")
		}
	}
}

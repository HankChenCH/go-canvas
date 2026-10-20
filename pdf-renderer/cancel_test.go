package pdfrenderer

// 渲染期控制面测试(ADR 0008):ctx 取消/超时映射渲染 code sentinel——
// 检查点在核心模板(根层循环每图层前/表行每行前),后端消费同一错误面;
// 失败即抛、渲染面丢弃、无产物逃逸

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/resolver"
)

func TestPreCancelledContextAbortsRender(t *testing.T) {
	// 预取消 ctx:根层循环检查点先于任何绘制/物化掐断,报 ErrRenderCancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	l := layer.NewImageLayer(layer.WithSize(20, 20))
	l.SetImage("https://example.com/a.png")
	_, err := renderAsPDFCtx(t, ctx, resolver.New(resolver.WithCacheRoot(t.TempDir())),
		canvas.New(40, 40, l))
	if err == nil {
		t.Fatal("预取消 ctx 的渲染必须报错")
	}
	if !errors.Is(err, resolver.ErrRenderCancelled) {
		t.Errorf("错误 = %v, want ErrRenderCancelled", err)
	}
}

func TestDeadlineExceededContextAbortsRender(t *testing.T) {
	// 已到期的 deadline:同一检查点报 ErrRenderDeadlineExceeded
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	time.Sleep(time.Millisecond) // 确保 deadline 已过期

	l := layer.NewImageLayer(layer.WithSize(20, 20))
	l.SetImage("https://example.com/a.png")
	_, err := renderAsPDFCtx(t, ctx, resolver.New(resolver.WithCacheRoot(t.TempDir())),
		canvas.New(40, 40, l))
	if err == nil {
		t.Fatal("到期 ctx 的渲染必须报错")
	}
	if !errors.Is(err, resolver.ErrRenderDeadlineExceeded) {
		t.Errorf("错误 = %v, want ErrRenderDeadlineExceeded", err)
	}
}

func TestDownloadFailureAbortsRenderWithoutProduct(t *testing.T) {
	// 下载失败注入:errors.Is ErrResourceDownloadFailed、无产物逃逸
	// (失败即抛、渲染面丢弃,End 不达;ctx 归因优先于资源 code 的语义
	// 由核心 resolver 负责,此处注入普通错误走资源 code 臂)
	fd := &fakeDownloader{err: errors.New("boom")}
	rs := NewDefaultResolver(
		resolver.WithDownloader(fd),
		resolver.WithCacheRoot(t.TempDir()),
	)
	l := layer.NewImageLayer(layer.WithSize(20, 20))
	l.SetImage("https://example.com/broken.png")

	pdf, err := renderAsPDF(t, rs, canvas.New(40, 40, l))
	if err == nil {
		t.Fatal("下载失败必须报错")
	}
	if !errors.Is(err, resolver.ErrResourceDownloadFailed) {
		t.Errorf("错误 = %v, want ErrResourceDownloadFailed", err)
	}
	if len(pdf) != 0 {
		t.Errorf("失败渲染必须无产物逃逸, got %d 字节", len(pdf))
	}
	if fd.calls == 0 {
		t.Error("下载器未被调用")
	}
}

func TestDownloadCtxCancelledMapsRenderCode(t *testing.T) {
	// 下载被打断于 ctx 取消:下载错误归因映射渲染 code(ctx 归因优先,
	// 同一失败两种归因,渲染 code 是三端锚点)
	fd := &fakeDownloader{err: context.Canceled}
	rs := NewDefaultResolver(
		resolver.WithDownloader(fd),
		resolver.WithCacheRoot(t.TempDir()),
	)
	l := layer.NewImageLayer(layer.WithSize(20, 20))
	l.SetImage("https://example.com/slow.png")

	_, err := renderAsPDF(t, rs, canvas.New(40, 40, l))
	if !errors.Is(err, resolver.ErrRenderCancelled) {
		t.Errorf("错误 = %v, want ErrRenderCancelled", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("底层 context.Canceled 应保留在错误链上: %v", err)
	}
}

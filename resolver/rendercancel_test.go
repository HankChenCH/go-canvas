package resolver_test

// 渲染期控制面错误面(工票 render-cancellation 03,ADR 0008):resolve 入口检查点
// + 下载错误 ctx 归因。code 语义权威 = php MaterializeException(工票 02),
// code 字符串为三端锚点逐字节一致(spec §5.2);下载自身的流超时仍归既有
// resource_download_failed,不另立 code。

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hankchen/go-canvas/layer"
	"github.com/hankchen/go-canvas/resolver"
)

func TestResolveLayerEntryCheckpoint(t *testing.T) {
	// 预取消 ctx:resolve 入口检查点先于任何下载——零调用、报渲染 code
	fake := &fakeDownloader{content: []byte("png-bytes")}
	r := resolver.New(
		resolver.WithDownloader(fake),
		resolver.WithCacheRoot(t.TempDir()),
	)

	image := layer.NewImageLayer(layer.WithSize(40, 24))
	image.SetImage("https://example.com/a.png")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := r.ResolveLayer(ctx, image)
	if err == nil {
		t.Fatal("预取消 ctx 的物化必须报错")
	}
	if !errors.Is(err, resolver.ErrRenderCancelled) {
		t.Errorf("错误 = %v, want ErrRenderCancelled", err)
	}
	if len(fake.calls) != 0 {
		t.Errorf("检查点必须先于下载,不应发生: %v", fake.calls)
	}
}

func TestDownloadErrorContextAttribution(t *testing.T) {
	// 下载错误先做 ctx 归因:错误链命中 ctx 两错误之一 → 渲染 code
	// (而非 resource_download_failed);原 ctx 错误经 %w 保留可继续 errors.Is
	tests := []struct {
		name    string
		dlErr   error
		want    error
		notWant error
		ctxErr  error
	}{
		{
			name:    "下载被打断于人为取消",
			dlErr:   fmt.Errorf("下载请求失败(%s): %w", "https://example.com/a.png", context.Canceled),
			want:    resolver.ErrRenderCancelled,
			notWant: resolver.ErrResourceDownloadFailed,
			ctxErr:  context.Canceled,
		},
		{
			name:    "下载被打断于deadline到点",
			dlErr:   context.DeadlineExceeded,
			want:    resolver.ErrRenderDeadlineExceeded,
			notWant: resolver.ErrResourceDownloadFailed,
			ctxErr:  context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeDownloader{err: tt.dlErr}
			r := resolver.New(
				resolver.WithDownloader(fake),
				resolver.WithCacheRoot(t.TempDir()),
			)

			image := layer.NewImageLayer(layer.WithSize(40, 24))
			image.SetImage("https://example.com/a.png")

			err := r.ResolveLayer(context.Background(), image)
			if err == nil {
				t.Fatal("下载失败必须报错")
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("错误 = %v, want %v", err, tt.want)
			}
			if errors.Is(err, tt.notWant) {
				t.Errorf("错误 = %v, 不得归因 %v", err, tt.notWant)
			}
			if !errors.Is(err, tt.ctxErr) {
				t.Errorf("错误 = %v, 原 ctx 错误必须保留在链上(%v)", err, tt.ctxErr)
			}
		})
	}
}

func TestDownloadErrorWithoutContextStaysResourceCode(t *testing.T) {
	// 未命中 ctx 两错误的普通下载失败仍归既有 resource_download_failed
	// (下载自身的流超时归资源 code,spec §5.2 不另立 code)
	fake := &fakeDownloader{err: fmt.Errorf("下载响应非 2xx:HTTP 404(https://example.com/a.png)")}
	r := resolver.New(
		resolver.WithDownloader(fake),
		resolver.WithCacheRoot(t.TempDir()),
	)

	image := layer.NewImageLayer(layer.WithSize(40, 24))
	image.SetImage("https://example.com/a.png")

	err := r.ResolveLayer(context.Background(), image)
	if !errors.Is(err, resolver.ErrResourceDownloadFailed) {
		t.Errorf("错误 = %v, want ErrResourceDownloadFailed", err)
	}
	if errors.Is(err, resolver.ErrRenderCancelled) || errors.Is(err, resolver.ErrRenderDeadlineExceeded) {
		t.Errorf("错误 = %v, 普通下载失败不得归因渲染 code", err)
	}
}

func TestHTTPDownloaderCancelledContextEndToEnd(t *testing.T) {
	// 真实下载器端到端:net/http 经 NewRequestWithContext 免费获得取消传播,
	// HTTPDownloader 返回的错误链可被归因为渲染 code
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("png-bytes"))
	}))
	defer srv.Close()

	r := resolver.New(resolver.WithCacheRoot(t.TempDir()))

	image := layer.NewImageLayer(layer.WithSize(40, 24))
	image.SetImage(srv.URL + "/a.png")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := r.ResolveLayer(ctx, image)
	if err == nil {
		t.Fatal("预取消 ctx 的真实下载必须报错")
	}
	if !errors.Is(err, resolver.ErrRenderCancelled) {
		t.Errorf("错误 = %v, want ErrRenderCancelled", err)
	}
	if errors.Is(err, resolver.ErrResourceDownloadFailed) {
		t.Errorf("错误 = %v, ctx 取消不得归因 resource_download_failed", err)
	}
}

func TestHTTPDownloaderDeadlineEndToEnd(t *testing.T) {
	// 真实下载器端到端·deadline:下载进行中 deadline 到点(net/http 免费打断),
	// 下载错误报渲染 code 而非 resource_download_failed(工票 03 验收原案)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := resolver.New(resolver.WithCacheRoot(t.TempDir()))

	image := layer.NewImageLayer(layer.WithSize(40, 24))
	image.SetImage(srv.URL + "/a.png")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := r.ResolveLayer(ctx, image)
	if err == nil {
		t.Fatal("deadline 到点的真实下载必须报错")
	}
	if !errors.Is(err, resolver.ErrRenderDeadlineExceeded) {
		t.Errorf("错误 = %v, want ErrRenderDeadlineExceeded", err)
	}
	if errors.Is(err, resolver.ErrResourceDownloadFailed) {
		t.Errorf("错误 = %v, ctx 到期不得归因 resource_download_failed", err)
	}
}

func TestRenderCodeSentinelsByteExact(t *testing.T) {
	// 三端锚点逐字断言:code 字符串与 php 权威端逐字节一致——
	// render_cancelled  = php-canvas-next src/Runtime/CancellationSource.php(工票 02)
	// deadline 两码同源 = src/Runtime/TimeoutCancellation.php;spec §5.2
	if got := resolver.ErrRenderCancelled.Error(); got != "render_cancelled" {
		t.Errorf("render code 漂移 = %q, want %q(php 侧为权威锚点)", got, "render_cancelled")
	}
	if got := resolver.ErrRenderDeadlineExceeded.Error(); got != "render_deadline_exceeded" {
		t.Errorf("render code 漂移 = %q, want %q(php 侧为权威锚点)", got, "render_deadline_exceeded")
	}
}

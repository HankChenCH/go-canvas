package resolver_test

// 画布背景图物化(ADR 0022):画布级入口与 ResolveLayer 同语义——入口检查点、
// 远程引用下载缓存(img_layers 同目录)、本地路径直通;背景不是图层、无 setter
// 回写,物化结果以返回值交渲染器。对拍基准 = php-canvas-next
// ResourceResolver::resolveBackground(v1.3.0)。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/HankChenCH/go-canvas/resolver"
)

func TestResolveBackgroundRemoteURLDownloadsAndCaches(t *testing.T) {
	fd := &fakeDownloader{content: []byte("png-bytes")}
	r, root := newResolver(t, fd, nil)

	path, err := r.ResolveBackground(context.Background(), "https://cdn.example.com/bg.png")
	if err != nil {
		t.Fatalf("ResolveBackground: %v", err)
	}
	if !strings.HasPrefix(path, root) || !strings.HasSuffix(path, ".png") {
		t.Errorf("物化结果 = %q, want %s 下 img_layers 缓存路径", path, root)
	}
	if len(fd.calls) != 1 || fd.calls[0] != "https://cdn.example.com/bg.png" {
		t.Errorf("下载调用 = %v, want 恰一次", fd.calls)
	}

	// 命中缓存不再下载(同 resolveLayer 语义)
	again, err := r.ResolveBackground(context.Background(), "https://cdn.example.com/bg.png")
	if err != nil {
		t.Fatalf("二次 ResolveBackground: %v", err)
	}
	if again != path {
		t.Errorf("缓存结果 = %q, want %q", again, path)
	}
	if len(fd.calls) != 1 {
		t.Errorf("二次调用下载次数 = %d, want 仍为 1", len(fd.calls))
	}
}

func TestResolveBackgroundLocalPathPassthrough(t *testing.T) {
	fd := &fakeDownloader{}
	r, _ := newResolver(t, fd, nil)

	path, err := r.ResolveBackground(context.Background(), "/data/bg.png")
	if err != nil {
		t.Fatalf("ResolveBackground: %v", err)
	}
	if path != "/data/bg.png" {
		t.Errorf("本地路径 = %q, want 原样直通", path)
	}
	if len(fd.calls) != 0 {
		t.Errorf("本地路径不得触发下载: %v", fd.calls)
	}
}

func TestResolveBackgroundDownloadFailureCarriesStableCode(t *testing.T) {
	fd := &fakeDownloader{err: errors.New("boom")}
	r, _ := newResolver(t, fd, nil)

	_, err := r.ResolveBackground(context.Background(), "https://cdn.example.com/missing.png")
	if err == nil {
		t.Fatal("下载失败必须报错")
	}
	if !errors.Is(err, resolver.ErrResourceDownloadFailed) {
		t.Errorf("错误 = %v, want ErrResourceDownloadFailed", err)
	}
}

func TestResolveBackgroundChecksContextBeforeDownload(t *testing.T) {
	// 入口检查点(与 ResolveLayer 同位,ADR 0008):已取消 ctx 在下载发起前掐断
	fd := &fakeDownloader{}
	r, _ := newResolver(t, fd, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.ResolveBackground(ctx, "https://cdn.example.com/bg.png")
	if !errors.Is(err, resolver.ErrRenderCancelled) {
		t.Errorf("错误 = %v, want ErrRenderCancelled", err)
	}
	if len(fd.calls) != 0 {
		t.Errorf("取消后不得发起下载: %v", fd.calls)
	}
}

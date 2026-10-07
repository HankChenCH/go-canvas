package resolver_test

// ResourceResolverTest 平移:假 Downloader(记录调用次数/可注入失败)、假 QR 物化器、
// 下载一次跨图层共享缓存、本地路径零调用;另补工单检查项——字体跳过语义、
// 缓存命中即跳过、缓存键方案、二维码键含宽度、容器递归下钻、QR 缝未接线报错。

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/resolver"
)

// fakeDownloader Downloader 手工桩:记录调用并返回预设内容,err 模拟下载失败
// (对齐 PHP tests/Support/FakeDownloader)
type fakeDownloader struct {
	calls   []string
	content []byte
	err     error
}

func (f *fakeDownloader) Download(_ context.Context, rawURL string) ([]byte, error) {
	f.calls = append(f.calls, rawURL)
	if f.err != nil {
		return nil, f.err
	}
	return f.content, nil
}

// qrCall 二维码物化调用记录
type qrCall struct {
	text  string
	width int
}

// fakeQR 假二维码物化器:记录调用并返回预设 PNG 字节(固定选项实现属 M2 后端,
// 核心测试只验缝的接线与缓存行为)
type fakeQR struct {
	calls []qrCall
	png   []byte
	err   error
}

func (f *fakeQR) Materialize(_ context.Context, text string, width int) ([]byte, error) {
	f.calls = append(f.calls, qrCall{text: text, width: width})
	if f.err != nil {
		return nil, f.err
	}
	return f.png, nil
}

// newResolver 组装被测物化器:注入临时缓存根目录与假 Downloader,
// qr 非 nil 时接线二维码缝;返回缓存根目录供断言落盘位置
func newResolver(t *testing.T, fd *fakeDownloader, fq resolver.QRMaterializer, opts ...resolver.Option) (*resolver.ResourceResolver, string) {
	t.Helper()
	root := t.TempDir()
	all := append([]resolver.Option{
		resolver.WithCacheRoot(root),
		resolver.WithDownloader(fd),
	}, opts...)
	if fq != nil {
		all = append(all, resolver.WithQRMaterializer(fq))
	}
	return resolver.New(all...), root
}

// cacheKey 图片/字体缓存键:sha256(完整 URL) 十六进制 + URL path 原扩展名,
// 与实现同构以锁定键方案(有意偏离 PHP basename 键)
func cacheKey(t *testing.T, rawURL string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(rawURL))
	ext := ""
	if u, err := url.Parse(rawURL); err == nil {
		ext = path.Ext(u.Path)
	}
	return hex.EncodeToString(sum[:]) + ext
}

// qrCacheKey 二维码缓存键:sha256(内容_宽度) 十六进制 + ".png"
func qrCacheKey(t *testing.T, text string, width int) string {
	t.Helper()
	sum := sha256.Sum256([]byte(text + "_" + strconv.Itoa(width)))
	return hex.EncodeToString(sum[:]) + ".png"
}

func TestRemoteImageDownloadedOnceAndCachedAcrossLayers(t *testing.T) {
	// PHP ResourceResolverTest::testRemoteImageDownloadedOnceAndCachedAcrossLayers:
	// Resolver 只负责把字节落盘,不校验图片内容,哑字节即可
	fd := &fakeDownloader{content: []byte("fake-image-bytes")}
	r, root := newResolver(t, fd, nil)
	rawURL := "https://cdn.example.com/img/red.png"

	c := canvas.New(10, 10,
		layer.NewImageLayer(layer.WithSize(10, 10), layer.WithImage(rawURL)),
		layer.NewImageLayer(layer.WithSize(10, 10), layer.WithImage(rawURL)),
	)
	if err := r.Resolve(context.Background(), c); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(fd.calls) != 1 {
		t.Fatalf("下载次数 = %d, want 1(%v)", len(fd.calls), fd.calls)
	}
	expected := filepath.Join(root, "img_layers", cacheKey(t, rawURL))
	content, err := os.ReadFile(expected)
	if err != nil || string(content) != "fake-image-bytes" {
		t.Fatalf("缓存文件 %s 内容 = %q, err = %v, want fake-image-bytes", expected, content, err)
	}
	for i, l := range c.GetLayers() {
		img := l.(*layer.ImageLayer)
		if got := img.ResolvedSrc(); got == nil || *got != expected {
			t.Errorf("图层 %d ResolvedSrc = %v, want %s", i, got, expected)
		}
	}
}

func TestLocalImageSkipsDownload(t *testing.T) {
	// PHP ResourceResolverTest::testLocalImageSkipsDownload:本地路径直接使用,
	// 不回写物化结果(getter 回退原始引用保持可见),下载零调用
	fd := &fakeDownloader{content: []byte("should-not-be-used")}
	r, _ := newResolver(t, fd, nil)

	l := layer.NewImageLayer(layer.WithSize(10, 10), layer.WithImage("/tmp/local.png"))
	if err := r.ResolveLayer(context.Background(), l); err != nil {
		t.Fatalf("ResolveLayer: %v", err)
	}

	if len(fd.calls) != 0 {
		t.Errorf("下载次数 = %d, want 0", len(fd.calls))
	}
	if got := l.ResolvedSrc(); got == nil || *got != "/tmp/local.png" {
		t.Errorf("ResolvedSrc = %v, want /tmp/local.png", got)
	}
}

func TestImageDownloadFailureReportsURL(t *testing.T) {
	// PHP ResourceResolverTest::testImageDownloadFailureThrows:失败报含 URL 的错误
	fd := &fakeDownloader{err: errors.New("boom")}
	r, _ := newResolver(t, fd, nil)

	l := layer.NewImageLayer(layer.WithSize(10, 10), layer.WithImage("https://cdn.example.com/missing.png"))
	err := r.ResolveLayer(context.Background(), l)
	// 工票 12(Q5 决议):绘制期错误 code 化,消息形态「code + 上下文」对齐 PHP
	if err == nil || !errors.Is(err, resolver.ErrResourceDownloadFailed) ||
		!strings.Contains(err.Error(), "https://cdn.example.com/missing.png") {
		t.Errorf("err = %v, want ErrResourceDownloadFailed 且消息含 URL", err)
	}
}

func TestImageCacheHitSkipsDownload(t *testing.T) {
	// 命中即跳过、无失效机制(语义与 PHP 一致):预置缓存文件后不再下载、不覆写
	fd := &fakeDownloader{content: []byte("fresh-bytes")}
	r, root := newResolver(t, fd, nil)
	rawURL := "https://cdn.example.com/img/blue.png"
	cached := filepath.Join(root, "img_layers", cacheKey(t, rawURL))
	if err := os.MkdirAll(filepath.Dir(cached), 0o755); err != nil {
		t.Fatalf("预置缓存目录: %v", err)
	}
	if err := os.WriteFile(cached, []byte("cached-bytes"), 0o644); err != nil {
		t.Fatalf("预置缓存文件: %v", err)
	}

	l := layer.NewImageLayer(layer.WithSize(10, 10), layer.WithImage(rawURL))
	if err := r.ResolveLayer(context.Background(), l); err != nil {
		t.Fatalf("ResolveLayer: %v", err)
	}

	if len(fd.calls) != 0 {
		t.Errorf("命中缓存后下载次数 = %d, want 0", len(fd.calls))
	}
	content, err := os.ReadFile(cached)
	if err != nil || string(content) != "cached-bytes" {
		t.Errorf("缓存文件被覆写: 内容 = %q, err = %v, want cached-bytes 原样", content, err)
	}
	if got := l.ResolvedSrc(); got == nil || *got != cached {
		t.Errorf("ResolvedSrc = %v, want %s", got, cached)
	}
}

func TestImageSaveFailureReportsError(t *testing.T) {
	// 落盘失败报错:缓存根目录被同名文件占用,MkdirAll 必败(对 root 用户也成立)
	fd := &fakeDownloader{content: []byte("fake-image-bytes")}
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("预置占位文件: %v", err)
	}

	r := resolver.New(resolver.WithCacheRoot(blocker), resolver.WithDownloader(fd))
	l := layer.NewImageLayer(layer.WithSize(10, 10), layer.WithImage("https://cdn.example.com/img/red.png"))
	if err := r.ResolveLayer(context.Background(), l); err == nil {
		t.Error("落盘失败应报错,得到 nil")
	}
}

func TestRemoteFontMaterialized(t *testing.T) {
	// PHP ResourceResolverTest::testRemoteFontMaterialized
	fd := &fakeDownloader{content: []byte("ttf-bytes")}
	r, root := newResolver(t, fd, nil)
	rawURL := "https://cdn.example.com/fonts/msyh.ttf"

	l := layer.NewTextLayer(
		layer.WithSize(100, 30),
		layer.WithText("文本"),
		layer.WithFont(rawURL, 12, "#000"),
	)
	if err := r.ResolveLayer(context.Background(), l); err != nil {
		t.Fatalf("ResolveLayer: %v", err)
	}

	expected := filepath.Join(root, "text_layers", cacheKey(t, rawURL))
	content, err := os.ReadFile(expected)
	if err != nil || string(content) != "ttf-bytes" {
		t.Fatalf("缓存文件 %s 内容 = %q, err = %v, want ttf-bytes", expected, content, err)
	}
	if got := l.ResolvedFont(); got != expected {
		t.Errorf("ResolvedFont = %q, want %s", got, expected)
	}
}

func TestFontSkipSemantics(t *testing.T) {
	// 工单检查项:空串/纯数字 = 渲染端内置默认字体语义,跳过;本地路径仅非 URL
	// 同样跳过——三者均零下载、零回写(getter 回退原始值)
	fd := &fakeDownloader{content: []byte("ttf-bytes")}
	r, _ := newResolver(t, fd, nil)

	for _, font := range []string{"", "123", "3.14", "/usr/share/fonts/msyh.ttf"} {
		l := layer.NewTextLayer(layer.WithSize(100, 30), layer.WithFont(font, 12, "#000"))
		if err := r.ResolveLayer(context.Background(), l); err != nil {
			t.Fatalf("font %q: ResolveLayer: %v", font, err)
		}
		if len(fd.calls) != 0 {
			t.Errorf("font %q: 下载次数 = %d, want 0", font, len(fd.calls))
		}
		if got := l.ResolvedFont(); got != font {
			t.Errorf("font %q: ResolvedFont = %q, want 原始值", font, got)
		}
	}
}

func TestQrCodeMaterializedToFile(t *testing.T) {
	// PHP ResourceResolverTest::testQrCodeMaterializedToPngFile 适配:核心包的
	// PNG 字节来自注入的物化器(ADR-0002),断言字节落盘、路径回写与走缝不走下载;
	// PNG 魔数校验属 M2 固定选项实现的验收,不在核心
	fq := &fakeQR{png: []byte("\x89PNG-fake")}
	fd := &fakeDownloader{content: []byte("should-not-be-used")}
	r, root := newResolver(t, fd, fq)
	qrText := "https://example.com/中文"

	l := layer.NewQrCodeLayer(layer.WithSize(60, 60), layer.WithQrText(qrText))
	if err := r.ResolveLayer(context.Background(), l); err != nil {
		t.Fatalf("ResolveLayer: %v", err)
	}

	if len(fd.calls) != 0 {
		t.Errorf("下载次数 = %d, want 0", len(fd.calls))
	}
	if len(fq.calls) != 1 || fq.calls[0] != (qrCall{text: qrText, width: 60}) {
		t.Errorf("物化调用 = %v, want [{%s 60}]", fq.calls, qrText)
	}
	expected := filepath.Join(root, "qr_layers", qrCacheKey(t, qrText, 60))
	content, err := os.ReadFile(expected)
	if err != nil || string(content) != "\x89PNG-fake" {
		t.Fatalf("缓存文件 %s 内容 = %q, err = %v, want PNG 字节", expected, content, err)
	}
	if got := l.ResolvedSrc(); got == nil || *got != expected {
		t.Errorf("ResolvedSrc = %v, want %s", got, expected)
	}
}

func TestQrCodeResolutionIsIdempotent(t *testing.T) {
	// PHP ResourceResolverTest::testQrCodeResolutionIsIdempotent:已有物化结果
	// 则跳过,路径稳定、文件仍在、物化器只调一次
	fq := &fakeQR{png: []byte("\x89PNG-fake")}
	r, _ := newResolver(t, &fakeDownloader{}, fq)

	l := layer.NewQrCodeLayer(layer.WithSize(60, 60), layer.WithQrText("https://example.com"))
	if err := r.ResolveLayer(context.Background(), l); err != nil {
		t.Fatalf("第一次 ResolveLayer: %v", err)
	}
	first := *l.ResolvedSrc()
	if err := r.ResolveLayer(context.Background(), l); err != nil {
		t.Fatalf("第二次 ResolveLayer: %v", err)
	}

	if got := *l.ResolvedSrc(); got != first {
		t.Errorf("二次物化路径 = %q, want %q", got, first)
	}
	if _, err := os.Stat(first); err != nil {
		t.Errorf("物化文件丢失: %v", err)
	}
	if len(fq.calls) != 1 {
		t.Errorf("物化调用 = %d, want 1", len(fq.calls))
	}
}

func TestQrCodeEmptyTextSkips(t *testing.T) {
	// 工单检查项:文本为空跳过物化
	fq := &fakeQR{png: []byte("\x89PNG-fake")}
	r, _ := newResolver(t, &fakeDownloader{}, fq)

	l := layer.NewQrCodeLayer(layer.WithSize(60, 60))
	if err := r.ResolveLayer(context.Background(), l); err != nil {
		t.Fatalf("ResolveLayer: %v", err)
	}

	if len(fq.calls) != 0 {
		t.Errorf("物化调用 = %d, want 0", len(fq.calls))
	}
	if got := l.ResolvedSrc(); got != nil {
		t.Errorf("ResolvedSrc = %v, want nil", got)
	}
}

func TestQrCodeCachedAcrossLayersByKey(t *testing.T) {
	// 工单检查项:二维码键 = 内容+宽度 hash——同内容同宽跨图层共享缓存只物化
	// 一次,同内容不同宽键不同
	fq := &fakeQR{png: []byte("\x89PNG-fake")}
	r, root := newResolver(t, &fakeDownloader{}, fq)
	qrText := "https://example.com"

	a := layer.NewQrCodeLayer(layer.WithSize(60, 0), layer.WithQrText(qrText))
	b := layer.NewQrCodeLayer(layer.WithSize(60, 0), layer.WithQrText(qrText))
	c := layer.NewQrCodeLayer(layer.WithSize(80, 0), layer.WithQrText(qrText))
	if err := r.Resolve(context.Background(), canvas.New(100, 100, a, b, c)); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(fq.calls) != 2 {
		t.Fatalf("物化调用 = %d, want 2(%v)", len(fq.calls), fq.calls)
	}
	if gotA, gotB := *a.ResolvedSrc(), *b.ResolvedSrc(); gotA != gotB {
		t.Errorf("同内容同宽路径不一致: %q vs %q", gotA, gotB)
	}
	expectedA := filepath.Join(root, "qr_layers", qrCacheKey(t, qrText, 60))
	expectedC := filepath.Join(root, "qr_layers", qrCacheKey(t, qrText, 80))
	if got := *a.ResolvedSrc(); got != expectedA {
		t.Errorf("宽 60 路径 = %q, want %q", got, expectedA)
	}
	if got := *c.ResolvedSrc(); got != expectedC {
		t.Errorf("宽 80 路径 = %q, want %q", got, expectedC)
	}
}

func TestQrCodeMaterializerRequired(t *testing.T) {
	// ADR-0002:核心只定义缝,遇二维码图层而未接线时报错而非静默跳过
	r, _ := newResolver(t, &fakeDownloader{}, nil)

	l := layer.NewQrCodeLayer(layer.WithSize(60, 60), layer.WithQrText("https://example.com"))
	err := r.ResolveLayer(context.Background(), l)
	if !errors.Is(err, resolver.ErrQRMaterializerRequired) {
		t.Errorf("err = %v, want ErrQRMaterializerRequired", err)
	}
}

func TestContainerRecursion(t *testing.T) {
	// 工单检查项:容器递归下钻 表→行→单元格→内容层;无内容层的单元格不报错
	fd := &fakeDownloader{content: []byte("fake-image-bytes")}
	r, root := newResolver(t, fd, nil)
	rawURL := "https://cdn.example.com/img/cell.png"

	img := layer.NewImageLayer(layer.WithSize(90, 20), layer.WithImage(rawURL))
	cell := layer.NewTableCellLayer(layer.WithSize(100, 20), layer.WithContent(img))
	empty := layer.NewTableCellLayer(layer.WithSize(100, 10))
	row := layer.NewTableRowLayer(layer.WithCells(cell, empty))
	table := layer.NewTableLayer(layer.WithSize(100, 30), layer.WithRows(row))
	if err := r.Resolve(context.Background(), canvas.New(100, 100, table)); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(fd.calls) != 1 {
		t.Fatalf("下载次数 = %d, want 1", len(fd.calls))
	}
	expected := filepath.Join(root, "img_layers", cacheKey(t, rawURL))
	if got := img.ResolvedSrc(); got == nil || *got != expected {
		t.Errorf("内容层 ResolvedSrc = %v, want %s", got, expected)
	}
}

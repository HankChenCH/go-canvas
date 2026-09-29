package renderer_test

// 渲染产物泛型辅助 RenderAs[T]/RenderLayerAs[T](工票13,ADR-0010):
// 产物契约维持「类型由后端决定」,any 弱约束的收敛方式是调用方显式声明期望
// 产物类型,断言集中一处。复用 renderer_test.go 的假 Backend seam。

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/layer"
	"github.com/hankchen/go-canvas/renderer"
	"github.com/hankchen/go-canvas/resolver"
)

func TestRenderAsPassesThroughEndProductWhenTypeMatches(t *testing.T) {
	backend := &fakeBackend{product: "fake-product"}

	product, err := renderer.RenderAs[string](context.Background(), backend, canvas.New(320, 240))
	if err != nil {
		t.Fatalf("RenderAs: %v", err)
	}
	if product != "fake-product" {
		t.Errorf("T 值应原样透传 End 产物: got %v", product)
	}
	assertOps(t, backend.ops, []string{"begin", "end"})
	if backend.begins[0] != [2]int{320, 240} {
		t.Errorf("begin 应以画布尺寸建面: got %v", backend.begins[0])
	}
}

func TestRenderAsTypeMismatchErrorsWithReadableMessage(t *testing.T) {
	backend := &fakeBackend{product: 42}

	_, err := renderer.RenderAs[string](context.Background(), backend, canvas.New(100, 100))
	if err == nil {
		t.Fatal("期望类型与后端产物不符应报错")
	}
	// 消息三要素:后端类型、实际产物类型、期望类型
	for _, want := range []string{"*renderer_test.fakeBackend", "int", "string"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误消息应含 %q: got %q", want, err.Error())
		}
	}
	// 类型不符发生在渲染成功之后:渲染流程本身走完(begin/end 齐全)
	assertOps(t, backend.ops, []string{"begin", "end"})
}

func TestRenderLayerAsUsesLayerOwnSizeAsSurface(t *testing.T) {
	// 语义同 RenderLayer:以图层自身尺寸建面,不做显隐过滤
	l := layer.NewImageLayer(layer.WithSize(30, 20))
	backend := &fakeBackend{product: "fake-product"}

	product, err := renderer.RenderLayerAs[string](context.Background(), backend, l)
	if err != nil {
		t.Fatalf("RenderLayerAs: %v", err)
	}
	if product != "fake-product" {
		t.Errorf("T 值应原样透传 End 产物: got %v", product)
	}
	assertOps(t, backend.ops, []string{"begin", "rect", "end"})
	if backend.begins[0] != [2]int{30, 20} {
		t.Errorf("begin 应以图层自身尺寸建面: got %v", backend.begins[0])
	}
}

// ---- 后端自带默认物化器(New 的 nil-resolver 组装路径,工票13)----

// stubDownloader 桩下载器:返回固定字节,把物化断言与网络解耦
type stubDownloader struct{ content []byte }

func (d stubDownloader) Download(context.Context, string) ([]byte, error) { return d.content, nil }

// selfResolvingBackend 自带默认物化器的假后端:实现核心的可选发现面
type selfResolvingBackend struct {
	fakeBackend
	carried *resolver.ResourceResolver
}

func (b *selfResolvingBackend) DefaultResolver() *resolver.ResourceResolver { return b.carried }

func TestRenderAsPrefersBackendCarriedDefaultResolver(t *testing.T) {
	// RenderAs 以 New(backend, nil) 组装:resolver 为 nil 时应优先采用后端
	// 自带物化器(位图后端的二维码缝默认接线属后端 module 知识,核心默认
	// 物化器不可知)——远程图片图层经自带物化器完成下载物化
	root := t.TempDir()
	carried := resolver.New(
		resolver.WithDownloader(stubDownloader{content: []byte("png-bytes")}),
		resolver.WithCacheRoot(root),
	)
	backend := &selfResolvingBackend{
		fakeBackend: fakeBackend{product: "fake-product"},
		carried:     carried,
	}
	l := layer.NewImageLayer(layer.WithSize(10, 10), layer.WithImage("https://example.com/a.png"))

	product, err := renderer.RenderAs[string](context.Background(), backend, canvas.New(10, 10, l))
	if err != nil {
		t.Fatalf("RenderAs: %v", err)
	}
	if product != "fake-product" {
		t.Errorf("T 值应原样透传 End 产物: got %v", product)
	}
	if l.ResolvedSrc() == nil {
		t.Fatalf("后端自带默认物化器未被采用(远程图片未物化)")
	}
	if filepath.Dir(*l.ResolvedSrc()) != filepath.Join(root, "img_layers") {
		t.Fatalf("物化结果 %s 不在后端自带物化器的缓存目录下", *l.ResolvedSrc())
	}
}

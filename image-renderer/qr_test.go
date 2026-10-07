package imagerenderer

// 二维码物化与渲染:平移 php-canvas-image-renderer ImageRendererTest 的二维码用例
// (finder 角点暗像素),并锁定工单 08 验收面——PNG 魔数、标准库可解码为位图、
// 方形容器、纯黑/纯白不透明、margin=0(角点即墨)、编码模式与 endroid(bacon
// chooseMode)对齐、缓存协同(同内容+宽度跨图层只生成一次)与默认接线。

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/renderer"
	"github.com/HankChenCH/go-canvas/resolver"
)

const qrSampleText = "https://example.com/go-canvas"

// materializeSample 用固定样例内容生成一次产物
func materializeSample(t *testing.T, width int) image.Image {
	t.Helper()
	data, err := QRMaterializer{}.Materialize(context.Background(), qrSampleText, width)
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	return decodeMaterialized(t, data)
}

// decodeMaterialized 工单验收面:PNG 魔数 + 标准库可解码为位图
func decodeMaterialized(t *testing.T, data []byte) image.Image {
	t.Helper()
	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("产物缺少 PNG 魔数,前 8 字节 = %x", data[:min(8, len(data))])
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("标准库解码 PNG: %v", err)
	}
	return img
}

func TestQRMaterializeProducesSquareDecodablePNG(t *testing.T) {
	img := materializeSample(t, 200)

	b := img.Bounds()
	if b.Dx() != b.Dy() {
		t.Fatalf("产物 %dx%d, want 正方形", b.Dx(), b.Dy())
	}
	// 块宽量化(endroid None 模式为精确 size,yeqown 为整数块宽向下取整):
	// 产物不超目标宽,且不会小于目标宽的一半(量化至多损失一个块宽)
	if b.Dx() > 200 || b.Dx() < 100 {
		t.Fatalf("产物边长 %d, 应在 (100, 200] 区间(目标宽 200 的整数块宽量化)", b.Dx())
	}
}

func TestQRMaterializeZeroMarginPutsFinderAtOrigin(t *testing.T) {
	// margin=0(库默认 40px 静区须显式归零):finder 图案左上外圈落在画布原点,
	// 原点即墨;有静区时原点为白
	img := materializeSample(t, 200)
	assertPixel(t, img, 0, 0, black)
}

func TestQRMaterializePureBlackWhiteOpaque(t *testing.T) {
	// 纯黑前景/纯白背景、全程不透明(endroid 黑 (0,0,0) / 白 (255,255,255))
	img := materializeSample(t, 120)
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if got != black && got != white {
				t.Fatalf("像素 (%d, %d) = %+v, 只允许纯黑/纯白", x, y, got)
			}
		}
	}
}

func TestQRMaterializeDeterministic(t *testing.T) {
	first, err := QRMaterializer{}.Materialize(context.Background(), qrSampleText, 200)
	if err != nil {
		t.Fatalf("第一次 Materialize: %v", err)
	}
	second, err := QRMaterializer{}.Materialize(context.Background(), qrSampleText, 200)
	if err != nil {
		t.Fatalf("第二次 Materialize: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("同输入两次产物不一致")
	}
}

func TestQRMaterializeClampsNonPositiveWidth(t *testing.T) {
	// size=max(图层宽,1):宽 ≤0 钳到 1,产物退化为块宽 1 的最小可用二维码
	for _, width := range []int{0, -5} {
		data, err := QRMaterializer{}.Materialize(context.Background(), qrSampleText, width)
		if err != nil {
			t.Fatalf("Materialize(width=%d): %v", width, err)
		}
		img := decodeMaterialized(t, data)
		b := img.Bounds()
		if b.Dx() != b.Dy() {
			t.Fatalf("width=%d 产物 %dx%d, want 正方形", width, b.Dx(), b.Dy())
		}
	}
}

func TestPickEncModeMatchesBacon(t *testing.T) {
	// 复刻 bacon chooseMode(UTF-8 提示)的三分:纯数字→Numeric、QR 字母集→
	// Alphanumeric、其余→Byte;**永不 Kanji**——endroid 传 UTF-8 时 bacon 只在
	// SHIFT-JIS 提示下才走 Kanji,纯中文内容两端矩阵因此一致(EncModeAuto 会选
	// Kanji,故适配器显式选模式)
	tests := []struct {
		text string
		want baconEncMode
	}{
		{"1234567890", encModeNumeric},
		{"HELLO WORLD 123", encModeAlphaNum},
		{"ABC-123/:", encModeAlphaNum},
		{"https://example.com/canvas?x=1", encModeByte},
		{"你好,canvas", encModeByte},
		{"中文内容", encModeByte},
	}
	for _, tt := range tests {
		if got := pickEncMode(tt.text); got != tt.want {
			t.Errorf("pickEncMode(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

// countingQR 包装真实物化器计数调用次数(锁定缓存协同:同内容+宽度只生成一次)
type countingQR struct {
	calls int
	inner resolver.QRMaterializer
}

func (c *countingQR) Materialize(ctx context.Context, text string, width int) ([]byte, error) {
	c.calls++
	return c.inner.Materialize(ctx, text, width)
}

// newQrRenderer 组装带二维码默认接线的渲染器,缓存指向临时目录。
// 返回具体后端,经 renderer.RenderAs 消费(自带物化器被核心 New 自动发现)
func newQrRenderer(t *testing.T, qr resolver.QRMaterializer) *Renderer {
	t.Helper()
	opts := []resolver.Option{resolver.WithCacheRoot(t.TempDir())}
	if qr != nil {
		opts = append(opts, resolver.WithQRMaterializer(qr))
	}
	return NewRenderer(resolver.New(opts...))
}

// renderQrLayerProduct 单图层便捷渲染,经 NewDefaultResolver 默认接线(带二维码)
func renderQrLayerProduct(t *testing.T, l layer.Layer) *image.NRGBA {
	t.Helper()
	img, err := renderer.RenderLayerAs[*image.NRGBA](
		context.Background(), NewRenderer(NewDefaultResolver(resolver.WithCacheRoot(t.TempDir()))), l)
	if err != nil {
		t.Fatalf("RenderLayerAs: %v", err)
	}
	return img
}

func TestQrRendersFinderPatternDarkAtCorner(t *testing.T) {
	// 平移 PHP 同名用例:60×60 盒铺放二维码,角点 (0,0) 为墨
	// (margin=0 + 渲染原语按宽正方形缩放铺放)
	l := layer.NewQrCodeLayer(layer.WithSize(60, 60), layer.WithQrText("https://example.com"))
	img := renderQrLayerProduct(t, l)

	assertPixelNear(t, img, 0, 0, black, 60)
}

func TestQrLayerResolvesToCachedPNG(t *testing.T) {
	root := t.TempDir()
	l := layer.NewQrCodeLayer(layer.WithSize(60, 60), layer.WithQrText(qrSampleText))
	c := canvas.New(60, 60, l)

	if _, err := renderer.RenderAs[*image.NRGBA](
		context.Background(), NewRenderer(NewDefaultResolver(resolver.WithCacheRoot(root))), c); err != nil {
		t.Fatalf("RenderAs: %v", err)
	}

	if l.ResolvedSrc() == nil {
		t.Fatalf("二维码图层未回写物化结果")
	}
	data, err := os.ReadFile(*l.ResolvedSrc())
	if err != nil {
		t.Fatalf("读取物化结果 %s: %v", *l.ResolvedSrc(), err)
	}
	decodeMaterialized(t, data)
	if filepath.Dir(*l.ResolvedSrc()) != filepath.Join(root, "qr_layers") {
		t.Fatalf("物化结果 %s 不在二维码缓存目录下", *l.ResolvedSrc())
	}
}

func TestIdenticalQrLayersMaterializeOnce(t *testing.T) {
	// 同内容同宽度跨图层:第一层生成落缓存,第二层命中跳过——与核心缓存协同
	counter := &countingQR{inner: QRMaterializer{}}
	l1 := layer.NewQrCodeLayer(layer.WithSize(60, 60), layer.WithQrText(qrSampleText))
	l2 := layer.NewQrCodeLayer(layer.WithSize(60, 60), layer.WithQrText(qrSampleText), layer.WithPosition(0, 60))
	c := canvas.New(60, 120, l1, l2)

	r := newQrRenderer(t, counter)
	if _, err := renderer.RenderAs[*image.NRGBA](context.Background(), r, c); err != nil {
		t.Fatalf("RenderAs: %v", err)
	}

	if counter.calls != 1 {
		t.Fatalf("物化器调用 %d 次, want 1(第二层应命中缓存)", counter.calls)
	}
	if l1.ResolvedSrc() == nil || l2.ResolvedSrc() == nil {
		t.Fatalf("两层均应回写物化结果")
	}
	if *l1.ResolvedSrc() != *l2.ResolvedSrc() {
		t.Fatalf("同内容两层的物化结果不一致: %s != %s", *l1.ResolvedSrc(), *l2.ResolvedSrc())
	}
}

func TestNewRendererDefaultsToWiredQRMaterializer(t *testing.T) {
	// NewRenderer(nil) = PHP new ImageRenderer() 对应物:二维码缝默认接线,
	// 遇二维码图层不再报 ErrQRMaterializerRequired——自带物化器经核心 New 的
	// nil-resolver 组装路径发现(RenderAs 即完整管线)。缓存落系统用户缓存根
	// (默认根即设计行为,键确定性、幂等命中,与 PHP 测试共用真实缓存目录同处境)
	l := layer.NewQrCodeLayer(layer.WithSize(60, 60), layer.WithQrText(qrSampleText))
	c := canvas.New(60, 60, l)

	if _, err := renderer.RenderAs[*image.NRGBA](context.Background(), NewRenderer(nil), c); err != nil {
		t.Fatalf("默认接线渲染二维码图层: %v", err)
	}
	if l.ResolvedSrc() == nil {
		t.Fatalf("默认接线未回写物化结果")
	}
}

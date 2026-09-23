package imagerenderer

// 平移 php-canvas-image-renderer 的 ImageRendererTest 像素用例(文本/二维码用例
// 归工单 07/08):背景填充、cover 裁切、padding 对齐、表格行堆叠、priority 叠加
// 与 PNG 落盘。经公开渲染管线(模板 → 后端)驱动,验收即用户可见像素。

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/layer"
	"github.com/hankchen/go-canvas/renderer"
	"github.com/hankchen/go-canvas/resolver"
)

// renderLayerProduct 单图层便捷渲染,产物断言为位图
func renderLayerProduct(t *testing.T, l layer.Layer) *image.NRGBA {
	t.Helper()
	product, err := newRenderer(t).RenderLayer(context.Background(), l)
	if err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}
	return productAsImage(t, product)
}

// newRenderer 组装被测渲染器:缓存指向临时目录,本地路径零 I/O
func newRenderer(t *testing.T) renderer.Renderer {
	t.Helper()
	return renderer.New(New(), resolver.New(resolver.WithCacheRoot(t.TempDir())))
}

// productAsImage End 产物契约:位图后端固定 *image.NRGBA
func productAsImage(t *testing.T, product any) *image.NRGBA {
	t.Helper()
	img, ok := product.(*image.NRGBA)
	if !ok {
		t.Fatalf("产物类型 %T, want *image.NRGBA", product)
	}
	return img
}

func TestRenderLayerPaintsBackground(t *testing.T) {
	// 平移 PHP 同名用例
	l := layer.NewImageLayer(layer.WithSize(10, 10), layer.WithBackground("#f00"))
	img := renderLayerProduct(t, l)

	if img.Bounds().Dx() != 10 || img.Bounds().Dy() != 10 {
		t.Fatalf("渲染面 = %dx%d, want 10x10", img.Bounds().Dx(), img.Bounds().Dy())
	}
	assertPixel(t, img, 5, 5, red)
}

func TestRenderLayerTransparentWhenNoContent(t *testing.T) {
	// 平移 PHP testRenderLayerReturnsRawImageWhenNoContent:无内容即透明位图
	img := renderLayerProduct(t, layer.NewImageLayer(layer.WithSize(8, 8)))
	if img.Bounds().Dx() != 8 {
		t.Fatalf("渲染面宽 = %d, want 8", img.Bounds().Dx())
	}
	assertPixel(t, img, 4, 4, clear)
}

func TestImageIsCoverCroppedIntoContentBox(t *testing.T) {
	// 40×10 图片:左半红右半蓝;cover 裁切进 20×20 盒后露出中缝两侧(平移 PHP 同名用例)
	wide := twoColorPng(t, 40, 10, red, blue, 20)

	l := layer.NewImageLayer(layer.WithSize(20, 20), layer.WithBackground("#fff"), layer.WithImage(wide))
	img := renderLayerProduct(t, l)

	assertPixel(t, img, 5, 10, red)
	assertPixel(t, img, 15, 10, blue)
}

func TestAlignOffsetsImageByPadding(t *testing.T) {
	// 平移 PHP 同名用例:left/top 对齐 + 左 padding 10 → 内容从 x=10 起
	src := solidPng(t, 8, 8, red)

	l := layer.NewImageLayer(
		layer.WithSize(100, 100),
		layer.WithBackground("#fff"),
		layer.WithImage(src),
		layer.WithPaddingTRBL(0, 0, 0, 10),
		layer.WithHorizontalAlign(layer.AlignLeft),
		layer.WithVerticalAlign(layer.AlignTop),
	)
	img := renderLayerProduct(t, l)

	assertPixel(t, img, 10, 4, red)
	assertPixel(t, img, 9, 4, white)
}

func TestTableStacksRowsVertically(t *testing.T) {
	// 平移 PHP 同名用例:行按行高纵向堆叠,表背景在余量处可见
	table := layer.NewTableLayer(layer.WithSize(100, 50), layer.WithBackground("#fff"))
	row1 := layer.NewTableRowLayer(layer.WithCells(
		layer.NewTableCellLayer(layer.WithSize(100, 10), layer.WithBackground("#f00")),
	))
	row2 := layer.NewTableRowLayer(layer.WithCells(
		layer.NewTableCellLayer(layer.WithSize(100, 20), layer.WithBackground("#0f0")),
	))
	table.AddRow(row1)
	table.AddRow(row2)

	img := renderLayerProduct(t, table)

	assertPixel(t, img, 50, 5, red)
	assertPixel(t, img, 50, 15, green)
	assertPixel(t, img, 50, 45, white)
}

func TestCanvasRenderCompositesAndSave(t *testing.T) {
	// 平移 PHP 同名用例:priority 高者先画在下层——红色大块垫底,绿色小块在上;
	// 产物落盘为 PNG(魔数 + 可解码 + 尺寸)
	redLayer := layer.NewImageLayer(
		layer.WithSize(60, 30), layer.WithBackground("#f00"),
		layer.WithImage(solidPng(t, 60, 30, red)), layer.WithPriority(5),
	)
	greenLayer := layer.NewImageLayer(
		layer.WithSize(30, 30), layer.WithBackground("#0f0"),
		layer.WithImage(solidPng(t, 30, 30, green)), layer.WithPriority(1),
	)

	product, err := newRenderer(t).Render(context.Background(), canvas.New(60, 30, redLayer, greenLayer))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := productAsImage(t, product)

	assertPixel(t, img, 10, 10, green)
	assertPixel(t, img, 50, 25, red)

	path := filepath.Join(t.TempDir(), "canvas-next-go-test.png")
	if err := SavePNG(path, img); err != nil {
		t.Fatalf("SavePNG: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取落盘产物: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("落盘文件缺 PNG 魔数: % x", data[:8])
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("落盘产物不可解码: %v", err)
	}
	if decoded.Bounds().Dx() != 60 || decoded.Bounds().Dy() != 30 {
		t.Errorf("落盘产物尺寸 = %dx%d, want 60x30", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}

func TestPNGBytesRoundTrip(t *testing.T) {
	// 编码出口:PNGBytes 产物带魔数且可解码回同尺寸
	src := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	src.SetNRGBA(1, 1, blue)

	data, err := PNGBytes(src)
	if err != nil {
		t.Fatalf("PNGBytes: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("PNGBytes 产物缺魔数: % x", data[:8])
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("解码: %v", err)
	}
	if decoded.Bounds().Dx() != 4 || decoded.Bounds().Dy() != 2 {
		t.Errorf("尺寸 = %dx%d, want 4x2", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}

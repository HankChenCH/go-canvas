package imagerenderer

// 像素 seam 的测试工具:点位取样断言 + 测试图生成(对齐 PHP CanvasTestCase 的
// pixel/assertPixelSame 与 twoColorPng/solidPng;像素取样即验收面)

import (
	"image"
	"image/color"
	"image/draw"
	"path/filepath"
	"testing"
)

var (
	red   = color.NRGBA{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}
	green = color.NRGBA{R: 0x00, G: 0xFF, B: 0x00, A: 0xFF}
	blue  = color.NRGBA{R: 0x00, G: 0x00, B: 0xFF, A: 0xFF}
	white = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}

	// clear 透明色(新建渲染面的初始态)
	clear = color.NRGBA{}
)

func ptr(s string) *string { return &s }

// assertPixel 点位取样:经 NRGBA 模型取 8 位直 alpha 值做精确断言(不透明像素零舍入)
func assertPixel(t *testing.T, img image.Image, x, y int, want color.NRGBA) {
	t.Helper()
	got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	if got != want {
		t.Errorf("像素 (%d, %d) = %+v, want %+v", x, y, got, want)
	}
}

// assertPixelNear 有损编码(JPEG)下的容差取样
func assertPixelNear(t *testing.T, img image.Image, x, y int, want color.NRGBA, tol int) {
	t.Helper()
	got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	for _, ch := range []struct {
		name string
		g, w int
	}{
		{"R", int(got.R), int(want.R)},
		{"G", int(got.G), int(want.G)},
		{"B", int(got.B), int(want.B)},
	} {
		if d := ch.g - ch.w; d < -tol || d > tol {
			t.Errorf("像素 (%d, %d) %s = %d, 与 %d 相差超容差 %d", x, y, ch.name, ch.g, ch.w, tol)
		}
	}
}

// writePngFixture 把测试图落盘为 PNG,返回路径(模拟物化后的本地图片)
func writePngFixture(t *testing.T, img image.Image) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.png")
	if err := SavePNG(path, img); err != nil {
		t.Fatalf("写测试图: %v", err)
	}
	return path
}

// solidPng 纯色测试图
func solidPng(t *testing.T, width, height int, c color.NRGBA) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Over)
	return writePngFixture(t, img)
}

// twoColorPng 左半 one、右半 other 的双色测试图,splitX 为分界列(对齐 PHP 同名方法)
func twoColorPng(t *testing.T, width, height int, one, other color.NRGBA, splitX int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if x < splitX {
				img.SetNRGBA(x, y, one)
			} else {
				img.SetNRGBA(x, y, other)
			}
		}
	}
	return writePngFixture(t, img)
}

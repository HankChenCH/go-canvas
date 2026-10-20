package pdfrenderer

// poppler 点位断言的测试工具:渲染管线装配、poppler 四件套包装(pdfinfo/
// pdftoppm/pdftotext/pdffonts)、栅格化取样断言与图片/字体夹具。
// 无黄金文件:结构断言(pdfinfo)+ 实心区内部点取样(pdftoppm -r 96 恰好
// 1:1 还原像素网格)+ 文本提取(pdftotext)+ 字体内嵌标记(pdffonts)

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/renderer"
	"github.com/HankChenCH/go-canvas/resolver"
)

var (
	red       = color.NRGBA{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}
	green     = color.NRGBA{R: 0x00, G: 0xFF, B: 0x00, A: 0xFF}
	blue      = color.NRGBA{R: 0x00, G: 0x00, B: 0xFF, A: 0xFF}
	yellow    = color.NRGBA{R: 0xFF, G: 0xFF, B: 0x00, A: 0xFF}
	lightGray = color.NRGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF}
	steelBlue = color.NRGBA{R: 0x33, G: 0x66, B: 0xCC, A: 0xFF}
	white     = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	black     = color.NRGBA{A: 0xFF}
)

func ptr(s string) *string { return &s }

// hexRGB 共享色值 → 十六进制串(#rrggbb):画布背景/字色等 wire 面参数吃
// 十六进制串,接线时由 NRGBA 变量落串——测试色值的单一事实源仍是上面的
// NRGBA 变量,取样断言与画布接线同源不漂移
func hexRGB(c color.NRGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// renderPdfCanvas 经标准管线渲染整画布(NewRenderer 默认接线形态,产物断言为
// PDF 字节);rs 为 nil 时用临时缓存根的**默认接线**物化器(QR 缝在位,
// 本地路径零下载 I/O)
func renderPdfCanvas(t *testing.T, rs *resolver.ResourceResolver, c *canvas.Canvas) []byte {
	t.Helper()
	if rs == nil {
		rs = NewDefaultResolver(resolver.WithCacheRoot(t.TempDir()))
	}
	product, err := renderer.RenderAs[[]byte](context.Background(), NewRenderer(rs), c)
	if err != nil {
		t.Fatalf("渲染 PDF: %v", err)
	}
	if len(product) == 0 {
		t.Fatal("PDF 产物为空")
	}
	return product
}

// renderAsPDF / renderAsPDFCtx 指定物化器跑完整管线,返回产物与错误
// (失败/取消注入面用:错误由调用方 errors.Is 断言,产物断言无逃逸);
// rs 为 nil 时与 renderPdfCanvas 同一口径 = 临时缓存根的默认接线物化器
// (QR 缝在位)——nil 不另设「裸 resolver」语义,消踩坑面
func renderAsPDF(t *testing.T, rs *resolver.ResourceResolver, c *canvas.Canvas) ([]byte, error) {
	t.Helper()
	return renderAsPDFCtx(t, context.Background(), rs, c)
}

func renderAsPDFCtx(t *testing.T, ctx context.Context, rs *resolver.ResourceResolver, c *canvas.Canvas) ([]byte, error) {
	t.Helper()
	if rs == nil {
		rs = NewDefaultResolver(resolver.WithCacheRoot(t.TempDir()))
	}
	product, err := renderer.RenderAs[[]byte](ctx, NewRenderer(rs), c)
	return product, err
}

// renderPdfLayer 单图层便捷渲染,产物断言为 PDF 字节(对齐位图后端 renderLayerProduct)
func renderPdfLayer(t *testing.T, l layer.Layer) []byte {
	t.Helper()
	product, err := renderer.RenderLayerAs[[]byte](context.Background(),
		NewRenderer(resolver.New(resolver.WithCacheRoot(t.TempDir()))), l)
	if err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}
	if len(product) == 0 {
		t.Fatal("PDF 产物为空")
	}
	return product
}

// writePdf 落盘 PDF 字节,返回路径(poppler 四件套都只吃文件)
func writePdf(t *testing.T, dir, name string, pdf []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		t.Fatalf("写临时 PDF: %v", err)
	}
	return path
}

// runPoppler 跑 poppler 工具(pdfPath 为最后一个形参),返回 stdout
func runPoppler(t *testing.T, tool string, args ...string) string {
	t.Helper()
	cmd := exec.Command(tool, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s: %v\nstderr: %s", tool, err, stderr.String())
	}
	return stdout.String()
}

// assertPDFStructure pdfinfo 结构断言:页数与页尺寸(= px × 0.75 pt)
func assertPDFStructure(t *testing.T, pdf []byte, wantPages, width, height int) {
	t.Helper()
	info := runPoppler(t, "pdfinfo", writePdf(t, t.TempDir(), "probe.pdf", pdf))
	if got := extractInfoField(t, info, "Pages"); got != strconv.Itoa(wantPages) {
		t.Errorf("pdfinfo Pages = %q, want %d", got, wantPages)
	}
	want := fmt.Sprintf("%g x %g pts", float64(width)*pxToPt, float64(height)*pxToPt)
	if got := extractInfoField(t, info, "Page size"); got != want {
		t.Errorf("pdfinfo Page size = %q, want %q", got, want)
	}
}

// extractInfoField 从 pdfinfo 输出取「key: value」字段值
func extractInfoField(t *testing.T, info, key string) string {
	t.Helper()
	for _, line := range strings.Split(info, "\n") {
		if v, ok := strings.CutPrefix(line, key+":"); ok {
			return strings.TrimSpace(v)
		}
	}
	t.Fatalf("pdfinfo 输出缺字段 %s:\n%s", key, info)
	return ""
}

// rasterize pdftoppm -r 96 栅格化:pt × 96/72 = px,恰好 1:1 还原原像素网格;
// 返回栅格(实际尺寸与期望不符即 Fatal——采样坐标失配无意义)
func rasterize(t *testing.T, pdf []byte, width, height int) *image.NRGBA {
	t.Helper()
	dir := t.TempDir()
	base := filepath.Join(dir, "grid")
	runPoppler(t, "pdftoppm", "-r", "96", "-png", "-singlefile",
		writePdf(t, dir, "probe.pdf", pdf), base)

	data, err := os.ReadFile(base + ".png")
	if err != nil {
		t.Fatalf("读栅格化产物: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("解码栅格化 PNG: %v", err)
	}
	got := image.NewNRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	draw.Draw(got, got.Bounds(), img, img.Bounds().Min, draw.Src)
	if got.Bounds().Dx() != width || got.Bounds().Dy() != height {
		t.Fatalf("栅格化尺寸 = %dx%d, want %dx%d(-r 96 应 1:1 还原像素网格)",
			got.Bounds().Dx(), got.Bounds().Dy(), width, height)
	}
	return got
}

// samplePixel 栅格取样(NRGBA 直 alpha)
func samplePixel(img *image.NRGBA, x, y int) color.NRGBA {
	return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
}

// assertPixel 点位取样精确断言(不透明实心区内部点,零舍入)
func assertPixel(t *testing.T, img *image.NRGBA, x, y int, want color.NRGBA) {
	t.Helper()
	if got := samplePixel(img, x, y); got != want {
		t.Errorf("像素 (%d, %d) = %+v, want %+v", x, y, got, want)
	}
}

// assertPixelNear 矢量抗锯齿/有损编码下的容差取样(边缘与边界带用)
func assertPixelNear(t *testing.T, img *image.NRGBA, x, y int, want color.NRGBA, tol int) {
	t.Helper()
	got := samplePixel(img, x, y)
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
			return
		}
	}
}

// boxHasInk 区域内存在暗于阈值的像素(墨迹判定:矢量字形边缘抗锯齿,
// 只做「有/无墨」的粗判,不锁具体像素)
func boxHasInk(img *image.NRGBA, box image.Rectangle, threshold uint8) bool {
	for y := max(box.Min.Y, 0); y < min(box.Max.Y, img.Bounds().Dy()); y++ {
		for x := max(box.Min.X, 0); x < min(box.Max.X, img.Bounds().Dx()); x++ {
			c := samplePixel(img, x, y)
			if c.R < threshold || c.G < threshold || c.B < threshold {
				return true
			}
		}
	}
	return false
}

// assertBoxInk 区域墨迹断言:want=true 期望有墨,false 期望无墨(白底前提)
func assertBoxInk(t *testing.T, img *image.NRGBA, box image.Rectangle, threshold uint8, want bool, label string) {
	t.Helper()
	if got := boxHasInk(img, box, threshold); got != want {
		t.Errorf("%s: 区域 %v 墨迹 = %v, want %v", label, box, got, want)
	}
}

// ---- 字体探针:CJK 腿只认单体 TTF(fpdf 子集内嵌不支持 TTC,见工票 Answer)----

// probeCJKFont 取可用 CJK TTF 字体:环境变量优先,再探 CI 显式安装的
// fonts-ipafont-gothic 与 macOS 自带单体 CJK TTF。全缺即 Skip 并留指引
// (pdftotext 的结构断言在 poppler 用内嵌字体下不依赖系统字库,仍有效)
func probeCJKFont(t *testing.T) string {
	t.Helper()
	candidates := []string{}
	if v := os.Getenv("GO_CANVAS_TEST_CJK_FONT"); v != "" {
		candidates = append(candidates, v)
	}
	candidates = append(candidates,
		"/usr/share/fonts/truetype/fonts-ipafont-gothic/ipag.ttf",
		"/usr/share/fonts/opentype/ipafont-gothic/ipag.ttf",
		"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
	)
	for _, c := range candidates {
		data, err := os.ReadFile(c)
		if err != nil {
			continue
		}
		if bytes.HasPrefix(data, []byte("ttcf")) {
			continue // 字体集不认
		}
		return c
	}
	t.Skip("本机无可用 CJK 单体 TTF:CI 腿经 fonts-ipafont-gothic 提供;本地可设 GO_CANVAS_TEST_CJK_FONT=<单体 .ttf 路径>")
	return ""
}

// ---- 图片夹具(对齐 image-renderer helpers/exif 测试手法)----

// writePngFixture 测试图落盘为 PNG,返回路径(模拟物化后的本地图片)
func writePngFixture(t *testing.T, img image.Image) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("写测试图: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("编码 PNG: %v", err)
	}
	return path
}

// writeJpegFixture 测试图落盘为 JPEG
func writeJpegFixture(t *testing.T, img image.Image) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.jpg")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("写测试图: %v", err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("编码 JPEG: %v", err)
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

// writeRawFixture 原始字节夹具落盘(GIF 等无 stdlib 编码器的格式)
func writeRawFixture(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("写测试图 %s: %v", name, err)
	}
	return path
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

// tiffWithOrientation 组装仅含 orientation 标签的 TIFF 块(IFD0 单条目)
func tiffWithOrientation(bo binary.ByteOrder, typ uint16, orientation int) []byte {
	var b bytes.Buffer
	if bo == binary.LittleEndian {
		b.WriteString("II")
	} else {
		b.WriteString("MM")
	}
	_ = binary.Write(&b, bo, uint16(42))     // 魔数
	_ = binary.Write(&b, bo, uint32(8))      // IFD0 偏移紧随头部
	_ = binary.Write(&b, bo, uint16(1))      // 条目数
	_ = binary.Write(&b, bo, uint16(0x0112)) // Orientation
	_ = binary.Write(&b, bo, typ)
	_ = binary.Write(&b, bo, uint32(1)) // 计数
	if typ == 3 {                       // SHORT 内联于值字段前 2 字节
		_ = binary.Write(&b, bo, uint16(orientation))
		_ = binary.Write(&b, bo, uint16(0))
	} else { // LONG
		_ = binary.Write(&b, bo, uint32(orientation))
	}
	_ = binary.Write(&b, bo, uint32(0)) // 下一 IFD 偏移
	return b.Bytes()
}

// jpegWithExifOrientation 以 img 为主体拼装 SOI + APP1(Exif) + 主体熵编码的 JPEG
func jpegWithExifOrientation(t *testing.T, img image.Image, bo binary.ByteOrder, typ uint16, orientation int) []byte {
	t.Helper()
	var body bytes.Buffer
	if err := jpeg.Encode(&body, img, nil); err != nil {
		t.Fatalf("编码 JPEG 主体: %v", err)
	}
	payload := append([]byte("Exif\x00\x00"), tiffWithOrientation(bo, typ, orientation)...)
	segLen := len(payload) + 2 // 段长字段含自身 2 字节
	out := []byte{0xFF, 0xD8, 0xFF, 0xE1, byte(segLen >> 8), byte(segLen)}
	out = append(out, payload...)
	return append(out, body.Bytes()[2:]...) // 去掉主体自身的 SOI
}

// twoBandExifJpeg 8×16 上红下蓝、带指定 EXIF orientation 的 JPEG 落盘
func twoBandExifJpeg(t *testing.T, orientation int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 8; x++ {
			if y < 8 {
				img.SetNRGBA(x, y, red)
			} else {
				img.SetNRGBA(x, y, blue)
			}
		}
	}
	path := filepath.Join(t.TempDir(), "exif-rotated.jpg")
	if err := os.WriteFile(path,
		jpegWithExifOrientation(t, img, binary.LittleEndian, 3, orientation), 0o644); err != nil {
		t.Fatalf("写 EXIF 测试图: %v", err)
	}
	return path
}

// fakeDownloader Downloader 手工桩:err 模拟下载失败(对齐核心 resolver 测试同款)
type fakeDownloader struct {
	calls int
	err   error
}

func (d *fakeDownloader) Download(_ context.Context, _ string) ([]byte, error) {
	d.calls++
	if d.err != nil {
		return nil, d.err
	}
	return []byte("png-bytes"), nil
}

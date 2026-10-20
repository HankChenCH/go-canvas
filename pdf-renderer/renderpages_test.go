package pdfrenderer

// 多页入口测试(工票 03):RenderPagesAs[T] 泛型断言 + RenderPages 便捷具体入口,
// 消费 []*canvas.Canvas(paginate.CompileResult.Canvases 同构形态,帧序×页序)
// 出单 PDF。验收面 = poppler 断言、无黄金文件:pdfinfo 页数/逐页尺寸、
// pdftoppm -r 96 逐页栅格化按页采样背景色(页序)、pdftotext 按页提取标记文本;
// 另锁 End 后实例复用(必须得到干净新文档,工票 02 Begin 续页语义的边界)、
// 第 k 页失败无产物逃逸、ctx 页间掐断 sentinel、与 paginate.Paginator 切页联测。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/paginate"
	"github.com/HankChenCH/go-canvas/renderer"
	"github.com/HankChenCH/go-canvas/resolver"
)

// ---- 测试工具:标记页构造、多页渲染、逐页 poppler 断言 ----

// markerPage 标记页画布:指定尺寸 + 纯色背景 + 左上角默认字体标记文本
// (文本走内置 Helvetica,ASCII-only,不依赖 CJK 字体腿;墨迹在角落,
// 不干扰中心点背景采样);bg 用共享 NRGBA 色值(helpers_test 变量),
// 接线时经 hexRGB 落成背景十六进制串——与取样断言同一事实源
func markerPage(width, height int, bg color.NRGBA, marker string) *canvas.Canvas {
	c := canvas.New(width, height, layer.NewTextLayer(
		layer.WithSize(width, height), layer.WithPosition(0, 0),
		layer.WithText(marker), layer.WithFont("", 12, "#111111"),
		layer.WithHorizontalAlign(layer.AlignLeft), layer.WithVerticalAlign(layer.AlignTop),
	))
	c.SetBackground(canvas.NewCanvasBackground(ptr(hexRGB(bg)), nil))
	return c
}

// renderPdfPages 多页便捷入口渲染(本 module Renderer 直出 PDF 字节);
// rs 为 nil 时用临时缓存根的默认接线物化器(与 renderPdfCanvas 同口径)
func renderPdfPages(t *testing.T, rs *resolver.ResourceResolver, pages []*canvas.Canvas) []byte {
	t.Helper()
	if rs == nil {
		rs = NewDefaultResolver(resolver.WithCacheRoot(t.TempDir()))
	}
	pdf, err := RenderPages(context.Background(), NewRenderer(rs), pages)
	if err != nil {
		t.Fatalf("RenderPages: %v", err)
	}
	if len(pdf) == 0 {
		t.Fatal("PDF 产物为空")
	}
	return pdf
}

// assertPageSizes pdfinfo 逐页尺寸断言(-f 1 -l N 展开 per-page 行):
// 每页 MediaBox = 各页 px × 0.75 pt(逐页可变尺寸是 Begin 续页契约的一部分)
func assertPageSizes(t *testing.T, pdf []byte, wantPx [][2]int) {
	t.Helper()
	info := runPoppler(t, "pdfinfo", "-f", "1", "-l", strconv.Itoa(len(wantPx)),
		writePdf(t, t.TempDir(), "probe.pdf", pdf))

	got := map[int]string{}
	for _, line := range strings.Split(info, "\n") {
		f := strings.Fields(line)
		if len(f) == 7 && f[0] == "Page" && f[2] == "size:" {
			n, err := strconv.Atoi(f[1])
			if err != nil {
				t.Fatalf("pdfinfo 逐页行不可解析: %q (%v)", line, err)
			}
			got[n] = fmt.Sprintf("%s x %s pts", f[3], f[5])
		}
	}
	for i, wh := range wantPx {
		want := fmt.Sprintf("%g x %g pts", float64(wh[0])*pxToPt, float64(wh[1])*pxToPt)
		if got[i+1] != want {
			t.Errorf("第 %d 页尺寸 = %q, want %q", i+1, got[i+1], want)
		}
	}
}

// rasterizePage pdftoppm 逐页栅格化(-f k -l k -singlefile):pt × 96/72 = px,
// 恰好 1:1 还原该页像素网格(与 rasterize 同口径,多出页选段)
func rasterizePage(t *testing.T, pdf []byte, page, width, height int) *image.NRGBA {
	t.Helper()
	dir := t.TempDir()
	base := filepath.Join(dir, "grid")
	runPoppler(t, "pdftoppm", "-r", "96", "-png", "-singlefile",
		"-f", strconv.Itoa(page), "-l", strconv.Itoa(page),
		writePdf(t, dir, "probe.pdf", pdf), base)

	data, err := os.ReadFile(base + ".png")
	if err != nil {
		t.Fatalf("读栅格化产物: %v", err)
	}
	img := decodeRasterPng(t, data)
	if img.Bounds().Dx() != width || img.Bounds().Dy() != height {
		t.Fatalf("第 %d 页栅格化尺寸 = %dx%d, want %dx%d(-r 96 应 1:1 还原像素网格)",
			page, img.Bounds().Dx(), img.Bounds().Dy(), width, height)
	}
	return img
}

// decodeRasterPng 栅格化 PNG 解码为 NRGBA(与 rasterize 同一收尾)
func decodeRasterPng(t *testing.T, data []byte) *image.NRGBA {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("解码栅格化 PNG: %v", err)
	}
	got := image.NewNRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	draw.Draw(got, got.Bounds(), img, img.Bounds().Min, draw.Src)
	return got
}

// pageText pdftotext 按页提取(-f k -l k)
func pageText(t *testing.T, pdf []byte, page int) string {
	t.Helper()
	return runPoppler(t, "pdftotext", "-f", strconv.Itoa(page), "-l", strconv.Itoa(page),
		writePdf(t, t.TempDir(), "probe.pdf", pdf), "-")
}

// pdfInfoOf pdfinfo 原始输出(poppler 断言的结构面)
func pdfInfoOf(t *testing.T, pdf []byte) string {
	t.Helper()
	return runPoppler(t, "pdfinfo", writePdf(t, t.TempDir(), "probe.pdf", pdf))
}

// ---- 多页契约:页数 / 逐页尺寸 / 页序 ----

func TestRenderPagesMultiPageCountSizesAndOrder(t *testing.T) {
	// 三页三尺寸三背景色:页数 = 页序列长度;各页 MediaBox = 各页 px×0.75;
	// 页序 = Canvases 序(背景色按页采样 + 标记文本按页提取双重锚定)
	pages := []*canvas.Canvas{
		markerPage(320, 240, red, "PAGE-ONE"),
		markerPage(200, 100, green, "PAGE-TWO"),
		markerPage(100, 80, blue, "PAGE-THREE"),
	}
	pdf := renderPdfPages(t, nil, pages)

	if got := extractInfoField(t, pdfInfoOf(t, pdf), "Pages"); got != "3" {
		t.Errorf("pdfinfo Pages = %q, want 3", got)
	}
	assertPageSizes(t, pdf, [][2]int{{320, 240}, {200, 100}, {100, 80}})

	// 页序:逐页栅格化采中心点背景色(实心区内部点,零抗锯齿干扰)
	centers := []struct {
		w, h int
		want color.NRGBA
	}{
		{320, 240, red},
		{200, 100, green},
		{100, 80, blue},
	}
	for i, cc := range centers {
		img := rasterizePage(t, pdf, i+1, cc.w, cc.h)
		assertPixel(t, img, cc.w/2, cc.h/2, cc.want)
	}

	// 页序:标记文本按页提取(默认字体 ASCII,ToUnicode 无关也可提取)
	wantMarkers := []string{"PAGE-ONE", "PAGE-TWO", "PAGE-THREE"}
	for i, marker := range wantMarkers {
		if text := pageText(t, pdf, i+1); !strings.Contains(text, marker) {
			t.Errorf("第 %d 页 pdftotext 应含标记 %q, got %q", i+1, marker, text)
		}
	}
	// 标记不跨页:第 1 页不得出现后页标记(页序错排在此暴露)
	if text := pageText(t, pdf, 1); strings.Contains(text, "PAGE-TWO") || strings.Contains(text, "PAGE-THREE") {
		t.Errorf("第 1 页出现后页标记,页序错排: %q", text)
	}
}

// ---- 退化:单 canvas 多页入口 vs 工票 02 单页渲染,结构一致 ----

func TestRenderPagesSingleCanvasDegeneratesLikeSingleRender(t *testing.T) {
	c := markerPage(240, 160, steelBlue, "DEGENERATE-PAGE")

	multi := renderPdfPages(t, resolver.New(resolver.WithCacheRoot(t.TempDir())), []*canvas.Canvas{c})
	single := renderPdfCanvas(t, nil, c) // 工票 02 单页入口(renderer.RenderAs[[]byte])

	// 结构一致(pdfinfo 页数/页尺寸):字节面不比——创建时间/文档 ID 必然不同,
	// 语义契约是结构同构
	for _, key := range []string{"Pages", "Page size"} {
		m, s := extractInfoField(t, pdfInfoOf(t, multi), key), extractInfoField(t, pdfInfoOf(t, single), key)
		if m != s {
			t.Errorf("pdfinfo %s 不一致: 多页入口 %q vs 单页入口 %q", key, m, s)
		}
	}
	// 退化页内容同构:标记文本与背景色在两个入口下等价呈现
	if text := pageText(t, multi, 1); !strings.Contains(text, "DEGENERATE-PAGE") {
		t.Errorf("多页入口单页应含标记文本, got %q", text)
	}
	assertPixel(t, rasterizePage(t, multi, 1, 240, 160), 120, 80, steelBlue)
}

// ---- End 后复用:同一 Renderer 实例必须得到干净新文档 ----

func TestRendererReuseAfterEndStartsFreshDocument(t *testing.T) {
	// 工票 02 的 Begin 是「文档已在则续页」语义:End 后必须复位会话,
	// 否则第二次渲染把页面追加进旧文档(页数累加、尺寸串页)
	r := NewRenderer(resolver.New(resolver.WithCacheRoot(t.TempDir())))
	ctx := context.Background()

	// 第一次:两页两尺寸
	pdf1, err := RenderPages(ctx, r, []*canvas.Canvas{
		markerPage(100, 80, red, "DOC1-A"),
		markerPage(120, 90, green, "DOC1-B"),
	})
	if err != nil {
		t.Fatalf("第一次 RenderPages: %v", err)
	}
	if got := extractInfoField(t, pdfInfoOf(t, pdf1), "Pages"); got != "2" {
		t.Fatalf("第一次渲染 Pages = %q, want 2", got)
	}

	// 第二次(同一实例,多页入口):一页不同尺寸——复位失效时会是 3 页旧+新混排
	pdf2, err := RenderPages(ctx, r, []*canvas.Canvas{markerPage(200, 150, blue, "DOC2-A")})
	if err != nil {
		t.Fatalf("第二次 RenderPages: %v", err)
	}
	if got := extractInfoField(t, pdfInfoOf(t, pdf2), "Pages"); got != "1" {
		t.Errorf("End 后复用必须得到干净新文档: Pages = %q, want 1", got)
	}
	assertPageSizes(t, pdf2, [][2]int{{200, 150}})
	assertPixel(t, rasterizePage(t, pdf2, 1, 200, 150), 100, 75, blue)

	// 第三次(同一实例,单页入口):单页渲染同样受益于复位语义
	pdf3, err := renderer.RenderAs[[]byte](ctx, r, markerPage(80, 60, yellow, "DOC3-A"))
	if err != nil {
		t.Fatalf("第三次 RenderAs: %v", err)
	}
	if got := extractInfoField(t, pdfInfoOf(t, pdf3), "Pages"); got != "1" {
		t.Errorf("End 后单页入口复用: Pages = %q, want 1", got)
	}
	assertPageSizes(t, pdf3, [][2]int{{80, 60}})
}

func TestRendererReuseAfterEndReregistersFonts(t *testing.T) {
	// 复位语义的字体内面:字体注册表随文档会话走,End 后必须重新注册——
	// 复用族名跳过注册会让第二份文档 SetFont 爆 undefined font
	font := probeCJKFont(t)
	r := NewRenderer(resolver.New(resolver.WithCacheRoot(t.TempDir())))
	ctx := context.Background()

	buildPage := func(marker string) *canvas.Canvas {
		return canvas.New(160, 60, layer.NewTextLayer(
			layer.WithSize(160, 60), layer.WithPosition(0, 0),
			layer.WithText(marker), layer.WithFont(font, 14, "#111111"),
			layer.WithHorizontalAlign(layer.AlignLeft), layer.WithVerticalAlign(layer.AlignTop),
		))
	}
	first, err := RenderPages(ctx, r, []*canvas.Canvas{buildPage("第一次文档")})
	if err != nil {
		t.Fatalf("第一次 RenderPages: %v", err)
	}
	second, err := RenderPages(ctx, r, []*canvas.Canvas{buildPage("第二次文档")})
	if err != nil {
		t.Fatalf("End 后复用 RenderPages: %v", err)
	}
	for i, pdf := range [][]byte{first, second} {
		marker := []string{"第一次文档", "第二次文档"}[i]
		if text := pageText(t, pdf, 1); !strings.Contains(text, marker) {
			t.Errorf("第 %d 份文档 pdftotext 应提取 CJK 标记(字体重注册生效), got %q", i+1, text)
		}
	}
}

// ---- 失败/取消注入:任一页失败整文档丢弃 ----

func TestRenderPagesDownloadFailureOnLaterPageNoProduct(t *testing.T) {
	// 下载失败落在第 2 页(第 1 页无资源、渲染完好):整文档丢弃、无产物逃逸
	fd := &fakeDownloader{err: errors.New("boom")}
	rs := NewDefaultResolver(
		resolver.WithDownloader(fd),
		resolver.WithCacheRoot(t.TempDir()),
	)
	brokenLayer := layer.NewImageLayer(layer.WithSize(20, 20))
	brokenLayer.SetImage("https://example.com/broken.png")
	broken := canvas.New(60, 40, brokenLayer)

	product, err := RenderPagesAs[[]byte](context.Background(), NewRenderer(rs),
		[]*canvas.Canvas{markerPage(60, 40, lightGray, "OK-PAGE"), broken})
	if err == nil {
		t.Fatal("第 2 页下载失败必须报错")
	}
	if !errors.Is(err, resolver.ErrResourceDownloadFailed) {
		t.Errorf("错误 = %v, want ErrResourceDownloadFailed", err)
	}
	if len(product) != 0 {
		t.Errorf("失败渲染必须无产物逃逸, got %d 字节", len(product))
	}
	if fd.calls == 0 {
		t.Error("下载器未被调用")
	}
}

func TestRenderPagesCancelArmedAtInterPageCheckpoint(t *testing.T) {
	// 第 1 页零根层(页内检查点全部不触发):预取消 ctx 的首个命中等检查点
	// 恰为页间一处(下一页 Begin 前)——页间检查点语义在 PDF 后端的落点验证
	pages := []*canvas.Canvas{
		canvas.New(60, 40),
		markerPage(60, 40, lightGray, "NEVER-REACHED"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	product, err := RenderPagesAs[[]byte](ctx, NewRenderer(resolver.New(resolver.WithCacheRoot(t.TempDir()))), pages)
	if err == nil {
		t.Fatal("页间取消必须报错")
	}
	if !errors.Is(err, resolver.ErrRenderCancelled) {
		t.Errorf("错误 = %v, want ErrRenderCancelled", err)
	}
	if len(product) != 0 {
		t.Errorf("取消渲染必须无产物逃逸, got %d 字节", len(product))
	}
}

func TestRenderPagesDeadlineArmedAtInterPageCheckpoint(t *testing.T) {
	// 同上检查点,deadline 已到期 → ErrRenderDeadlineExceeded
	pages := []*canvas.Canvas{
		canvas.New(60, 40),
		markerPage(60, 40, lightGray, "NEVER-REACHED"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	time.Sleep(time.Millisecond) // 确保 deadline 已过期

	_, err := RenderPagesAs[[]byte](ctx, NewRenderer(resolver.New(resolver.WithCacheRoot(t.TempDir()))), pages)
	if !errors.Is(err, resolver.ErrRenderDeadlineExceeded) {
		t.Errorf("错误 = %v, want ErrRenderDeadlineExceeded", err)
	}
}

// ---- 与分页生产端联测:Paginator 切页 → 多页入口 → 页几何抽验 ----

func TestPaginatedCanvasesRenderAsMultiPagePdf(t *testing.T) {
	// 6 行 × 60px 驱动表,页高 200px:装箱切 2 页(3 行/页,180 ≤ 200 < 240),
	// 每页画布 200×200(页高即页几何,行序保序不切开)——页序列进多页入口,
	// pdfinfo 页数/逐页尺寸与按页行文本抽验
	rows := make([]*layer.TableRowLayer, 0, 6)
	for i := 1; i <= 6; i++ {
		rows = append(rows, layer.NewTableRowLayer(
			layer.WithSize(200, 60),
			layer.WithCells(layer.NewTableCellLayer(
				layer.WithSize(200, 60),
				layer.WithContent(layer.NewTextLayer(
					layer.WithSize(200, 60), layer.WithPosition(0, 0),
					layer.WithText(fmt.Sprintf("ROW-%d", i)),
					layer.WithFont("", 12, "#111111"),
					layer.WithHorizontalAlign(layer.AlignLeft), layer.WithVerticalAlign(layer.AlignTop),
				)),
			)),
		))
	}
	source := canvas.New(200, 400, layer.NewTableLayer(layer.WithSize(200, 360), layer.WithRows(rows...)))

	pageHeight := 200
	pages, err := paginate.NewPaginator(paginate.PaginateOptions{PageHeight: &pageHeight}).Paginate(source)
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("切页数 = %d, want 2", len(pages))
	}
	for i, p := range pages {
		if p.Width() != 200 || p.Height() != pageHeight {
			t.Errorf("第 %d 页画布几何 = %dx%d, want 200x%d", i+1, p.Width(), p.Height(), pageHeight)
		}
	}

	pdf := renderPdfPages(t, nil, pages)
	if got := extractInfoField(t, pdfInfoOf(t, pdf), "Pages"); got != "2" {
		t.Errorf("pdfinfo Pages = %q, want 2", got)
	}
	assertPageSizes(t, pdf, [][2]int{{200, 200}, {200, 200}})

	// 行文本按页分布:第 1 页 ROW-1..3、第 2 页 ROW-4..6(帧序×页序的消费语义)
	page1, page2 := pageText(t, pdf, 1), pageText(t, pdf, 2)
	for _, m := range []string{"ROW-1", "ROW-2", "ROW-3"} {
		if !strings.Contains(page1, m) {
			t.Errorf("第 1 页应含 %q, got %q", m, page1)
		}
	}
	for _, m := range []string{"ROW-4", "ROW-5", "ROW-6"} {
		if !strings.Contains(page2, m) {
			t.Errorf("第 2 页应含 %q, got %q", m, page2)
		}
	}
	if strings.Contains(page1, "ROW-4") {
		t.Errorf("第 1 页出现第 2 页行文本,装箱页序错排: %q", page1)
	}
}

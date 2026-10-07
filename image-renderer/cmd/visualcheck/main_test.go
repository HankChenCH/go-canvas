package main

// 目验样图的程序化守门(工单 11):候选列表与缺字体提示、priority 叠加与图层
// 编排的结构断言、渲染冒烟(渲染成功且无 panic、产物为合法 PNG)。字体属环境
// 依赖:无候选字体时冒烟跳过,与脚本运行时的明确报错互为补充。

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/HankChenCH/go-canvas/image-renderer"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/renderer"
	"github.com/HankChenCH/go-canvas/resolver"
)

// testFont 取本机候选字体;无候选时跳过依赖字体的用例
func testFont(t *testing.T) string {
	t.Helper()
	font, err := pickFont(fontCandidates)
	if err != nil {
		t.Skipf("环境无候选字体: %v", err)
	}
	return font
}

func TestFontCandidatesMatchPhpScript(t *testing.T) {
	// 回退顺序与 php-canvas-image-renderer/scripts/visual-check.php 逐项对齐,
	// CJK 字形优先(内置点阵仅 ASCII,中文必须真实字体)。钉子只对比本地副本:
	// 若 PHP 脚本候选列表变化,须人工同步两处
	want := []string{
		"/System/Library/Fonts/STHeiti Medium.ttc",
		"/System/Library/Fonts/Supplemental/Songti.ttc",
		"/System/Library/Fonts/Hiragino Sans GB.ttc",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
	}
	if !reflect.DeepEqual(fontCandidates, want) {
		t.Errorf("字体候选 = %v, want %v(与 PHP 脚本回退顺序一致)", fontCandidates, want)
	}
}

func TestPickFontAllMissingGivesClearHint(t *testing.T) {
	// 空文件存在但非合法字体:连同不存在的候选,报错逐条列明落选原因并给出提示
	stub := filepath.Join(t.TempDir(), "stub.ttf")
	if err := os.WriteFile(stub, nil, 0o644); err != nil {
		t.Fatalf("写候选字体桩: %v", err)
	}

	_, err := pickFont([]string{"/nonexistent/a.ttf", stub})
	if err == nil {
		t.Fatal("全缺时报错, got nil")
	}
	msg := err.Error()
	for _, fragment := range []string{"未找到可用字体", "/nonexistent/a.ttf", "(不存在)", stub, "(不可加载"} {
		if !strings.Contains(msg, fragment) {
			t.Errorf("报错未含 %q: %s", fragment, msg)
		}
	}
}

func TestPickFontMissingGivesClearHint(t *testing.T) {
	_, err := pickFont([]string{"/nonexistent/a.ttf", "/nonexistent/b.ttc"})
	if err == nil {
		t.Fatal("全缺时报错, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "未找到可用字体") {
		t.Errorf("报错未说明缺字体: %s", msg)
	}
	for _, p := range []string{"/nonexistent/a.ttf", "/nonexistent/b.ttc"} {
		if !strings.Contains(msg, p) {
			t.Errorf("报错未列出候选 %s: %s", p, msg)
		}
	}
}

func TestBuildSampleArrangesContent(t *testing.T) {
	c, _, err := buildSample(testFont(t))
	if err != nil {
		t.Fatalf("buildSample: %v", err)
	}

	if c.Width() != 400 || c.Height() != 520 { // V2 模板表样例区加高(工票 12)+ autoWidth 样例区(工单 02)
		t.Fatalf("画布 = %dx%d, want 400x520(V2 模板表 + autoWidth 样例区)", c.Width(), c.Height())
	}

	// priority 叠加:降序 [白底 11, 头图 10, 标题 5, 段落/表格/二维码/条带/页脚
	// /autoWidth 样例 4…],等优先级保持插入序
	layers := c.GetLayers()
	gotTypes := make([]string, 0, len(layers))
	gotPriorities := make([]int, 0, len(layers))
	for _, l := range layers {
		gotTypes = append(gotTypes, l.TypeName())
		gotPriorities = append(gotPriorities, l.Priority())
	}
	// V2 段(工票 12):bgV2 垫底 ImageLayer + 模板表 TableLayer;
	// autoWidth 段(工单 02):末尾两个宽自适应文本层
	wantTypes := []string{
		layer.TypeImage, layer.TypeImage, layer.TypeImage, layer.TypeText, layer.TypeText,
		layer.TypeTable, layer.TypeQrCode, layer.TypeImage, layer.TypeText,
		layer.TypeTable, layer.TypeText, layer.TypeText,
	}
	wantPriorities := []int{11, 11, 10, 5, 4, 4, 4, 4, 4, 4, 4, 4}
	if !reflect.DeepEqual(gotTypes, wantTypes) {
		t.Errorf("图层类型序 = %v, want %v", gotTypes, wantTypes)
	}
	if !reflect.DeepEqual(gotPriorities, wantPriorities) {
		t.Errorf("priority 序 = %v, want %v", gotPriorities, wantPriorities)
	}

	// 嵌套 auto 高度链:行高取最高单元格(内容行高+padding),表高 = 行高累计。
	// 取第一个 TableLayer(版式表;末位为 V2 模板表样例)
	var table *layer.TableLayer
	for _, l := range layers {
		if tl, ok := l.(*layer.TableLayer); ok {
			table = tl
			break
		}
	}
	if table == nil {
		t.Fatal("样图缺少表格图层")
	}
	if got := table.Height(); got != 72 {
		t.Errorf("表高 = %d, want 72(3 行 × 24)", got)
	}
}

func TestRenderSampleSmoke(t *testing.T) {
	c, _, err := buildSample(testFont(t))
	if err != nil {
		t.Fatalf("buildSample: %v", err)
	}

	// 缓存指向临时目录:冒烟不污染用户缓存;QR 物化缝经 NewDefaultResolver 接线
	rs := imagerenderer.NewDefaultResolver(resolver.WithCacheRoot(t.TempDir()))
	product, err := renderer.New(imagerenderer.New(), rs).Render(context.Background(), c)
	if err != nil {
		t.Fatalf("渲染样图: %v", err)
	}
	img, ok := product.(*image.NRGBA)
	if !ok {
		t.Fatalf("产物类型 %T, want *image.NRGBA", product)
	}
	if img.Bounds().Dx() != 400 || img.Bounds().Dy() != 520 { // V2 样例区 + autoWidth 样例区
		t.Fatalf("产物 = %dx%d, want 400x520(V2 模板表 + autoWidth 样例区)", img.Bounds().Dx(), img.Bounds().Dy())
	}

	// 落盘产物:PNG 魔数 + 可解码
	out := filepath.Join(t.TempDir(), "visual-check.png")
	if err := imagerenderer.SavePNG(out, img); err != nil {
		t.Fatalf("落盘: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("读产物: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("产物非 PNG 魔数: % x", data[:8])
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("产物不可解码: %v", err)
	}
	if decoded.Bounds().Dx() != 400 || decoded.Bounds().Dy() != 520 { // V2 样例区 + autoWidth 样例区
		t.Fatalf("解码产物 = %dx%d, want 400x520(V2 模板表 + autoWidth 样例区)", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}

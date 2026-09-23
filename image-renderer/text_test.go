package imagerenderer

// 文本绘制原语的点位取样测试:字体加载(TTF/OTF/TTC)、空字体=内置默认字体、
// 对齐锚点(真实宽度水平对齐 + metrics 垂直基线)、角度旋转。经公开渲染管线
// (模板 → 后端)的用例平移 PHP ImageRendererTest 的文字用例

import (
	"image"
	"os"
	"testing"

	"github.com/hankchen/go-canvas/layer"
)

// systemFont 按扩展名给出本机候选字体文件(跨平台测试用),找不到返回空串。
// 等价 PHP CanvasTestCase::systemTtf:无可用字体文件的宿主上文字用例跳过
func systemFont(ext string) string {
	var candidates []string
	switch ext {
	case ".ttf":
		candidates = []string{
			// Linux
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
			// macOS
			"/System/Library/Fonts/Supplemental/Arial.ttf",
			"/System/Library/Fonts/Supplemental/Times New Roman.ttf",
		}
	case ".otf":
		candidates = []string{
			"/System/Library/Fonts/Supplemental/NotoSansCanadianAboriginal-Regular.otf",
		}
	case ".ttc":
		candidates = []string{
			// macOS(Apple AAT 字体的 kern 表 sfnt 读不了,须选可解析的候选)
			"/System/Library/Fonts/Supplemental/AmericanTypewriter.ttc",
		}
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// inkBox 文字墨迹包围盒:透明渲染面上非透明像素的扫描结果;无墨迹时 ok=false
func inkBox(img image.Image) (minX, minY, maxX, maxY int, ok bool) {
	b := img.Bounds()
	minX, minY = b.Max.X, b.Max.Y
	maxX, maxY = b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				ok = true
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	return minX, minY, maxX, maxY, ok
}

// countDarkPixels 平移 PHP countDarkPixels:前景 R<128 的像素计数
func countDarkPixels(img image.Image) int {
	dark := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.At(x, y)
			r, _, _, _ := c.RGBA()
			if r < 128 {
				dark++
			}
		}
	}
	return dark
}

// drawTextOn 建面 width×height 的透明面并绘制一行,返回产物位图
func drawTextOn(t *testing.T, width, height int, invoke func(r *Renderer) error) *image.NRGBA {
	t.Helper()
	r := New()
	if err := r.Begin(width, height); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := invoke(r); err != nil {
		t.Fatalf("DrawText: %v", err)
	}
	return r.End().(*image.NRGBA)
}

func TestDrawTextEmptyLineSkipsInk(t *testing.T) {
	// 对齐 PHP `$line === ''` 守卫:空行零像素副作用、不报错
	img := drawTextOn(t, 20, 20, func(r *Renderer) error {
		return r.DrawText("", 10, 10, "", 12, "#000", "left", "top", 0)
	})
	if _, _, _, _, ok := inkBox(img); ok {
		t.Fatal("空行不应产生墨迹")
	}
}

func TestDrawTextDefaultFontInkExists(t *testing.T) {
	// 空字体 = 内置默认字体(Go 无 GD 内置字体,取 x/image basicfont 兜底)
	for _, fontFile := range []string{"", "1", "42", "1.5"} {
		img := drawTextOn(t, 40, 20, func(r *Renderer) error {
			return r.DrawText("abc", 5, 15, fontFile, 12, "#000", "left", "top", 0)
		})
		if _, _, _, _, ok := inkBox(img); !ok {
			t.Errorf("fontFile=%q 内置默认字体应产生墨迹", fontFile)
		}
	}
}

func TestDrawTextHorizontalAnchorsWithDefaultFont(t *testing.T) {
	// 水平对齐锚点:(x,y) 是对齐语义锚点——左对齐墨迹从 x 起笔、右对齐墨迹收于 x、
	// 居中墨迹关于 x 对称;宽度来自真实度量(默认字体 advance 恒 7,"WWW" 宽 21)
	const fontFile, y = "", 5

	left := drawTextOn(t, 60, 20, func(r *Renderer) error {
		return r.DrawText("WWW", 20, y, fontFile, 12, "#000", "left", "top", 0)
	})
	minX, _, _, _, ok := inkBox(left)
	if !ok {
		t.Fatal("left 无墨迹")
	}
	if minX < 19 || minX > 22 {
		t.Errorf("left 首列墨迹 x=%d, want ∈[19,22]", minX)
	}

	right := drawTextOn(t, 60, 20, func(r *Renderer) error {
		return r.DrawText("WWW", 40, y, fontFile, 12, "#000", "right", "top", 0)
	})
	_, _, maxX, _, ok := inkBox(right)
	if !ok {
		t.Fatal("right 无墨迹")
	}
	if maxX < 38 || maxX > 41 {
		t.Errorf("right 末列墨迹 x=%d, want ∈[38,41]", maxX)
	}

	center := drawTextOn(t, 60, 20, func(r *Renderer) error {
		return r.DrawText("WWW", 30, y, fontFile, 12, "#000", "center", "top", 0)
	})
	cminX, _, cmaxX, _, ok := inkBox(center)
	if !ok {
		t.Fatal("center 无墨迹")
	}
	if lgap, rgap := 30-cminX, cmaxX-30; lgap-rgap > 2 || rgap-lgap > 2 {
		t.Errorf("center 墨迹 [%d,%d] 应关于 x=30 对称(左距 %d 右距 %d)", cminX, cmaxX, lgap, rgap)
	}
}

func TestDrawTextVerticalAnchorsFromFontMetrics(t *testing.T) {
	// 垂直对齐用字体 metrics(ascent/descent)落基线,锚点是行盒对齐语义位置:
	// top 锚点在行盒顶(墨迹全部位于锚点下方),bottom 在行盒底(基线按 descent
	// 抬起,墨迹收于锚点上方),center 跨锚点两侧;同锚点下 top 与 bottom 的
	// 墨迹整体错开一个 metrics 线高(断言均为相对关系,不依赖具体字体的 cap 高)
	const fontFile = ""
	inkSpanY := func(vAlign string, y int) (int, int) {
		img := drawTextOn(t, 40, 40, func(r *Renderer) error {
			return r.DrawText("ABC", 10, y, fontFile, 12, "#000", "left", vAlign, 0)
		})
		_, minY, _, maxY, ok := inkBox(img)
		if !ok {
			t.Fatalf("vAlign=%s 无墨迹", vAlign)
		}
		return minY, maxY
	}

	const y = 20
	tminY, tmaxY := inkSpanY("top", y)
	bminY, bmaxY := inkSpanY("bottom", y)
	if tminY < y-1 {
		t.Errorf("top 首行墨迹 y=%d, 不得高于锚点上方 1px", tminY)
	}
	if bmaxY > y+1 {
		t.Errorf("bottom 末行墨迹 y=%d, 应收于锚点 y=%d 上方(基线按 descent 抬起)", bmaxY, y)
	}
	if tminY <= bminY || tmaxY <= bmaxY {
		t.Errorf("top 墨迹 [%d,%d] 应整体低于 bottom 墨迹 [%d,%d] 一个 metrics 线高", tminY, tmaxY, bminY, bmaxY)
	}

	cminY, cmaxY := inkSpanY("center", y)
	if cminY >= y || cmaxY <= y {
		t.Errorf("center 墨迹 [%d,%d] 应跨越锚点 y=%d 两侧", cminY, cmaxY, y)
	}
}

func TestDrawTextUnknownAlignFallsBackLeftTop(t *testing.T) {
	// 未知对齐取值归 left/top(PHP match default 臂同语义)
	fallback := drawTextOn(t, 60, 30, func(r *Renderer) error {
		return r.DrawText("ABC", 10, 5, "", 12, "#000", "diagonal", "middle", 0)
	})
	leftTop := drawTextOn(t, 60, 30, func(r *Renderer) error {
		return r.DrawText("ABC", 10, 5, "", 12, "#000", "left", "top", 0)
	})
	fx, fy, _, _, ok := inkBox(fallback)
	if !ok {
		t.Fatal("fallback 无墨迹")
	}
	lx, ly, _, _, ok := inkBox(leftTop)
	if !ok {
		t.Fatal("leftTop 无墨迹")
	}
	if fx != lx || fy != ly {
		t.Errorf("未知取值应与 left/top 起墨一致: got (%d,%d), want (%d,%d)", fx, fy, lx, ly)
	}
}

func TestDrawTextAngleRotatesAboutAnchor(t *testing.T) {
	// 角度 90(逆时针):水平行文绕锚点转为纵向——横向跨距收缩为字面高度,
	// 纵向跨距膨胀为文本宽度,墨迹向上越过锚点;与 angle=0 同锚点布局对照
	// (断言为旋转前后跨距的相对关系,不依赖对齐偏移的具体数值)
	boxOf := func(angle int) (minX, minY, maxX, maxY int) {
		img := drawTextOn(t, 80, 80, func(r *Renderer) error {
			return r.DrawText("WWW", 40, 40, "", 12, "#000", "left", "center", angle)
		})
		minX, minY, maxX, maxY, ok := inkBox(img)
		if !ok {
			t.Fatalf("angle=%d 无墨迹", angle)
		}
		return minX, minY, maxX, maxY
	}

	ux0, uy0, ux1, uy1 := boxOf(0)
	rx0, ry0, rx1, ry1 := boxOf(90)

	if spanX := rx1 - rx0; spanX >= ux1-ux0 {
		t.Errorf("旋转后横向跨距 %d, 应小于未旋转的 %d", spanX, ux1-ux0)
	}
	if spanY := ry1 - ry0; spanY <= uy1-uy0 {
		t.Errorf("旋转后纵向跨距 %d, 应大于未旋转的 %d", spanY, uy1-uy0)
	}
	if ry0 >= 40 {
		t.Errorf("逆时针旋转后墨迹顶 y=%d, 应向上越过锚点 y=40", ry0)
	}
}

func TestDrawTextFontFormatsLoad(t *testing.T) {
	// TTF/OTF/TTC 三格式经 opentype Parse/ParseCollection;宿主缺某格式则跳过该格式
	for _, ext := range []string{".ttf", ".otf", ".ttc"} {
		fontFile := systemFont(ext)
		if fontFile == "" {
			t.Logf("宿主无 %s 字体,跳过该格式", ext)
			continue
		}
		img := drawTextOn(t, 80, 30, func(r *Renderer) error {
			return r.DrawText("AFFE", 5, 20, fontFile, 16, "#000", "left", "top", 0)
		})
		if _, _, _, _, ok := inkBox(img); !ok {
			t.Errorf("%s (%s) 应产生墨迹", ext, fontFile)
		}
	}
}

func TestDrawTextRealFontAnchorsAndMetrics(t *testing.T) {
	// 真实字体下的锚点语义:右对齐收于锚点、居中对称、bottom 基线按 descent 抬起
	fontFile := systemFont(".ttf")
	if fontFile == "" {
		t.Skip("无可用系统 TTF,等价 Imagick 无字体跳用例")
	}

	right := drawTextOn(t, 80, 30, func(r *Renderer) error {
		return r.DrawText("WALT", 60, 5, fontFile, 14, "#000", "right", "top", 0)
	})
	_, _, maxX, _, ok := inkBox(right)
	if !ok {
		t.Fatal("right 无墨迹")
	}
	if maxX > 61 || maxX < 55 {
		t.Errorf("right 末列墨迹 x=%d, 应收于锚点 x=60 附近", maxX)
	}

	bottom := drawTextOn(t, 80, 40, func(r *Renderer) error {
		return r.DrawText("WALT", 5, 35, fontFile, 14, "#000", "left", "bottom", 0)
	})
	_, minY, _, maxY, ok := inkBox(bottom)
	if !ok {
		t.Fatal("bottom 无墨迹")
	}
	if maxY >= 35 {
		t.Errorf("bottom 末行墨迹 y=%d, 应收于锚点 y=35 上方", maxY)
	}
	if minY <= 5 {
		t.Errorf("bottom 首行墨迹 y=%d, 应位于锚点上方区段", minY)
	}
}

func TestDrawTextMissingFontFileAborts(t *testing.T) {
	// 非空非纯数字但不可读:报错中止(与 drawImage 读取失败同路径)
	r := New()
	if err := r.Begin(20, 20); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := r.DrawText("x", 5, 5, "/nonexistent/go-canvas-font.ttf", 12, "#000", "left", "top", 0); err == nil {
		t.Fatal("字体文件不可读应报错中止")
	}
}

func TestDrawTextInvalidFontBytesAborts(t *testing.T) {
	// 可读但非字体内容(拿 PNG 冒充 TTF):解析报错中止
	r := New()
	if err := r.Begin(20, 20); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := r.DrawText("x", 5, 5, solidPng(t, 2, 2, red), 12, "#000", "left", "top", 0); err == nil {
		t.Fatal("非字体内容应解析报错中止")
	}
}

func TestDrawTextReusesSessionFacesAcrossRenders(t *testing.T) {
	// Face 按渲染会话(Renderer 实例)持有:同字体多次绘制与 Begin 重建渲染面后
	// 复用均可用(Renderer 不并发安全,见类型文档)
	r := New()
	if err := r.Begin(60, 20); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for i := range 3 {
		if err := r.DrawText("abc", 5+i, 15, "1", 12, "#000", "left", "top", 0); err != nil {
			t.Fatalf("第 %d 次绘制: %v", i+1, err)
		}
	}
	img := r.End().(*image.NRGBA)
	if _, _, _, _, ok := inkBox(img); !ok {
		t.Fatal("会话内多次绘制应有墨迹")
	}

	if err := r.Begin(60, 20); err != nil {
		t.Fatalf("重建渲染面: %v", err)
	}
	if err := r.DrawText("abc", 5, 15, "1", 12, "#000", "left", "top", 0); err != nil {
		t.Fatalf("重建渲染面后绘制: %v", err)
	}
	if _, _, _, _, ok := inkBox(r.End().(*image.NRGBA)); !ok {
		t.Fatal("重建渲染面后应有墨迹")
	}
}

func TestSessionFaceCachesPerFontAndSize(t *testing.T) {
	// 缓存语义:真实字体按(路径, 字号)为键缓存——同键命中同一 Face 实例,
	// 异字号各占一格;纯数字 id 走内置默认单例,不占缓存
	fontFile := systemFont(".ttf")
	if fontFile == "" {
		t.Skip("无可用系统 TTF,等价 Imagick 无字体跳用例")
	}

	r := New()
	if err := r.Begin(80, 20); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	draw := func(size int) {
		t.Helper()
		if err := r.DrawText("ABC", 2, 15, fontFile, size, "#000", "left", "top", 0); err != nil {
			t.Fatalf("DrawText(size=%d): %v", size, err)
		}
	}

	draw(12)
	if len(r.faces) != 1 {
		t.Fatalf("同字体同字号一次绘制后缓存 = %d 格, want 1", len(r.faces))
	}
	first := r.faces[fontKey{fontFile, 12}]
	draw(12)
	if got := r.faces[fontKey{fontFile, 12}]; got != first {
		t.Fatal("同键二次取用应命中同一 Face 实例")
	}
	draw(16)
	if len(r.faces) != 2 {
		t.Fatalf("同字体异字号后缓存 = %d 格, want 2", len(r.faces))
	}

	r.DrawText("abc", 2, 15, "1", 12, "#000", "left", "top", 0)
	if len(r.faces) != 2 {
		t.Errorf("纯数字 id 走内置默认单例,不应占缓存:缓存 = %d 格, want 2", len(r.faces))
	}
}

// --- 渲染级用例(模板 → 后端,平移 PHP ImageRendererTest 文字用例) ---

func TestTextRenderedDarkPixelsWithSystemFont(t *testing.T) {
	// 平移 PHP 同名用例:白底黑字,断言"文本区域存在暗像素"
	ttf := systemFont(".ttf")
	if ttf == "" {
		t.Skip("无可用系统字体文件,等价 Imagick 无字体跳用例")
	}

	l := layer.NewTextLayer(
		layer.WithSize(100, 30), layer.WithBackground("#fff"),
		layer.WithText("ABC测试"), layer.WithFont(ttf, 12, "#000"),
	)
	img := renderLayerProduct(t, l)
	if dark := countDarkPixels(img); dark == 0 {
		t.Fatal("文本区域应存在暗像素")
	}
}

func TestNumericFontIdFallsBackToDefaultFont(t *testing.T) {
	// 平移 PHP 同名用例:旧库纯数字 GD 字体编号按"无字体文件"处理,
	// 走渲染端内置默认字体(Go 恒有内置默认,无需 PHP 的驱动区分跳过)
	l := layer.NewTextLayer(
		layer.WithSize(100, 30), layer.WithBackground("#fff"),
		layer.WithText("fallback"), layer.WithFont("1", 12, "#000"),
	)
	img := renderLayerProduct(t, l)
	if dark := countDarkPixels(img); dark == 0 {
		t.Fatal("纯数字字体 id 应回退内置默认字体渲染出暗像素")
	}
}

func TestRenderTextStacksWrappedLinesByLineHeight(t *testing.T) {
	// 承接工单 02:断行仍走图层启发式度量(绘制宽度只影响绘内对齐,不改断行),
	// 模板按行高像素逐行落笔——墨迹行带数 = 断行行数,行带起点间距 = LineHeightPx
	l := layer.NewTextLayer(
		layer.WithSize(100, 120), layer.WithBackground("#fff"),
		layer.WithText("AAAAAAAAAA BBBBBBBBBB"), layer.WithFont("1", 12, "#000"),
		layer.WithAutowrap(true),
	)
	lines := l.Lines()
	if len(lines) != 2 {
		t.Fatalf("启发式断行应得 2 行, got %d 行 %q", len(lines), lines)
	}

	img := renderLayerProduct(t, l)

	// 逐行扫描墨迹行带(白底上含暗像素的行构成的连续区段)
	var bands [][2]int // [起,止] 闭区间
	start := -1
	for y := range img.Bounds().Dy() {
		rowHasInk := false
		for x := range img.Bounds().Dx() {
			if r, _, _, _ := img.At(x, y).RGBA(); r < 128 {
				rowHasInk = true
				break
			}
		}
		switch {
		case rowHasInk && start < 0:
			start = y
		case !rowHasInk && start >= 0:
			bands = append(bands, [2]int{start, y - 1})
			start = -1
		}
	}
	if start >= 0 {
		bands = append(bands, [2]int{start, img.Bounds().Dy() - 1})
	}

	if len(bands) != 2 {
		t.Fatalf("墨迹行带 = %v, want 2 条", bands)
	}
	if gap := bands[1][0] - bands[0][0]; gap != l.LineHeightPx() {
		t.Errorf("行带起点间距 = %d, want LineHeightPx %d", gap, l.LineHeightPx())
	}
}

package pdfrenderer

// 文本原语测试:空行守卫、Helvetica 兜底、对齐锚点(实测宽度水平对齐 +
// metrics 垂直基线)、角度旋转、TTF 子集内嵌(CJK)与失败防线。
// 墨迹断言为相对关系(锚点方位/跨距变化),不依赖具体字体度量数值——
// 手法对齐 image-renderer text_test.go 的 inkBox 断言

import (
	"image"
	"strings"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
)

// inkBox 非白墨迹包围盒(pdftoppm 无透明:透明页面栅格化为白底,
// 墨迹 = 任一通道暗于阈值的像素);无墨迹时 ok=false
func inkBox(img *image.NRGBA) (minX, minY, maxX, maxY int, ok bool) {
	b := img.Bounds()
	minX, minY = b.Max.X, b.Max.Y
	maxX, maxY = b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := samplePixel(img, x, y)
			if c.R < 128 || c.G < 128 || c.B < 128 {
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

// drawTextOn 建面 width×height 并绘制一行,栅格化返回白底位图
func drawTextOn(t *testing.T, width, height int, invoke func(r *Renderer) error) *image.NRGBA {
	t.Helper()
	r := New()
	if err := r.Begin(width, height); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := invoke(r); err != nil {
		t.Fatalf("DrawText: %v", err)
	}
	product := r.End()
	pdf, ok := product.([]byte)
	if !ok || len(pdf) == 0 {
		t.Fatalf("产物 = %T, want []byte", product)
	}
	return rasterize(t, pdf, width, height)
}

func TestDrawTextEmptyLineNoOp(t *testing.T) {
	// 空行零副作用(PHP 同款空串守卫):不报错、无墨迹
	img := drawTextOn(t, 20, 20, func(r *Renderer) error {
		return r.DrawText("", 10, 10, "", 12, "#000", "left", "top", 0)
	})
	if _, _, _, _, ok := inkBox(img); ok {
		t.Fatal("空行不应产生墨迹")
	}
}

func TestDrawTextHelveticaFallbackInkExists(t *testing.T) {
	// 空串/纯数字字体 = PDF 核心字体 Helvetica 兜底(ASCII-only 语义,
	// 对齐位图后端 basicfont:ASCII 可渲)
	for _, fontFile := range []string{"", "1", "42", "1.5"} {
		img := drawTextOn(t, 40, 20, func(r *Renderer) error {
			return r.DrawText("abc", 5, 15, fontFile, 12, "#000", "left", "top", 0)
		})
		if _, _, _, _, ok := inkBox(img); !ok {
			t.Errorf("fontFile=%q Helvetica 兜底应产生墨迹", fontFile)
		}
	}
}

func TestDrawTextHorizontalAnchors(t *testing.T) {
	// 水平对齐锚点:左对齐墨迹从 x 起笔、右对齐墨迹收于 x、居中关于 x 对称;
	// 宽度来自 fpdf 实测(GetStringWidth,与落笔同一度量路径)
	// (x,y)=(20,5);Helvetica 12px "WWW" 宽 ≈ 28px,墨迹收敛判定留 ±2 容差
	const y = 5

	left := drawTextOn(t, 60, 20, func(r *Renderer) error {
		return r.DrawText("WWW", 20, y, "", 12, "#000", "left", "top", 0)
	})
	minX, _, _, _, ok := inkBox(left)
	if !ok {
		t.Fatal("left 无墨迹")
	}
	if minX < 18 || minX > 22 {
		t.Errorf("left 首列墨迹 x=%d, want ∈[18,22]", minX)
	}

	right := drawTextOn(t, 60, 20, func(r *Renderer) error {
		return r.DrawText("WWW", 40, y, "", 12, "#000", "right", "top", 0)
	})
	_, _, maxX, _, ok := inkBox(right)
	if !ok {
		t.Fatal("right 无墨迹")
	}
	if maxX < 38 || maxX > 42 {
		t.Errorf("right 末列墨迹 x=%d, want ∈[38,42]", maxX)
	}

	center := drawTextOn(t, 60, 20, func(r *Renderer) error {
		return r.DrawText("WWW", 30, y, "", 12, "#000", "center", "top", 0)
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
	// 垂直对齐用字体 metrics 落基线:top 墨迹全在锚点下方、bottom 收于锚点上方、
	// center 跨锚点两侧;top 与 bottom 整体错开一个 metrics 线高(相对关系断言)
	inkSpanY := func(vAlign string) (int, int) {
		img := drawTextOn(t, 40, 40, func(r *Renderer) error {
			return r.DrawText("ABC", 10, 20, "", 12, "#000", "left", vAlign, 0)
		})
		_, minY, _, maxY, ok := inkBox(img)
		if !ok {
			t.Fatalf("vAlign=%s 无墨迹", vAlign)
		}
		return minY, maxY
	}

	tminY, tmaxY := inkSpanY("top")
	bminY, bmaxY := inkSpanY("bottom")
	if tminY < 19 {
		t.Errorf("top 首行墨迹 y=%d, 不得高于锚点上方 1px", tminY)
	}
	if bmaxY > 21 {
		t.Errorf("bottom 末行墨迹 y=%d, 应收于锚点 y=20 上方(基线按 descent 抬起)", bmaxY)
	}
	if tminY <= bminY || tmaxY <= bmaxY {
		t.Errorf("top 墨迹 [%d,%d] 应整体低于 bottom 墨迹 [%d,%d] 一个 metrics 线高", tminY, tmaxY, bminY, bmaxY)
	}

	cminY, cmaxY := inkSpanY("center")
	if cminY >= 20 || cmaxY <= 20 {
		t.Errorf("center 墨迹 [%d,%d] 应跨越锚点 y=20 两侧", cminY, cmaxY)
	}
}

func TestDrawTextUnknownAlignFallsBackLeftTop(t *testing.T) {
	// 未知对齐取值归 left/top(switch 缺省臂,image-renderer text.go 同一防线)
	boxOf := func(h, v string) (int, int) {
		img := drawTextOn(t, 60, 30, func(r *Renderer) error {
			return r.DrawText("ABC", 10, 5, "", 12, "#000", h, v, 0)
		})
		x, y, _, _, ok := inkBox(img)
		if !ok {
			t.Fatalf("align=%s/%s 无墨迹", h, v)
		}
		return x, y
	}
	fx, fy := boxOf("diagonal", "middle")
	lx, ly := boxOf("left", "top")
	if fx != lx || fy != ly {
		t.Errorf("未知取值应与 left/top 起墨一致: got (%d,%d), want (%d,%d)", fx, fy, lx, ly)
	}
}

func TestDrawTextAngleRotatesAboutAnchor(t *testing.T) {
	// 角度 90(逆时针):水平行文绕锚点转为纵向——横向跨距收缩为字面高度,
	// 纵向跨距膨胀为文本宽度,墨迹向上越过锚点;角度 -90 镜像向下
	// (相对关系断言,不依赖对齐偏移具体数值)
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
	lx0, _, _, ly1 := boxOf(-90)

	if spanX := rx1 - rx0; spanX >= ux1-ux0 {
		t.Errorf("旋转后横向跨距 %d, 应小于未旋转的 %d", spanX, ux1-ux0)
	}
	if spanY := ry1 - ry0; spanY <= uy1-uy0 {
		t.Errorf("旋转后纵向跨距 %d, 应大于未旋转的 %d", spanY, uy1-uy0)
	}
	if ry0 >= 40 {
		t.Errorf("逆时针旋转后墨迹顶 y=%d, 应向上越过锚点 y=40", ry0)
	}
	if ly1 <= 40 {
		t.Errorf("顺时针(-90)旋转后墨迹底 y=%d, 应向下越过锚点 y=40", ly1)
	}
	if lx0 >= 40 {
		t.Errorf("顺时针(-90)旋转后墨迹左缘 x=%d, 应向左越过锚点 x=40", lx0)
	}
}

func TestDrawTextPipelineExtractsASCIIAndCJK(t *testing.T) {
	// 管线级:pdftotext 提取 ASCII + CJK(ToUnicode 随子集内嵌);pdffonts
	// 标记内嵌子集。字体探针只认单体 TTF(TTC 不支持,见工票 Answer)
	font := probeCJKFont(t)

	c := canvas.New(200, 40, layer.NewTextLayer(
		layer.WithSize(200, 40), layer.WithPosition(0, 0),
		layer.WithText("go-canvas 证书 PDF"), layer.WithFont(font, 20, "#111111"),
		layer.WithHorizontalAlign(layer.AlignLeft), layer.WithVerticalAlign(layer.AlignTop),
		layer.WithPadding(6),
	))
	pdf := renderPdfCanvas(t, nil, c)

	text := runPoppler(t, "pdftotext", writePdf(t, t.TempDir(), "probe.pdf", pdf), "-")
	if !strings.Contains(text, "go-canvas") {
		t.Errorf("pdftotext 应提取 ASCII 串, got %q", text)
	}
	if !strings.Contains(text, "证书") {
		t.Errorf("pdftotext 应提取 CJK 串(ToUnicode), got %q", text)
	}

	fonts := runPoppler(t, "pdffonts", writePdf(t, t.TempDir(), "probe.pdf", pdf))
	assertEmbeddedSubsetFont(t, fonts, "TrueType")
}

func TestDrawTextHelveticaFallbackNotEmbedded(t *testing.T) {
	// 兜底字体 = 核心字体 Helvetica:ASCII 兜底可渲且**不内嵌**(核心字体
	// 由阅读器提供,与 TTF 子集内嵌互证)
	c := canvas.New(100, 30, layer.NewTextLayer(
		layer.WithSize(100, 30), layer.WithPosition(0, 0),
		layer.WithText("fallback ascii"), layer.WithFont("", 14, "#111111"),
		layer.WithHorizontalAlign(layer.AlignLeft), layer.WithVerticalAlign(layer.AlignTop),
	))
	pdf := renderPdfCanvas(t, nil, c)
	text := runPoppler(t, "pdftotext", writePdf(t, t.TempDir(), "probe.pdf", pdf), "-")
	if !strings.Contains(text, "fallback ascii") {
		t.Errorf("pdftotext 应提取兜底字体串, got %q", text)
	}
	fonts := runPoppler(t, "pdffonts", writePdf(t, t.TempDir(), "probe.pdf", pdf))
	assertEmbeddedSubsetFont(t, fonts, "Helvetica")
}

// assertEmbeddedSubsetFont pdffonts 输出断言:
// kind="TrueType" 时须存在内嵌子集行(emb/sub/uni 全 yes);
// kind="Helvetica" 时须存在不内嵌的核心字体行(emb=no)
func assertEmbeddedSubsetFont(t *testing.T, pdffonts, kind string) {
	t.Helper()
	for _, line := range strings.Split(pdffonts, "\n") {
		switch kind {
		case "TrueType":
			// fpdf 子集内嵌:emb=yes(整字体内嵌)+ uni=yes(ToUnicode);
			// 子集名无 6 字母前缀(pdffonts sub=no),subset 语义由
			// GenerateCutFont 仅保留用到的字形保证(偏差记录见工票 Answer)
			if strings.Contains(line, "CID TrueType") &&
				strings.Contains(line, "yes") && strings.Contains(line, "Identity-H") {
				return
			}
		case "Helvetica":
			if strings.Contains(line, "Helvetica") && strings.Contains(line, "no") {
				return
			}
		}
	}
	t.Fatalf("pdffonts 未找到 %s 对应行:\n%s", kind, pdffonts)
}

func TestDrawTextInvalidFontAborts(t *testing.T) {
	// 失败防线:垃圾字体字节报错中止;TTC 字体集显式拒收(fpdf 静默吞错,
	// 探针先行拦下并给出可读错误)
	r := New()
	if err := r.Begin(20, 20); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	junk := writeRawFixture(t, "junk.ttf", []byte("not a font"))
	if err := r.DrawText("x", 5, 10, junk, 12, "#000", "left", "top", 0); err == nil {
		t.Error("垃圾字体字节应报错")
	}

	ttc := writeRawFixture(t, "fake.ttc", []byte("ttcf............"))
	if err := r.DrawText("x", 5, 10, ttc, 12, "#000", "left", "top", 0); err == nil {
		t.Error("TTC 字体集应报错")
	} else if !strings.Contains(err.Error(), "TTC") {
		t.Errorf("TTC 错误应可读: %v", err)
	}
}

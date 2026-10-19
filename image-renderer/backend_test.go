package imagerenderer

// 后端原语的点位取样单元测试:直接驱动 Backend 五原语,锁定 drawRect 的
// 背景/边框像素语义与 drawImage 的放置/跳过语义(渲染级联见 render_test.go)

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/HankChenCH/go-canvas/layer"
)

// newSurfaceBackend 建面 20×20 的后端
func newSurfaceBackend(t *testing.T) *Renderer {
	t.Helper()
	r := New()
	if err := r.Begin(20, 20); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	return r
}

func TestDrawRectFillsBackgroundAndInsetBorders(t *testing.T) {
	r := newSurfaceBackend(t)
	border := layer.Border{
		Top:    &layer.BorderSide{Width: 2, Color: "#f00"},
		Bottom: &layer.BorderSide{Width: 2, Color: "#f00"},
		Left:   &layer.BorderSide{Width: 2, Color: "#f00"},
		Right:  &layer.BorderSide{Width: 2, Color: "#f00"},
	}
	if err := r.DrawRect(0, 0, 20, 20, ptr("#fff"), border); err != nil {
		t.Fatalf("DrawRect: %v", err)
	}
	img := r.End().(*image.NRGBA)

	assertPixel(t, img, 10, 10, white) // 内部保持背景
	// 边框 = 四边各一条线宽=边框宽的直线(非描边矩形),按盒内嵌绘制
	assertPixel(t, img, 10, 1, red)  // top 带覆盖 y ∈ [0, 2)
	assertPixel(t, img, 10, 18, red) // bottom 带覆盖 y ∈ [18, 20)
	assertPixel(t, img, 1, 10, red)  // left 带覆盖 x ∈ [0, 2)
	assertPixel(t, img, 18, 10, red) // right 带覆盖 x ∈ [18, 20)
	assertPixel(t, img, 1, 1, red)   // 角部落在交叉带内
}

func TestDrawRectSkipsNilBackgroundAndUnsetBorders(t *testing.T) {
	r := newSurfaceBackend(t)
	onlyTop := layer.Border{Top: &layer.BorderSide{Width: 3, Color: "#00f"}}
	if err := r.DrawRect(0, 0, 20, 20, nil, onlyTop); err != nil {
		t.Fatalf("DrawRect: %v", err)
	}
	img := r.End().(*image.NRGBA)

	assertPixel(t, img, 10, 1, blue)   // 仅 top 边
	assertPixel(t, img, 10, 10, clear) // 背景为 null 跳过填充,保持透明
	assertPixel(t, img, 1, 10, clear)  // 未设的 left 边
	assertPixel(t, img, 10, 19, clear) // 未设的 bottom 边
}

func TestDrawRectEmptyStringBackgroundSkipped(t *testing.T) {
	// 对齐 PHP `$bgColor !== null && $bgColor !== ''` 守卫:空串同 null
	r := newSurfaceBackend(t)
	if err := r.DrawRect(0, 0, 20, 20, ptr(""), layer.Border{}); err != nil {
		t.Fatalf("DrawRect: %v", err)
	}
	img := r.End().(*image.NRGBA)
	assertPixel(t, img, 10, 10, clear)
}

func TestDrawRectInvalidColorAborts(t *testing.T) {
	r := newSurfaceBackend(t)
	if err := r.DrawRect(0, 0, 20, 20, ptr("nope"), layer.Border{}); err == nil {
		t.Fatal("非法背景色应报错中止")
	}
	if err := r.DrawRect(0, 0, 20, 20, ptr("#fff"), layer.Border{
		Top: &layer.BorderSide{Width: 1, Color: "bogus"},
	}); err == nil {
		t.Fatal("非法边框色应报错中止")
	}
}

func TestDrawRectWireInjectedZeroWidthBorderIgnored(t *testing.T) {
	// 构造面 borderSide 保证非 nil 即 width≥1,但 wire 解码可注入 width=0 边:不画
	r := newSurfaceBackend(t)
	if err := r.DrawRect(0, 0, 20, 20, nil, layer.Border{
		Top: &layer.BorderSide{Width: 0, Color: "#f00"},
	}); err != nil {
		t.Fatalf("DrawRect: %v", err)
	}
	img := r.End().(*image.NRGBA)
	assertPixel(t, img, 10, 0, clear)
}

func TestDrawImageBlitsWholeImageIntoTargetBox(t *testing.T) {
	// 整图哨兵(srcRect 空矩形):按解码实际尺寸取整图,拉伸铺满目标盒
	r := newSurfaceBackend(t)
	src := solidPng(t, 4, 4, red)
	if err := r.DrawImage(src, image.Rectangle{}, image.Rect(2, 3, 8, 9)); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
	img := r.End().(*image.NRGBA)

	assertPixel(t, img, 2, 3, red) // 铺满目标盒 (2,3)-(8,9)
	assertPixel(t, img, 7, 8, red)
	assertPixel(t, img, 1, 3, clear)
	assertPixel(t, img, 7, 9, clear)
}

func TestDrawImageCropsSourceWindowThenStretches(t *testing.T) {
	// 窗口 blit:只取 srcRect 子矩形搬运至 dstRect(cover 裁窗/ tile 可见块的
	// 后端落点),窗外部分不参与——40×40 四象限图取左上 20×20 窗进 10×10 盒
	r := newSurfaceBackend(t)
	src := quadPng(t, 40, 40, red, green, blue, magenta)
	if err := r.DrawImage(src, image.Rect(0, 0, 20, 20), image.Rect(0, 0, 10, 10)); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
	img := r.End().(*image.NRGBA)

	assertPixel(t, img, 0, 0, red)
	assertPixel(t, img, 9, 9, red)
	assertPixel(t, img, 10, 5, clear) // 窗外不参与
	assertPixel(t, img, 5, 10, clear)
}

func TestDrawImageStretchesWithoutKeepingRatio(t *testing.T) {
	// 拉伸无关等比(fill 形态的后端落点):20×10 横条进 20×20 方盒——宽向 1:1、
	// 纵向 ×2 拉伸,分界列按源图比例映射、纵向满铺(取样点远离分界,避开插值过渡带)
	r := newSurfaceBackend(t)
	src := twoColorPng(t, 20, 10, red, blue, 10)
	if err := r.DrawImage(src, image.Rect(0, 0, 20, 10), image.Rect(0, 0, 20, 20)); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
	img := r.End().(*image.NRGBA)

	for _, p := range []struct{ x, y int }{{2, 0}, {7, 5}, {2, 19}} {
		assertPixel(t, img, p.x, p.y, red)
	}
	for _, p := range []struct{ x, y int }{{12, 5}, {17, 0}, {17, 19}} {
		assertPixel(t, img, p.x, p.y, blue)
	}
}

func TestDrawImageSkipsEmptyDstRect(t *testing.T) {
	// dstRect 空矩形直接跳过(PHP dw/dh ≤0 防护同款),不留像素;
	// 防护先于读取——src 不可读也不报错(分派层几何保证不发生,防御性契约)
	r := newSurfaceBackend(t)
	src := solidPng(t, 4, 4, red)
	if err := r.DrawImage(src, image.Rect(0, 0, 4, 4), image.Rectangle{}); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
	if err := r.DrawImage("/nonexistent/go-canvas-fixture.png", image.Rectangle{}, image.Rectangle{}); err != nil {
		t.Fatalf("空 dstRect 应先于读取跳过: %v", err)
	}
	img := r.End().(*image.NRGBA)
	assertPixel(t, img, 5, 5, clear)
}

func TestDrawImageMissingFileAborts(t *testing.T) {
	r := newSurfaceBackend(t)
	if err := r.DrawImage("/nonexistent/go-canvas-fixture.png", image.Rectangle{}, image.Rect(0, 0, 5, 5)); err == nil {
		t.Fatal("读取失败应报错中止")
	}
}

func TestDrawImageCorruptedImageAborts(t *testing.T) {
	// 真损坏图在 blit 解码处报错(ADR 0024 结局 parity:分派层退化 fill 形态
	// 照常分派,结局收敛到后端解码失败)
	r := newSurfaceBackend(t)
	src := filepath.Join(t.TempDir(), "broken.png")
	if err := os.WriteFile(src, []byte("definitely-not-an-image"), 0o644); err != nil {
		t.Fatalf("写损坏测试图: %v", err)
	}
	if err := r.DrawImage(src, image.Rectangle{}, image.Rect(0, 0, 5, 5)); err == nil {
		t.Fatal("解码失败应报错中止")
	}
}

func TestDrawImageExifRotatedWholeImageUpright(t *testing.T) {
	// EXIF 转正(整图哨兵路径):8×16 上红下蓝、orientation=6(顺时针转 90°)
	// 的 JPEG,观察方向为 16 宽×8 高、左蓝右红(原下缘转到左缘)——转正后
	// 拉伸进目标盒(JPEG 有损,容差取样)
	path := twoBandExifJpeg(t, 6)

	r := newSurfaceBackend(t)
	if err := r.DrawImage(path, image.Rectangle{}, image.Rect(0, 0, 16, 8)); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
	img := r.End().(*image.NRGBA)
	assertPixelNear(t, img, 4, 4, blue, 60) // 观察方向左半 = 原下缘
	assertPixelNear(t, img, 11, 4, red, 60) // 观察方向右半 = 原上缘
}

func TestDrawImageExifRotatedCropsWindowInIntrinsicCoordinates(t *testing.T) {
	// 窗口 blit 的 EXIF 次序(裁窗在内在坐标系、转正后置,ADR 0024 接口契约):
	// 8×16 上红下蓝、orientation=6,取内在坐标下部半窗 Rect(0,8,8,16)(蓝带)
	// → 转正为观察方向 8×8 蓝块 → 拉伸进目标盒(JPEG 有损,容差取样)
	path := twoBandExifJpeg(t, 6)

	r := newSurfaceBackend(t)
	if err := r.DrawImage(path, image.Rect(0, 8, 8, 16), image.Rect(0, 0, 10, 10)); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
	img := r.End().(*image.NRGBA)
	assertPixelNear(t, img, 2, 5, blue, 60)
	assertPixelNear(t, img, 8, 5, blue, 60)
}

func TestEndBeforeBeginReturnsNil(t *testing.T) {
	if product := New().End(); product != nil {
		t.Errorf("未建面时 End 应返回 nil, got %v", product)
	}
}

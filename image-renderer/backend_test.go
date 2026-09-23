package imagerenderer

// 后端原语的点位取样单元测试:直接驱动 Backend 五原语,锁定 drawRect 的
// 背景/边框像素语义与 drawImage 的放置/跳过语义(渲染级联见 render_test.go)

import (
	"image"
	"testing"

	"github.com/hankchen/go-canvas/layer"
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

func TestDrawImagePlacesCoveredIntoTargetBox(t *testing.T) {
	r := newSurfaceBackend(t)
	src := solidPng(t, 4, 4, red)
	if err := r.DrawImage(src, 2, 3, 6, 6); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
	img := r.End().(*image.NRGBA)

	assertPixel(t, img, 2, 3, red) // 放置于 (x, y)
	assertPixel(t, img, 7, 8, red) // 铺满目标盒
	assertPixel(t, img, 1, 3, clear)
	assertPixel(t, img, 7, 9, clear)
}

func TestDrawImageSkipsNonPositiveSize(t *testing.T) {
	// 宽/高 ≤0 直接跳过(PHP 同款守卫),不留任何像素
	r := newSurfaceBackend(t)
	src := solidPng(t, 4, 4, red)
	if err := r.DrawImage(src, 0, 0, 0, 10); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
	if err := r.DrawImage(src, 0, 0, 10, -1); err != nil {
		t.Fatalf("DrawImage: %v", err)
	}
	img := r.End().(*image.NRGBA)
	assertPixel(t, img, 5, 5, clear)
}

func TestCoverExtremeAspectRatioStillFills(t *testing.T) {
	// 极端宽高比(100×4 → 20×100)下裁切窗截断会得 0 宽:钳到 ≥1,不得输出空图
	src := image.NewNRGBA(image.Rect(0, 0, 100, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 100; x++ {
			src.SetNRGBA(x, y, red)
		}
	}
	got := coverImage(src, 20, 100)
	assertPixel(t, got, 10, 50, red)
}

func TestDrawImageMissingFileAborts(t *testing.T) {
	r := newSurfaceBackend(t)
	if err := r.DrawImage("/nonexistent/go-canvas-fixture.png", 0, 0, 5, 5); err == nil {
		t.Fatal("读取失败应报错中止")
	}
}

func TestEndBeforeBeginReturnsNil(t *testing.T) {
	if product := New().End(); product != nil {
		t.Errorf("未建面时 End 应返回 nil, got %v", product)
	}
}

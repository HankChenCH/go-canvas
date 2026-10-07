package renderer_test

// PositionResolverTest 平移:九锚点偏移 + 子层大于父盒的负溢出。
// 期望值逐条对齐 php-canvas-next tests/Renderer/PositionResolverTest.php。

import (
	"testing"

	"github.com/HankChenCH/go-canvas/renderer"
)

// testNineAnchors 平移 testNineAnchors:父盒 100×80、子层 20×10,九锚点偏移
func testNineAnchors(t *testing.T) {
	t.Helper()

	cases := []struct {
		anchor string
		wantX  int
		wantY  int
	}{
		{"top-left", 0, 0},
		{"top", 40, 0},
		{"top-right", 80, 0},
		{"left", 0, 35},
		{"center", 40, 35},
		{"right", 80, 35},
		{"bottom-left", 0, 70},
		{"bottom", 40, 70},
		{"bottom-right", 80, 70},
	}

	for _, tc := range cases {
		x, y := renderer.ResolveAnchor(tc.anchor, 100, 80, 20, 10)
		if x != tc.wantX || y != tc.wantY {
			t.Errorf("锚点 %s 偏移不符: got (%d, %d), want (%d, %d)", tc.anchor, x, y, tc.wantX, tc.wantY)
		}
	}
}

// testChildLargerThanParentAllowsNegativeOverflow 平移同名用例:
// 与旧库 intervention insert 语义一致——不做钳位,负偏移即溢出摆放
func testChildLargerThanParentAllowsNegativeOverflow(t *testing.T) {
	t.Helper()

	x, y := renderer.ResolveAnchor("bottom-right", 10, 10, 50, 50)
	if x != -40 || y != -40 {
		t.Errorf("负溢出偏移不符: got (%d, %d), want (-40, -40)", x, y)
	}
}

func TestPositionResolver(t *testing.T) {
	t.Run("testNineAnchors", testNineAnchors)
	t.Run("testChildLargerThanParentAllowsNegativeOverflow", testChildLargerThanParentAllowsNegativeOverflow)
}

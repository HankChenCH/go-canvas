package layer_test

// fit 几何矩阵(ADR 0024,平移 php-canvas-next ImageLayerTest 的
// testCoverRectMatrix/testTileRectsMatrix):cover 等比居中裁窗与 tile 可见块
// 矩形组都是纯布局计算——源窗恒居中裁(对齐不进源窗),tile 单元 = 内在尺寸
// 1:1、首块按对齐锚定、网格向右下无限延伸、绘制面止于网格 ∩ 内容盒

import (
	"image"
	"testing"

	"github.com/HankChenCH/go-canvas/layer"
)

// rect 四元组 convenience:期望值按 PHP [sx, sy, sw, sh] 平移
func rect(sx, sy, sw, sh int) image.Rectangle {
	return image.Rect(sx, sy, sx+sw, sy+sh)
}

func TestCoverRectMatrix(t *testing.T) {
	cases := []struct {
		name                 string
		boxW, boxH           int
		horizontal, vertical string
		padT, padR           float64
		padB, padL           float64
		imgW, imgH           int
		want                 image.Rectangle
	}{
		// 竖图(1:4)进横盒(2:1):源窗贴满宽、垂直居中裁
		{"竖图进横盒", 100, 50, "center", "center", 0, 0, 0, 0, 100, 400, rect(0, 175, 100, 50)},
		// 方图进横盒:源窗全宽、中部横带(sy = (40-20)/2 = 10)
		{"方图进横盒", 100, 50, "center", "center", 0, 0, 0, 0, 40, 40, rect(0, 10, 40, 20)},
		// 横图(4:1)进竖盒:源窗贴满高、水平居中裁(sx = int(87.5))
		{"横图进竖盒", 50, 100, "center", "center", 0, 0, 0, 0, 200, 50, rect(87, 0, 25, 50)},
		// 等比:源窗 = 整图,零裁切
		{"等比整图", 100, 50, "center", "center", 0, 0, 0, 0, 200, 100, rect(0, 0, 200, 100)},
		// padding 收缩内容盒 → 窗口随之收窄(80×30 内容盒 → 40×15 窗)
		{"padding 收窄", 100, 50, "center", "center", 10, 10, 10, 10, 40, 40, rect(0, 12, 40, 15)},
		// 对齐不影响源窗(cover 恒居中裁)
		{"对齐不进源窗", 100, 50, "left", "top", 0, 0, 0, 0, 40, 40, rect(0, 10, 40, 20)},
	}

	for _, tc := range cases {
		l := layer.NewImageLayer(
			layer.WithSize(tc.boxW, tc.boxH),
			layer.WithPaddingTRBL(tc.padT, tc.padR, tc.padB, tc.padL),
			layer.WithHorizontalAlign(tc.horizontal),
			layer.WithVerticalAlign(tc.vertical),
		)
		if got := l.CoverRect(tc.imgW, tc.imgH); got != tc.want {
			t.Errorf("%s: CoverRect(%d, %d) = %s, want %s",
				tc.name, tc.imgW, tc.imgH, got, tc.want)
		}
	}
}

// block 组 convenience:期望值按 PHP [['src' => [..], 'dst' => [..]]] 平移(行主序)
func blocks(pairs ...[2]image.Rectangle) []layer.TileBlock {
	out := make([]layer.TileBlock, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, layer.TileBlock{Src: p[0], Dst: p[1]})
	}
	return out
}

func TestTileRectsMatrix(t *testing.T) {
	t.Run("小图 center 网格居中右下半块截断", func(t *testing.T) {
		// 40×40 单元进 100×50 盒(center):网格整体居中、右/下溢出半块截断(行主序)
		l := layer.NewImageLayer(layer.WithSize(100, 50))
		got := l.TileRects(40, 40)
		want := blocks(
			[2]image.Rectangle{rect(0, 0, 40, 40), rect(30, 5, 40, 40)},
			[2]image.Rectangle{rect(0, 0, 30, 40), rect(70, 5, 30, 40)},
			[2]image.Rectangle{rect(0, 0, 40, 5), rect(30, 45, 40, 5)},
			[2]image.Rectangle{rect(0, 0, 30, 5), rect(70, 45, 30, 5)},
		)
		if len(got) != len(want) {
			t.Fatalf("块数 = %d, want %d:\n got  %v\n want %v", len(got), len(want), got, want)
		}
		for i, b := range want {
			if got[i] != b {
				t.Errorf("块 %d = {src %s, dst %s}, want {src %s, dst %s}",
					i, got[i].Src, got[i].Dst, b.Src, b.Dst)
			}
		}
	})

	t.Run("大图 center 负锚点露中部", func(t *testing.T) {
		// 大图 center:锚点为负露出图片中部(可见块只有网格首块的中部子矩形)
		l := layer.NewImageLayer(layer.WithSize(100, 50))
		got := l.TileRects(120, 80)
		want := blocks([2]image.Rectangle{rect(10, 15, 100, 50), rect(0, 0, 100, 50)})
		if len(got) != 1 || got[0] != want[0] {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("align right 首块贴右空隙在左", func(t *testing.T) {
		// align=right:首块贴右、锚点前空隙透明露下层,下方照常补行
		l := layer.NewImageLayer(
			layer.WithSize(100, 50),
			layer.WithHorizontalAlign(layer.AlignRight),
			layer.WithVerticalAlign(layer.AlignTop),
		)
		got := l.TileRects(40, 40)
		want := blocks(
			[2]image.Rectangle{rect(0, 0, 40, 40), rect(60, 0, 40, 40)},
			[2]image.Rectangle{rect(0, 0, 40, 10), rect(60, 40, 40, 10)},
		)
		if len(got) != len(want) {
			t.Fatalf("块数 = %d, want %d", len(got), len(want))
		}
		for i, b := range want {
			if got[i] != b {
				t.Errorf("块 %d = {src %s, dst %s}, want {src %s, dst %s}",
					i, got[i].Src, got[i].Dst, b.Src, b.Dst)
			}
		}
	})

	t.Run("非均匀 padding 网格止于交集", func(t *testing.T) {
		// padding 上20/右10/下5/左0 + left/top:内容盒 (0,20)-(90,45),
		// 网格止于交集(末列 10px 窄条)
		l := layer.NewImageLayer(
			layer.WithSize(100, 50),
			layer.WithPaddingTRBL(20, 10, 5, 0),
			layer.WithHorizontalAlign(layer.AlignLeft),
			layer.WithVerticalAlign(layer.AlignTop),
		)
		got := l.TileRects(40, 40)
		want := blocks(
			[2]image.Rectangle{rect(0, 0, 40, 25), rect(0, 20, 40, 25)},
			[2]image.Rectangle{rect(0, 0, 40, 25), rect(40, 20, 40, 25)},
			[2]image.Rectangle{rect(0, 0, 10, 25), rect(80, 20, 10, 25)},
		)
		if len(got) != len(want) {
			t.Fatalf("块数 = %d, want %d:\n got  %v\n want %v", len(got), len(want), got, want)
		}
		for i, b := range want {
			if got[i] != b {
				t.Errorf("块 %d = {src %s, dst %s}, want {src %s, dst %s}",
					i, got[i].Src, got[i].Dst, b.Src, b.Dst)
			}
		}
	})

	t.Run("非对称 padding 加 center 锚点可落内容盒前", func(t *testing.T) {
		// 非对称 padding + center:锚点按整盒中点(与 contain 同门公式,可落在
		// 内容盒起点之前),网格 ∩ 内容盒的可见块以 src 偏移修正(大图露中部:srcY = 45)
		l := layer.NewImageLayer(
			layer.WithSize(100, 50),
			layer.WithPaddingTRBL(20, 10, 5, 0),
		)
		got := l.TileRects(100, 100)
		want := blocks([2]image.Rectangle{rect(0, 45, 90, 25), rect(0, 20, 90, 25)})
		if len(got) != 1 || got[0] != want[0] {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("网格线恰压内容盒底边零高交集不出块", func(t *testing.T) {
		l := layer.NewImageLayer(
			layer.WithSize(80, 40),
			layer.WithHorizontalAlign(layer.AlignLeft),
			layer.WithVerticalAlign(layer.AlignTop),
		)
		got := l.TileRects(40, 40)
		want := blocks(
			[2]image.Rectangle{rect(0, 0, 40, 40), rect(0, 0, 40, 40)},
			[2]image.Rectangle{rect(0, 0, 40, 40), rect(40, 0, 40, 40)},
		)
		if len(got) != len(want) {
			t.Fatalf("块数 = %d, want %d", len(got), len(want))
		}
		for i, b := range want {
			if got[i] != b {
				t.Errorf("块 %d = {src %s, dst %s}, want {src %s, dst %s}",
					i, got[i].Src, got[i].Dst, b.Src, b.Dst)
			}
		}
	})

	t.Run("内容区被 padding 吃满交集为空", func(t *testing.T) {
		l := layer.NewImageLayer(layer.WithSize(40, 40), layer.WithPadding(20))
		if got := l.TileRects(40, 40); len(got) != 0 {
			t.Errorf("零交集应零块, got %v", got)
		}
	})
}

func TestCoverSourceWindowSharedByBackgroundArm(t *testing.T) {
	// 裁窗公式唯一落点(ADR 0024):包级函数供画布背景臂共用,与图片臂
	// CoverRect 同一公式形态(图片臂 = 内容盒投影)
	box := layer.CoverSourceWindow(100, 50, 40, 40)
	l := layer.NewImageLayer(layer.WithSize(100, 50))
	if box != l.CoverRect(40, 40) {
		t.Errorf("背景臂公式 = %s, 图片臂 = %s, 两门必须同式", box, l.CoverRect(40, 40))
	}
}

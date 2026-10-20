package renderer_test

// paintImage 圆形档分派契约(ADR 0026,平移 php-canvas-next ImageClipPaintingTest):
// clip=circle 走三步——椭圆背景填充、内容按 fit 绘出后遮罩 blit(掩码矩形 =
// 内切圆矩形,fit 几何照旧)、椭圆周描边;矩形盒 DrawRect/DrawImage 路径不参与
// (整层渲染面裁进圆)。未知 clip 值与后端缺形状能力口降级矩形(fit 未知值纪律
// 同门);内在尺寸不可用退化 fill 形态再裁圆(ADR 0024 纪律)。
// 核心包只断言分派几何(假后端,DrawImageMasked 不真解码),像素点位在
// image-renderer module

import (
	"context"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/renderer"
)

// ---- 形状能力口假后端:在 fakeBackend 之上追加椭圆/掩码 blit 留痕 ----
// (对齐 PHP CoordinateRecordingRenderer:实现形状能力口并留痕参数;
// 缺能力降级断言走未实现的 fakeBackend)

type ellipseCall struct {
	x, y, w, h int
	fill       *string
	stroke     *layer.BorderSide
}

type maskedCall struct {
	src                        string
	srcRect, dstRect, maskRect image.Rectangle
}

type shapeCapableBackend struct {
	*fakeBackend
	ellipses []ellipseCall
	masked   []maskedCall
}

func (b *shapeCapableBackend) DrawEllipse(x, y, w, h int, fill *string, stroke *layer.BorderSide) error {
	b.ops = append(b.ops, "ellipse")
	b.ellipses = append(b.ellipses, ellipseCall{x, y, w, h, fill, stroke})
	return nil
}

func (b *shapeCapableBackend) DrawImageMasked(src string, srcRect, dstRect, maskRect image.Rectangle) error {
	b.ops = append(b.ops, "masked")
	b.masked = append(b.masked, maskedCall{src, srcRect, dstRect, maskRect})
	return nil
}

var _ renderer.ShapeCapableBackend = (*shapeCapableBackend)(nil)

func strPtr(s string) *string { return &s }

func sidePtr(width int, color string) *layer.BorderSide {
	return &layer.BorderSide{Width: width, Color: color}
}

// clipViaWire 经 wire 往返给图片图层注入 spec.clip(模拟第三方手写 graph:
// 白名单外枚举串的透传形态,不经 SetClip 归一门)
func clipViaWire(t *testing.T, l *layer.ImageLayer, clip string) *layer.ImageLayer {
	t.Helper()
	raw, err := json.Marshal(l.Graph())
	if err != nil {
		t.Fatalf("序列化 graph: %v", err)
	}
	var node layer.Node
	if err := json.Unmarshal(raw, &node); err != nil {
		t.Fatalf("解码 graph: %v", err)
	}
	node.Spec.Clip = &clip
	return layer.ImageFromGraph(node)
}

// assertEllipseCalls 断言 DrawEllipse 调用序列(逐字段比较)
func assertEllipseCalls(t *testing.T, got []ellipseCall, want []ellipseCall) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ellipse 调用数 = %d, want %d:\n got  %v\n want %v", len(got), len(want), got, want)
	}
	for i, w := range want {
		g := got[i]
		if g.x != w.x || g.y != w.y || g.w != w.w || g.h != w.h {
			t.Errorf("ellipse[%d] 几何 = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
				i, g.x, g.y, g.w, g.h, w.x, w.y, w.w, w.h)
		}
		if (g.fill == nil) != (w.fill == nil) || (g.fill != nil && *g.fill != *w.fill) {
			t.Errorf("ellipse[%d] fill = %v, want %v", i, g.fill, w.fill)
		}
		if (g.stroke == nil) != (w.stroke == nil) || (g.stroke != nil && *g.stroke != *w.stroke) {
			t.Errorf("ellipse[%d] stroke = %v, want %v", i, g.stroke, w.stroke)
		}
	}
}

// assertMaskedCalls 断言 DrawImageMasked 调用序列(src/三矩形逐对比较)
func assertMaskedCalls(t *testing.T, got []maskedCall, want []maskedCall) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("masked 调用数 = %d, want %d:\n got  %v\n want %v", len(got), len(want), got, want)
	}
	for i, w := range want {
		g := got[i]
		if g.src != w.src || g.srcRect != w.srcRect || g.dstRect != w.dstRect || g.maskRect != w.maskRect {
			t.Errorf("masked[%d] = {src %s, srcRect %s, dstRect %s, maskRect %s}, want {src %s, srcRect %s, dstRect %s, maskRect %s}",
				i, g.src, g.srcRect, g.dstRect, g.maskRect, w.src, w.srcRect, w.dstRect, w.maskRect)
		}
	}
}

func brokenPngPath(t *testing.T) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "broken.png")
	if err := os.WriteFile(src, []byte("definitely-not-an-image"), 0o644); err != nil {
		t.Fatalf("写损坏测试图: %v", err)
	}
	return src
}

func TestCircleDispatchesThreeStepsWithoutRect(t *testing.T) {
	// 圆形档三步:椭圆背景填充 → 遮罩 blit → 椭圆周描边;矩形盒 rect 原语不调用
	// (背景进圆、边框成环)。方盒 100×100:内切圆矩形 = 整盒
	src := writeSolidPng(t, 40, 40)
	backend := &shapeCapableBackend{fakeBackend: &fakeBackend{}}
	l := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#ffffff"), layer.WithImage(src), layer.WithBorder(6, "#f00"))
	l.SetClip(layer.ClipCircle)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertOps(t, backend.ops, []string{"begin", "ellipse", "masked", "ellipse", "end"})
	assertEllipseCalls(t, backend.ellipses, []ellipseCall{
		{0, 0, 100, 100, strPtr("#ffffff"), nil},
		{0, 0, 100, 100, nil, sidePtr(6, "#f00")},
	})
	// fit 几何照旧(cover:40×40 方图进方盒零裁切),掩码矩形 = 内切圆矩形
	assertMaskedCalls(t, backend.masked, []maskedCall{
		{src, image.Rect(0, 0, 40, 40), image.Rect(0, 0, 100, 100), image.Rect(0, 0, 100, 100)},
	})
	if len(backend.rects) != 0 || len(backend.images) != 0 {
		t.Errorf("矩形路径不得参与: rects = %v, images = %v", backend.rects, backend.images)
	}
}

func TestNonSquareBoxKeepsPerfectCircleGeometry(t *testing.T) {
	// 非方盒 100×50:内切圆矩形 (25,0)-(75,50)(恒正圆),fit 的 dstRect 仍是内容盒
	src := writeSolidPng(t, 40, 40)
	backend := &shapeCapableBackend{fakeBackend: &fakeBackend{}}
	l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithBackground("#0f0"), layer.WithImage(src))
	l.SetClip(layer.ClipCircle)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertEllipseCalls(t, backend.ellipses, []ellipseCall{
		{25, 0, 50, 50, strPtr("#0f0"), nil},
	})
	// cover:40×40 方图进 100×50 内容盒 → 源窗中部横带 (0,10,40,30)、dst 全内容盒
	assertMaskedCalls(t, backend.masked, []maskedCall{
		{src, image.Rect(0, 10, 40, 30), image.Rect(0, 0, 100, 50), image.Rect(25, 0, 75, 50)},
	})
}

func TestCircleWithoutImageStillFillsAndStrokes(t *testing.T) {
	// 无图:背景圆 + 描边环两步,零 blit
	backend := &shapeCapableBackend{fakeBackend: &fakeBackend{}}
	l := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#fff"), layer.WithBorder(2, "#00f"))
	l.SetClip(layer.ClipCircle)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertOps(t, backend.ops, []string{"begin", "ellipse", "ellipse", "end"})
	assertEllipseCalls(t, backend.ellipses, []ellipseCall{
		{0, 0, 100, 100, strPtr("#fff"), nil},
		{0, 0, 100, 100, nil, sidePtr(2, "#00f")},
	})
	assertMaskedCalls(t, backend.masked, nil)
}

func TestCircleWithoutDeclaredBorderSkipsStrokeEllipse(t *testing.T) {
	// 四边全未声明不描边:只两步(背景圆 + 遮罩 blit)
	src := writeSolidPng(t, 40, 40)
	backend := &shapeCapableBackend{fakeBackend: &fakeBackend{}}
	l := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#fff"), layer.WithImage(src))
	l.SetClip(layer.ClipCircle)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertOps(t, backend.ops, []string{"begin", "ellipse", "masked", "end"})
	assertEllipseCalls(t, backend.ellipses, []ellipseCall{
		{0, 0, 100, 100, strPtr("#fff"), nil},
	})
}

func TestCircleStrokeParamsFollowParsingRule(t *testing.T) {
	// 描边参数解析规则进分派:宽取四边已声明最小值(2 < 8)、色按 top→right→bottom→left 首个非空(top)
	src := writeSolidPng(t, 40, 40)
	backend := &shapeCapableBackend{fakeBackend: &fakeBackend{}}
	l := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBackground("#fff"),
		layer.WithImage(src), layer.WithBorderTop(2, "#f00"), layer.WithBorderBottom(8, "#00f"))
	l.SetClip(layer.ClipCircle)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertEllipseCalls(t, backend.ellipses, []ellipseCall{
		{0, 0, 100, 100, strPtr("#fff"), nil},
		{0, 0, 100, 100, nil, sidePtr(2, "#f00")},
	})
}

func TestCircleAllFitModesDispatchMaskedBlits(t *testing.T) {
	// circle × fit 正交(先绘后裁):contain/tile/fill 各档几何照旧,只换成遮罩 blit 形态
	src := writeSolidPng(t, 40, 40)

	// contain:40×20 宽图进 100×100 盒 → 等比放大 100×50 垂直居中(ADR 0023 几何不动)
	wide := writeSolidPng(t, 40, 20)
	contain := &shapeCapableBackend{fakeBackend: &fakeBackend{}}
	containLayer := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage(wide))
	containLayer.SetFit(layer.FitContain)
	containLayer.SetClip(layer.ClipCircle)
	if _, err := newRenderer(t, contain).RenderLayer(context.Background(), containLayer); err != nil {
		t.Fatalf("RenderLayer(contain): %v", err)
	}
	assertMaskedCalls(t, contain.masked, []maskedCall{
		{wide, image.Rect(0, 0, 40, 20), image.Rect(0, 25, 100, 75), image.Rect(0, 0, 100, 100)},
	})

	fill := &shapeCapableBackend{fakeBackend: &fakeBackend{}}
	fillLayer := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage(src))
	fillLayer.SetFit(layer.FitFill)
	fillLayer.SetClip(layer.ClipCircle)
	if _, err := newRenderer(t, fill).RenderLayer(context.Background(), fillLayer); err != nil {
		t.Fatalf("RenderLayer(fill): %v", err)
	}
	assertMaskedCalls(t, fill.masked, []maskedCall{
		{src, image.Rect(0, 0, 40, 40), image.Rect(0, 0, 100, 100), image.Rect(0, 0, 100, 100)},
	})

	// tile:可见块逐块遮罩 blit(掩码矩形同圆;40×40 单元 center 锚定 (30,30),右/下溢出截断)
	tile := &shapeCapableBackend{fakeBackend: &fakeBackend{}}
	tileLayer := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage(src))
	tileLayer.SetFit(layer.FitTile)
	tileLayer.SetClip(layer.ClipCircle)
	if _, err := newRenderer(t, tile).RenderLayer(context.Background(), tileLayer); err != nil {
		t.Fatalf("RenderLayer(tile): %v", err)
	}
	assertMaskedCalls(t, tile.masked, []maskedCall{
		{src, image.Rect(0, 0, 40, 40), image.Rect(30, 30, 70, 70), image.Rect(0, 0, 100, 100)},
		{src, image.Rect(0, 0, 30, 40), image.Rect(70, 30, 100, 70), image.Rect(0, 0, 100, 100)},
		{src, image.Rect(0, 0, 40, 30), image.Rect(30, 70, 70, 100), image.Rect(0, 0, 100, 100)},
		{src, image.Rect(0, 0, 30, 30), image.Rect(70, 70, 100, 100), image.Rect(0, 0, 100, 100)},
	})
}

func TestCircleIntrinsicSizeFailureDegradesToFillFormMasked(t *testing.T) {
	// 内在尺寸不可用(ADR 0024 纪律):退化整图拉伸(fill 形态、srcRect 零哨兵)
	// 再裁圆——掩码 blit 原语形态,遮罩几何不变
	src := brokenPngPath(t)

	backend := &shapeCapableBackend{fakeBackend: &fakeBackend{}}
	l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithImage(src))
	l.SetClip(layer.ClipCircle)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertMaskedCalls(t, backend.masked, []maskedCall{
		{src, image.Rectangle{}, image.Rect(0, 0, 100, 50), image.Rect(25, 0, 75, 50)},
	})
}

func TestCircleWithDegenerateContentAreaDrawsNothing(t *testing.T) {
	// 内容区被 padding 吃满:内切圆边长 ≤0,零几何零绘制
	src := writeSolidPng(t, 40, 40)
	backend := &shapeCapableBackend{fakeBackend: &fakeBackend{}}
	l := layer.NewImageLayer(layer.WithSize(40, 40), layer.WithBackground("#fff"),
		layer.WithImage(src), layer.WithPadding(20), layer.WithBorder(2, "#f00"))
	l.SetClip(layer.ClipCircle)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertOps(t, backend.ops, []string{"begin", "end"})
	assertEllipseCalls(t, backend.ellipses, nil)
}

func TestUnknownClipValueDegradesToRectangle(t *testing.T) {
	// 未知 clip 值渲染面按矩形降级(白名单在渲染臂,不在解码门);wire 透传不受扰。
	// 用带形状能力口的后端证明双门中白名单先拦(能力口在场也不走圆形三步)
	src := writeSolidPng(t, 40, 40)
	l := clipViaWire(t,
		layer.NewImageLayer(layer.WithSize(100, 50), layer.WithBackground("#fff"), layer.WithImage(src)),
		"star")

	backend := &shapeCapableBackend{fakeBackend: &fakeBackend{}}
	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	if got := l.Clip(); got != "star" {
		t.Errorf("Clip() = %q, want star(透传)", got)
	}
	assertOps(t, backend.ops, []string{"begin", "rect", "image", "end"})
	assertEllipseCalls(t, backend.ellipses, nil)
	assertMaskedCalls(t, backend.masked, nil)
}

func TestClipCircleDegradesToRectangleWhenBackendLacksShapeCapability(t *testing.T) {
	// 后端缺形状能力口(未实现 ShapeCapableBackend)降级矩形路径:可选能力口契约
	src := writeSolidPng(t, 40, 40)
	backend := &fakeBackend{}
	l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithBackground("#fff"), layer.WithImage(src))
	l.SetClip(layer.ClipCircle)

	if _, err := newRenderer(t, backend).RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}

	assertOps(t, backend.ops, []string{"begin", "rect", "image", "end"})
}

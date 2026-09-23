package renderer_test

// 渲染模板 seam(工单05):假 Backend 记录五原语调用序列与坐标,
// 驱动模板的遍历/分派/容器下钻/定位断言。布局期望值按 PHP AbstractRenderer
// 逐分支手算对齐;文本断行结果以注入的桩断行器固定,与文本子系统解耦。

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/layer"
	"github.com/hankchen/go-canvas/renderer"
	"github.com/hankchen/go-canvas/resolver"
	"github.com/hankchen/go-canvas/text"
)

// ---- 假 Backend:记录原语调用序列与坐标 ----

type rectCall struct {
	x, y, w, h int
	bg         *string
	border     layer.Border
}

type imageCall struct {
	src        string
	x, y, w, h int
}

type textCall struct {
	line            string
	x, y            int
	font            string
	fontSize        int
	fontColor       string
	horizontalAlign string
	verticalAlign   string
	angle           int
}

type fakeBackend struct {
	product     any
	beginErr    error
	drawRectErr error

	ops    []string
	begins [][2]int
	rects  []rectCall
	images []imageCall
	texts  []textCall
}

func (f *fakeBackend) Begin(width, height int) error {
	f.ops = append(f.ops, "begin")
	f.begins = append(f.begins, [2]int{width, height})
	return f.beginErr
}

func (f *fakeBackend) End() any {
	f.ops = append(f.ops, "end")
	return f.product
}

func (f *fakeBackend) DrawRect(x, y, width, height int, bgColor *string, border layer.Border) error {
	f.ops = append(f.ops, "rect")
	f.rects = append(f.rects, rectCall{x, y, width, height, bgColor, border})
	return f.drawRectErr
}

func (f *fakeBackend) DrawImage(src string, x, y, width, height int) error {
	f.ops = append(f.ops, "image")
	f.images = append(f.images, imageCall{src, x, y, width, height})
	return nil
}

func (f *fakeBackend) DrawText(line string, x, y int, fontFile string, fontSize int, fontColor, horizontalAlign, verticalAlign string, angle int) error {
	f.ops = append(f.ops, "text")
	f.texts = append(f.texts, textCall{line, x, y, fontFile, fontSize, fontColor, horizontalAlign, verticalAlign, angle})
	return nil
}

// stubBreaker 桩断行器:返回预设行,把渲染遍历测试与文本子系统解耦
type stubBreaker struct{ lines []string }

func (s stubBreaker) BreakText(string, float64, text.TextMeasurer) []string { return s.lines }

// newRenderer 组装被测渲染器:缓存指向临时目录,本地路径零 I/O
func newRenderer(t *testing.T, backend renderer.Backend) renderer.Renderer {
	t.Helper()
	return renderer.New(backend, resolver.New(resolver.WithCacheRoot(t.TempDir())))
}

// assertOps 断言原语调用序列
func assertOps(t *testing.T, got, want []string) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("原语调用序列不符:\n got  %v\n want %v", got, want)
	}
}

// ---- 模板流程 ----

func TestRenderEmptyCanvasBeginsAndEndsWithCanvasSize(t *testing.T) {
	backend := &fakeBackend{product: "fake-product"}
	r := newRenderer(t, backend)

	product, err := r.Render(context.Background(), canvas.New(320, 240))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if product != "fake-product" {
		t.Errorf("产物透传不符: got %v", product)
	}
	assertOps(t, backend.ops, []string{"begin", "end"})
	if len(backend.begins) != 1 || backend.begins[0] != [2]int{320, 240} {
		t.Errorf("begin 应以画布尺寸建面: got %v", backend.begins)
	}
}

func TestRenderPaintsLayersInPriorityOrder(t *testing.T) {
	// priority 越大越先渲染(越垫底):B(5) 在 A(0) 之前绘制
	b := layer.NewImageLayer(layer.WithSize(20, 20), layer.WithPriority(5))
	a := layer.NewImageLayer(layer.WithSize(10, 10))
	backend := &fakeBackend{}
	r := newRenderer(t, backend)

	if _, err := r.Render(context.Background(), canvas.New(100, 100, a, b)); err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertOps(t, backend.ops, []string{"begin", "rect", "rect", "end"})
	if backend.rects[0].w != 20 || backend.rects[1].w != 10 {
		t.Errorf("绘制次序不符: 先 %d 后 %d, want 先 20(priority 5) 后 10(priority 0)",
			backend.rects[0].w, backend.rects[1].w)
	}
}

func TestRenderResolvesAnchorWithinCanvas(t *testing.T) {
	// center 锚点在 100×80 内解析为 (40, 35),加偏移 (5, 3) → 绝对 (45, 38)
	l := layer.NewImageLayer(layer.WithSize(20, 10), layer.WithPosition(5, 3, layer.AnchorCenter))
	backend := &fakeBackend{}
	r := newRenderer(t, backend)

	if _, err := r.Render(context.Background(), canvas.New(100, 80, l)); err != nil {
		t.Fatalf("Render: %v", err)
	}
	rect := backend.rects[0]
	if rect.x != 45 || rect.y != 38 || rect.w != 20 || rect.h != 10 {
		t.Errorf("rect = (%d, %d, %d, %d), want (45, 38, 20, 10)", rect.x, rect.y, rect.w, rect.h)
	}
}

func TestRenderLayerUsesLayerOwnSizeAsSurface(t *testing.T) {
	// 单图层便捷渲染:以图层自身尺寸为面
	l := layer.NewImageLayer(layer.WithSize(30, 20))
	backend := &fakeBackend{product: "fake-product"}
	r := newRenderer(t, backend)

	product, err := r.RenderLayer(context.Background(), l)
	if err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}
	if product != "fake-product" {
		t.Errorf("产物透传不符: got %v", product)
	}
	assertOps(t, backend.ops, []string{"begin", "rect", "end"})
	if backend.begins[0] != [2]int{30, 20} {
		t.Errorf("begin 应以图层尺寸建面: got %v", backend.begins[0])
	}
	if rect := backend.rects[0]; rect.x != 0 || rect.y != 0 || rect.w != 30 || rect.h != 20 {
		t.Errorf("rect = (%d, %d, %d, %d), want (0, 0, 30, 20)", rect.x, rect.y, rect.w, rect.h)
	}
}

// ---- 模板首步即物化 ----

func TestResolverRunsBeforeBegin(t *testing.T) {
	// 二维码图层需要物化而缝未接线:Render 在建面前即失败,原语零调用
	qr := layer.NewQrCodeLayer(layer.WithSize(30, 30), layer.WithQrText("payload"))
	backend := &fakeBackend{}
	r := renderer.New(backend, resolver.New(resolver.WithCacheRoot(t.TempDir())))

	_, err := r.Render(context.Background(), canvas.New(100, 100, qr))
	if !errors.Is(err, resolver.ErrQRMaterializerRequired) {
		t.Fatalf("err = %v, want ErrQRMaterializerRequired", err)
	}
	if len(backend.ops) != 0 {
		t.Errorf("物化失败后不应触达任何原语: got %v", backend.ops)
	}
}

// ---- 每类图层:先 drawRect(背景+边框)再内容 ----

func TestDrawRectReceivesBackgroundAndBorder(t *testing.T) {
	l := layer.NewImageLayer(
		layer.WithSize(50, 40),
		layer.WithBackground("#eee"),
		layer.WithBorder(2, "#f00"),
	)
	backend := &fakeBackend{}
	r := newRenderer(t, backend)

	if _, err := r.RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}
	rect := backend.rects[0]
	if rect.bg == nil || *rect.bg != "#eee" {
		t.Errorf("背景色透传不符: got %v, want #eee", rect.bg)
	}
	for name, side := range map[string]*layer.BorderSide{
		"top": rect.border.Top, "bottom": rect.border.Bottom,
		"left": rect.border.Left, "right": rect.border.Right,
	} {
		if side == nil || side.Width != 2 || side.Color != "#f00" {
			t.Errorf("%s 边框透传不符: got %+v, want {2 #f00}", name, side)
		}
	}
}

func TestImageLayerDrawsRectThenContentAtContentOrigin(t *testing.T) {
	// 内容区对齐起点(left/top → padding)后按内容尺寸放置
	l := layer.NewImageLayer(
		layer.WithSize(100, 100),
		layer.WithPadding(10),
		layer.WithHorizontalAlign(layer.AlignLeft),
		layer.WithVerticalAlign(layer.AlignTop),
		layer.WithPosition(2, 3),
		layer.WithImage("pic.png"),
	)
	l.SetResolvedSrc("pic-local.png")
	backend := &fakeBackend{}
	r := newRenderer(t, backend)

	if _, err := r.RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}
	assertOps(t, backend.ops, []string{"begin", "rect", "image", "end"})
	if rect := backend.rects[0]; rect.x != 2 || rect.y != 3 || rect.w != 100 || rect.h != 100 {
		t.Errorf("rect = (%d, %d, %d, %d), want (2, 3, 100, 100)", rect.x, rect.y, rect.w, rect.h)
	}
	img := backend.images[0]
	// 起点 = 原点 + padding(left/top 取 padding 值);尺寸 = 内容盒 80×80
	if img.src != "pic-local.png" || img.x != 12 || img.y != 13 || img.w != 80 || img.h != 80 {
		t.Errorf("image = (%s, %d, %d, %d, %d), want (pic-local.png, 12, 13, 80, 80)",
			img.src, img.x, img.y, img.w, img.h)
	}
}

func TestImageLayerWithoutReferenceSkipsDrawImage(t *testing.T) {
	// 引用为 nil 才跳过放置;仅设本地路径未物化时经 ResolvedSrc 回退原始引用直画
	// (对齐 PHP getResolvedSrc 的 `resolvedSrc ?? rawImg`)
	l := layer.NewImageLayer(layer.WithSize(50, 40))
	backend := &fakeBackend{}
	r := newRenderer(t, backend)

	if _, err := r.RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}
	assertOps(t, backend.ops, []string{"begin", "rect", "end"})

	// 本地路径零物化直画:引用即来源
	local := layer.NewImageLayer(layer.WithSize(50, 40), layer.WithImage("pic.png"))
	backend2 := &fakeBackend{}
	if _, err := newRenderer(t, backend2).RenderLayer(context.Background(), local); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}
	if img := backend2.images[0]; img.src != "pic.png" {
		t.Errorf("本地路径应直画原始引用: got %q", img.src)
	}
}

func TestTextLayerDrawsLinesWithLineHeightAccumulation(t *testing.T) {
	// 逐行绘制:起点 = padding + origin,行距按行高像素(ceil(12×1)=12)累加;
	// 桩断行器固定两行,垂直默认 bottom + autowrap → origin.y = 100-12×(2-1) = 88
	l := layer.NewTextLayer(
		layer.WithSize(20, 100),
		layer.WithAutowrap(true),
		layer.WithLineBreaker(stubBreaker{lines: []string{"aaa", "a"}}),
		layer.WithPosition(7, 9),
	)
	backend := &fakeBackend{}
	r := newRenderer(t, backend)

	if _, err := r.RenderLayer(context.Background(), l); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}
	assertOps(t, backend.ops, []string{"begin", "rect", "text", "text", "end"})

	if len(backend.texts) != 2 {
		t.Fatalf("text 调用数 = %d, want 2", len(backend.texts))
	}
	first, second := backend.texts[0], backend.texts[1]
	if first.line != "aaa" || first.x != 7 || first.y != 97 {
		t.Errorf("首行 = (%q, %d, %d), want (aaa, 7, 97)", first.line, first.x, first.y)
	}
	if second.line != "a" || second.x != 7 || second.y != 109 {
		t.Errorf("次行 = (%q, %d, %d), want (a, 7, 109)", second.line, second.x, second.y)
	}
	// 字体参数逐项透传:默认字号 12、色 #000000、水平 left、垂直 bottom、角度 0
	for i, tc := range []textCall{first, second} {
		if tc.font != "" || tc.fontSize != 12 || tc.fontColor != "#000000" ||
			tc.horizontalAlign != "left" || tc.verticalAlign != "bottom" || tc.angle != 0 {
			t.Errorf("第 %d 行字体参数不符: %+v", i+1, tc)
		}
	}
}

func TestQrCodeLayerPlacedSquareByWidth(t *testing.T) {
	// 二维码按宽度正方形铺放,忽略声明高度 50
	qr := layer.NewQrCodeLayer(layer.WithSize(30, 50), layer.WithPosition(1, 1))
	qr.SetResolvedSrc("/tmp/qr.png")
	backend := &fakeBackend{}
	r := newRenderer(t, backend)

	if _, err := r.RenderLayer(context.Background(), qr); err != nil {
		t.Fatalf("RenderLayer: %v", err)
	}
	assertOps(t, backend.ops, []string{"begin", "rect", "image", "end"})
	if rect := backend.rects[0]; rect.x != 1 || rect.y != 1 || rect.w != 30 || rect.h != 50 {
		t.Errorf("rect = (%d, %d, %d, %d), want (1, 1, 30, 50)", rect.x, rect.y, rect.w, rect.h)
	}
	if img := backend.images[0]; img.src != "/tmp/qr.png" || img.x != 1 || img.y != 1 || img.w != 30 || img.h != 30 {
		t.Errorf("image = (%s, %d, %d, %d, %d), want (/tmp/qr.png, 1, 1, 30, 30)",
			img.src, img.x, img.y, img.w, img.h)
	}
}

// ---- 容器下钻:表按行高纵向累加、行按单元格宽横向累加、单元格下钻内容层 ----

func TestTableContainerDrillDown(t *testing.T) {
	// 表 100×60 @ (10,10):
	//   行1(高 30)@ y=10:单元格1 40×20、单元格2 60×30(含内容层)
	//   行2(高 25,带偏移 (0,5))@ y=40+5:单元格3 50×25
	content := layer.NewImageLayer()
	cell2 := layer.NewTableCellLayer(layer.WithSize(60, 30), layer.WithContent(content))
	row1 := layer.NewTableRowLayer(layer.WithCells(
		layer.NewTableCellLayer(layer.WithSize(40, 20)),
		cell2,
	))
	row2 := layer.NewTableRowLayer(
		layer.WithPosition(0, 5),
		layer.WithCells(layer.NewTableCellLayer(layer.WithSize(50, 25))),
	)
	table := layer.NewTableLayer(
		layer.WithSize(100, 60),
		layer.WithBackground("#eee"),
		layer.WithPosition(10, 10),
		layer.WithRows(row1, row2),
	)
	backend := &fakeBackend{}
	r := newRenderer(t, backend)

	if _, err := r.Render(context.Background(), canvas.New(200, 200, table)); err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertOps(t, backend.ops, []string{"begin", "rect", "rect", "rect", "rect", "rect", "rect", "rect", "end"})

	type expect struct{ x, y, w, h int }
	// 行按行高纵向累加、单元格按宽度横向累加、内容层相对单元格原点直画;
	// 行2的 (0,5) 偏移证明容器内图层同样经锚点定位,且累加不受偏移影响
	want := []expect{
		{10, 10, 100, 60}, // 表
		{10, 10, 100, 30}, // 行1(行高取最高单元格 30,行宽同步表宽)
		{10, 10, 40, 20},  // 单元格1
		{50, 10, 60, 30},  // 单元格2(x 累加单元格1 宽 40)
		{50, 10, 60, 30},  // 单元格2 的内容层(宽被同步为单元格宽、高压平为单元格高)
		{10, 45, 100, 25}, // 行2(y 累加行1 高 30,再叠加偏移 5)
		{10, 45, 50, 25},  // 单元格3
	}
	if len(backend.rects) != len(want) {
		t.Fatalf("rect 调用数 = %d, want %d", len(backend.rects), len(want))
	}
	for i, e := range want {
		got := backend.rects[i]
		if got.x != e.x || got.y != e.y || got.w != e.w || got.h != e.h {
			t.Errorf("rect[%d] = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
				i, got.x, got.y, got.w, got.h, e.x, e.y, e.w, e.h)
		}
	}
	if backend.rects[0].bg == nil || *backend.rects[0].bg != "#eee" {
		t.Errorf("表背景透传不符: got %v", backend.rects[0].bg)
	}
}

// ---- 分派守卫:未知图层类型 ----

type alienLayer struct{}

func (alienLayer) TypeName() string  { return "AlienLayer" }
func (alienLayer) Priority() int     { return 0 }
func (alienLayer) Graph() layer.Node { return layer.Node{Type: "AlienLayer"} }

func TestUnknownLayerTypeAbortsRender(t *testing.T) {
	backend := &fakeBackend{}
	r := newRenderer(t, backend)

	_, err := r.Render(context.Background(), canvas.New(100, 100, alienLayer{}))
	if !errors.Is(err, layer.ErrUnknownLayerType) {
		t.Fatalf("err = %v, want ErrUnknownLayerType", err)
	}
	assertOps(t, backend.ops, []string{"begin"})
}

func TestRenderLayerUnknownLayerTypeAborts(t *testing.T) {
	backend := &fakeBackend{}
	r := newRenderer(t, backend)

	if _, err := r.RenderLayer(context.Background(), alienLayer{}); !errors.Is(err, layer.ErrUnknownLayerType) {
		t.Fatalf("err = %v, want ErrUnknownLayerType", err)
	}
	if len(backend.ops) != 0 {
		t.Errorf("未知类型不应触达任何原语: got %v", backend.ops)
	}
}

// ---- 错误传播 ----

func TestBackendErrorsAbortRender(t *testing.T) {
	drawRectErr := errors.New("绘制失败")
	l := layer.NewImageLayer(layer.WithSize(50, 40))
	backend := &fakeBackend{drawRectErr: drawRectErr}
	r := newRenderer(t, backend)

	_, err := r.RenderLayer(context.Background(), l)
	if !errors.Is(err, drawRectErr) {
		t.Fatalf("err = %v, want 绘制失败", err)
	}
	assertOps(t, backend.ops, []string{"begin", "rect"})
}

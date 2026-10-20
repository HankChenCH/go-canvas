package renderer_test

// 多页 walk 模板(工票 01,canvas-compiler 工票 07「AbstractRenderer 多页钩子」
// 的 Go 同构物):每页 建面→背景铺装(ADR 0022 逐页显式携带)→逐层 paint,
// 全部页完成后单文档统一 End 一次;页间新增一处 ctx 检查点(下一页 Begin 前,
// ADR 0008),页内既有根层每图层前/表行每行前检查点不动;任一页任一原语/物化
// 失败即 error、End 不达、整个文档无产物逃逸。
// 对拍基准 = 本包 renderSurface 单页编排(存量测试保绿 = 单页路径零行为变化)
// 与 php-canvas-next 多页钩子契约(canvas-compiler 工票 07)。

import (
	"context"
	"errors"
	"fmt"
	"image"
	"path/filepath"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/renderer"
	"github.com/HankChenCH/go-canvas/resolver"
)

// failFromPageBackend 页感知失败注入:Begin 计页(1 起),第 fromPage 页
// (1 起)起 DrawRect 一律报 err——驱动「第 k 页绘制失败」的确定性注入
type failFromPageBackend struct {
	*fakeBackend
	fromPage int
	page     int
	err      error
}

func (b *failFromPageBackend) Begin(w, h int) error {
	b.page++
	return b.fakeBackend.Begin(w, h)
}

func (b *failFromPageBackend) DrawRect(x, y, w, h int, bg *string, border layer.Border) error {
	if b.page >= b.fromPage {
		return b.err
	}
	return b.fakeBackend.DrawRect(x, y, w, h, bg, border)
}

// walkOps 剥除建面/收尾哨兵,取页内 walk 原语序(多页同构断言用)
func walkOps(ops []string) []string {
	out := make([]string, 0, len(ops))
	for _, op := range ops {
		if op != "begin" && op != "end" {
			out = append(out, op)
		}
	}
	return out
}

// rectKeys rect 调用的可比投影(坐标 + 背景色值):画布逐次新建,背景色
// *string 指针地址不同但指值相同,直接 fmt.Sprint 结构体必然假阳性
func rectKeys(rects []rectCall) []string {
	out := make([]string, 0, len(rects))
	for _, r := range rects {
		bg := "<nil>"
		if r.bg != nil {
			bg = *r.bg
		}
		out = append(out, fmt.Sprintf("(%d,%d,%d,%d,%s)", r.x, r.y, r.w, r.h, bg))
	}
	return out
}

// ---- 编排契约:n 页 = n 次 Begin + 1 次 End,页序即入参序 ----

func TestRenderPagesBeginsEachPageEndsOnce(t *testing.T) {
	// 两页两尺寸:每页以自身画布尺寸建面,End 全部页完成后恰好一次
	p1 := canvas.New(320, 240, layer.NewImageLayer(layer.WithSize(20, 10)))
	p2 := canvas.New(100, 80, layer.NewImageLayer(layer.WithSize(30, 20)))
	backend := &fakeBackend{product: "fake-product"}

	product, err := renderer.RenderPages(context.Background(), backend, []*canvas.Canvas{p1, p2})
	if err != nil {
		t.Fatalf("RenderPages: %v", err)
	}
	if product != "fake-product" {
		t.Errorf("产物应透传 End 返回值: got %v", product)
	}
	assertOps(t, backend.ops, []string{"begin", "rect", "begin", "rect", "end"})
	wantBegins := [][2]int{{320, 240}, {100, 80}}
	if fmt.Sprint(backend.begins) != fmt.Sprint(wantBegins) {
		t.Errorf("逐页建面尺寸不符: got %v, want %v", backend.begins, wantBegins)
	}
}

// ---- 背景逐页铺装(ADR 0022:分页页 canvas 背景显式携带,每页各自画布背景)----

func TestRenderPagesPaintsBackgroundPerPage(t *testing.T) {
	// 每页先铺自己的背景(先色后图)再走图层;第二页色+图,第一页仅色
	p1 := canvas.New(320, 240, layer.NewImageLayer(layer.WithSize(20, 10)))
	p1.SetBackground(canvas.NewCanvasBackground(strp("#ffffff"), nil))
	p2 := canvas.New(100, 80)
	p2.SetBackground(canvas.NewCanvasBackground(strp("#eee"), strp("/tmp/p2-bg.png")))
	backend := &fakeBackend{}

	if _, err := renderer.RenderPages(context.Background(), backend, []*canvas.Canvas{p1, p2}); err != nil {
		t.Fatalf("RenderPages: %v", err)
	}

	assertOps(t, backend.ops, []string{"begin", "rect", "rect", "begin", "rect", "image", "end"})
	// 第 1 页:背景色整幅铺装,无边框
	bg1 := backend.rects[0]
	if bg1.x != 0 || bg1.y != 0 || bg1.w != 320 || bg1.h != 240 ||
		bg1.bg == nil || *bg1.bg != "#ffffff" || bg1.border != (layer.Border{}) {
		t.Errorf("第 1 页背景色铺装不符: %+v", bg1)
	}
	// 第 2 页:先色后图;/tmp/p2-bg.png 不在盘上,内在尺寸读取失败退化 fill
	// 形态,srcRect 走零哨兵、dstRect 铺满本页画布矩形
	bg2 := backend.rects[2]
	if bg2.x != 0 || bg2.y != 0 || bg2.w != 100 || bg2.h != 80 ||
		bg2.bg == nil || *bg2.bg != "#eee" {
		t.Errorf("第 2 页背景色铺装不符: %+v", bg2)
	}
	img := backend.images[0]
	if img.src != "/tmp/p2-bg.png" || !img.srcRect.Empty() ||
		img.dstRect != (image.Rect(0, 0, 100, 80)) {
		t.Errorf("第 2 页背景图绘制 = %+v, want (/tmp/p2-bg.png, 零哨兵, (0,0)-(100,80))", img)
	}
}

// ---- 页内 walk 与单页 renderSurface 同构 ----

func TestRenderPagesWalkIsomorphicToSinglePage(t *testing.T) {
	// 多页第 i 页的原语序与坐标 = 单独 Render 第 i 画布(begin/end 剥除后逐项相等);
	// 页 1 覆盖表容器下钻(表→行→单元格→内容层)+ 文本行,页 2 覆盖普通图层
	buildTableCanvas := func() *canvas.Canvas {
		content := layer.NewImageLayer(layer.WithSize(30, 20), layer.WithBackground("#0f0"))
		row1 := layer.NewTableRowLayer(layer.WithCells(
			layer.NewTableCellLayer(layer.WithSize(50, 20)),
			layer.NewTableCellLayer(layer.WithSize(50, 20), layer.WithContent(content)),
		))
		row2 := layer.NewTableRowLayer(layer.WithCells(
			layer.NewTableCellLayer(layer.WithSize(100, 15)),
		))
		table := layer.NewTableLayer(layer.WithSize(100, 35), layer.WithRows(row1, row2))
		text := layer.NewTextLayer(
			layer.WithSize(40, 12),
			layer.WithLineBreaker(stubBreaker{lines: []string{"a", "b"}}),
		)
		return canvas.New(200, 100, table, text)
	}
	buildPlainCanvas := func() *canvas.Canvas {
		return canvas.New(80, 60, layer.NewImageLayer(layer.WithSize(20, 10), layer.WithPosition(2, 3)))
	}

	single1 := &fakeBackend{}
	if _, err := newRenderer(t, single1).Render(context.Background(), buildTableCanvas()); err != nil {
		t.Fatalf("单页 Render(表画布): %v", err)
	}
	single2 := &fakeBackend{}
	if _, err := newRenderer(t, single2).Render(context.Background(), buildPlainCanvas()); err != nil {
		t.Fatalf("单页 Render(普通画布): %v", err)
	}

	multi := &fakeBackend{}
	pages := []*canvas.Canvas{buildTableCanvas(), buildPlainCanvas()}
	if _, err := renderer.RenderPages(context.Background(), multi, pages); err != nil {
		t.Fatalf("RenderPages: %v", err)
	}

	wantOps := append([]string{"begin"}, walkOps(single1.ops)...)
	wantOps = append(wantOps, "begin")
	wantOps = append(wantOps, walkOps(single2.ops)...)
	wantOps = append(wantOps, "end")
	assertOps(t, multi.ops, wantOps)
	if fmt.Sprint(rectKeys(multi.rects)) != fmt.Sprint(append(append([]string{}, rectKeys(single1.rects)...), rectKeys(single2.rects)...)) {
		t.Errorf("逐页 rect 坐标序与单页不同构:\n multi %v\n single %v + %v",
			rectKeys(multi.rects), rectKeys(single1.rects), rectKeys(single2.rects))
	}
	if fmt.Sprint(multi.texts) != fmt.Sprint(append(append([]textCall{}, single1.texts...), single2.texts...)) {
		t.Errorf("逐页 text 序与单页不同构:\n multi %v\n single %v + %v",
			multi.texts, single1.texts, single2.texts)
	}
}

func TestRenderPagesSkipsHiddenRootLayersPerPage(t *testing.T) {
	// 显隐过滤逐页进行(隐藏 = 最终输出排除,与单页 Render 同语义):
	// 首页空、次页可见+隐藏各一,隐藏层不触达任何原语
	visible := layer.NewImageLayer(layer.WithSize(40, 20), layer.WithBackground("#0f0"))
	hidden := layer.NewImageLayer(layer.WithSize(40, 20), layer.WithBackground("#f00"))
	hidden.SetVisible(false)
	p1 := canvas.New(40, 20)
	p2 := canvas.New(40, 20, visible, hidden)
	backend := &fakeBackend{}

	if _, err := renderer.RenderPages(context.Background(), backend, []*canvas.Canvas{p1, p2}); err != nil {
		t.Fatalf("RenderPages: %v", err)
	}
	assertOps(t, backend.ops, []string{"begin", "begin", "rect", "end"})
	if backend.rects[0].bg == nil || *backend.rects[0].bg != "#0f0" {
		t.Errorf("只应绘制可见层: got bg %v", backend.rects[0].bg)
	}
}

// ---- 失败语义:任一页失败即 error、End 不达、整个文档无产物逃逸 ----

func TestRenderPagesPageFailureDropsWholeDocument(t *testing.T) {
	// 第 2 页首笔绘制失败:第 1 页已画内容一并作废,End 未调用
	errBoom := errors.New("第二页绘制失败")
	p1 := canvas.New(100, 100, layer.NewImageLayer(layer.WithSize(20, 10), layer.WithBackground("#0f0")))
	p2 := canvas.New(80, 60, layer.NewImageLayer(layer.WithSize(20, 10), layer.WithBackground("#f00")))
	backend := &failFromPageBackend{fakeBackend: &fakeBackend{}, fromPage: 2, err: errBoom}

	product, err := renderer.RenderPages(context.Background(), backend, []*canvas.Canvas{p1, p2})
	if err == nil {
		t.Fatal("第 2 页绘制失败必须报错")
	}
	if !errors.Is(err, errBoom) {
		t.Errorf("错误 = %v, want 注入的绘制失败", err)
	}
	if product != nil {
		t.Errorf("失败渲染必须无产物, got %v", product)
	}
	// 第 1 页完整走完(begin+图层盒),第 2 页建面后首笔即失败
	assertOps(t, backend.ops, []string{"begin", "rect", "begin"})
}

// ---- 页间检查点(本票唯一新增检查点):下一页 Begin 前掐断 ----

func TestRenderPagesCancelArmedOnPageCutAtInterPageCheckpoint(t *testing.T) {
	// 第 1 页最后一笔绘制武装取消:页内检查点已全部通过,下一处检查点即页间
	// (第二页 Begin 前)——页间检查点缺失时本测试必失败(第二页照画且 End 达)
	p1 := canvas.New(100, 100, layer.NewImageLayer(layer.WithSize(20, 10)))
	p2 := canvas.New(80, 60, layer.NewImageLayer(layer.WithSize(20, 10)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend := &cancelAfterRectBackend{fakeBackend: &fakeBackend{}, cancel: cancel, afterN: 1}

	_, err := renderer.RenderPages(ctx, backend, []*canvas.Canvas{p1, p2})
	if err == nil {
		t.Fatal("页间取消必须报错")
	}
	if !errors.Is(err, resolver.ErrRenderCancelled) {
		t.Errorf("错误 = %v, want ErrRenderCancelled", err)
	}
	// 第二页 Begin 未达:检查点先于下一页建面
	if len(backend.begins) != 1 {
		t.Errorf("页间检查点应先于第二页 Begin,Begin 次数 = %d, want 1", len(backend.begins))
	}
	for _, op := range backend.ops {
		if op == "end" {
			t.Error("渲染面必须丢弃(end 不得调用)")
		}
	}
}

func TestRenderPagesExpiredDeadlineCutsAtInterPageCheckpoint(t *testing.T) {
	// 首页为空画布(无背景无图层 → 页内零检查点):页间检查点是首个掐断点,
	// 证明「空页之后、下一页 Begin 之前」的检查点真实存在
	p1 := canvas.New(100, 100)
	p2 := canvas.New(80, 60, layer.NewImageLayer(layer.WithSize(20, 10)))
	backend := &fakeBackend{}
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	_, err := renderer.RenderPages(ctx, backend, []*canvas.Canvas{p1, p2})
	if err == nil {
		t.Fatal("页间超时必须报错")
	}
	if !errors.Is(err, resolver.ErrRenderDeadlineExceeded) {
		t.Errorf("错误 = %v, want ErrRenderDeadlineExceeded", err)
	}
	if len(backend.begins) != 1 {
		t.Errorf("页间检查点应先于第二页 Begin,Begin 次数 = %d, want 1", len(backend.begins))
	}
	for _, op := range backend.ops {
		if op == "end" {
			t.Error("渲染面必须丢弃(end 不得调用)")
		}
	}
}

// ---- 入参守卫与组装路径 ----

func TestRenderPagesEmptyPageSequenceIsUsageError(t *testing.T) {
	// 空页序列属调用侧用法错误(零页文档无意义,渲染成功之后/之前的用法错
	// 不设 code):报错且不触达任何原语——「0 次 Begin + 1 次 End」的空文档
	// 语义不由模板代答
	backend := &fakeBackend{}
	_, err := renderer.RenderPages(context.Background(), backend, nil)
	if err == nil {
		t.Fatal("空页序列必须报错")
	}
	if len(backend.ops) != 0 {
		t.Errorf("空页序列不应触达任何原语: got %v", backend.ops)
	}
}

func TestRenderPagesPrefersBackendCarriedDefaultResolver(t *testing.T) {
	// 复用 New(backend, nil) 模板路径(nil resolver → 默认物化器,后端自带优先,
	// 与 RenderAs 同款):工票 03 的 PDF 后端以自带物化器经本入口获得完整管线
	root := t.TempDir()
	carried := resolver.New(
		resolver.WithDownloader(stubDownloader{content: []byte("png-bytes")}),
		resolver.WithCacheRoot(root),
	)
	backend := &selfResolvingBackend{
		fakeBackend: fakeBackend{product: "fake-product"},
		carried:     carried,
	}
	l := layer.NewImageLayer(layer.WithSize(10, 10), layer.WithImage("https://example.com/a.png"))
	p1 := canvas.New(10, 10, l)
	p2 := canvas.New(10, 10)

	product, err := renderer.RenderPages(context.Background(), backend, []*canvas.Canvas{p1, p2})
	if err != nil {
		t.Fatalf("RenderPages: %v", err)
	}
	if product != "fake-product" {
		t.Errorf("产物应透传 End 返回值: got %v", product)
	}
	if l.ResolvedSrc() == nil {
		t.Fatalf("后端自带默认物化器未被采用(远程图片未物化)")
	}
	if filepath.Dir(*l.ResolvedSrc()) != filepath.Join(root, "img_layers") {
		t.Fatalf("物化结果 %s 不在后端自带物化器的缓存目录下", *l.ResolvedSrc())
	}
	assertOps(t, backend.ops, []string{"begin", "rect", "image", "begin", "end"})
}

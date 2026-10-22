// Package renderer 渲染契约与模板:后端作者只实现五个绘制原语(Backend)即可
// 接入完整渲染管线——资源物化、遍历、分派、容器下钻、锚点定位全由模板完成
// (M1 功能闭环的最后一环)。模板以后端为主动方向画布拉取结构树,所有图层
// 直画同一渲染面,坐标经参数传递(无栈)。
package renderer

import (
	"context"
	"fmt"
	"image"
	"os"

	// 内在尺寸探测按格式注册(ADR 0024 全 fit 前置):与位图后端解码集一致,
	// stdlib 三格式——核心 module 纯 stdlib 红线不变,其余格式按读取失败退化 fill 形态
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/resolver"
)

// Renderer 渲染器契约:渲染管线对使用方的公开面(对应 PHP RendererInterface,
// 另含单图层便捷渲染)。产物类型由后端决定
type Renderer interface {
	// Render 渲染整棵结构树:begin(画布尺寸) → 按 priority 序逐层 paint
	// (绘制分派前按需物化,spec §4.3) → end;失败即抛、渲染面丢弃、无产物逃逸
	Render(ctx context.Context, c *canvas.Canvas) (any, error)

	// RenderLayer 便捷:以图层自身尺寸为渲染面渲染单个图层
	RenderLayer(ctx context.Context, l layer.Layer) (any, error)
}

// paintable paint 分派所需的图层结构面:六种内置图层经嵌入 base 全部满足;
// 模板只消费该最小面,与 PHP paint(AbstractLayer $layer) 的形参角色对应
type paintable interface {
	layer.Layer
	Position() (string, int, int)
	Width() int
	Height() int
}

// template 渲染模板:通用逻辑(建面 → 遍历/分派 → 容器下钻 → 收尾)。
// 渲染器无预遍历,资源物化收敛到三个叶子 paint 分支开头按需发生
// (spec §4.3 渲染期物化,PHP AbstractRenderer 同款;ResourceResolver 逻辑
// 不变、只改调用时机——全画布预遍历 API 保留但渲染器不再调用)。
// Backend 与物化器均为非导出字段——绘制原语只能经 Render/RenderLayer 模板流程
// 触达,无法绕过遍历/定位/下钻逻辑直接调用
type template struct {
	backend  Backend
	resolver *resolver.ResourceResolver
}

// defaultResolverSource 后端可选实现:自带默认物化器的后端经此暴露其接线。
// 位图后端的二维码缝默认接线属 image-renderer module 的知识(ADR-0002 核心
// 纯 stdlib,核心默认物化器 resolver.New() 无二维码实现)——New 以 nil
// resolver 组装时优先采用后端自带物化器,后端未实现或返回 nil 回退核心默认
type defaultResolverSource interface {
	DefaultResolver() *resolver.ResourceResolver
}

// New 以绘制原语组装渲染器。resolver 为 nil 时取默认物化器:后端自带
// (defaultResolverSource)优先,否则 resolver.New()
// (对齐 PHP ImageRenderer 的 `$resolver ?? new ResourceResolver()`)
func New(backend Backend, rs *resolver.ResourceResolver) Renderer {
	return newTemplate(backend, rs)
}

// newTemplate 模板组装(New 的实体构造,供包内多入口共享 nil-resolver 归一逻辑)
func newTemplate(backend Backend, rs *resolver.ResourceResolver) *template {
	if rs == nil {
		if src, ok := backend.(defaultResolverSource); ok {
			rs = src.DefaultResolver()
		}
		if rs == nil {
			rs = resolver.New()
		}
	}
	return &template{backend: backend, resolver: rs}
}

// Render implements Renderer:建面 → 背景铺装 → 逐层 paint(绘制分派前按需物化)→ 收尾。
// 失败即抛、渲染面丢弃、无产物逃逸(begin 已调用但 end 不达)。
// 根层渲染循环跳过 visible=false(layer-panel-ux 工单 01,隐藏=最终输出排除):
// 过滤在建面循环之前,隐藏层的绘制分派与惰性物化均不发生。
// 背景只随 Render(画布)走(ADR 0022 / spec §4 Q9):RenderLayer 无画布矩形语义不画
func (r *template) Render(ctx context.Context, c *canvas.Canvas) (any, error) {
	return r.renderSurface(ctx, c.Width(), c.Height(), visibleLayers(c), c.Background())
}

// visibleLayers 画布根层显隐过滤(layer-panel-ux 工单 01,隐藏=最终输出排除):
// Render 与 renderPages 共享的画布渲染循环前置段,过滤先于建面与任何绘制分派
func visibleLayers(c *canvas.Canvas) []layer.Layer {
	layers := c.GetLayers()
	visible := make([]layer.Layer, 0, len(layers))
	for _, l := range layers {
		if l.Visible() {
			visible = append(visible, l)
		}
	}
	return visible
}

// RenderLayer implements Renderer:以其自身尺寸建面,物化时机同 Render(惰性)。
// 不做显隐过滤——visible 语义属画布渲染循环,单图层渲染所见即所得;不画背景
func (r *template) RenderLayer(ctx context.Context, l layer.Layer) (any, error) {
	p, ok := l.(paintable)
	if !ok {
		return nil, fmt.Errorf("%w: %T", layer.ErrUnknownLayerType, l)
	}
	return r.renderSurface(ctx, p.Width(), p.Height(), []layer.Layer{l}, nil)
}

// renderSurface 建面 → 背景铺装 → 逐层 paint → 收尾(单页编排:页内段 +
// End 收尾两段式,页内段与多页 renderPages 共享);
// 任一原语/物化报错即中止(渲染面丢弃)。
// 根层循环每图层前设控制面检查点(与 php render 根层循环同位,工票 03):
// ctx 到期/取消报渲染 code,先于下一层的绘制分派与惰性物化
func (r *template) renderSurface(ctx context.Context, width, height int, layers []layer.Layer, background *canvas.CanvasBackground) (any, error) {
	if err := r.paintPage(ctx, width, height, layers, background); err != nil {
		return nil, err
	}
	return r.backend.End(), nil
}

// paintPage 单页段:建面 → 背景铺装 → 逐层 paint(不含 End 收尾)。
// 单页 renderSurface 与多页 renderPages 共享的页内编排——多页是本段的页循环
// 外扩(canvas-compiler 工票 07「每页一次 begin/walk,单文档统一 end」);
// 根层循环每图层前设控制面检查点(与 php render 根层循环同位,ADR 0008):
// ctx 到期/取消报渲染 code,先于下一层的绘制分派与惰性物化
func (r *template) paintPage(ctx context.Context, width, height int, layers []layer.Layer, background *canvas.CanvasBackground) error {
	if err := r.backend.Begin(width, height); err != nil {
		return err
	}
	if err := r.paintBackground(ctx, background, width, height); err != nil {
		return err
	}
	for _, l := range layers {
		if err := resolver.CheckContext(ctx); err != nil {
			return err
		}
		if err := r.paint(ctx, l, 0, 0, width, height); err != nil {
			return err
		}
	}
	return nil
}

// renderPages 多页编排:逐页复用单页段(建面→背景铺装→逐层 paint),全部页
// 完成后单文档统一 End 一次(canvas-compiler 工票 07 的 Go 同构物,工票 01)。
// ctx 检查点页间一处新增(下一页 Begin 前,ADR 0008)——首页前不另设检查点,
// 与单页 Render 同构;页内既有根层每图层前/表行每行前检查点不动。
// 显隐过滤逐页进行(与 Render 同语义);任一页任一原语/物化失败即中止,
// End 不达、整个文档无产物逃逸
func (r *template) renderPages(ctx context.Context, pages []*canvas.Canvas) (any, error) {
	if len(pages) == 0 {
		// 空页序列属调用侧用法错误(零页文档无意义,不设 code):不触达任何原语,
		// 「0 次 Begin + 1 次 End」的空文档语义不由模板代答
		return nil, fmt.Errorf("RenderPages: 页序列为空,至少需要一页")
	}
	for i, c := range pages {
		if i > 0 {
			// 页间检查点(ADR 0008,本票唯一新增):先于下一页的建面与背景物化
			// 掐断已取消/到期的 ctx,报渲染稳定 code
			if err := resolver.CheckContext(ctx); err != nil {
				return nil, err
			}
		}
		if err := r.paintPage(ctx, c.Width(), c.Height(), visibleLayers(c), c.Background()); err != nil {
			return nil, err
		}
	}
	return r.backend.End(), nil
}

// paintBackground 背景铺装(ADR 0022 / spec §4):根层循环之前的唯一插入点,先色后图——
// 背景之下无物(画布初始透明保留),背景图透明区露背景色。铺色复用 DrawRect 原语
// (无边框),铺图复用 DrawImage 原语;背景图画布级惰性物化(绘制分派前、失败即抛、
// 产物整体丢弃),无背景零开销。铺装恒 cover(ADR 0022):分派点读内在尺寸算等比
// 居中裁窗(与图片臂共用同一公式 CoverSourceWindow,ADR 0024)后 blit 进画布矩形,
// 读取失败统一退化 fill 形态(真损坏图在后端解码抛,结局 parity)
func (r *template) paintBackground(ctx context.Context, background *canvas.CanvasBackground, width, height int) error {
	if background == nil {
		return nil
	}

	if color := background.Color; color != nil {
		if err := r.backend.DrawRect(0, 0, width, height, color, layer.Border{}); err != nil {
			return err
		}
	}

	bgImage := background.Image
	if bgImage == nil {
		return nil
	}
	src, err := r.resolver.ResolveBackground(ctx, *bgImage)
	if err != nil {
		return err
	}
	dst := image.Rect(0, 0, width, height)
	if iw, ih, ok := intrinsicSize(src); ok {
		return r.backend.DrawImage(src, layer.CoverSourceWindow(width, height, iw, ih), dst)
	}
	return r.backend.DrawImage(src, image.Rectangle{}, dst)
}

// paint 在绝对坐标 (originX, originY) 处按图层自身定位设定绘制:
// 九锚点相对父盒 parentWidth×parentHeight 解析,锚点偏移 + 定位偏移 = 绝对坐标;
// instanceof 分派移植为类型 switch,未知类型报错
func (r *template) paint(ctx context.Context, l layer.Layer, originX, originY, parentWidth, parentHeight int) error {
	p, ok := l.(paintable)
	if !ok {
		return fmt.Errorf("%w: %T", layer.ErrUnknownLayerType, l)
	}

	anchor, x, y := p.Position()
	anchorX, anchorY := ResolveAnchor(anchor, parentWidth, parentHeight, p.Width(), p.Height())

	absX := originX + anchorX + x
	absY := originY + anchorY + y

	switch t := p.(type) {
	case *layer.TableLayer:
		return r.paintTable(ctx, t, absX, absY)
	case *layer.TableRowLayer:
		return r.paintRow(ctx, t, absX, absY)
	case *layer.TableCellLayer:
		return r.paintCell(ctx, t, absX, absY)
	case *layer.ImageLayer:
		return r.paintImage(ctx, t, absX, absY)
	case *layer.TextLayer:
		return r.paintText(ctx, t, absX, absY)
	case *layer.QrCodeLayer:
		return r.paintQrCode(ctx, t, absX, absY)
	default:
		return fmt.Errorf("%w: %T", layer.ErrUnknownLayerType, l)
	}
}

// paintTable 表容器:先画自身盒,再按行高累加纵向排布各行
// (行锚点相对表盒解析;全部直画同一渲染面,坐标经参数传递)。
// 行循环每行前设控制面检查点(与 php paintTable 同位,工票 03)——大表是
// 真实长杆,行间逐行掐断
func (r *template) paintTable(ctx context.Context, l *layer.TableLayer, x, y int) error {
	if err := r.backend.DrawRect(x, y, l.Width(), l.Height(), l.Background(), l.Border()); err != nil {
		return err
	}

	posy := y
	for _, row := range l.Rows() {
		if err := resolver.CheckContext(ctx); err != nil {
			return err
		}
		if err := r.paint(ctx, row, x, posy, l.Width(), l.Height()); err != nil {
			return err
		}
		posy += row.Height()
	}
	return nil
}

// paintRow 行容器:先画自身盒,再按单元格宽累加横向排布各单元格
func (r *template) paintRow(ctx context.Context, l *layer.TableRowLayer, x, y int) error {
	if err := r.backend.DrawRect(x, y, l.Width(), l.Height(), l.Background(), l.Border()); err != nil {
		return err
	}

	posx := x
	for _, cell := range l.Cells() {
		if err := r.paint(ctx, cell, posx, y, l.Width(), l.Height()); err != nil {
			return err
		}
		posx += cell.Width()
	}
	return nil
}

// paintCell 单元格:先画自身盒,再下钻内容层(内容层锚点相对单元格盒解析)
func (r *template) paintCell(ctx context.Context, l *layer.TableCellLayer, x, y int) error {
	if err := r.backend.DrawRect(x, y, l.Width(), l.Height(), l.Background(), l.Border()); err != nil {
		return err
	}

	if content := l.ContentLayer(); content != nil {
		return r.paint(ctx, content, x, y, l.Width(), l.Height())
	}
	return nil
}

// paintImage 图片臂全 fit 分派(ADR 0024 / spec §4):原语是双矩形 blit、零 fit 语义,
// 四档 fit 的几何全部在此算定(白名单分派,未知值落 cover 臂)——
// cover 等比居中裁窗 / fill 整图(与 cover 同门 dstRect,唯一差别在 srcRect)/
// contain 收缩(ADR 0023 语义不动)/ tile 可见块逐块 blit。
// clip 声明(ADR 0026)改写绘制形态:circle 档且后端实现形状能力口走圆形三步
// (背景填进内切圆、内容按 fit 绘出后遮罩 blit、边框沿圆周描环),矩形盒
// DrawRect/DrawImage 路径不参与;未知 clip 值与后端缺能力口降级矩形
// (fit 未知值纪律同门)。
// 内在尺寸读取为全 fit 前置:绘制分派点一次(image.DecodeConfig 属绘制分派期 I/O,
// 只读头块,与 resolve 同批语义;注册集 PNG/JPEG/GIF 与位图后端解码集一致),
// 读取失败统一退化 fill 形态——真损坏图在后端解码处照旧报错,渲染结局与迁移前 parity。
// src 取 ResolvedSrc(物化结果优先,回退原始引用——本地路径零物化直画),引用为 nil 时只画盒
func (r *template) paintImage(ctx context.Context, l *layer.ImageLayer, x, y int) error {
	if err := r.resolver.ResolveLayer(ctx, l); err != nil {
		return err
	}

	// 圆形档(ADR 0026):clip 白名单 + 后端能力口探测双门,否则矩形路径照旧
	if l.Clip() == layer.ClipCircle {
		if sb, ok := r.backend.(ShapeCapableBackend); ok {
			return r.paintImageCircle(sb, l, x, y)
		}
	}

	if err := r.backend.DrawRect(x, y, l.Width(), l.Height(), l.Background(), l.Border()); err != nil {
		return err
	}

	src := l.ResolvedSrc()
	if src == nil {
		return nil
	}

	// 矩形路径:fit 分派层算定几何、DrawImage 原语直绘
	return r.paintFitBlits(l, *src, x, y, func(src string, srcRect, dstRect image.Rectangle) error {
		return r.backend.DrawImage(src, srcRect, dstRect)
	})
}

// paintImageCircle 圆形档三步(ADR 0026,完整圆形框——整层渲染面裁进内切圆):
// ① 背景色填充进内切圆(矩形盒 DrawRect 不画——背景/边框都不落矩形盒);
// ② 内容按 fit 绘出后遮罩 blit 进圆(fit 几何照旧,只换成掩码形态原语,
// 掩码矩形 = 内切圆矩形,内在尺寸不可用退化 fill 形态再裁圆);
// ③ 边框沿圆周描成环(描边参数解析规则见 ImageLayer.CircleStroke)。
// 内切圆边长 ≤0(内容区被 padding 吃满)零几何零绘制。
// 本方法私有且仅由 paintImage 的能力口探测(类型断言)命中臂调用——
// DrawEllipse/DrawImageMasked 只在实现了 ShapeCapableBackend 的后端上存在
func (r *template) paintImageCircle(sb ShapeCapableBackend, l *layer.ImageLayer, x, y int) error {
	circle := l.InscribedCircleRect().Add(image.Pt(x, y))
	if circle.Dx() <= 0 || circle.Dy() <= 0 {
		return nil
	}

	if err := sb.DrawEllipse(circle.Min.X, circle.Min.Y, circle.Dx(), circle.Dy(), l.Background(), nil); err != nil {
		return err
	}

	if src := l.ResolvedSrc(); src != nil {
		if err := r.paintFitBlits(l, *src, x, y, func(src string, srcRect, dstRect image.Rectangle) error {
			return sb.DrawImageMasked(src, srcRect, dstRect, circle)
		}); err != nil {
			return err
		}
	}

	if stroke := l.CircleStroke(); stroke != nil {
		return sb.DrawEllipse(circle.Min.X, circle.Min.Y, circle.Dx(), circle.Dy(), nil, stroke)
	}
	return nil
}

// paintFitBlits 全 fit 白名单分派(cover/fill/contain/tile + 内在尺寸退化),blit 落点
// 由调用臂注入(矩形路径 DrawImage / 圆形路径 DrawImageMasked)——
// 两形态共用同一份分派几何防数学漂移(先绘后裁,fit 与 clip 正交)
func (r *template) paintFitBlits(l *layer.ImageLayer, src string, x, y int, blit func(src string, srcRect, dstRect image.Rectangle) error) error {
	// cover/fill 共用目标矩形(同门):getImageOrigin + 内容宽高
	originX, originY := l.ImageOrigin()
	dst := image.Rect(x+originX, y+originY, x+originX+l.ContentWidth(), y+originY+l.ContentHeight())

	imgWidth, imgHeight, ok := intrinsicSize(src)
	if !ok {
		// 内在尺寸不可用:整图拉伸进目标矩形(fill 形态),srcRect 零哨兵由后端按解码实际尺寸取整图
		return blit(src, image.Rectangle{}, dst)
	}

	switch l.Fit() {
	case layer.FitContain:
		// contain(ADR 0023):按内在比例收缩目标矩形再整图 blit
		cx, cy, cw, ch := l.ContainRect(imgWidth, imgHeight)
		return blit(src, image.Rect(0, 0, imgWidth, imgHeight),
			image.Rect(x+cx, y+cy, x+cx+cw, y+cy+ch))
	case layer.FitTile:
		// tile:可见块逐块 blit(src/dst 可见子矩形 1:1),裁切纯几何完成、原语零裁剪参数
		for _, block := range l.TileRects(imgWidth, imgHeight) {
			if err := blit(src, block.Src, block.Dst.Add(image.Pt(x, y))); err != nil {
				return err
			}
		}
		return nil
	case layer.FitFill:
		// fill:整图进与 cover 相同的目标矩形(非等比拉伸,不裁切不留空隙)
		return blit(src, image.Rect(0, 0, imgWidth, imgHeight), dst)
	default:
		// cover(缺省/显式 cover 归一/未知值白名单外降级):等比居中裁窗
		return blit(src, l.CoverRect(imgWidth, imgHeight), dst)
	}
}

// intrinsicSize 源图内在尺寸(全 fit 前置,ADR 0024):绘制分派点一次读取,
// 不可用(损坏/非图文件/零维)返回 ok=false——调用方统一退化 fill 形态
func intrinsicSize(src string) (width, height int, ok bool) {
	f, err := os.Open(src)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

// paintText 文本图层:绘制分派前按需物化(须先于 Lines()——真实字体度量
// 依赖物化后的本地路径),先画自身盒,再逐行绘制——起点 = padding + 文本对齐
// 锚点,行距按行高像素累加;(x, y) 交由后端按对齐语义与字体 metrics 落笔
func (r *template) paintText(ctx context.Context, l *layer.TextLayer, x, y int) error {
	if err := r.resolver.ResolveLayer(ctx, l); err != nil {
		return err
	}
	if err := r.backend.DrawRect(x, y, l.Width(), l.Height(), l.Background(), l.Border()); err != nil {
		return err
	}

	padding := l.Padding()
	originX, originY := l.TextOrigin()
	posx := x + int(padding.Left) + originX
	posy := y + int(padding.Top) + originY

	for _, line := range l.Lines() {
		if err := r.backend.DrawText(
			line,
			posx,
			posy,
			l.ResolvedFont(),
			l.FontSize(),
			l.FontColor(),
			l.HorizontalAlign(),
			l.VerticalAlign(),
			l.Angle(),
		); err != nil {
			return err
		}
		posy += l.LineHeightPx()
	}
	return nil
}

// paintQrCode 二维码图层:绘制分派前按需物化(spec §4.3,失败即抛渲染面丢弃),
// 先画自身盒,再内切正方形铺放——边长 = min(内容区宽高)(quiet zone 语义,padding
// 留白即码外静区),对齐 + padding 定位(与 PHP 模板一致,镜像 paintImage 语义);
// 边长 ≤0 由后端 DrawImage 防护只画盒;src 未物化为 nil 时只画盒
func (r *template) paintQrCode(ctx context.Context, l *layer.QrCodeLayer, x, y int) error {
	if err := r.resolver.ResolveLayer(ctx, l); err != nil {
		return err
	}
	if err := r.backend.DrawRect(x, y, l.Width(), l.Height(), l.Background(), l.Border()); err != nil {
		return err
	}

	src := l.ResolvedSrc()
	if src == nil {
		return nil
	}
	// 二维码内切于内容盒:边长 = min(内容区宽高)(quiet zone 语义,padding
	// 留白即码外静区),对齐 + padding 定位(与 PHP 模板一致,镜像 paintImage 语义);
	// QR 产物恒正方形(ADR 0015)、目标恒正方形——cover 窗 = 整图,srcRect 零哨兵
	// 整图 blit、零内在尺寸读取(ADR 0024);边长 ≤0 由 blit 后端防护只画盒
	originX, originY := l.QrOrigin()
	side := l.ContentSide()
	return r.backend.DrawImage(*src, image.Rectangle{},
		image.Rect(x+originX, y+originY, x+originX+side, y+originY+side))
}

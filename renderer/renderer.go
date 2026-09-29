// Package renderer 渲染契约与模板:后端作者只实现五个绘制原语(Backend)即可
// 接入完整渲染管线——资源物化、遍历、分派、容器下钻、锚点定位全由模板完成
// (M1 功能闭环的最后一环)。模板以后端为主动方向画布拉取结构树,所有图层
// 直画同一渲染面,坐标经参数传递(无栈)。
package renderer

import (
	"context"
	"fmt"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/layer"
	"github.com/hankchen/go-canvas/resolver"
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

// Render implements Renderer:建面 → 逐层 paint(绘制分派前按需物化)→ 收尾。
// 失败即抛、渲染面丢弃、无产物逃逸(begin 已调用但 end 不达)。
// 根层渲染循环跳过 visible=false(layer-panel-ux 工单 01,隐藏=最终输出排除):
// 过滤在建面循环之前,隐藏层的绘制分派与惰性物化均不发生
func (r *template) Render(ctx context.Context, c *canvas.Canvas) (any, error) {
	layers := c.GetLayers()
	visible := make([]layer.Layer, 0, len(layers))
	for _, l := range layers {
		if l.Visible() {
			visible = append(visible, l)
		}
	}
	return r.renderSurface(ctx, c.Width(), c.Height(), visible)
}

// RenderLayer implements Renderer:以其自身尺寸建面,物化时机同 Render(惰性)。
// 不做显隐过滤——visible 语义属画布渲染循环,单图层渲染所见即所得
func (r *template) RenderLayer(ctx context.Context, l layer.Layer) (any, error) {
	p, ok := l.(paintable)
	if !ok {
		return nil, fmt.Errorf("%w: %T", layer.ErrUnknownLayerType, l)
	}
	return r.renderSurface(ctx, p.Width(), p.Height(), []layer.Layer{l})
}

// renderSurface 建面 → 逐层 paint → 收尾;任一原语/物化报错即中止(渲染面丢弃)。
// 根层循环每图层前设控制面检查点(与 php render 根层循环同位,工票 03):
// ctx 到期/取消报渲染 code,先于下一层的绘制分派与惰性物化
func (r *template) renderSurface(ctx context.Context, width, height int, layers []layer.Layer) (any, error) {
	if err := r.backend.Begin(width, height); err != nil {
		return nil, err
	}
	for _, l := range layers {
		if err := resolver.CheckContext(ctx); err != nil {
			return nil, err
		}
		if err := r.paint(ctx, l, 0, 0, width, height); err != nil {
			return nil, err
		}
	}
	return r.backend.End(), nil
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

// paintImage 图片图层:绘制分派前按需物化(spec §4.3,失败即抛渲染面丢弃),
// 先画自身盒,再按内容区对齐起点放置。
// src 取 ResolvedSrc(物化结果优先,回退原始引用——本地路径零物化直画),
// 引用为 nil 时只画盒
func (r *template) paintImage(ctx context.Context, l *layer.ImageLayer, x, y int) error {
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

	originX, originY := l.ImageOrigin()
	return r.backend.DrawImage(*src, x+originX, y+originY, l.ContentWidth(), l.ContentHeight())
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
// 先画自身盒,再按宽度正方形铺放
// (与 PHP 模板一致,忽略声明高度;src 未物化为 nil 时只画盒)
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
	return r.backend.DrawImage(*src, x, y, l.Width(), l.Width())
}

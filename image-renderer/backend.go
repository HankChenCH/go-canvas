// Package imagerenderer 核心包的位图渲染后端:以标准库 image 新建透明位图为
// 渲染面,实现五个绘制原语接入渲染模板;承载 PNG/JPEG 解码、EXIF 转正、
// blit 拉伸搬运(ADR 0024,源窗 → 目标矩形的像素搬运)、文本绘制(opentype
// 字体 + 内置默认字体兜底)、二维码物化(go-qrcode 适配,QRMaterializer 缝的
// 默认接线)与 PNG 编码落盘。对应 PHP 侧的 php-canvas-image-renderer 包,
// End 产物为 *image.NRGBA。
package imagerenderer

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"

	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/renderer"
	"github.com/HankChenCH/go-canvas/resolver"
	"golang.org/x/image/font"
)

// Renderer 位图渲染后端:实现 renderer.Backend 与可选形状能力口
// renderer.ShapeCapableBackend(ADR 0026),产物为透明底 *image.NRGBA。
// 同一实例可跨多次渲染复用,Begin 即重置渲染面。**非并发安全**:字体 Face
// 按渲染会话(实例)持有(opentype.Face 与渲染面位图均非并发安全),并发
// 渲染请各自 New
type Renderer struct {
	surface *image.NRGBA
	faces   map[fontKey]font.Face

	// defaultResolver 后端自带的默认物化器(NewRenderer 接线,可为 nil);
	// 核心 New 以 nil resolver 组装时经 DefaultResolver 发现并优先采用
	defaultResolver *resolver.ResourceResolver
}

var _ renderer.Backend = (*Renderer)(nil)
var _ renderer.ShapeCapableBackend = (*Renderer)(nil)

// New 构造位图渲染后端
func New() *Renderer { return &Renderer{} }

// DefaultResolver 后端自带的默认物化器(核心渲染模板 nil-resolver 组装路径
// 的可选发现面);未经 NewRenderer 接线时返回 nil,核心回退自身默认
func (r *Renderer) DefaultResolver() *resolver.ResourceResolver { return r.defaultResolver }

// face 取渲染面;未建面即调用绘制原语属用法错误(模板保证先 Begin)
func (r *Renderer) face() (*image.NRGBA, error) {
	if r.surface == nil {
		return nil, errors.New("位图渲染面未初始化:应经渲染模板 Begin 建面")
	}
	return r.surface, nil
}

// Begin implements renderer.Backend:新建 width×height 透明位图
func (r *Renderer) Begin(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("渲染面尺寸非法: %dx%d", width, height)
	}
	r.surface = image.NewNRGBA(image.Rect(0, 0, width, height))
	return nil
}

// End implements renderer.Backend:返回渲染面 *image.NRGBA;未建面返回 nil 接口
// (非 typed nil,调用方 nil 判定才有意义)
func (r *Renderer) End() any {
	if r.surface == nil {
		return nil
	}
	return r.surface
}

// DrawRect implements renderer.Backend:背景色 + 四边边框。
// 背景为 nil 或空串跳过填充(PHP 同款守卫:非 null 且非空串才填充);
// 边框 = 四边各一条线宽=边框宽的直线(非描边矩形),按盒内嵌绘制,逐边可 nil
func (r *Renderer) DrawRect(x, y, width, height int, bgColor *string, border layer.Border) error {
	surface, err := r.face()
	if err != nil {
		return err
	}

	if bgColor != nil && *bgColor != "" {
		c, err := parseColor(*bgColor)
		if err != nil {
			return err
		}
		fillRect(surface, image.Rect(x, y, x+width, y+height), c)
	}

	for _, edge := range borderBands(x, y, width, height, border) {
		if err := fillEdge(surface, edge.side, edge.band); err != nil {
			return err
		}
	}
	return nil
}

// edgeBand 一条边的线宽带:边框设定 + 盒内嵌的覆盖区
type edgeBand struct {
	side *layer.BorderSide
	band image.Rectangle
}

// borderBands 四条边的线宽带,序为 top/bottom/left/right(PHP 绘制序,
// 后画覆盖先画);未设或 width≤0(wire 解码可注入)的边不计
func borderBands(x, y, width, height int, border layer.Border) []edgeBand {
	bands := make([]edgeBand, 0, 4)
	if s := border.Top; s != nil && s.Width > 0 {
		bands = append(bands, edgeBand{s, image.Rect(x, y, x+width, y+s.Width)})
	}
	if s := border.Bottom; s != nil && s.Width > 0 {
		bands = append(bands, edgeBand{s, image.Rect(x, max(y, y+height-s.Width), x+width, y+height)})
	}
	if s := border.Left; s != nil && s.Width > 0 {
		bands = append(bands, edgeBand{s, image.Rect(x, y, x+s.Width, y+height)})
	}
	if s := border.Right; s != nil && s.Width > 0 {
		bands = append(bands, edgeBand{s, image.Rect(max(x, x+width-s.Width), y, x+width, y+height)})
	}
	return bands
}

// DrawImage implements renderer.Backend(双矩形 blit,ADR 0024):解码 → 裁 srcRect
// 源窗 → EXIF 转正 → 拉伸至 dstRect → 合成到渲染面。srcRect 空矩形是整图哨兵:
// 按解码实际尺寸取整图(分派层内在尺寸不可用的退化形态,ADR 0024 接口契约);
// dstRect 空矩形直接跳过(PHP dw/dh ≤0 防护同款,防护先于读取)。裁窗在内在坐标系
// (与分派层 DecodeConfig 同系)进行、EXIF 转正后置——orientation 是恒距变换
// (保居中、保比例),裁窗后转正与「转正后取对应窗」等价;拉伸无关等比(cover/fill
// 的差别只在分派层给出的矩形形状)。防御:源窗与解码图不相交(分派层几何保证
// 不发生,wire 可注入)时静默不画
func (r *Renderer) DrawImage(src string, srcRect, dstRect image.Rectangle) error {
	surface, err := r.face()
	if err != nil {
		return err
	}
	if dstRect.Empty() {
		return nil
	}

	stretched, err := blitSource(src, srcRect, dstRect)
	if err != nil {
		return err
	}
	if stretched == nil {
		return nil
	}
	draw.Draw(surface, dstRect, stretched, image.Point{}, draw.Over)
	return nil
}

// DrawImageMasked implements renderer.ShapeCapableBackend(掩码 blit 原语,ADR 0026,
// DrawImage 的掩码形态):双矩形 blit(语义与 DrawImage 完全一致,含 srcRect 空矩形
// 整图哨兵)后以 maskRect 外接矩形的内切椭圆为掩码裁切,掩码外透明——分派层传
// 正方形即圆。掩码矩形是画布绝对坐标,与目标矩形独立(换算到目标坐标系后超出
// 部分自然落在目标位图界外)。dstRect/maskRect 空矩形防护不画
func (r *Renderer) DrawImageMasked(src string, srcRect, dstRect, maskRect image.Rectangle) error {
	surface, err := r.face()
	if err != nil {
		return err
	}
	if dstRect.Empty() || maskRect.Empty() {
		return nil
	}

	stretched, err := blitSource(src, srcRect, dstRect)
	if err != nil {
		return err
	}
	if stretched == nil {
		return nil
	}

	applyEllipseMask(stretched, maskRect.Sub(dstRect.Min))
	draw.Draw(surface, dstRect, stretched, image.Point{}, draw.Over)
	return nil
}

// blitSource blit 共享前段(DrawImage/DrawImageMasked 同源):解码 → 裁 srcRect
// 源窗 → EXIF 转正 → 拉伸至 dstRect 尺寸;源窗与解码图不相交返回 nil
// (静默不画,调用方跳过合成),其余错误随通道返回
func blitSource(src string, srcRect, dstRect image.Rectangle) (*image.NRGBA, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("读取图片 %s: %w", src, err)
	}
	raw, err := decode(data)
	if err != nil {
		return nil, fmt.Errorf("解码图片 %s: %w", src, err)
	}

	region := srcRect
	if region.Empty() {
		region = raw.Bounds()
	}
	region = region.Intersect(raw.Bounds())
	if region.Empty() {
		return nil, nil
	}

	cropped := image.NewNRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	draw.Draw(cropped, cropped.Bounds(), raw, region.Min, draw.Over)
	if o := exifOrientation(data); o != 1 {
		cropped = applyOrientation(cropped, o)
	}

	return scaleImage(cropped, dstRect.Dx(), dstRect.Dy()), nil
}

// fillRect 以 Over 合成填充矩形(后画者覆盖先画者,对齐图层叠加语义);
// 空矩形为自然 no-op
func fillRect(surface *image.NRGBA, band image.Rectangle, c color.NRGBA) {
	draw.Draw(surface, band, &image.Uniform{C: c}, image.Point{}, draw.Over)
}

// fillEdge 以边框色填充一条边的线宽带
func fillEdge(surface *image.NRGBA, side *layer.BorderSide, band image.Rectangle) error {
	c, err := parseColor(side.Color)
	if err != nil {
		return fmt.Errorf("解析边框色: %w", err)
	}
	fillRect(surface, band, c)
	return nil
}

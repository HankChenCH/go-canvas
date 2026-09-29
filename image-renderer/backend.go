// Package imagerenderer 核心包的位图渲染后端:以标准库 image 新建透明位图为
// 渲染面,实现五个绘制原语接入渲染模板;承载 PNG/JPEG 解码、EXIF 转正、
// cover 缩放裁切、文本绘制(opentype 字体 + 内置默认字体兜底)、二维码物化
// (go-qrcode 适配,QRMaterializer 缝的默认接线)与 PNG 编码落盘。对应 PHP 侧
// 的 php-canvas-image-renderer 包,End 产物为 *image.NRGBA。
package imagerenderer

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"

	"github.com/hankchen/go-canvas/layer"
	"github.com/hankchen/go-canvas/renderer"
	"github.com/hankchen/go-canvas/resolver"
	"golang.org/x/image/font"
)

// Renderer 位图渲染后端:实现 renderer.Backend,产物为透明底 *image.NRGBA。
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

// DrawImage implements renderer.Backend:解码 → EXIF 转正 → cover 填满目标盒
// → 放置于 (x, y)。宽/高 ≤0 直接跳过(PHP 同款守卫)
func (r *Renderer) DrawImage(src string, x, y, width, height int) error {
	surface, err := r.face()
	if err != nil {
		return err
	}
	if width <= 0 || height <= 0 {
		return nil
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("读取图片 %s: %w", src, err)
	}
	img, err := decodeOriented(data)
	if err != nil {
		return fmt.Errorf("解码图片 %s: %w", src, err)
	}
	covered := coverImage(img, width, height)
	draw.Draw(surface, image.Rect(x, y, x+width, y+height), covered, image.Point{}, draw.Over)
	return nil
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

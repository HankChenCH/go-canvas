// Package pdfrenderer 核心包的 PDF 渲染后端:以 go-pdf/fpdf 新建单文档为渲染面,
// 实现五个绘制原语接入渲染模板;承载 PNG/JPEG/GIF 直嵌(clip + 放置矩阵裁窗/转正)、
// TTF 子集内嵌文本(CJK 门槛,内置兜底 = PDF 核心字体 Helvetica)、二维码物化
// (go-qrcode 适配,QRMaterializer 缝的默认接线)。对应 Go 侧位图后端
// image-renderer 的 PDF 姊妹件,End 产物为 PDF 文档字节 []byte。
//
// 单位契约:核心接口坐标恒为抽象像素,px→pt 换算责任全在后端(spec §px→pt,
// 96dpi 口径,pt = px × 0.75)——页 MediaBox、原语坐标、字号一律 ×0.75。
package pdfrenderer

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/renderer"
	"github.com/HankChenCH/go-canvas/resolver"

	"github.com/go-pdf/fpdf"
)

// pxToPt px→pt 换算系数(96dpi 口径:pt = px × 72/96)。核心接口单位恒抽象像素,
// 换算责任全在后端(与编译期页几何 A4 @96dpi ≈ 1123px ≈ 842.25pt 对齐)
const pxToPt = 0.75

// Renderer PDF 渲染后端:实现 renderer.Backend,产物为 PDF 文档字节 []byte
// (经 renderer.RenderAs[[]byte] 消费,多页经 renderer.RenderPages / 本 module
// RenderPages 消费)。实例即一个 PDF 文档会话:首次 Begin 建档,后续 Begin
// 同一文档续页(单文档连续性,多页入口工票 03 逐页 Begin);End 产出即终结
// 会话——doc 与字体注册表一并复位,同一实例再次渲染(无论单页还是多页入口)
// 必然得到干净新文档,不会把页面追加进旧文档(工票 03 验收锁定)。**非并发
// 安全**:fpdf 实例按渲染会话(实例)持有且非并发安全,并发渲染请各自 New
type Renderer struct {
	doc    *fpdf.Fpdf // Begin 建立的 PDF 文档(会话内单文档连续性,多页逐页 AddPage)
	width  int        // 渲染面宽(px,页面几何换算基准)
	height int        // 渲染面高(px)

	// fonts 已注册字体表(路径 → 合成字体族名):多行文本逐行 DrawText,
	// 同一路径只读盘/注册一次;文档内嵌去重由 fpdf 按字体键自持
	fonts map[string]string

	// defaultResolver 后端自带的默认物化器(NewRenderer 接线,可为 nil);
	// 核心 New 以 nil resolver 组装时经 DefaultResolver 发现并优先采用
	defaultResolver *resolver.ResourceResolver
}

// 编译期全量契约断言:直接对 renderer.Backend 断言,五个绘制原语签名齐备才算
// 实现(匿名接口只点名三方法,DrawImage/DrawText 的签名漂移会漏检)
var _ renderer.Backend = (*Renderer)(nil)

// New 构造 PDF 渲染后端(不带默认物化器;经 NewRenderer 组装即完整管线)
func New() *Renderer { return &Renderer{} }

// DefaultResolver 后端自带的默认物化器(核心渲染模板 nil-resolver 组装路径
// 的可选发现面);未经 NewRenderer 接线时返回 nil,核心回退自身默认
func (r *Renderer) DefaultResolver() *resolver.ResourceResolver { return r.defaultResolver }

// docf 取渲染面;未建面即调用绘制原语属用法错误(模板保证先 Begin)
func (r *Renderer) docf() (*fpdf.Fpdf, error) {
	if r.doc == nil {
		return nil, errors.New("PDF 渲染面未初始化:应经渲染模板 Begin 建面")
	}
	return r.doc, nil
}

// Begin implements renderer.Backend:首次调用新建单文档(页尺寸 = px × 0.75 pt,
// MediaBox 与编译期页几何同口径)并 AddPage 建第一页;文档已在(多页入口,
// 工票 03)则以本页尺寸续页——同一 fpdf 实例逐页 AddPage(单文档连续性)。
// 关闭自动分页:绘制坐标全由模板给足,内容溢出不切页(与位图后端出界即裁同语义)
func (r *Renderer) Begin(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("渲染面尺寸非法: %dx%d", width, height)
	}
	size := fpdf.SizeType{Wd: float64(width) * pxToPt, Ht: float64(height) * pxToPt}
	if r.doc == nil {
		doc := fpdf.NewCustom(&fpdf.InitType{
			OrientationStr: "P",
			UnitStr:        "pt", // 文档单位即 pt,坐标换算只剩 pxToPt 一个系数
			Size:           size,
		})
		doc.SetAutoPageBreak(false, 0)
		doc.AddPage()
		r.doc = doc
	} else {
		r.doc.AddPageFormat("P", size)
	}
	r.width = width
	r.height = height
	return nil
}

// End implements renderer.Backend:全程内存构建,Output 到字节缓冲即 PDF 产物;
// 未建面返回 nil 接口(非 typed nil)。绘制期任一原语报错时模板即中止、End
// 不达(失败路径无产物逃逸);此处仍防御性复查文档错误位——错误 sticky 的
// fpdf 在流残缺时也可能"完成"输出,宁可无产物不可出半成品。
// End 一次性终结文档会话:无论产出与否,doc 与字体注册表一并复位——Begin 的
// 「文档已在则续页」分支只应活在单次渲染会话内(多页逐页 Begin),End 后实例
// 复用必须从建档起步,否则第二次渲染把页面追加进旧文档
func (r *Renderer) End() any {
	if r.doc == nil {
		return nil
	}
	product := r.emit()
	r.doc = nil   // 会话复位:再次 Begin 即建档新文档(End 后复用语义,工票 03)
	r.fonts = nil // 字体注册随文档会话走:族名映射只在注册进当前文档后有意义,新文档须重新注册
	return product
}

// emit 产物发射:错误位复查 + Output 到内存缓冲;任一失败返回 nil(无产物逃逸)
func (r *Renderer) emit() any {
	if r.doc.Err() {
		return nil
	}
	var buf bytes.Buffer
	if err := r.doc.Output(&buf); err != nil {
		return nil
	}
	return buf.Bytes()
}

// DrawRect implements renderer.Backend:背景色 + 四边边框。
// 背景为 nil 或空串跳过填充(位图后端同款守卫:非 null 且非空串才填充);
// 边框 = 盒内嵌色带(非描边矩形),序 top/bottom/left/right,逐边可 nil、
// width≤0 的边不计(语义对齐 image-renderer borderBands,PHP 绘制序)
func (r *Renderer) DrawRect(x, y, width, height int, bgColor *string, border layer.Border) error {
	doc, err := r.docf()
	if err != nil {
		return err
	}

	if bgColor != nil && *bgColor != "" {
		c, err := parseColor(*bgColor)
		if err != nil {
			return err
		}
		paintRect(doc, float64(x), float64(y), float64(width), float64(height), c)
	}

	for _, edge := range borderBands(x, y, width, height, border) {
		c, err := parseColor(edge.side.Color)
		if err != nil {
			return fmt.Errorf("解析边框色: %w", err)
		}
		band := edge.band
		paintRect(doc, float64(band.Min.X), float64(band.Min.Y), float64(band.dx()), float64(band.dy()), c)
	}
	return doc.Error()
}

// paintRect 以像素坐标画一个实心矩形(内部 ×0.75 转 pt);半透明色经
// ExtGState 挂 alpha,画完恢复不透明——后续元素不吃残留(文本/下一色块)
func paintRect(doc *fpdf.Fpdf, x, y, w, h float64, c rgba) {
	doc.SetAlpha(float64(c.A)/255, "Normal")
	doc.SetFillColor(int(c.R), int(c.G), int(c.B))
	doc.Rect(x*pxToPt, y*pxToPt, w*pxToPt, h*pxToPt, "F")
	doc.SetAlpha(1, "Normal")
}

// edgeBand 一条边的线宽带:边框设定 + 盒内嵌的覆盖区
type edgeBand struct {
	side *layer.BorderSide
	band rect
}

// rect 像素整型矩形(避让 image 包的 Rectangle:PDF 后端只做几何搬运,
// 不需要 image.Rectangle 的点/绘图面)
type rect struct {
	Min, Max point
}

type point struct{ X, Y int }

func (b rect) dx() int { return b.Max.X - b.Min.X }
func (b rect) dy() int { return b.Max.Y - b.Min.Y }

func rectXY(x0, y0, x1, y1 int) rect { return rect{point{x0, y0}, point{x1, y1}} }

// borderBands 四条边的线宽带,序为 top/bottom/left/right(PHP 绘制序,
// 后画覆盖先画);未设或 width≤0(wire 解码可注入)的边不计。
// 与 image-renderer borderBands 同一公式的小件复制——几何契约的一部分,
// 两后端防线必须对称(跨端矩阵平移纪律)
func borderBands(x, y, width, height int, border layer.Border) []edgeBand {
	bands := make([]edgeBand, 0, 4)
	if s := border.Top; s != nil && s.Width > 0 {
		bands = append(bands, edgeBand{s, rectXY(x, y, x+width, y+s.Width)})
	}
	if s := border.Bottom; s != nil && s.Width > 0 {
		bands = append(bands, edgeBand{s, rectXY(x, max(y, y+height-s.Width), x+width, y+height)})
	}
	if s := border.Left; s != nil && s.Width > 0 {
		bands = append(bands, edgeBand{s, rectXY(x, y, x+s.Width, y+height)})
	}
	if s := border.Right; s != nil && s.Width > 0 {
		bands = append(bands, edgeBand{s, rectXY(max(x, x+width-s.Width), y, x+width, y+height)})
	}
	return bands
}

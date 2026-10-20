package pdfrenderer

// 图片绘制原语(双矩形 blit,ADR 0024 的 PDF 直嵌形态):源字节原样进文档
// (PNG/JPEG/GIF 零重编码),srcRect→dstRect 的裁窗与拉伸、EXIF 归正全部经
// 裁剪路径 + 放置矩阵在 PDF 图形态完成——像素不被搬动,与位图后端的
// 「解码 → 裁窗 → 转正 → 拉伸」像素管线语义等价(等价域与反例域:
// 单页放置面等价;位图后端像素级插值 vs PDF 矢量缩放的采样差异不在契约内)。

import (
	"bytes"
	"fmt"
	"image"
	"os"

	"github.com/go-pdf/fpdf"
)

// DrawImage implements renderer.Backend(双矩形 blit,ADR 0024):直嵌源字节 →
// ClipRect 裁 dstRect → 放置矩阵把 srcRect 源窗搬运至 dstRect。srcRect 空矩形是
// 整图哨兵:按解码实际尺寸取整图(分派层内在尺寸不可用的退化形态,ADR 0024 接口
// 契约);dstRect 空矩形直接跳过(PHP dw/dh ≤0 防护同款,防护先于读取)。
// 裁窗在内在坐标系(与分派层 DecodeConfig 同系)进行、EXIF 归正由放置矩阵后置
// ——orientation 是恒距变换(保居中、保比例),「内在坐标系裁窗 → 矩阵归正」与
// 位图后端「裁窗后转正像素」等价(推导见 orientAffine docblock);拉伸无关等比
// (cover/fill 的差别只在分派层给出的矩形形状)。防御:源窗与解码图不相交
// (分派层几何保证不发生,wire 可注入)时静默不画
func (r *Renderer) DrawImage(src string, srcRect, dstRect image.Rectangle) error {
	doc, err := r.docf()
	if err != nil {
		return err
	}
	if dstRect.Empty() {
		return nil
	}
	return r.embedBlit(doc, src, srcRect, dstRect, func(doc *fpdf.Fpdf) {
		doc.ClipRect(
			float64(dstRect.Min.X)*pxToPt, float64(dstRect.Min.Y)*pxToPt,
			float64(dstRect.Dx())*pxToPt, float64(dstRect.Dy())*pxToPt,
			false,
		)
	})
}

// DrawImageMasked implements renderer.ShapeCapableBackend(掩码 blit 原语,ADR 0026,
// DrawImage 的掩码形态):双矩形 blit(语义与 DrawImage 完全一致,含 srcRect 空矩形
// 整图哨兵)后以 maskRect 外接矩形的内切椭圆为掩码裁切,掩码外不画——分派层传
// 正方形即圆。矢量承接 = ClipEllipse 替位既有 ClipRect(ADR 0026),直嵌零重编码
// 与放置矩阵机制全复用(embedBlit 共享主体);掩码矩形是画布绝对坐标,与目标矩形
// 独立,裁剪路径在未入放置矩阵的用户空间施加。dstRect/maskRect 空矩形防护不画
func (r *Renderer) DrawImageMasked(src string, srcRect, dstRect, maskRect image.Rectangle) error {
	doc, err := r.docf()
	if err != nil {
		return err
	}
	if dstRect.Empty() || maskRect.Empty() {
		return nil
	}
	return r.embedBlit(doc, src, srcRect, dstRect, func(doc *fpdf.Fpdf) {
		cx, cy, rx, ry := inscribedEllipse(maskRect)
		doc.ClipEllipse(cx, cy, rx, ry, false)
	})
}

// embedBlit 直嵌 blit 共享主体(DrawImage/DrawImageMasked 同源):读取 → 魔数
// 嗅探 → 内在尺寸 → srcRect 源窗与放置矩阵 P 算定 → 直嵌注册 → 调用臂给定的
// 裁剪原语内发射放置矩阵与整幅图。裁剪闭包在 TransformBegin 之前调用——双空间
// 纪律:fpdf 裁剪/变换入参恒用户空间(内部自换算),矩阵换算归 placeAffine
func (r *Renderer) embedBlit(doc *fpdf.Fpdf, src string, srcRect, dstRect image.Rectangle, clip func(*fpdf.Fpdf)) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("读取图片 %s: %w", src, err)
	}
	tp, err := sniffType(data)
	if err != nil {
		return fmt.Errorf("解码图片 %s: %w", src, err)
	}
	bounds, err := decodeConfig(data)
	if err != nil {
		return fmt.Errorf("解码图片 %s: %w", src, err)
	}

	region := srcRect
	if region.Empty() {
		region = bounds
	}
	region = region.Intersect(bounds)
	if region.Empty() {
		return nil
	}

	// 直嵌注册:字节经 Reader 进文档(扩展名不可信,ImageType 用魔数嗅探结果);
	// 同名图片 fpdf 去重——tile 多块引用同一源只嵌一份
	doc.RegisterImageOptionsReader(src, fpdf.ImageOptions{ImageType: tp}, bytes.NewReader(data))
	if doc.Err() {
		return fmt.Errorf("嵌入图片 %s: %w", src, doc.Error())
	}

	// 放置矩阵 P = 窗映射 ∘ EXIF 归正(先归正、后映射,见两函数 docblock)
	T := orientAffine(exifOrientation(data), bounds.Dx(), bounds.Dy())
	box := transformRect(T, region)
	sx := float64(dstRect.Dx()) / float64(box.dx())
	sy := float64(dstRect.Dy()) / float64(box.dy())
	W := affine{
		sx, 0, 0, sy,
		float64(dstRect.Min.X) - sx*float64(box.Min.X),
		float64(dstRect.Min.Y) - sy*float64(box.Min.Y),
	}
	P := W.after(T)

	clip(doc)
	doc.TransformBegin()
	r.placeAffine(doc, P)
	// 整幅图按内在尺寸入坐标(矩阵负责裁窗与映射),放置点恒 (0,0) 不为负
	doc.ImageOptions(src, 0, 0, float64(bounds.Dx())*pxToPt, float64(bounds.Dy())*pxToPt,
		false, fpdf.ImageOptions{ImageType: tp}, 0, "")
	doc.TransformEnd()
	doc.ClipEnd()
	if doc.Err() {
		return fmt.Errorf("绘制图片 %s: %w", src, doc.Error())
	}
	return nil
}

// inscribedEllipse 矩形的内切椭圆(fpdf ClipEllipse 入参形态:圆心 + 半轴)。
// 像素空间算定后统一 ×pxToPt 发射——掩码 = 椭圆内切于给定矩形的唯一几何落点
func inscribedEllipse(r image.Rectangle) (cx, cy, rx, ry float64) {
	cx = (float64(r.Min.X) + float64(r.Dx())/2) * pxToPt
	cy = (float64(r.Min.Y) + float64(r.Dy())/2) * pxToPt
	rx = float64(r.Dx()) / 2 * pxToPt
	ry = float64(r.Dy()) / 2 * pxToPt
	return cx, cy, rx, ry
}

// affine 2×3 仿射矩阵(y 向下坐标系,先 x 后 y 的行向量约定):
// x' = a·x + b·y + e;y' = c·x + d·y + f。像素空间参与运算,发射时统一 ×pxToPt
type affine struct{ a, b, c, d, e, f float64 }

// after 复合:this ∘ other(先施加 other、后施加 this)
func (m affine) after(n affine) affine {
	return affine{
		a: m.a*n.a + m.b*n.c,
		b: m.a*n.b + m.b*n.d,
		c: m.c*n.a + m.d*n.c,
		d: m.c*n.b + m.d*n.d,
		e: m.a*n.e + m.b*n.f + m.e,
		f: m.c*n.e + m.d*n.f + m.f,
	}
}

// apply 点变换
func (m affine) apply(x, y float64) (float64, float64) {
	return m.a*x + m.b*y + m.e, m.c*x + m.d*y + m.f
}

// orientAffine EXIF orientation 的恒距归正矩阵(y 向下,整幅内在图 sw×sh → 观察方向)。
// 与位图后端 applyOrientation 的逐像素映射逐向对齐(2/4 镜像、3 旋转 180°、
// 6/8 旋转 90°、5/7 沿对角镜像),推导:位图实现以「观察方向格点 → 内在格点」
// 给出,取其逆映射并连续化(格点离散 ±1 由恒距变换的平移项吸收):
//
//	o=2: x'=sw-x        o=3: x'=sw-x, y'=sh-y    o=4: y'=sh-y
//	o=5: x'=y, y'=x(转置)  o=6: x'=sh-y, y'=x(顺时针 90°)
//	o=7: x'=sh-y, y'=sw-x  o=8: x'=y, y'=sw-x(逆时针 90°)
//
// 工票 02 开放点 3 的定稿式:矩阵以 2×3 仿射显式给出并经八向 fixture 标定,
// 不经像素搬运直嵌字节零重编码
func orientAffine(orientation, sw, sh int) affine {
	w, h := float64(sw), float64(sh)
	switch orientation {
	case 2:
		return affine{-1, 0, 0, 1, w, 0}
	case 3:
		return affine{-1, 0, 0, -1, w, h}
	case 4:
		return affine{1, 0, 0, -1, 0, h}
	case 5: // 转置(主对角镜像)
		return affine{0, 1, 1, 0, 0, 0}
	case 6: // 旋转 90° 顺时针
		return affine{0, -1, 1, 0, h, 0}
	case 7: // 反转置(反对角镜像)
		return affine{0, -1, -1, 0, h, w}
	case 8: // 旋转 90° 逆时针
		return affine{0, 1, -1, 0, 0, w}
	default: // 1 及越界归一:恒等
		return affine{1, 0, 0, 1, 0, 0}
	}
}

// transformRect 矩形经仿射后的轴对齐包络(8 向归正均为轴排列/翻转,四角即足)
func transformRect(m affine, r image.Rectangle) rect {
	x0, y0 := m.apply(float64(r.Min.X), float64(r.Min.Y))
	x1, y1 := m.apply(float64(r.Max.X), float64(r.Max.Y))
	x2, y2 := m.apply(float64(r.Min.X), float64(r.Max.Y))
	x3, y3 := m.apply(float64(r.Max.X), float64(r.Min.Y))
	out := rectXY(
		int(min(x0, x1, x2, x3)), int(min(y0, y1, y2, y3)),
		int(max(x0, x1, x2, x3)), int(max(y0, y1, y2, y3)),
	)
	return out
}

// placeAffine 发射放置矩阵:像素空间(y 向下)仿射换算为 PDF 原生坐标系
// (y 向上,pt)的 cm 矩阵后叠加到当前变换。推导:原生/像素空间的轴翻转
// F(x, y) = (x, H−y)(H = 页高 pt)夹逼 M = F ∘ P ∘ F,展开得
//
//	A = a   B = −c   C = −b   D = d
//	E = b·H + e   F = H·(1−d) − f   (e, f 已 ×pxToPt)
//
// 恒等阵自检:M = I;镜像/旋转用例由 EXIF 八向与旋转文本的栅格测试锁定
func (r *Renderer) placeAffine(doc *fpdf.Fpdf, m affine) {
	H := float64(r.height) * pxToPt
	doc.Transform(fpdf.TransformMatrix{
		A: m.a,
		B: -m.c,
		C: -m.b,
		D: m.d,
		E: m.b*H + m.e*pxToPt,
		F: H*(1-m.d) - m.f*pxToPt,
	})
}

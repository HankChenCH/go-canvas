package pdfrenderer

// 文本绘制原语:TTF 子集内嵌(CJK 门槛)+ 对齐锚点落笔 + 角度旋转。
// (x, y) 为对齐语义锚点——水平按 fpdf 实测文本宽度(GetStringWidth,与落笔
// Tj 同一度量路径)偏移,垂直用字体 metrics(GetFontDesc,per-1000 归一)把
// 基线落到锚点语义对应位置;布局层只返回纯对齐锚点,字体基线差统一在渲染端
// 消化(ADR-0003)。内置默认字体兜底 = PDF 核心字体 Helvetica(ASCII-only,
// 语义对齐位图后端 basicfont.Face7x13:ASCII 可渲、CJK 必须真实字体文件)。

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/resolver"

	"github.com/go-pdf/fpdf"
)

// helvetica 标准 14 字体的度量(Adobe Helvetica AFM:Ascender 718 / Descender
// -207,per-1000)。核心字体不走 TTF 解析、fpdf 的 GetFontDesc 对核心字体恒 0
// (AFM 度量未入 Desc),故以常量持有——PDF 标准字体度量是规范值,非实现细节
const (
	helveticaAscentPerEm  = 718
	helveticaDescentPerEm = -207
)

// DrawText implements renderer.Backend:空行零副作用(PHP 同款空串守卫);
// 水平对齐偏移 = 实测文本宽度(GetStringWidth,与落笔同一度量路径)× 对齐语义;
// 垂直基线 = 锚点 + metrics 偏移(top: +ascent / center: +(ascent+descent)/2 /
// bottom: -descent,descent 为负,未知取值归 top——image-renderer text.go 同一
// 公式)。角度为度,正值逆时针绕锚点 TransformRotate(GD 惯例对齐);
// 字号 px→pt ×0.75
func (r *Renderer) DrawText(
	line string,
	x, y int,
	fontFile string,
	fontSize int,
	fontColor string,
	horizontalAlign string,
	verticalAlign string,
	angle int,
) error {
	doc, err := r.docf()
	if err != nil {
		return err
	}
	if line == "" {
		return nil
	}
	c, err := parseColor(fontColor)
	if err != nil {
		return fmt.Errorf("解析字色: %w", err)
	}

	sizePt := float64(fontSize) * pxToPt
	if sizePt <= 0 {
		return fmt.Errorf("字号非法: %d", fontSize)
	}

	// 字体分派:空串/纯数字 = 内置默认字体语义 → 核心字体 Helvetica
	// (ASCII-only 兜底);其余按物化路径 TTF 子集内嵌(CJK 硬门槛)
	var ascentPt, descentPt float64
	if fontFile == "" || resolver.IsNumeric(fontFile) {
		doc.SetFont("helvetica", "", sizePt)
		ascentPt = float64(helveticaAscentPerEm) / 1000 * sizePt
		descentPt = float64(helveticaDescentPerEm) / 1000 * sizePt
	} else {
		family, err := r.addUTF8Font(doc, fontFile)
		if err != nil {
			return err
		}
		doc.SetFont(family, "", sizePt)
		// GetFontDesc 空族取当前字体:Ascent/Descent per-1000 归一
		// (UTF8 字体经 hhea/OS2 解析,fpdf 内部完成)
		desc := doc.GetFontDesc("", "")
		ascentPt = float64(desc.Ascent) / 1000 * sizePt
		descentPt = float64(desc.Descent) / 1000 * sizePt
	}
	if doc.Err() {
		return fmt.Errorf("设置字体 %s: %w", fontFile, doc.Error())
	}

	// 水平:笔起点相对锚点的偏移(left: 0 / center: -宽一半 / right: -宽)
	widthPt := doc.GetStringWidth(line)
	var penX float64
	switch horizontalAlign {
	case layer.AlignCenter:
		penX = -widthPt / 2
	case layer.AlignRight:
		penX = -widthPt
	}

	// 垂直:基线相对锚点的偏移,由字体 metrics 消化基线差(ADR-0003);
	// 未知取值归 top(switch 缺省臂,与 image-renderer text.go 同一防线)
	baselineY := ascentPt
	switch verticalAlign {
	case layer.AlignCenter:
		baselineY = (ascentPt + descentPt) / 2
	case layer.AlignBottom:
		baselineY = -descentPt
	}

	// 锚点 px→pt;对齐偏移(penX/baselineY)已由实测宽度/metrics 以 pt 计,
	// 不再乘换算系数——单位混乘会把对齐整体缩水 0.75 倍
	tx := float64(x)*pxToPt + penX
	ty := float64(y)*pxToPt + baselineY

	doc.SetAlpha(float64(c.A)/255, "Normal")
	doc.SetTextColor(int(c.R), int(c.G), int(c.B))
	if angle == 0 {
		doc.Text(tx, ty, line)
	} else {
		// 正值逆时针绕锚点旋转:fpdf TransformRotate 在 PDF 原生空间(y 向上)
		// 构造标准逆时针阵,视觉方向与位图后端([[cos,sin],[-sin,cos]] 在
		// y 向下空间)一致;对齐偏移随落笔点一起绕锚点旋转
		doc.TransformBegin()
		doc.TransformRotate(float64(angle), float64(x)*pxToPt, float64(y)*pxToPt)
		doc.Text(tx, ty, line)
		doc.TransformEnd()
	}
	doc.SetAlpha(1, "Normal")
	if doc.Err() {
		return fmt.Errorf("绘制文本: %w", doc.Error())
	}
	return nil
}

// addUTF8Font 注册 TTF 子集内嵌字体,返回合成字体族名。族名 = "f" + 路径
// sha256 前 16 位hex——**不得用原始路径**:路径会原样写进 PDF Name 对象
// (/BaseFont),其中的 "/" 与空格破坏字典解析(poppler 报 "Dictionary key
// must be a name object" 且字体面整体降级)。同路径复用;fpdf 按字体键去重,
// 整文档同一字体只嵌一份子集。TTC/OTC 集合显式拒收:fpdf 的 UTF8 解析器
// 不认 ttcf 魔数且失败路径不设错误位(仅向 stdout 打日志)——静默吞掉会留到
// 后续 SetFont 才爆"undefined font",故先魔数嗅探、再以字体描述符非零校验注册
// 确已生效(解析失败的字节缓存不得入册)
func (r *Renderer) addUTF8Font(doc *fpdf.Fpdf, fontFile string) (string, error) {
	if family, ok := r.fonts[fontFile]; ok {
		return family, nil // 本会话已注册
	}
	data, err := os.ReadFile(fontFile)
	if err != nil {
		return "", fmt.Errorf("读取字体 %s: %w", fontFile, err)
	}
	if bytes.HasPrefix(data, []byte("ttcf")) {
		return "", fmt.Errorf("字体 %s 为 TTC/OTC 字体集:PDF 子集内嵌仅支持单体 TTF", fontFile)
	}
	sum := sha256.Sum256([]byte(fontFile))
	family := fmt.Sprintf("f%x", sum[:8])
	doc.AddUTF8FontFromBytes(family, "", data)
	if doc.Err() {
		return "", fmt.Errorf("注册字体 %s: %w", fontFile, doc.Error())
	}
	if (fpdf.FontDescType{} == doc.GetFontDesc(family, "")) {
		return "", fmt.Errorf("注册字体 %s: 字体解析失败(TTF 格式不合法或为 fpdf 不支持的变体)", fontFile)
	}
	if r.fonts == nil {
		r.fonts = make(map[string]string)
	}
	r.fonts[fontFile] = family
	return family, nil
}

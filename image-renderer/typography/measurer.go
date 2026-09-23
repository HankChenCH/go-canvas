package typography

// 真实字体度量:opentype Face 的 GlyphAdvance/Kern 累加实现核心 text.TextMeasurer
// 接口(研究文档 §4.3 选型)。默认仍是核心启发式对齐版(ADR-0003);本实现经
// text.MeasurerFactory 接缝注入,追求排版质量时启用。

import (
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/hankchen/go-canvas/resolver"
	"github.com/hankchen/go-canvas/text"
)

// OpenTypeMeasurerFactory 真实字体度量器工厂,类型与核心 text.MeasurerFactory
// 接缝对齐,可直接作 WithMeasurerFactory 实参。字号即像素(DPI 72,与渲染端一致)。
//
// 字体值语义与渲染端同口径:空串/纯数字 → 内置默认字体(basicfont 点阵)度量;
// 其余按字体文件路径加载真实字体。加载失败(文件缺失/解析失败/远程字体尚未经
// ResourceResolver 物化)时**降级为核心启发式度量**,布局不中断——降级路径即
// 默认对齐版行为。
//
// 并发注意(ADR-0003 同款约束):返回的度量器持有 opentype.Face,**非并发安全**,
// 限单 goroutine 串行使用;工厂每次调用新建独立实例,跨 goroutine 各自经工厂获取。
// 解析结果(*opentype.Font)进程内缓存共享,Face 构建成本因此摊薄
func OpenTypeMeasurerFactory(fontFile string, fontSize float64) text.TextMeasurer {
	if fontFile == "" || resolver.IsNumeric(fontFile) {
		return &OpenTypeMeasurer{face: BuiltinFace()}
	}

	face, err := LoadFontFace(fontFile, fontSize)
	if err != nil {
		return text.NewHeuristicMeasurer(fontSize)
	}
	return &OpenTypeMeasurer{face: face}
}

// OpenTypeMeasurer 真实字体度量器:逐 rune 累加 GlyphAdvance 与相邻 Kern。
// 实例非并发安全(持有 Face),见工厂注
type OpenTypeMeasurer struct {
	face font.Face
}

// Measure implements text.TextMeasurer:返回渲染宽度(像素)。
// 累加语义复刻 font.MeasureString(渲染端 font.Drawer.MeasureString 的实现体,
// 绘内对齐的同一度量路径)——逐 rune 累加 GlyphAdvance 与相邻 Kern,**忽略缺字形
// ok 标志无条件累加**:opentype 缺字形 advance 为 0;basicfont 缺字形映射 U+FFFD
// 替换字形(宽 7px)。两种面都由 Face 自己定义,本实现只照抄口径
func (m *OpenTypeMeasurer) Measure(s string) float64 {
	var advance fixed.Int26_6
	prev := rune(-1)
	for _, r := range s {
		if prev >= 0 {
			advance += m.face.Kern(prev, r)
		}
		adv, _ := m.face.GlyphAdvance(r)
		advance += adv
		prev = r
	}
	return float64(advance) / 64
}

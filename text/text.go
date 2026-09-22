// Package text 文本子系统契约与对齐版默认实现:度量、断行、字素切分接缝。
// 核心 module 零第三方依赖(ADR-0002),依赖方向 text ← layer ← canvas——
// 图层经契约消费文本能力,宽度来源(启发式估算/字体度量表)与切分实现均可注入(ADR-0003)。
package text

// TextMeasurer 文本宽度度量器(已配置好字体/字号等上下文)。
// 度量与断行分离:断行器只关心"这一段放不放得下",宽度从哪来(启发式估算/
// 字体度量表/驱动测量)由实现决定
type TextMeasurer interface {
	// Measure 度量一段文本的渲染宽度(像素)
	Measure(text string) float64
}

// MeasurerFactory 度量器工厂(策略注入点):函数类型即工厂。
// 图层字号各不相同,注入工厂而非度量器实例,保证所有图层按自身字体/字号
// 得到同一种度量策略(例如统一切换为字体度量表实现)
type MeasurerFactory func(fontFile string, fontSize float64) TextMeasurer

// LineBreaker 断行器:把文本按盒宽断行为若干行
type LineBreaker interface {
	// BreakText 断行。text 为原始文本,显式换行符保留(空段产出空行);
	// boxWidth 为内容盒宽度(像素);m 为已配置的宽度度量器。
	// 返回断行后的行内容(不含换行符)
	BreakText(text string, boxWidth float64, m TextMeasurer) []string
}

// Segmenter 字素簇切分接缝:断行的最小单元应是"用户感知字符"——emoji 的
// ZWJ 组合序列、组合字符都算一个单元(UAX #29)。
// 默认实现 CodePointSegmenter 按**码点**切分,等价 PHP 无 ext-intl 的降级路径
// (组合序列会被拆开,仅作零依赖降级);增强的 UAX #29 切分实现经本接口注入(M3)。
type Segmenter interface {
	// Split 把文本切为字素簇序列;空文本返回空序列
	Split(text string) []string
}

// CodePointSegmenter 默认字素切分:按码点切分(等价 PHP 无 ext-intl 的降级路径)
type CodePointSegmenter struct{}

// Split implements Segmenter:range 遍历按码点切分
func (CodePointSegmenter) Split(text string) []string {
	if text == "" {
		return nil
	}
	out := make([]string, 0, len(text))
	for _, r := range text {
		out = append(out, string(r))
	}
	return out
}

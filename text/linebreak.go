package text

import (
	"slices"
	"strings"
)

// Uax14LineBreaker UAX #14 简化版断行器(默认实现,逐条照搬 PHP 同名类)
//
//   - 以字素簇为最小单元,emoji 等组合序列不被拆碎(切分经 Segmenter 接缝,
//     默认码点切分 = PHP 无 ext-intl 的降级路径,UAX #29 实现可注入);
//   - CJK 字间皆可断;拉丁文本优先在空格/连字符处断行(词边界优先,装不下再逐字硬断);
//   - 禁则处理(kinsoku):行首不出现收尾类标点(上移到上一行,允许轻微溢出),
//     行末不出现起始类标点(下移到下一行行首);
//   - 断点处剔除行尾与行首的空白。
//
// 完整的 UAX #14 规则表与 Knuth–Plass 全局最优断行不在本实现范围内,
// 需要时可通过 LineBreaker 接口注入更强实现
type Uax14LineBreaker struct {
	segmenter Segmenter
}

// NewUax14LineBreaker 构造默认断行器:字素切分用码点降级路径
func NewUax14LineBreaker() *Uax14LineBreaker {
	return &Uax14LineBreaker{segmenter: CodePointSegmenter{}}
}

// WithSegmenter 注入字素簇切分实现(等价 PHP 有 ext-intl 的正常路径);nil 忽略
func (b *Uax14LineBreaker) WithSegmenter(s Segmenter) *Uax14LineBreaker {
	if s != nil {
		b.segmenter = s
	}
	return b
}

// lineStartForbidden 行首禁则:收尾类标点不能出现在行首(逐字符照搬 PHP,27 个)
var lineStartForbidden = strings.Fields(`
， 。 、 ； ： ！ ？ 」 』 ） 】 》 〉 … — ～ · ! ? % , . ; : ) ] }
`)

// lineEndForbidden 行末禁则:起始类标点不能出现在行末(逐字符照搬 PHP,11 个)
var lineEndForbidden = strings.Fields(`
「 『 （ 【 《 〈 “ ‘ ( [ {
`)

// BreakText implements LineBreaker。
// 过程照搬 PHP:显式换行分段(空段保留空行)→ 逐簇贪心收录(行首首簇无条件收录,
// 保证断行过程必然前进)→ 放不下时定断点(词边界优先 + 禁则修正 + 剔除断点空白)。
func (b *Uax14LineBreaker) BreakText(text string, boxWidth float64, m TextMeasurer) []string {
	if text == "" {
		return nil
	}

	var lines []string
	for _, segment := range strings.Split(text, "\n") {
		chars := b.segmenter.Split(segment)
		if len(chars) == 0 {
			// 空段保留为空行,尊重显式换行的版式意图
			lines = append(lines, "")
			continue
		}

		var current []string
		for _, char := range chars {
			// 行首第一个字符无条件收录,保证断行过程必然前进
			if len(current) == 0 || fits(join(current)+char, boxWidth, m) {
				current = append(current, char)
				continue
			}

			line, rest := resolveBreak(current, char)
			if len(line) > 0 {
				lines = append(lines, join(line))
			}
			current = rest
		}

		if len(current) > 0 {
			lines = append(lines, join(current))
		}
	}

	return lines
}

func join(clusters []string) string { return strings.Join(clusters, "") }

func fits(s string, boxWidth float64, m TextMeasurer) bool {
	return m.Measure(s) <= boxWidth
}

// resolveBreak 在 current 末尾放不下 char 时确定断点,返回 [上一行, 余下内容](字素簇序列)
func resolveBreak(current []string, char string) (line, rest []string) {
	line = current
	rest = []string{char}

	// 词边界优先:空格断点弃在行尾,连字符断点保留在行尾
	if breakAt := findWordBoundary(current); breakAt != -1 {
		keep := 0
		if current[breakAt] == "-" {
			keep = 1
		}
		head := current[:breakAt+keep]
		tail := current[breakAt+1:]
		if len(head) > 0 {
			line = head
			rest = append(append(make([]string, 0, len(tail)+1), tail...), char)
		}
	}

	line = trimRightClusters(line)
	rest = trimLeftClusters(rest)

	return applyKinsoku(line, rest)
}

// findWordBoundary 返回词边界断点(空格/连字符所在下标,从行尾向前找),没有则 -1。
// 下标 0 不作断点(对齐 PHP 循环条件 i > 0)
func findWordBoundary(line []string) int {
	for i := len(line) - 1; i > 0; i-- {
		if line[i] == " " || line[i] == "-" {
			return i
		}
	}
	return -1
}

// applyKinsoku 禁则修正:收尾类标点上移到上一行行尾,起始类标点下移到下一行行首。
// 按字素簇整体比对与搬移;PHP 的 mb_substr 按码点索引——码点路径下两者精确等价,
// 注入簇切分后,复合簇(如标点+变体选择符)PHP 取首码点仍命中禁则而本实现整簇比对
// 不命中,属簇语义下更有原则的行为(仅注入路径可触发)
func applyKinsoku(line, rest []string) ([]string, []string) {
	for len(rest) > 0 && slices.Contains(lineStartForbidden, rest[0]) {
		line = append(slices.Clone(line), rest[0])
		rest = rest[1:]
	}

	for len(line) > 0 && slices.Contains(lineEndForbidden, line[len(line)-1]) {
		rest = append([]string{line[len(line)-1]}, rest...)
		line = line[:len(line)-1]
	}

	return line, rest
}

// trimRightClusters 剔除行尾空白簇(PHP rtrim " \t")
func trimRightClusters(line []string) []string {
	for len(line) > 0 && isBlank(line[len(line)-1]) {
		line = line[:len(line)-1]
	}
	return line
}

// trimLeftClusters 剔除行首空白簇(PHP ltrim " \t")
func trimLeftClusters(rest []string) []string {
	for len(rest) > 0 && isBlank(rest[0]) {
		rest = rest[1:]
	}
	return rest
}

func isBlank(cluster string) bool { return cluster == " " || cluster == "\t" }

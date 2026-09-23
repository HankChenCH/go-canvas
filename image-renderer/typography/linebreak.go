package typography

// 完整 UAX #14 断行:go-text/typesetting LineIterator 给出全部断行机会
// (研究文档 §4.3 选型),贪心按盒宽定行。默认仍是核心对齐版贪心断行器
// (ADR-0003);本实现经 text.LineBreaker 接缝注入,追求排版质量时启用。
// 与核心版的语义差异(词内不硬断/CR LF/LS/结尾换行等)在测试表中逐条标注。

import (
	"strings"

	"github.com/go-text/typesetting/segmenter"

	"github.com/hankchen/go-canvas/text"
)

// Uax14LineBreaker 完整 UAX #14 断行器:断点候选来自 LineIterator
// (软断点 = 空格串后/CJK 字间等,强制断点 = LF/CR LF/LS/NEL 及文末),
// 在候选之上按盒宽贪心定行。零状态,值类型可共享;每次调用用独立的局部
// Segmenter,多 goroutine 并发调用安全(度量器的并发约束见其自身文档)
type Uax14LineBreaker struct{}

// NewUax14LineBreaker 构造完整 UAX #14 断行器
func NewUax14LineBreaker() *Uax14LineBreaker { return &Uax14LineBreaker{} }

var _ text.LineBreaker = (*Uax14LineBreaker)(nil)

// mandatoryBreakRunes 强制换行符集合(CR LF 成对给出,按字符集剥离即同时覆盖)
const mandatoryBreakRunes = "\r\n\u0085\u2028\u2029"

// BreakText implements text.LineBreaker。
// LineIterator 的行段含行尾空格串与行尾强制换行符本身(UAX #14 断点在空格串后,
// 空格归属旧行;LB3 恒在文末产出强制断行):剥离换行符还原段内容后,
// 按段贪心收录——行首无条件收录保证过程必然前进(超宽段独占一行溢出),
// 放不下时定行,行尾/新行行首的空格剔除。
// 非法 UTF-8 时 typesetting 拒绝解析,降级核心对齐版断行器
func (b *Uax14LineBreaker) BreakText(s string, boxWidth float64, m text.TextMeasurer) []string {
	if s == "" {
		return nil
	}

	var seg segmenter.Segmenter
	if err := seg.InitWithString(s); err != nil {
		return text.NewUax14LineBreaker().BreakText(s, boxWidth, m)
	}

	var lines []string
	current := ""
	it := seg.LineIterator()
	for it.Next() {
		ln := it.Line()
		word := strings.TrimRight(string(ln.Text), mandatoryBreakRunes)

		switch {
		case current == "":
			current = word
		case m.Measure(current+word) <= boxWidth:
			current += word
		default:
			lines = append(lines, strings.TrimRight(current, " \t"))
			current = strings.TrimLeft(word, " \t")
		}

		if ln.IsMandatoryBreak {
			lines = append(lines, strings.TrimRight(current, " \t"))
			current = ""
		}
	}
	return lines
}

package text_test

// Uax14LineBreakerTest 平移:照搬版贪心断行器 12 例,期望值逐条来自 phpunit 断言。
// PHP 测试跑在有 ext-intl 的环境(字素簇切分);Go 默认码点切分(降级路径),
// emoji 用例经注入字素桩(等价 ext-intl 行为)锁定切分接缝,完整 UAX #29 属 M3。

import (
	"reflect"
	"testing"

	"github.com/HankChenCH/go-canvas/text"
)

const fontSize10 = 10

// breakerMeasurer 度量器按字号 10 估算:半角字符 5.5px,全角字符 10px(PHP 测试同款)
func breakerMeasurer() text.TextMeasurer {
	return text.NewHeuristicMeasurer(fontSize10)
}

// zwjSegmenter 字素桩:等价 ext-intl 的最小行为——ZWJ 组合序列(U+200D 连接)并为一簇,
// 其余码点独立。足以覆盖家庭 emoji 用例;完整 UAX #29 切分属 M3 增强实现
type zwjSegmenter struct{}

const zwj = rune(0x200D)

func (zwjSegmenter) Split(s string) []string {
	runes := []rune(s)
	var out []string
	for i := 0; i < len(runes); {
		j := i + 1
		for j+1 <= len(runes)-1 && runes[j] == zwj {
			j += 2 // ZWJ + 后续码点并入当前簇
		}
		out = append(out, string(runes[i:j]))
		i = j
	}
	return out
}

func TestCjkBreaksAtAnyCharacter(t *testing.T) {
	// PHP Uax14LineBreakerTest::testCjkBreaksAtAnyCharacter:每字 10px,盒宽 20px → 每行两字
	got := text.NewUax14LineBreaker().BreakText("一二三四五", 20, breakerMeasurer())
	if !reflect.DeepEqual(got, []string{"一二", "三四", "五"}) {
		t.Errorf("BreakText = %q, want [一二 三四 五]", got)
	}
}

func TestEnglishPrefersWordBoundaryOverMidWordBreak(t *testing.T) {
	// PHP Uax14LineBreakerTest::testEnglishPrefersWordBoundaryOverMidWordBreak:
	// "hello w" 放到 40px 时 'o' 溢出,回退到空格断点
	got := text.NewUax14LineBreaker().BreakText("hello world", 40, breakerMeasurer())
	if !reflect.DeepEqual(got, []string{"hello", "world"}) {
		t.Errorf("BreakText = %q, want [hello world]", got)
	}
}

func TestHyphenBreakKeepsHyphenAtLineEnd(t *testing.T) {
	// PHP Uax14LineBreakerTest::testHyphenBreakKeepsHyphenAtLineEnd
	got := text.NewUax14LineBreaker().BreakText("co-op", 20, breakerMeasurer())
	if !reflect.DeepEqual(got, []string{"co-", "op"}) {
		t.Errorf("BreakText = %q, want [co- op]", got)
	}
}

func TestLineStartForbiddenPunctuationMovesUp(t *testing.T) {
	// PHP Uax14LineBreakerTest::testLineStartForbiddenPunctuationMovesUp:
	// '，' 不能落到行首 → 上移到上一行行尾,允许上一行轻微溢出
	got := text.NewUax14LineBreaker().BreakText("一二三，四五六", 30, breakerMeasurer())
	if !reflect.DeepEqual(got, []string{"一二三，", "四五六"}) {
		t.Errorf("BreakText = %q, want [一二三， 四五六]", got)
	}
}

func TestLineEndForbiddenPunctuationMovesDown(t *testing.T) {
	// PHP Uax14LineBreakerTest::testLineEndForbiddenPunctuationMovesDown:
	// '（' 不能留在行尾 → 下移到下一行行首;'（三四' 恰好 30px 放得下
	got := text.NewUax14LineBreaker().BreakText("一二（三四", 30, breakerMeasurer())
	if !reflect.DeepEqual(got, []string{"一二", "（三四"}) {
		t.Errorf("BreakText = %q, want [一二 （三四]", got)
	}
}

func TestTrailingWhitespaceTrimmedAtBreak(t *testing.T) {
	// PHP Uax14LineBreakerTest::testTrailingWhitespaceTrimmedAtBreak
	got := text.NewUax14LineBreaker().BreakText("ab cd", 20, breakerMeasurer())
	if !reflect.DeepEqual(got, []string{"ab", "cd"}) {
		t.Errorf("BreakText = %q, want [ab cd]", got)
	}
}

func TestExplicitNewlinesPreserved(t *testing.T) {
	// PHP Uax14LineBreakerTest::testExplicitNewlinesPreserved
	got := text.NewUax14LineBreaker().BreakText("ab\ncd", 100, breakerMeasurer())
	if !reflect.DeepEqual(got, []string{"ab", "cd"}) {
		t.Errorf("BreakText = %q, want [ab cd]", got)
	}
}

func TestEmptySegmentKeptAsEmptyLine(t *testing.T) {
	// PHP Uax14LineBreakerTest::testEmptySegmentKeptAsEmptyLine:空段保留为空行
	got := text.NewUax14LineBreaker().BreakText("a\n\nb", 100, breakerMeasurer())
	if !reflect.DeepEqual(got, []string{"a", "", "b"}) {
		t.Errorf("BreakText = %q, want [a  b](含空行)", got)
	}
}

func TestEmptyTextProducesNoLines(t *testing.T) {
	// PHP Uax14LineBreakerTest::testEmptyTextProducesNoLines
	if got := text.NewUax14LineBreaker().BreakText("", 100, breakerMeasurer()); len(got) != 0 {
		t.Errorf("BreakText = %q, want 无行", got)
	}
}

func TestLongWordHardBreaksWhenNoBoundary(t *testing.T) {
	// PHP Uax14LineBreakerTest::testLongWordHardBreaksWhenNoBoundary
	got := text.NewUax14LineBreaker().BreakText("abcdefghij", 20, breakerMeasurer())
	if !reflect.DeepEqual(got, []string{"abc", "def", "ghi", "j"}) {
		t.Errorf("BreakText = %q, want [abc def ghi j]", got)
	}
}

func TestSingleCharacterWiderThanBoxStillRendered(t *testing.T) {
	// PHP Uax14LineBreakerTest::testSingleCharacterWiderThanBoxStillRendered
	got := text.NewUax14LineBreaker().BreakText("一", 5, breakerMeasurer())
	if !reflect.DeepEqual(got, []string{"一"}) {
		t.Errorf("BreakText = %q, want [一]", got)
	}
}

func TestEmojiZWJSequenceKeptAsOneUnit(t *testing.T) {
	// PHP Uax14LineBreakerTest::testEmojiZWJSequenceKeptAsOneUnit:
	// 家庭 emoji(多人 ZWJ 组合)按一个字素簇计宽与断行,不拆碎(注入字素桩等价 ext-intl)
	family := "👨‍👩‍👧"
	got := text.NewUax14LineBreaker().WithSegmenter(zwjSegmenter{}).BreakText(family+"👍", 15, breakerMeasurer())
	if !reflect.DeepEqual(got, []string{family, "👍"}) {
		t.Errorf("BreakText = %q, want [%s 👍]", got, family)
	}
}

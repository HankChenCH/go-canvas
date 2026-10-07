package typography_test

// Uax14LineBreaker(完整版)测试:typesetting LineIterator 给出全部 UAX #14
// 断行机会,贪心按盒宽定行。表驱动与核心对齐版并存——core 字段锁定同输入下
// 核心断行器的期望(验证两者确实并存、各司其职);diff 非空的用例即工单要求的
// 「与 PHP 预期行数不同的用例单独标注原因」。
// 度量用 perRuneMeasurer(每码点 10px),与具体字体无关,断行算术可控。

import (
	"reflect"
	"testing"

	"github.com/HankChenCH/go-canvas/image-renderer/typography"
	"github.com/HankChenCH/go-canvas/text"
)

// seam 锁定:实现可注入核心断行器接缝
var _ text.LineBreaker = typography.NewUax14LineBreaker()

func TestUax14LineBreaker(t *testing.T) {
	const family = "👨‍👩‍👧‍👦"
	tests := []struct {
		name string
		in   string
		box  float64
		want []string
		core []string // 核心对齐版断行器同输入期望;nil 跳过比对
		diff string   // 与 PHP 对齐版行为不同的原因(工单 09 要求标注)
	}{
		{"空文本", "", 100, nil, nil, ""},
		{"单行不换行", "abc", 30, []string{"abc"}, []string{"abc"}, ""},
		{"CJK 逐字断行", "中文断行", 20, []string{"中文", "断行"}, []string{"中文", "断行"}, ""},
		{"拉丁词边界优先", "ab cd", 20, []string{"ab", "cd"}, []string{"ab", "cd"}, ""},
		{"连字符保留行尾", "co-op", 30, []string{"co-", "op"}, []string{"co-", "op"}, ""},
		{"断点空白剔除", "ab cd e", 30, []string{"ab", "cd", "e"}, []string{"ab", "cd", "e"}, ""},
		{"结尾行尾空格剔除", "ab ", 30,
			[]string{"ab"}, []string{"ab "},
			"UAX #14 文末恒有强制断行,定行剔除行尾空白;PHP 简化版只剔除断点空白,文末行尾空格保留"},
		{"行末禁则起始标点下移", "ab（cd", 30, []string{"ab", "（cd"}, []string{"ab", "（cd"}, ""},
		{"行首禁则收尾标点粘前段", "ab，cd", 30, []string{"ab，", "cd"}, []string{"ab，", "cd"}, ""},
		{"单字超宽仍渲染", "中", 5, []string{"中"}, []string{"中"}, ""},
		{"显式换行空行保留", "a\n\nb", 100, []string{"a", "", "b"}, []string{"a", "", "b"}, ""},
		{"长词溢出不硬断", "abcdefgh", 30,
			[]string{"abcdefgh"}, []string{"abc", "def", "gh"},
			"完整 UAX #14 词内无断行机会,超宽单词独占一行溢出;PHP 简化版逐字硬断"},
		{"数值串保持完整", "3.14 x", 30,
			[]string{"3.14", "x"}, []string{"3.1", "4 x"},
			"UAX #14 数值规则数字串内无断行机会;PHP 简化版无此规则,逐字硬断"},
		{"ZWJ emoji 不拆碎", family + "x", 50,
			[]string{family, "x"}, []string{"👨‍👩‍👧", "\u200d👦x"},
			"核心默认码点切分把 ZWJ 序列撕到两行;UAX #14 emoji 内无断行机会(本断行器内建,无需另注字素)"},
		{"显式结尾换行不补空行", "a\n", 100,
			[]string{"a"}, []string{"a", ""},
			"UAX #14 单次强制断行;PHP explode 语义在结尾换行后多产出一个空行"},
		{"CR LF 成对消化", "a\r\nb", 100,
			[]string{"a", "b"}, []string{"a\r", "b"},
			"UAX #14 识别 CR LF 为一个强制换行;PHP explode 只按 LF 分段,CR 残留行尾"},
		{"行分隔符 U+2028", "a\u2028b", 100,
			[]string{"a", "b"}, []string{"a\u2028b"},
			"UAX #14 识别 LS/NEL/VT/FF 等强制换行;PHP 简化版只认 LF"},
		{"VT FF 强制断行", "a\vb", 100,
			[]string{"a", "b"}, []string{"a\vb"},
			"UAX #14 BK 类(VT/FF)为强制换行;PHP 简化版只认 LF"},
		{"行首空格断行产出空行", " a", 10,
			[]string{"", "a"}, []string{"a"},
			"UAX #14 在行首空格后允许断行(空行);PHP 简化版定行时不产出空行"},
		{"TAB 后断行", "aaaa\tbb", 30,
			[]string{"aaaa", "bb"}, []string{"aaa", "a\tb", "b"},
			"UAX #14 TAB(BA 类)后有断行机会;PHP 简化版只认空格/连字符词边界,此处逐字硬断"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := typography.NewUax14LineBreaker().BreakText(tt.in, tt.box, perRuneMeasurer(10))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BreakText(%q, %v) = %q, want %q", tt.in, tt.box, got, tt.want)
			}
			if tt.core == nil {
				return
			}
			coreGot := text.NewUax14LineBreaker().BreakText(tt.in, tt.box, perRuneMeasurer(10))
			if !reflect.DeepEqual(coreGot, tt.core) {
				t.Errorf("对齐版并存比对: BreakText(%q, %v) = %q, want %q(%s)",
					tt.in, tt.box, coreGot, tt.core, tt.diff)
			}
		})
	}
}

func TestUax14LineBreakerWithRealFont(t *testing.T) {
	// 真实字体端到端:盒宽取 goregular 下「Hello Wor」的真实宽度,
	// 「Hello World」放不下 → 在词边界定行,两行均不超盒
	path := writeGoRegular(t)
	m := typography.OpenTypeMeasurerFactory(path, 16)
	box := m.Measure("Hello Wor")

	got := typography.NewUax14LineBreaker().BreakText("Hello World", box, m)
	want := []string{"Hello", "World"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BreakText = %q, want %q", got, want)
	}
	for _, line := range got {
		if w := m.Measure(line); w > box {
			t.Errorf("行 %q 宽 %v 超盒 %v", line, w, box)
		}
	}
}

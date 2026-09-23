package typography_test

// GraphemeSegmenter 测试:UAX #29 字素簇切分,适配核心 text.Segmenter 接缝。
// 期望值来自 UAX #29 语义(ZWJ 组合序列/regional pair/组合字符为单簇),
// 经 go-text/typesetting GraphemeIterator 实现。

import (
	"reflect"
	"testing"

	"github.com/hankchen/go-canvas/image-renderer/typography"
	"github.com/hankchen/go-canvas/text"
)

// seam 锁定:实现可注入核心字素接缝
var _ text.Segmenter = typography.NewGraphemeSegmenter()

func TestGraphemeSegmenter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"空文本", "", nil},
		{"ASCII 逐码点", "ab", []string{"a", "b"}},
		{"CJK 逐字", "中文", []string{"中", "文"}},
		{"ZWJ 家庭 emoji 整簇", "👨‍👩‍👧‍👦", []string{"👨‍👩‍👧‍👦"}},
		{"emoji 与后续字符分开", "👨‍👩‍👧‍👦!", []string{"👨‍👩‍👧‍👦", "!"}},
		{"国旗 regional pair 整簇", "🇨🇳cn", []string{"🇨🇳", "c", "n"}},
		{"组合字符单簇", "e\u0301", []string{"e\u0301"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := typography.NewGraphemeSegmenter().Split(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Split(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGraphemeSegmenterInvalidUTF8FallsBackToCodePoints(t *testing.T) {
	// typesetting 拒绝非法 UTF-8 → 降级核心码点切分(等价 PHP 无 ext-intl 路径)
	got := typography.NewGraphemeSegmenter().Split("a\xffb")
	if !reflect.DeepEqual(got, []string{"a", "\uFFFD", "b"}) {
		t.Errorf("Split(非法 UTF-8) = %q, want 码点降级 [a \\uFFFD b]", got)
	}
}

func TestGraphemeSegmenterRepairsCoreCodePointFallback(t *testing.T) {
	// 注入核心对齐版断行器:补齐核心默认码点切分的降级缺口——
	// ZWJ emoji 在窄盒下不再被拆碎换行(默认码点切分会把簇拆到两行)
	breaker := text.NewUax14LineBreaker().WithSegmenter(typography.NewGraphemeSegmenter())

	// 每码点 10px、家庭 emoji 7 码点 = 70px,盒宽 50:
	// 簇完整 → emoji 独占一行;码点降级会把前 5 个码点撕到第一行
	const family = "👨‍👩‍👧‍👦"
	lines := breaker.BreakText(family+"x", 50, perRuneMeasurer(10))
	want := []string{family, "x"}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("簇注入后断行 = %q, want %q", lines, want)
	}
}

// perRuneMeasurer 测量假实现:每码点固定宽度,断行算法测试用(与具体字体无关)
type perRuneMeasurer float64

func (m perRuneMeasurer) Measure(s string) float64 {
	w := 0.0
	for range s {
		w += float64(m)
	}
	return w
}

package text_test

// HeuristicMeasurerTest 平移:启发式度量器按字符分类计权(可打印 ASCII 0.55 字宽、
// 其余 1.0),乘以字号;期望值逐条来自 phpunit 断言。

import (
	"math"
	"testing"

	"github.com/HankChenCH/go-canvas/text"
)

const delta = 0.001

func expectMeasure(t *testing.T, m text.TextMeasurer, s string, want float64) {
	t.Helper()
	if got := m.Measure(s); math.Abs(got-want) > delta {
		t.Errorf("Measure(%q) = %v, want %v", s, got, want)
	}
}

func TestHalfWidthAsciiCountsAsHalfUnit(t *testing.T) {
	// PHP HeuristicMeasurerTest::testHalfWidthAsciiCountsAsHalfUnit:3 个半角 × 0.55 × 12px
	expectMeasure(t, text.NewHeuristicMeasurer(12), "abc", 0.55*3*12)
}

func TestFullWidthCjkCountsAsWholeUnit(t *testing.T) {
	// PHP HeuristicMeasurerTest::testFullWidthCjkCountsAsWholeUnit
	expectMeasure(t, text.NewHeuristicMeasurer(12), "中文", 2.0*12)
}

func TestMixedTextSumsPerCharacterClass(t *testing.T) {
	// PHP HeuristicMeasurerTest::testMixedTextSumsPerCharacterClass
	expectMeasure(t, text.NewHeuristicMeasurer(10), "a中", (0.55+1.0)*10)
}

func TestEmptyTextMeasuresZero(t *testing.T) {
	// PHP HeuristicMeasurerTest::testEmptyTextMeasuresZero
	expectMeasure(t, text.NewHeuristicMeasurer(12), "", 0.0)
}

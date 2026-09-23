package typography_test

// 图层级集成:三组件经核心既有接缝注入 TextLayer,非默认;并验证
// 度量/断行/切分各自可独立替换(工单 09 验收)。

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hankchen/go-canvas/image-renderer/typography"
	"github.com/hankchen/go-canvas/layer"
	"github.com/hankchen/go-canvas/text"
)

func TestTextLayerInjectsEnhancedStack(t *testing.T) {
	// 完整增强栈:真实度量 + 完整 UAX #14,经既有接缝注入,默认仍是核心对齐版
	fontPath := writeGoRegular(t)
	l := layer.NewTextLayer(
		layer.WithSize(80, 0), layer.WithAutoHeight(),
		layer.WithText("Hello wonderful World"), layer.WithFont(fontPath, 16, "#000"),
		layer.WithAutowrap(true),
		layer.WithLineBreaker(typography.NewUax14LineBreaker()),
		layer.WithMeasurerFactory(typography.OpenTypeMeasurerFactory),
	)

	lines := l.Lines()
	if len(lines) < 2 {
		t.Fatalf("真实度量下 80px 盒应断为多行, got %q", lines)
	}
	// 行内容完整(词不被拆碎)且各行宽不超内容盒
	if got, want := strings.Join(lines, " "), "Hello wonderful World"; got != want {
		t.Errorf("断行丢失内容: %q != %q", got, want)
	}
	m := typography.OpenTypeMeasurerFactory(fontPath, 16)
	for _, line := range lines {
		if w := m.Measure(line); w > 80 {
			t.Errorf("行 %q 宽 %v 超盒 80", line, w)
		}
	}
	// 自动高度跟随真实断行行数
	if want := l.LineHeightPx() * len(lines); l.Height() != want {
		t.Errorf("Height() = %d, want 行数×行高 %d", l.Height(), want)
	}
}

func TestEnhancedComponentsIndependentlySwappable(t *testing.T) {
	fontPath := writeGoRegular(t)

	// 增强断行器 + 核心启发式度量:各自可换(度量与断行解耦)
	heuristic := layer.NewTextLayer(
		layer.WithSize(50, 0), layer.WithAutoHeight(),
		layer.WithText("一二三四五六"), layer.WithFont(fontPath, 10, "#000"),
		layer.WithAutowrap(true),
		layer.WithLineBreaker(typography.NewUax14LineBreaker()),
	)
	if got := heuristic.Lines(); !reflect.DeepEqual(got, []string{"一二三四五", "六"}) {
		t.Errorf("增强断行器×启发式度量 Lines() = %q", got)
	}

	// 核心对齐版断行器 + 增强真实度量:反向组合同样成立
	m := typography.OpenTypeMeasurerFactory(fontPath, 16)
	box := int(m.Measure("Hello Wor"))
	mixed := layer.NewTextLayer(
		layer.WithSize(box, 0), layer.WithAutoHeight(),
		layer.WithText("Hello World"), layer.WithFont(fontPath, 16, "#000"),
		layer.WithAutowrap(true),
		layer.WithMeasurerFactory(typography.OpenTypeMeasurerFactory),
	)
	if got := mixed.Lines(); !reflect.DeepEqual(got, []string{"Hello", "World"}) {
		t.Errorf("对齐版断行器×真实度量 Lines() = %q (盒宽 %d)", got, box)
	}
}

// 编译期锁定:增强三组件实现的都是核心接缝类型(与各单测的断言互补)
var (
	_ text.LineBreaker     = typography.NewUax14LineBreaker()
	_ text.MeasurerFactory = typography.OpenTypeMeasurerFactory
	_ text.Segmenter       = typography.NewGraphemeSegmenter()
)

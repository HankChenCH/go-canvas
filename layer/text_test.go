package layer_test

// TextLayerTest 平移:文本图层高度/断行结果/策略注入/graph wire 面,期望值逐条
// 来自 phpunit 断言。适配项:
//   - PHP setter 链改为 functional options(DESIGN.md「有意偏离」#4);
//   - 度量工厂注入用例为工单 02 新增(锁定工厂注入语义);
//   - TextOrigin 用例锁定 ADR-0003 有意偏离(PHP 的 GD 基线魔数不移植)。

import (
	"encoding/json"
	"testing"

	"github.com/hankchen/go-canvas/layer"
	"github.com/hankchen/go-canvas/text"
)

// fakeBreaker 断行假实现:锁定断行器注入语义(PHP 匿名类同款)
type fakeBreaker struct{}

func (fakeBreaker) BreakText(string, float64, text.TextMeasurer) []string {
	return []string{"x", "y", "z"}
}

// fixedMeasurer 度量假实现:任何文本都量出固定宽度
type fixedMeasurer float64

func (m fixedMeasurer) Measure(string) float64 { return float64(m) }

func TestTextFixedHeightReturnsDeclaredHeight(t *testing.T) {
	// PHP TextLayerTest::testFixedHeightReturnsDeclaredHeight
	l := layer.NewTextLayer(layer.WithSize(100, 40), layer.WithText("内容"))

	if got := l.Height(); got != 40 {
		t.Errorf("Height() = %d, want 40", got)
	}
}

func TestTextAutoHeightSingleLineUsesLineHeight(t *testing.T) {
	// PHP TextLayerTest::testAutoHeightSingleLineUsesLineHeight:字号 20、行高 1 → 单行 20px
	l := layer.NewTextLayer(layer.WithSize(100, 0), layer.WithAutoHeight(),
		layer.WithText("内容"), layer.WithFont("", 20, "#000"))

	if got := l.Height(); got != 20 {
		t.Errorf("Height() = %d, want 20", got)
	}
}

func TestTextAutoHeightPaddingOnlyWhenTextEmpty(t *testing.T) {
	// PHP TextLayerTest::testAutoHeightPaddingOnlyWhenTextEmpty
	l := layer.NewTextLayer(layer.WithSize(100, 0), layer.WithAutoHeight(), layer.WithPadding(10))

	if got := l.Height(); got != 20 {
		t.Errorf("Height() = %d, want 20(仅 padding)", got)
	}
}

func TestTextAutoHeightRespectsLineHeight(t *testing.T) {
	// PHP TextLayerTest::testAutoHeightRespectsLineHeight
	l := layer.NewTextLayer(layer.WithSize(100, 0), layer.WithAutoHeight(),
		layer.WithText("内容"), layer.WithFont("", 20, "#000"), layer.WithLineHeight(1.5))

	if got := l.Height(); got != 30 {
		t.Errorf("Height() = %d, want 30", got)
	}
}

func TestTextAutowrapHeightFollowsBrokenLines(t *testing.T) {
	// PHP TextLayerTest::testAutowrapHeightFollowsBrokenLines:
	// 内容盒宽 50,字号 10:每字 10px → '一二三四五六七' 断为 '一二三四五' + '六七'
	l := layer.NewTextLayer(layer.WithSize(50, 0), layer.WithAutoHeight(),
		layer.WithText("一二三四五六七"), layer.WithFont("", 10, "#000"), layer.WithAutowrap(true))

	lines := l.Lines()
	if len(lines) != 2 || lines[0] != "一二三四五" || lines[1] != "六七" {
		t.Errorf("Lines() = %q, want [一二三四五 六七]", lines)
	}
	if got := l.Height(); got != 20 {
		t.Errorf("Height() = %d, want 20", got)
	}
}

func TestTextAutowrapKeepsExplicitNewlines(t *testing.T) {
	// PHP TextLayerTest::testAutowrapKeepsExplicitNewlines
	l := layer.NewTextLayer(layer.WithSize(500, 0), layer.WithAutoHeight(),
		layer.WithText("ab\ncd"), layer.WithAutowrap(true))

	lines := l.Lines()
	if len(lines) != 2 || lines[0] != "ab" || lines[1] != "cd" {
		t.Errorf("Lines() = %q, want [ab cd]", lines)
	}
}

func TestTextGetLinesWithoutAutowrapReturnsSingleLine(t *testing.T) {
	// PHP TextLayerTest::testGetLinesWithoutAutowrapReturnsSingleLine;
	// 适配断言:空文本同样整段单行(工单 02)
	long := "很长很长很长很长很长很长很长很长"
	tests := []struct {
		name string
		text string
	}{
		{"长文本单行", long},
		{"空文本单行", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := layer.NewTextLayer(layer.WithSize(50, 0), layer.WithAutoHeight(), layer.WithText(tt.text))
			lines := l.Lines()
			if len(lines) != 1 || lines[0] != tt.text {
				t.Errorf("Lines() = %q, want [%s]", lines, tt.text)
			}
		})
	}
}

func TestTextLineBreakerInjectable(t *testing.T) {
	// PHP TextLayerTest::testLineBreakerInjectable:假断行器 3 行 → 高度 3×行高
	l := layer.NewTextLayer(layer.WithSize(100, 0), layer.WithAutoHeight(),
		layer.WithText("任意"), layer.WithFont("", 10, "#000"),
		layer.WithAutowrap(true), layer.WithLineBreaker(fakeBreaker{}))

	if got := l.Height(); got != 30 {
		t.Errorf("Height() = %d, want 30", got)
	}
}

func TestTextMeasurerFactoryInjectable(t *testing.T) {
	// 适配新增(工单 02):假度量工厂锁定注入语义——任何文本都量 100px,
	// 盒宽 50 → 'ab' 每字硬断一行
	l := layer.NewTextLayer(layer.WithSize(50, 0), layer.WithAutoHeight(),
		layer.WithText("ab"), layer.WithFont("", 10, "#000"), layer.WithAutowrap(true),
		layer.WithMeasurerFactory(func(string, float64) text.TextMeasurer { return fixedMeasurer(100) }))

	lines := l.Lines()
	if len(lines) != 2 || lines[0] != "a" || lines[1] != "b" {
		t.Errorf("Lines() = %q, want [a b]", lines)
	}
	if got := l.Height(); got != 20 {
		t.Errorf("Height() = %d, want 20", got)
	}
}

func TestTextMeasurerFactoryReceivesResolvedFont(t *testing.T) {
	// 工单 09 适配:真实字体度量必须拿到物化后的本地路径才能加载字体——
	// Lines() 传给度量工厂的字体值是 ResolvedFont()(物化优先,未物化回落原始值);
	// 默认启发式工厂忽略字体值,该口径对其无观察差异
	var gotFont string
	capture := func(fontFile string, _ float64) text.TextMeasurer {
		gotFont = fontFile
		return fixedMeasurer(10)
	}

	l := layer.NewTextLayer(layer.WithSize(100, 0), layer.WithText("文本"),
		layer.WithFont("https://cdn.example.com/msyh.ttf", 12, "#000"),
		layer.WithAutowrap(true),
		layer.WithMeasurerFactory(capture))
	l.Lines()
	if gotFont != "https://cdn.example.com/msyh.ttf" {
		t.Errorf("未物化时工厂收到字体 = %q, want 原始值", gotFont)
	}

	l.SetResolvedFont("/cache/go-canvas/fonts/msyh.ttf")
	l.Lines()
	if gotFont != "/cache/go-canvas/fonts/msyh.ttf" {
		t.Errorf("物化后工厂收到字体 = %q, want 物化路径", gotFont)
	}
}

func TestTextGraphKeepsFullFontValue(t *testing.T) {
	// PHP TextLayerTest::testGraphKeepsFullFontValue:无损——font 保留完整原始值
	// (旧库只存 basename);data 恒含 expression 空串占位(PHP 字节面)
	const fontURL = "https://cdn.example.com/fonts/msyh.ttf"
	l := layer.NewTextLayer(layer.WithSize(10, 10), layer.WithText("文本"),
		layer.WithFont(fontURL, 12, "#f00"))

	got := jsonOf(t, l.Graph())
	want := `{"type":"TextLayer","priority":0,"spec":{"shape":{"width":10,"height":10,` +
		`"autoWidth":false,"autoHeight":false,"lineHeight":1,` +
		`"padding":{"top":0,"bottom":0,"left":0,"right":0},` +
		`"border":{"top":null,"bottom":null,"left":null,"right":null},` +
		`"backgroundColor":null},` +
		`"align":{"horizontal":"left","vertical":"bottom"},` +
		`"position":{"x":0,"y":0,"position":"top-left"},` +
		`"fontFamily":{"font":"https://cdn.example.com/fonts/msyh.ttf","fontSize":12,` +
		`"fontColor":"#f00","angle":0,"autowrap":false}},` +
		`"data":{"valueType":"StaticValue","expression":"","value":"文本"}}`
	if got != want {
		t.Errorf("graph JSON 键结构不符:\n got  %s\n want %s", got, want)
	}
}

func TestTextFromGraphRoundtrip(t *testing.T) {
	// PHP TextLayerTest::testFromGraphRoundtrip:graph → JSON → 解码 → 工厂重建 → graph 恒等
	l := layer.NewTextLayer(layer.WithSize(100, 50), layer.WithBackground("#fff"),
		layer.WithText("正文内容"), layer.WithFont("https://cdn.example.com/fonts/msyh.ttf", 14, "#333"),
		layer.WithAutowrap(true), layer.WithAngle(90), layer.WithPadding(2),
		layer.WithPosition(3, 4), layer.WithPriority(5))

	var node layer.Node
	if err := json.Unmarshal([]byte(jsonOf(t, l.Graph())), &node); err != nil {
		t.Fatalf("解码 graph 节点: %v", err)
	}
	rebuilt, err := layer.FromGraph(node)
	if err != nil {
		t.Fatalf("FromGraph: %v", err)
	}
	textLayer, ok := rebuilt.(*layer.TextLayer)
	if !ok {
		t.Fatalf("重建类型 = %T, want *layer.TextLayer", rebuilt)
	}

	if got, want := jsonOf(t, textLayer.Graph()), jsonOf(t, l.Graph()); got != want {
		t.Errorf("往返 graph 不恒等:\n got  %s\n want %s", got, want)
	}
	if got := textLayer.Text(); got != "正文内容" {
		t.Errorf("重建后 Text() = %q, want 正文内容", got)
	}
	if got := textLayer.FontSize(); got != 14 {
		t.Errorf("重建后 FontSize() = %d, want 14", got)
	}
}

func TestTextDefaultVerticalAlignBottom(t *testing.T) {
	// PHP TextLayer 字段覆写:垂直默认 bottom(基类 top)、水平沿用基类 left
	l := layer.NewTextLayer()

	if l.HorizontalAlign() != layer.AlignLeft || l.VerticalAlign() != layer.AlignBottom {
		t.Errorf("文本图层默认对齐 = (%s, %s), want (left, bottom)", l.HorizontalAlign(), l.VerticalAlign())
	}
}

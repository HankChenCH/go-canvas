package layer_test

// AbstractLayerTest 平移:Go 无匿名子类,基类用例经图片图层承载(图片图层默认 center/center;
// 文本图层落地后其水平 left 默认与垂直 bottom 覆写在 text_test.go 断言,基类垂直 top
// 默认已在 table_test.go 经表格图层补齐——容器图层无 PHP 覆写)。
// 两个 PHP 用例不平移:
//   - testWidthHeightCastToInt('30'→30):字符串 setter 面属 Go 类型系统替代范围(spec Out of Scope);
//   - testSettersAreFluent:fluent 链已被 functional options 替代(DESIGN.md「有意偏离」#4)。

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hankchen/go-canvas/layer"
)

func strPtr(s string) *string { return &s }

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}

func TestDefaultState(t *testing.T) {
	// PHP AbstractLayerTest::testDefaultState
	l := layer.NewImageLayer()

	if l.Width() != 0 {
		t.Errorf("默认宽度 = %d, want 0", l.Width())
	}
	if l.Height() != 0 {
		t.Errorf("默认高度 = %d, want 0", l.Height())
	}
	if anchor, x, y := l.Position(); anchor != layer.AnchorTopLeft || x != 0 || y != 0 {
		t.Errorf("默认定位 = (%s, %d, %d), want (top-left, 0, 0)", anchor, x, y)
	}
	if l.Priority() != 0 {
		t.Errorf("默认 priority = %d, want 0", l.Priority())
	}
	if l.Background() != nil {
		t.Errorf("默认背景 = %v, want nil", *l.Background())
	}
	// 图片图层适配断言:默认 center/center(PHP ImageLayer 字段覆写)
	if l.HorizontalAlign() != layer.AlignCenter || l.VerticalAlign() != layer.AlignCenter {
		t.Errorf("图片图层默认对齐 = (%s, %s), want (center, center)", l.HorizontalAlign(), l.VerticalAlign())
	}
}

func TestContentSizeSubtractsPadding(t *testing.T) {
	// PHP AbstractLayerTest::testContentSizeSubtractsPadding
	l := layer.NewImageLayer(layer.WithSize(100, 50), layer.WithPaddingVH(10, 20))

	if got := l.ContentWidth(); got != 60 {
		t.Errorf("ContentWidth = %d, want 60", got)
	}
	if got := l.ContentHeight(); got != 30 {
		t.Errorf("ContentHeight = %d, want 30", got)
	}
}

func TestPaddingCssStyles(t *testing.T) {
	// PHP AbstractLayerTest::testPaddingCssStyles:1/2/3/4 参各别选项,CSS 展开语义
	tests := []struct {
		name string
		pad  layer.Padding
		want layer.Padding
	}{
		{"1 值全边", layer.NewImageLayer(layer.WithPadding(1)).Padding(),
			layer.Padding{Top: 1, Bottom: 1, Left: 1, Right: 1}},
		{"2 值上下/左右", layer.NewImageLayer(layer.WithPaddingVH(1, 2)).Padding(),
			layer.Padding{Top: 1, Bottom: 1, Left: 2, Right: 2}},
		{"3 值上/左右/下", layer.NewImageLayer(layer.WithPaddingTHB(1, 2, 3)).Padding(),
			layer.Padding{Top: 1, Bottom: 3, Left: 2, Right: 2}},
		{"4 值上右下左", layer.NewImageLayer(layer.WithPaddingTRBL(1, 2, 3, 4)).Padding(),
			layer.Padding{Top: 1, Right: 2, Bottom: 3, Left: 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.pad, tt.want) {
				t.Errorf("padding = %+v, want %+v", tt.pad, tt.want)
			}
		})
	}
}

func TestBorderSetAllAndClearWithZero(t *testing.T) {
	// PHP AbstractLayerTest::testBorderSetAllAndClearWithZero:width=0 即清除(默认色 #000)
	l := layer.NewImageLayer(layer.WithBorder(2, "#f00"), layer.WithBorderTop(0))

	if got := l.Border().Top; got != nil {
		t.Errorf("width=0 后 top 边框 = %+v, want nil", got)
	}
	if got := l.Border().Left; got == nil || *got != (layer.BorderSide{Width: 2, Color: "#f00"}) {
		t.Errorf("left 边框 = %+v, want &{2 #f00}", got)
	}
	if got := l.Border().Bottom; got == nil || got.Color != "#f00" || got.Width != 2 {
		t.Errorf("bottom 边框 = %+v, want &{2 #f00}", got)
	}
	if got := l.Border().Right; got == nil || got.Color != "#f00" || got.Width != 2 {
		t.Errorf("right 边框 = %+v, want &{2 #f00}", got)
	}

	// 缺省颜色 = #000(PHP 默认参数)
	d := layer.NewImageLayer(layer.WithBorder(1)).Border()
	if d.Top == nil || d.Top.Color != layer.DefaultBorderColor {
		t.Errorf("缺省边框色 = %+v, want color #000", d.Top)
	}
}

func TestSetPosition(t *testing.T) {
	// PHP AbstractLayerTest::testSetPosition;锚点串不做校验、直通存储(PHP 'bottom-center' 同款)
	l := layer.NewImageLayer(layer.WithPosition(5, 6, "bottom-center"))

	if anchor, x, y := l.Position(); anchor != "bottom-center" || x != 5 || y != 6 {
		t.Errorf("定位 = (%s, %d, %d), want (bottom-center, 5, 6)", anchor, x, y)
	}

	// 定位含默认锚点:省略 anchor 即 top-left
	d := layer.NewImageLayer(layer.WithPosition(7, 8))
	if anchor, x, y := d.Position(); anchor != layer.AnchorTopLeft || x != 7 || y != 8 {
		t.Errorf("默认锚点定位 = (%s, %d, %d), want (top-left, 7, 8)", anchor, x, y)
	}
}

func TestGraphStructure(t *testing.T) {
	// PHP AbstractLayerTest::testGraphStructure:键名与键序逐字段对齐 PHP graph()(适配:type=ImageLayer、
	// align=center/center 来自图片图层默认;数值 0/1 对应 PHP 输出 0.0/1.0 的 float 字面差异)
	l := layer.NewImageLayer(layer.WithSize(10, 20), layer.WithPriority(2))

	want := `{"type":"ImageLayer","priority":2,"spec":{"shape":{"width":10,"height":20,` +
		`"autoWidth":false,"autoHeight":false,"lineHeight":1,` +
		`"padding":{"top":0,"bottom":0,"left":0,"right":0},` +
		`"border":{"top":null,"bottom":null,"left":null,"right":null},` +
		`"backgroundColor":null},` +
		`"align":{"horizontal":"center","vertical":"center"},` +
		`"position":{"x":0,"y":0,"position":"top-left"}},` +
		`"data":{"valueType":"StaticValue","value":null}}`
	if got := jsonOf(t, l.Graph()); got != want {
		t.Errorf("graph JSON 键结构不符:\n got  %s\n want %s", got, want)
	}
}

func TestAutoFlags(t *testing.T) {
	// PHP AbstractLayerTest::testAutoFlags:auto 尺寸 → 宽高 0、graph 布尔标志置位
	l := layer.NewImageLayer(layer.WithAutoWidth(), layer.WithAutoHeight())

	if l.Width() != 0 || l.Height() != 0 {
		t.Errorf("auto 尺寸宽高 = (%d, %d), want (0, 0)", l.Width(), l.Height())
	}
	g := l.Graph()
	if !g.Spec.Shape.AutoWidth || !g.Spec.Shape.AutoHeight {
		t.Errorf("graph auto 标志 = (%t, %t), want (true, true)", g.Spec.Shape.AutoWidth, g.Spec.Shape.AutoHeight)
	}
}

func TestGraphRoundtripBase(t *testing.T) {
	// PHP AbstractLayerTest::testFromGraphRoundtripBase:graph → JSON → 解码 → 重建 → graph 恒等
	l := layer.NewImageLayer(
		layer.WithSize(30, 40),
		layer.WithPaddingVH(1, 2),
		layer.WithBorder(2, "#123456"),
		layer.WithPosition(3, 4, "bottom-center"),
		layer.WithPriority(7),
	)

	var node layer.Node
	if err := json.Unmarshal([]byte(jsonOf(t, l.Graph())), &node); err != nil {
		t.Fatalf("解码 graph 节点: %v", err)
	}
	rebuilt, err := layer.FromGraph(node)
	if err != nil {
		t.Fatalf("FromGraph: %v", err)
	}
	if rebuilt.TypeName() != layer.TypeImage {
		t.Errorf("重建类型 = %s, want %s", rebuilt.TypeName(), layer.TypeImage)
	}
	if got, want := jsonOf(t, rebuilt.Graph()), jsonOf(t, l.Graph()); got != want {
		t.Errorf("往返 graph 不恒等:\n got  %s\n want %s", got, want)
	}
}

// name/visible 契约(layer-panel-ux 工单 01):缺省态键省略、带值条件写键、往返恒等。
// PHP 对位 AbstractLayerTest::testNameVisible*(字节面/键序三端一致)
func TestNameVisibleWireKeys(t *testing.T) {
	// 缺省态:不写 name/visible 键(字节面与无字段版本一致)
	def := jsonOf(t, layer.NewImageLayer().Graph())
	if strings.Contains(def, `"name"`) || strings.Contains(def, `"visible"`) {
		t.Errorf("缺省态不应写出 name/visible 键: %s", def)
	}
	if !layer.NewImageLayer().Visible() {
		t.Errorf("默认 visible = false, want true")
	}
	if layer.NewImageLayer().DisplayName() != "" {
		t.Errorf("默认 name = %q, want 空串", layer.NewImageLayer().DisplayName())
	}

	// 带键形态:键序钉在 type 之后、priority 之前;visible 仅 false 写键
	l := layer.NewImageLayer(layer.WithSize(10, 20))
	l.SetDisplayName("标题层")
	l.SetVisible(false)
	want := `{"type":"ImageLayer","name":"标题层","visible":false,"priority":0,` +
		`"spec":{"shape":{"width":10,"height":20,` +
		`"autoWidth":false,"autoHeight":false,"lineHeight":1,` +
		`"padding":{"top":0,"bottom":0,"left":0,"right":0},` +
		`"border":{"top":null,"bottom":null,"left":null,"right":null},` +
		`"backgroundColor":null},` +
		`"align":{"horizontal":"center","vertical":"center"},` +
		`"position":{"x":0,"y":0,"position":"top-left"}},` +
		`"data":{"valueType":"StaticValue","value":null}}`
	if got := jsonOf(t, l.Graph()); got != want {
		t.Errorf("带键 graph JSON 不符:\n got  %s\n want %s", got, want)
	}

	// 往返恒等:解码读取 name/visible,再序列化字节一致
	var node layer.Node
	if err := json.Unmarshal([]byte(want), &node); err != nil {
		t.Fatalf("解码 graph 节点: %v", err)
	}
	rebuilt := layer.ImageFromGraph(node)
	if rebuilt.DisplayName() != "标题层" || rebuilt.Visible() {
		t.Errorf("重建 name/visible = (%q, %t), want (标题层, false)", rebuilt.DisplayName(), rebuilt.Visible())
	}
	if got := jsonOf(t, rebuilt.Graph()); got != want {
		t.Errorf("往返 graph 不恒等:\n got  %s\n want %s", got, want)
	}

	// 第三方缺省值形态(name 空串、visible true)归一为缺省 → 再序列化键省略
	var norm layer.Node
	if err := json.Unmarshal([]byte(`{"type":"ImageLayer","name":"","visible":true,"priority":0}`), &norm); err != nil {
		t.Fatalf("解码缺省值形态: %v", err)
	}
	normalized := jsonOf(t, layer.ImageFromGraph(norm).Graph())
	if strings.Contains(normalized, `"name"`) || strings.Contains(normalized, `"visible"`) {
		t.Errorf("缺省值形态未归一: %s", normalized)
	}
}

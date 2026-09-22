package canvas_test

// CanvasTest 平移。PHP testFromGraphRoundtripWithNestedTable 含文本/二维码/表格嵌套,
// 对应类型属工单 02/03;本工单以多图片图层的画布级往返承载,完整平移待 02/03 落地后补全。

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/layer"
)

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}

func derefOr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func TestNewStoresDeclaredSize(t *testing.T) {
	// PHP CanvasTest::testMakeStoresDeclaredSize
	c := canvas.New(100, 80)

	if c.Width() != 100 || c.Height() != 80 {
		t.Errorf("画布尺寸 = (%d, %d), want (100, 80)", c.Width(), c.Height())
	}
}

func TestLayersSortedByPriorityDescWithStableInsertion(t *testing.T) {
	// PHP CanvasTest::testLayersSortedByPriorityDescWithStableInsertion:priority 越大越先,等优先级保持插入序
	lowA := layer.NewImageLayer(layer.WithSize(1, 1), layer.WithImage("low-a.png"), layer.WithPriority(1))
	high := layer.NewImageLayer(layer.WithSize(1, 1), layer.WithImage("high.png"), layer.WithPriority(5))
	lowB := layer.NewImageLayer(layer.WithSize(1, 1), layer.WithImage("low-b.png"), layer.WithPriority(1))

	c := canvas.New(10, 10, lowA)
	c.AddLayer(high)
	c.AddLayer(lowB)

	got := c.GetLayers()
	if len(got) != 3 {
		t.Fatalf("图层数 = %d, want 3", len(got))
	}
	for i, want := range []*layer.ImageLayer{high, lowA, lowB} {
		if got[i] != layer.Layer(want) {
			t.Errorf("排序位 %d 不是期望图层实例(want priority=%d image=%s 的原对象)", i, want.Priority(), derefOr(want.Image()))
		}
	}
}

func TestGraphIsRepeatableAndNonDestructive(t *testing.T) {
	// PHP CanvasTest::testGraphIsRepeatableAndNonDestructive:graph 可重复消费、非排干式
	c := canvas.New(10, 10, layer.NewImageLayer(layer.WithSize(1, 1), layer.WithPriority(3)))

	first := jsonOf(t, c.Graph())
	if again := jsonOf(t, c.Graph()); first != again {
		t.Errorf("graph 不可重复消费:\n first %s\n again %s", first, again)
	}
	if n := len(c.GetLayers()); n != 1 {
		t.Errorf("graph 后图层数 = %d, want 1", n)
	}
}

func TestGraphContainsCanvasSizeAndLayerSpecs(t *testing.T) {
	// PHP CanvasTest::testGraphContainsCanvasSizeAndLayerSpecs:画布尺寸与图层 type/priority 键面
	c := canvas.New(100, 80, layer.NewImageLayer(layer.WithSize(10, 10), layer.WithPriority(2)))

	want := `{"canvas":{"width":100,"height":80},"layers":[{"type":"ImageLayer","priority":2,` +
		`"spec":{"shape":{"width":10,"height":10,"autoWidth":false,"autoHeight":false,"lineHeight":1,` +
		`"padding":{"top":0,"bottom":0,"left":0,"right":0},` +
		`"border":{"top":null,"bottom":null,"left":null,"right":null},"backgroundColor":null},` +
		`"align":{"horizontal":"center","vertical":"center"},` +
		`"position":{"x":0,"y":0,"position":"top-left"}},` +
		`"data":{"valueType":"StaticValue","value":null}}]}`
	if got := jsonOf(t, c.Graph()); got != want {
		t.Errorf("画布 graph JSON 键结构不符:\n got  %s\n want %s", got, want)
	}
}

func TestGetLayersReturnsCopy(t *testing.T) {
	// PHP getLayers 返回值语义(数组值拷贝):Go 切片必须显式副本,外部改写不得影响画布
	l := layer.NewImageLayer(layer.WithSize(1, 1), layer.WithPriority(3))
	c := canvas.New(10, 10, l)

	got := c.GetLayers()
	got[0] = layer.NewImageLayer(layer.WithSize(2, 2))

	if c.GetLayers()[0] != layer.Layer(l) {
		t.Errorf("外部改写 getter 副本影响了画布内部图层")
	}
}

func TestFromGraphRoundtrip(t *testing.T) {
	// PHP CanvasTest::testFromGraphRoundtrip(画布级嵌套适配版:工单 01 仅图片图层)
	cover := layer.NewImageLayer(
		layer.WithSize(100, 100),
		layer.WithBackground("#f00"),
		layer.WithImage("a.png"),
		layer.WithPriority(1),
	)
	fore := layer.NewImageLayer(
		layer.WithSize(30, 20),
		layer.WithAutoHeight(),
		layer.WithLineHeight(1.5),
		layer.WithPaddingTHB(1, 2, 3),
		layer.WithBorder(2, "#123456"),
		layer.WithPosition(3, 4, "bottom-right"),
		layer.WithImage("b.png"),
		layer.WithPriority(3),
	)

	c := canvas.New(100, 100, cover, fore)

	// graph → JSON → 解码 → 重建 → graph 恒等(wire 面走真实 JSON 解码)
	var g canvas.Graph
	if err := json.Unmarshal([]byte(jsonOf(t, c.Graph())), &g); err != nil {
		t.Fatalf("解码画布 graph: %v", err)
	}
	rebuilt, err := canvas.FromGraph(g)
	if err != nil {
		t.Fatalf("FromGraph: %v", err)
	}

	if got, want := jsonOf(t, rebuilt.Graph()), jsonOf(t, c.Graph()); got != want {
		t.Errorf("画布往返 graph 不恒等:\n got  %s\n want %s", got, want)
	}
	if n := len(rebuilt.GetLayers()); n != 2 {
		t.Errorf("重建图层数 = %d, want 2", n)
	}
}

func TestFromGraphRejectsUnknownType(t *testing.T) {
	// PHP CanvasTest::testFromGraphRejectsUnknownType:未知类型报错,消息含类型名
	g := canvas.Graph{
		Canvas: canvas.CanvasSize{Width: 10, Height: 10},
		Layers: []layer.Node{{Type: "VideoLayer", Priority: 0}},
	}

	_, err := canvas.FromGraph(g)
	if err == nil {
		t.Fatal("未知图层类型未报错")
	}
	if !strings.Contains(err.Error(), "VideoLayer") {
		t.Errorf("错误消息不含类型名: %v", err)
	}
	if want := "未知图层类型: VideoLayer"; err.Error() != want {
		t.Errorf("错误消息 = %q, want %q", err.Error(), want)
	}
}

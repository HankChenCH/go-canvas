package layer_test

// ImageLayerTest 平移:图片图层只存引用、物化回写仅供 resolver、graph data 键面。

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hankchen/go-canvas/layer"
)

func TestSetImageStoresRawValueOnly(t *testing.T) {
	// PHP ImageLayerTest::testSetImageStoresRawValueOnly:setter 不做任何下载 I/O,物化属于渲染前的 Resolver
	l := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage("https://example.com/a.png"))

	if got := l.Image(); got == nil || *got != "https://example.com/a.png" {
		t.Errorf("Image() = %v, want https://example.com/a.png", got)
	}
}

func TestEmptyImageValueIsIgnored(t *testing.T) {
	// PHP ImageLayerTest::testEmptyImageValueIsIgnored:空串归 null
	l := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage(""))

	if l.Image() != nil {
		t.Errorf("Image() = %v, want nil", *l.Image())
	}
	if got := l.Graph().Data.Value; got != nil {
		t.Errorf("graph data.value = %q, want null", *got)
	}
}

func TestResolvedSrcPreferredOverRaw(t *testing.T) {
	// PHP ImageLayerTest::testResolvedSrcPreferredOverRaw:引用 getter 回退原始值
	l := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage("https://example.com/a.png"))

	if got := l.ResolvedSrc(); got == nil || *got != "https://example.com/a.png" {
		t.Errorf("未物化时 ResolvedSrc() = %v, want 回退原始引用", got)
	}

	l.SetResolvedSrc("/tmp/local-a.png")
	if got := l.ResolvedSrc(); got == nil || *got != "/tmp/local-a.png" {
		t.Errorf("物化后 ResolvedSrc() = %v, want /tmp/local-a.png", got)
	}

	// 适配补充(PHP setImage 行为):重新设置引用时同步清空已物化结果,回退到新引用
	l.SetImage("https://example.com/b.png")
	if got := l.ResolvedSrc(); got == nil || *got != "https://example.com/b.png" {
		t.Errorf("重设引用后 ResolvedSrc() = %v, want 回退新引用", got)
	}
}

func TestGraphKeepsRawValue(t *testing.T) {
	// PHP ImageLayerTest::testGraphKeepsRawValue:data 仅 {valueType,value},无 expression 预留键
	l := layer.NewImageLayer(layer.WithSize(10, 10), layer.WithImage("https://example.com/a.png"))

	got := jsonOf(t, l.Graph())
	want := `{"type":"ImageLayer","priority":0,"spec":{"shape":{"width":10,"height":10,` +
		`"autoWidth":false,"autoHeight":false,"lineHeight":1,` +
		`"padding":{"top":0,"bottom":0,"left":0,"right":0},` +
		`"border":{"top":null,"bottom":null,"left":null,"right":null},` +
		`"backgroundColor":null},` +
		`"align":{"horizontal":"center","vertical":"center"},` +
		`"position":{"x":0,"y":0,"position":"top-left"}},` +
		`"data":{"valueType":"StaticValue","value":"https://example.com/a.png"}}`
	if got != want {
		t.Errorf("graph JSON 键结构不符:\n got  %s\n want %s", got, want)
	}
	if strings.Contains(got, "expression") {
		t.Errorf("图片 data 不应含 expression 预留键: %s", got)
	}
}

func TestFromGraphRoundtrip(t *testing.T) {
	// PHP ImageLayerTest::testFromGraphRoundtrip
	l := layer.NewImageLayer(
		layer.WithSize(30, 20),
		layer.WithBackground("#fff"),
		layer.WithImage("https://example.com/a.png"),
		layer.WithPosition(1, 2),
	)

	var node layer.Node
	if err := json.Unmarshal([]byte(jsonOf(t, l.Graph())), &node); err != nil {
		t.Fatalf("解码 graph 节点: %v", err)
	}
	rebuilt := layer.ImageFromGraph(node)

	if got, want := jsonOf(t, rebuilt.Graph()), jsonOf(t, l.Graph()); got != want {
		t.Errorf("往返 graph 不恒等:\n got  %s\n want %s", got, want)
	}
	if got := rebuilt.Image(); got == nil || *got != "https://example.com/a.png" {
		t.Errorf("重建后 Image() = %v, want https://example.com/a.png", got)
	}
}

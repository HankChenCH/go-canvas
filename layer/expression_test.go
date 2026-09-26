package layer_test

// 数据表达式载体（TableLayer V2 spec §3.1 / go-canvas 工票 12）:
// valueType = ExpressionValue 门控 + data.expression 承载插值模板 +
// data.value 恒镜像原文（旧端降级可见、审计可读）。wire 键面:
// TextLayer 恒写 expression 键（现状三键形态）;Image/Qr 条件写键
// （标记态三键,未标记两键——保 Go↔PHP 字节 parity）。
// 对拍基准 = php-canvas-next tests/Layer/ExpressionMarkingTest.php。

import (
	"encoding/json"
	"testing"

	"github.com/hankchen/go-canvas/layer"
)

func dataOf(t *testing.T, n layer.Node) string {
	t.Helper()
	if n.Data == nil {
		t.Fatal("节点无 data 键")
	}
	return jsonOf(t, n.Data)
}

func markedNode(t *testing.T, l layer.Layer) layer.Node {
	t.Helper()
	var node layer.Node
	if err := json.Unmarshal([]byte(jsonOf(t, l.Graph())), &node); err != nil {
		t.Fatalf("解码节点: %v", err)
	}
	return node
}

func TestTextExpressionMarkingLifecycle(t *testing.T) {
	// 文本图层恒写 expression 键（三键形态不随标记切换）;value 恒镜像表达式原文
	text := layer.NewTextLayer(layer.WithSize(100, 30), layer.WithFont("", 12, "#000"))
	text.SetText("静态文案")
	if got, want := dataOf(t, text.Graph()), `{"valueType":"StaticValue","expression":"","value":"静态文案"}`; got != want {
		t.Errorf("未标记 data = %s, want %s", got, want)
	}

	text.SetExpression("姓名：{{row.name}}")
	if got, want := dataOf(t, text.Graph()), `{"valueType":"ExpressionValue","expression":"姓名：{{row.name}}","value":"姓名：{{row.name}}"}`; got != want {
		t.Errorf("标记态 data = %s, want %s", got, want)
	}

	// 字面 setter 解除标记
	text.SetText("改写字面")
	if got, want := dataOf(t, text.Graph()), `{"valueType":"StaticValue","expression":"","value":"改写字面"}`; got != want {
		t.Errorf("解除标记后 data = %s, want %s", got, want)
	}
}

func TestImageExpressionConditionalKeys(t *testing.T) {
	// 图片图层条件写键:未标记两键（无 expression 键）、标记态三键
	image := layer.NewImageLayer(layer.WithSize(60, 60))
	image.SetImage("/a.png")
	if got, want := dataOf(t, image.Graph()), `{"valueType":"StaticValue","value":"/a.png"}`; got != want {
		t.Errorf("未标记 data = %s, want %s", got, want)
	}

	image.SetExpression("{{row.avatar}}")
	if got, want := dataOf(t, image.Graph()), `{"valueType":"ExpressionValue","expression":"{{row.avatar}}","value":"{{row.avatar}}"}`; got != want {
		t.Errorf("标记态 data = %s, want %s", got, want)
	}

	image.SetImage("/b.png")
	if got, want := dataOf(t, image.Graph()), `{"valueType":"StaticValue","value":"/b.png"}`; got != want {
		t.Errorf("解除标记后 data = %s, want %s", got, want)
	}

	// 空 src:value 恒写键（可为 null,PHP 同款）
	image.SetImage("")
	if got, want := dataOf(t, image.Graph()), `{"valueType":"StaticValue","value":null}`; got != want {
		t.Errorf("空 src data = %s, want %s", got, want)
	}
}

func TestQrExpressionConditionalKeys(t *testing.T) {
	qr := layer.NewQrCodeLayer(layer.WithSize(60, 60))
	qr.SetText("https://example.com")
	if got, want := dataOf(t, qr.Graph()), `{"valueType":"StaticValue","value":"https://example.com"}`; got != want {
		t.Errorf("未标记 data = %s, want %s", got, want)
	}

	qr.SetExpression("{{row.code}}")
	if got, want := dataOf(t, qr.Graph()), `{"valueType":"ExpressionValue","expression":"{{row.code}}","value":"{{row.code}}"}`; got != want {
		t.Errorf("标记态 data = %s, want %s", got, want)
	}

	qr.SetText("https://other.example.com")
	if got, want := dataOf(t, qr.Graph()), `{"valueType":"StaticValue","value":"https://other.example.com"}`; got != want {
		t.Errorf("解除标记后 data = %s, want %s", got, want)
	}
}

func TestMarkedLayersFromGraphRoundtrip(t *testing.T) {
	// 标记态 wire → 解码 → graph 恒等（门控恢复标记,求值语义归展开步骤）
	text := layer.NewTextLayer(layer.WithSize(100, 30), layer.WithFont("", 12, "#000"))
	text.SetExpression("姓名：{{row.name}}（{{$index}}）")
	image := layer.NewImageLayer(layer.WithSize(60, 60))
	image.SetExpression("{{row.avatar}}")
	qr := layer.NewQrCodeLayer(layer.WithSize(60, 60))
	qr.SetExpression("{{row.code}}")

	for name, l := range map[string]layer.Layer{"text": text, "image": image, "qr": qr} {
		node := markedNode(t, l)
		if node.Data == nil || node.Data.ValueType != layer.ValueTypeExpression {
			t.Fatalf("%s: 解码节点 valueType = %v, want ExpressionValue", name, node.Data)
		}
		var rebuilt layer.Layer
		switch n := node; n.Type {
		case layer.TypeText:
			rebuilt = layer.TextFromGraph(n)
		case layer.TypeImage:
			rebuilt = layer.ImageFromGraph(n)
		case layer.TypeQrCode:
			rebuilt = layer.QrCodeFromGraph(n)
		}
		if got, want := jsonOf(t, rebuilt.Graph()), jsonOf(t, l.Graph()); got != want {
			t.Errorf("%s: 标记态往返不恒等:\n got  %s\n want %s", name, got, want)
		}
	}
}

package layer_test

// QrCodeLayerTest 平移:高度按宽兜底、只存文本(物化属 Resolver)、graph data.value
// 恒携带内容(与是否已物化无关)、往返恒等。

import (
	"encoding/json"
	"testing"

	"github.com/hankchen/go-canvas/layer"
)

func TestQrCodeDeclaredHeightWinsWhenNotAuto(t *testing.T) {
	// PHP QrCodeLayerTest::testDeclaredHeightWinsWhenNotAuto
	l := layer.NewQrCodeLayer(layer.WithSize(80, 40), layer.WithQrText("https://example.com"))

	if got := l.Height(); got != 40 {
		t.Errorf("Height = %d, want 40", got)
	}
}

func TestQrCodeHeightFallsBackToWidth(t *testing.T) {
	// PHP QrCodeLayerTest::testHeightFallsBackToWidth:auto 高度按宽度兜底(正方形铺放)。
	// 适配补充:声明高度 0(非 auto)同样按宽兜底,避免 0 高盒子
	l := layer.NewQrCodeLayer(
		layer.WithSize(60, 0),
		layer.WithAutoHeight(),
		layer.WithQrText("https://example.com"),
	)
	if got := l.Height(); got != 60 {
		t.Errorf("auto 高度 = %d, want 60", got)
	}

	z := layer.NewQrCodeLayer(layer.WithSize(60, 0), layer.WithQrText("https://example.com"))
	if got := z.Height(); got != 60 {
		t.Errorf("未声明高度 = %d, want 60", got)
	}
}

func TestQrCodeContentHeightUsesDynamicHeight(t *testing.T) {
	// 内容区高须按动态高(宽度兜底)计算,PHP getContentHeight 经 $this->getHeight()
	// 动态分派;auto/未声明高度 60、上下 padding 各 5 → 60-10 = 50
	l := layer.NewQrCodeLayer(
		layer.WithSize(60, 0), layer.WithAutoHeight(),
		layer.WithPaddingVH(5, 0), layer.WithQrText("https://example.com"),
	)
	if got := l.ContentHeight(); got != 50 {
		t.Errorf("ContentHeight = %d, want 50", got)
	}
}

func TestQrCodeGraphAlwaysCarriesValue(t *testing.T) {
	// PHP QrCodeLayerTest::testGraphAlwaysCarriesValue:无损,value 与是否已物化无关
	l := layer.NewQrCodeLayer(layer.WithSize(60, 60))

	if got := l.Graph().Data.Value; got == nil || *got != "" {
		t.Errorf("未设内容时 graph data.value = %v, want 空串", got)
	}

	l.SetText("https://example.com/中文")
	if got := l.Graph().Data.Value; got == nil || *got != "https://example.com/中文" {
		t.Errorf("graph data.value = %v, want https://example.com/中文", got)
	}
}

func TestQrCodeFromGraphRoundtrip(t *testing.T) {
	// PHP QrCodeLayerTest::testFromGraphRoundtrip
	l := layer.NewQrCodeLayer(
		layer.WithSize(60, 60),
		layer.WithQrText("https://example.com"),
		layer.WithPosition(2, 2),
	)

	var node layer.Node
	if err := json.Unmarshal([]byte(jsonOf(t, l.Graph())), &node); err != nil {
		t.Fatalf("解码 graph 节点: %v", err)
	}
	rebuilt := layer.QrCodeFromGraph(node)

	if got, want := jsonOf(t, rebuilt.Graph()), jsonOf(t, l.Graph()); got != want {
		t.Errorf("往返 graph 不恒等:\n got  %s\n want %s", got, want)
	}
	if got := rebuilt.Text(); got != "https://example.com" {
		t.Errorf("重建后 Text() = %q, want https://example.com", got)
	}
}

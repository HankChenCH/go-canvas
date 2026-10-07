package layer_test

// QrCodeLayerTest 平移:高度按宽兜底、只存文本(物化属 Resolver)、graph data.value
// 恒携带内容(与是否已物化无关)、往返恒等。

import (
	"encoding/json"
	"testing"

	"github.com/HankChenCH/go-canvas/layer"
)

func TestQrCodeDeclaredHeightWinsWhenNotAuto(t *testing.T) {
	// PHP QrCodeLayerTest::testDeclaredHeightWinsWhenNotAuto
	l := layer.NewQrCodeLayer(layer.WithSize(80, 40), layer.WithQrText("https://example.com"))

	if got := l.Height(); got != 40 {
		t.Errorf("Height = %d, want 40", got)
	}
}

func TestQrCodeAutoWidthStaysDeclaredZero(t *testing.T) {
	// PHP QrCodeLayerTest(工单 02):autoWidth 仅 TextLayer 有义——QrCode 无自然宽
	// 概念(任意尺寸合法),宽度求值不覆盖、按宽兜底正方形取的仍是声明宽,flag 维持无义
	l := layer.NewQrCodeLayer(layer.WithAutoWidth(), layer.WithAutoHeight(),
		layer.WithQrText("https://example.com"))

	if got := l.Width(); got != 0 {
		t.Errorf("Width = %d, want 0(声明宽,无求值)", got)
	}
	if got := l.Height(); got != 0 {
		t.Errorf("Height = %d, want 0(按宽兜底取的仍是声明宽)", got)
	}
	if !l.Graph().Spec.Shape.AutoWidth {
		t.Error("graph autoWidth 标志必须为 true")
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

// TestQrContentSide 内切正方形边长 = min(内容区宽, 内容区高)(quiet zone 语义,
// PHP QrCodeLayerTest::testContentSide* 同款):padding 内缩后负值直通,
// 绘制端 DrawImage ≤0 防护只画盒
func TestQrContentSide(t *testing.T) {
	// 80×40、上下 5 左右 7:内容盒 66×30 → side 30
	l := layer.NewQrCodeLayer(layer.WithSize(80, 40), layer.WithPaddingVH(5, 7),
		layer.WithQrText("https://example.com"))
	if got := l.ContentSide(); got != 30 {
		t.Errorf("ContentSide = %d, want 30", got)
	}

	// padding 吃满内容区:负值直通(PHP intval 不钳零,40-25-25 = -10)
	z := layer.NewQrCodeLayer(layer.WithSize(40, 40), layer.WithPadding(25),
		layer.WithQrText("https://example.com"))
	if got := z.ContentSide(); got != -10 {
		t.Errorf("ContentSide 负值应直通 = %d, want -10", got)
	}
}

// TestQrOrigin 二维码在内容盒内的放置起点(渲染模板 paintQrCode 消费)。
// 镜像 ImageLayer.ImageOrigin 的 match 分支语义,内容尺寸换成内切边长:
// left → padding.left、center → (宽-边长)/2、right → 宽-边长(default 臂归 0),垂直同构
func TestQrOrigin(t *testing.T) {
	cases := []struct {
		name         string
		horizontal   string
		vertical     string
		padT, padR   float64
		padB, padL   float64
		wantX, wantY int
	}{
		// 尺寸 100×60、padding 左15右5/上7下3:内容盒 80×50,边长 = min(80, 50) = 50
		// 水平 left=15 / center=(100-50)/2=25 / right=100-50=50,垂直 top=7 / center=5 / bottom=10
		{"center-center 整盒内居中", "center", "center", 7, 5, 3, 15, 25, 5},
		{"left-top 取 padding", "left", "top", 7, 5, 3, 15, 15, 7},
		{"right-bottom 取宽高差", "right", "bottom", 7, 5, 3, 15, 50, 10},
		// 未知取值归 0(PHP match default 臂,不取 padding)
		{"未知取值归零", "diagonal", "middle", 7, 5, 3, 15, 0, 0},
	}

	for _, tc := range cases {
		l := layer.NewQrCodeLayer(
			layer.WithSize(100, 60),
			layer.WithPaddingTRBL(tc.padT, tc.padR, tc.padB, tc.padL),
			layer.WithHorizontalAlign(tc.horizontal),
			layer.WithVerticalAlign(tc.vertical),
			layer.WithQrText("https://example.com"),
		)
		x, y := l.QrOrigin()
		if x != tc.wantX || y != tc.wantY {
			t.Errorf("%s: QrOrigin() = (%d, %d), want (%d, %d)", tc.name, x, y, tc.wantX, tc.wantY)
		}
	}
}

package layer_test

// ImageLayerTest 平移:图片图层只存引用、物化回写仅供 resolver、graph data 键面。

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/HankChenCH/go-canvas/layer"
)

func TestSetImageStoresRawValueOnly(t *testing.T) {
	// PHP ImageLayerTest::testSetImageStoresRawValueOnly:setter 不做任何下载 I/O,物化属于渲染前的 Resolver
	l := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage("https://example.com/a.png"))

	if got := l.Image(); got == nil || *got != "https://example.com/a.png" {
		t.Errorf("Image() = %v, want https://example.com/a.png", got)
	}
}

func TestImageAutoWidthStaysDeclaredZero(t *testing.T) {
	// PHP ImageLayerTest(工单 02):autoWidth 仅 TextLayer 有义——Image 自然尺寸需
	// 物化资源才可知(ADR 0005 物化红线,物化不前移到布局期),宽度求值不覆盖,
	// flag 维持无义
	l := layer.NewImageLayer(layer.WithSize(0, 40), layer.WithAutoWidth(),
		layer.WithImage("https://example.com/a.png"))

	if got := l.Width(); got != 0 {
		t.Errorf("Width = %d, want 0(声明宽,无求值)", got)
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

// TestImageOrigin 图片在内容盒内的放置起点(渲染模板 paintImage 消费)。
// PHP 无专属用例,按 getImageOrigin 的 match 分支语义逐臂锁定:left → padding.left、
// center → (宽-内容宽)/2、right → 宽-内容宽(default 臂归 0),垂直同构
func TestImageOrigin(t *testing.T) {
	cases := []struct {
		name         string
		horizontal   string
		vertical     string
		padT, padR   float64
		padB, padL   float64
		wantX, wantY int
	}{
		// 尺寸 100×60、padding 左15右5/上7下3:内容盒 80×50,
		// 水平 left=15 / center=10 / right=20,垂直 top=7 / center=5 / bottom=10
		{"center-center 默认", "center", "center", 7, 5, 3, 15, 10, 5},
		{"left-top 取 padding", "left", "top", 7, 5, 3, 15, 15, 7},
		{"right-bottom 取宽高差", "right", "bottom", 7, 5, 3, 15, 20, 10},
		// 未知取值归 0(PHP match default 臂,不取 padding)
		{"未知取值归零", "diagonal", "middle", 7, 5, 3, 15, 0, 0},
		// 整除向零截断:(100-97)/2=1.5 → 1
		{"除不尽向零截断", "center", "center", 0, 3, 0, 0, 1, 0},
	}

	for _, tc := range cases {
		l := layer.NewImageLayer(
			layer.WithSize(100, 60),
			layer.WithPaddingTRBL(tc.padT, tc.padR, tc.padB, tc.padL),
			layer.WithHorizontalAlign(tc.horizontal),
			layer.WithVerticalAlign(tc.vertical),
		)
		x, y := l.ImageOrigin()
		if x != tc.wantX || y != tc.wantY {
			t.Errorf("%s: ImageOrigin() = (%d, %d), want (%d, %d)", tc.name, x, y, tc.wantX, tc.wantY)
		}
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

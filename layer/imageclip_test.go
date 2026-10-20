package layer_test

// 裁剪形状 clip(ADR 0026,平移 php-canvas-next ImageLayerTest 裁剪形状组):
// wire 条件写键——缺省(rect)整键不落、殿后 spec 末位 fit 之后(键序
// shape/align/position/fit/clip)、显式 rect 归一缺省(cover 归一先例)、
// 未知值宽容透传保往返恒等;内切圆矩形与圆周描边参数解析纯函数矩阵
// (与 PHP 同公式:直径 = min(内容区宽高)、盒内对齐定位、整除向零截断)

import (
	"encoding/json"
	"image"
	"strings"
	"testing"

	"github.com/HankChenCH/go-canvas/layer"
)

// clipViaWire 经 wire 往返给图片图层注入 spec.clip(模拟第三方手写 graph:
// 显式 rect / 白名单外枚举串的表达形态,不经 SetClip 归一门)
func clipViaWire(t *testing.T, l *layer.ImageLayer, clip string) *layer.ImageLayer {
	t.Helper()
	raw, err := json.Marshal(l.Graph())
	if err != nil {
		t.Fatalf("序列化 graph: %v", err)
	}
	var node layer.Node
	if err := json.Unmarshal(raw, &node); err != nil {
		t.Fatalf("解码 graph: %v", err)
	}
	node.Spec.Clip = &clip
	return layer.ImageFromGraph(node)
}

func TestClipOmittedByDefault(t *testing.T) {
	// 缺省 = 矩形不裁:整键不落,Clip() 回空串(缺省态字节面与无字段版本一致)
	l := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage("https://example.com/a.png"))

	if got := l.Clip(); got != "" {
		t.Errorf("Clip() = %q, want 空串(rect 缺省)", got)
	}
	if l.Graph().Spec.Clip != nil {
		t.Errorf("graph spec.clip = %v, want nil(缺省整键不落)", *l.Graph().Spec.Clip)
	}
}

func TestClipCircleWrittenLastInSpecAfterFit(t *testing.T) {
	// 条件写键 + 键序钉死:clip 殿后于 shape/align/position/fit(ADR 0026)
	l := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage("https://example.com/a.png"))
	l.SetFit(layer.FitContain)
	l.SetClip(layer.ClipCircle)

	got := jsonOf(t, l.Graph())
	want := `{"type":"ImageLayer","priority":0,"spec":{"shape":{"width":100,"height":100,` +
		`"autoWidth":false,"autoHeight":false,"lineHeight":1,` +
		`"padding":{"top":0,"bottom":0,"left":0,"right":0},` +
		`"border":{"top":null,"bottom":null,"left":null,"right":null},` +
		`"backgroundColor":null},` +
		`"align":{"horizontal":"center","vertical":"center"},` +
		`"position":{"x":0,"y":0,"position":"top-left"},` +
		`"fit":"contain","clip":"circle"},` +
		`"data":{"valueType":"StaticValue","value":"https://example.com/a.png"}}`
	if got != want {
		t.Errorf("graph JSON 键结构不符:\n got  %s\n want %s", got, want)
	}
}

func TestClipCircleWithoutFitStillWrittenLast(t *testing.T) {
	// 未声明 fit 时 clip 同样殿后 spec 末位
	l := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage("https://example.com/a.png"))
	l.SetClip(layer.ClipCircle)

	if got := jsonOf(t, l.Graph()); !strings.Contains(got, `"position":"top-left"},"clip":"circle"}`) {
		t.Errorf("clip 应殿后 spec 末位: %s", got)
	}
}

func TestExplicitRectWireNormalizedToKeyless(t *testing.T) {
	// 解码宽容:第三方写入显式 rect 与缺省同归一(fit 显式 cover 归一先例),再序列化整键不落
	l := layer.NewImageLayer(layer.WithSize(10, 10), layer.WithImage("x"))
	rebuilt := clipViaWire(t, l, layer.ClipRect)

	if got := rebuilt.Clip(); got != "" {
		t.Errorf("重建后 Clip() = %q, want 空串(rect 归一缺省)", got)
	}
	if rebuilt.Graph().Spec.Clip != nil {
		t.Errorf("再序列化 spec.clip = %v, want nil(整键不落)", *rebuilt.Graph().Spec.Clip)
	}
	if l.SetClip(layer.ClipRect); l.Clip() != "" {
		t.Errorf("SetClip(rect) 后 Clip() = %q, want 空串(显式赋值归一)", l.Clip())
	}
}

func TestUnknownClipValuePassesThrough(t *testing.T) {
	// 未知值契约面原样透传(往返恒等);渲染面按矩形降级属渲染臂职责。
	// 'star' 是从未入册的任意未知串
	l := layer.NewImageLayer(layer.WithSize(10, 10), layer.WithImage("x"))
	rebuilt := clipViaWire(t, l, "star")

	if got := rebuilt.Clip(); got != "star" {
		t.Errorf("重建后 Clip() = %q, want star(未知值透传)", got)
	}
	if got := rebuilt.Graph().Spec.Clip; got == nil || *got != "star" {
		t.Errorf("再序列化 spec.clip = %v, want star", got)
	}
}

func TestClipRoundtripIdentity(t *testing.T) {
	l := layer.NewImageLayer(layer.WithSize(30, 20), layer.WithImage("https://example.com/a.png"))
	l.SetClip(layer.ClipCircle)

	rebuilt := clipViaWire(t, l, layer.ClipCircle)

	if got := rebuilt.Clip(); got != layer.ClipCircle {
		t.Errorf("重建后 Clip() = %q, want circle", got)
	}
	if got, want := jsonOf(t, rebuilt.Graph()), jsonOf(t, l.Graph()); got != want {
		t.Errorf("往返 graph 不恒等:\n got  %s\n want %s", got, want)
	}
}

func TestClipSurvivesSrcRewriteAndExpressionMarking(t *testing.T) {
	// clip 与 src/fit 正交:改图、打标均不扰动裁剪声明
	l := layer.NewImageLayer(layer.WithSize(10, 10))
	l.SetClip(layer.ClipCircle)
	l.SetImage("https://example.com/a.png")
	l.SetExpression("{{row.photo}}")

	if got := l.Clip(); got != layer.ClipCircle {
		t.Errorf("打标后 Clip() = %q, want circle", got)
	}

	var node layer.Node
	if err := json.Unmarshal([]byte(jsonOf(t, l.Graph())), &node); err != nil {
		t.Fatalf("解码 graph 节点: %v", err)
	}
	if got := layer.ImageFromGraph(node).Clip(); got != layer.ClipCircle {
		t.Errorf("wire 往返后 Clip() = %q, want circle", got)
	}

	l.SetImage("https://example.com/b.png")
	if got := l.Clip(); got != layer.ClipCircle {
		t.Errorf("改图后 Clip() = %q, want circle", got)
	}
}

// TestInscribedCircleRectMatrix 内切圆矩形纯计算矩阵(ADR 0026,平移 PHP
// testInscribedCircleRectMatrix):直径 = min(内容区宽高)、盒内对齐定位
// (alignedX/alignedY 同门公式——center 取整盒中点、left/top 贴 padding 原点侧)、
// 非方盒恒正方几何(渲染面即正圆)留透明边不变形为椭圆
func TestInscribedCircleRectMatrix(t *testing.T) {
	cases := []struct {
		name                 string
		boxW, boxH           int
		horizontal, vertical string
		padT, padR           float64
		padB, padL           float64
		want                 image.Rectangle
	}{
		// 方盒:内切圆 = 整个内容盒
		{"方盒全内容", 100, 100, "center", "center", 0, 0, 0, 0, image.Rect(0, 0, 100, 100)},
		// 非方盒(横盒 2:1):直径取短边 50、垂直居中,左右留透明边
		{"非方盒居中", 100, 50, "center", "center", 0, 0, 0, 0, image.Rect(25, 0, 75, 50)},
		// 对齐分量:left/top 贴内容盒原点侧,right/bottom 贴对侧(恒正方 side=50)
		{"left-top 贴原点", 100, 50, "left", "top", 0, 0, 0, 0, image.Rect(0, 0, 50, 50)},
		{"right-bottom 贴对侧", 100, 50, "right", "bottom", 0, 0, 0, 0, image.Rect(50, 0, 100, 50)},
		// padding 收缩内容盒:直径 = min(内容宽高)(内边距即框外留白)
		{"全边 padding", 100, 100, "center", "center", 10, 10, 10, 10, image.Rect(10, 10, 90, 90)},
		// 非对称 padding:center 按整盒中点、left/top 取各自 padding 分量(contain 矩阵同款期望)
		{"非对称 padding center", 100, 50, "center", "center", 20, 10, 5, 0, image.Rect(37, 12, 62, 37)},
		{"非对称 padding left", 100, 50, "left", "center", 20, 10, 5, 0, image.Rect(0, 12, 25, 37)},
		{"非对称 padding top", 100, 50, "center", "top", 20, 10, 5, 0, image.Rect(37, 20, 62, 45)},
	}

	for _, tc := range cases {
		l := layer.NewImageLayer(
			layer.WithSize(tc.boxW, tc.boxH),
			layer.WithPaddingTRBL(tc.padT, tc.padR, tc.padB, tc.padL),
			layer.WithHorizontalAlign(tc.horizontal),
			layer.WithVerticalAlign(tc.vertical),
		)
		if got := l.InscribedCircleRect(); got != tc.want {
			t.Errorf("%s: InscribedCircleRect() = %s, want %s", tc.name, got, tc.want)
		}
	}
}

// TestCircleStrokeParsingMatrix 圆周描边参数解析(ADR 0026,平移 PHP
// testCircleStrokeParsingMatrix):宽取四边已声明宽的最小值、颜色按
// top→right→bottom→left 取首个非空边色、四边全未声明不描边
func TestCircleStrokeParsingMatrix(t *testing.T) {
	assertStroke := func(t *testing.T, l *layer.ImageLayer, want *layer.BorderSide) {
		t.Helper()
		got := l.CircleStroke()
		if want == nil {
			if got != nil {
				t.Errorf("CircleStroke() = %+v, want nil(不描边)", *got)
			}
			return
		}
		if got == nil || *got != *want {
			t.Errorf("CircleStroke() = %+v, want %+v", got, want)
		}
	}

	// 四边全未声明:不描边
	assertStroke(t, layer.NewImageLayer(layer.WithSize(100, 100)), nil)

	// 单边声明:宽与色都取该边
	assertStroke(t, layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBorderTop(4, "#f00")),
		&layer.BorderSide{Width: 4, Color: "#f00"})
	assertStroke(t, layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBorderRight(3, "#0f0")),
		&layer.BorderSide{Width: 3, Color: "#0f0"})

	// 多边声明:宽取最小值(top 2 < bottom 8),色按 top→right→bottom→left 取首个非空(top)
	assertStroke(t, layer.NewImageLayer(layer.WithSize(100, 100),
		layer.WithBorderTop(2, "#f00"), layer.WithBorderBottom(8, "#00f")),
		&layer.BorderSide{Width: 2, Color: "#f00"})

	// 首个非空按序回退:top 未声明、right 5 蓝色、left 2 黄色 → 宽 2、色取 right 蓝
	assertStroke(t, layer.NewImageLayer(layer.WithSize(100, 100),
		layer.WithBorderRight(5, "#00f"), layer.WithBorderLeft(2, "#ff0")),
		&layer.BorderSide{Width: 2, Color: "#00f"})

	// 全边同值:宽色一致
	assertStroke(t, layer.NewImageLayer(layer.WithSize(100, 100), layer.WithBorder(4, "#abc")),
		&layer.BorderSide{Width: 4, Color: "#abc"})
}

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

// ---- 适配模式 fit(ADR 0023,PHP ImageLayerTest fit 组用例平移) ----

func TestFitOmittedByDefault(t *testing.T) {
	// fit 缺省 = cover:整键不落,Fit() 回空串(缺省态字节面与无字段版本一致)
	l := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage("https://example.com/a.png"))

	if got := l.Fit(); got != "" {
		t.Errorf("Fit() = %q, want 空串(cover 缺省)", got)
	}
	if l.Graph().Spec.Fit != nil {
		t.Errorf("graph spec.fit = %v, want nil(缺省整键不落)", *l.Graph().Spec.Fit)
	}
}

func TestFitContainWrittenLastInSpec(t *testing.T) {
	// 条件写键 + 键序钉死:仅 fit≠cover 落键,殿后于 shape/align/position(spec §3)
	l := layer.NewImageLayer(layer.WithSize(100, 100), layer.WithImage("https://example.com/a.png"))
	l.SetFit(layer.FitContain)

	got := jsonOf(t, l.Graph())
	want := `{"type":"ImageLayer","priority":0,"spec":{"shape":{"width":100,"height":100,` +
		`"autoWidth":false,"autoHeight":false,"lineHeight":1,` +
		`"padding":{"top":0,"bottom":0,"left":0,"right":0},` +
		`"border":{"top":null,"bottom":null,"left":null,"right":null},` +
		`"backgroundColor":null},` +
		`"align":{"horizontal":"center","vertical":"center"},` +
		`"position":{"x":0,"y":0,"position":"top-left"},` +
		`"fit":"contain"},` +
		`"data":{"valueType":"StaticValue","value":"https://example.com/a.png"}}`
	if got != want {
		t.Errorf("graph JSON 键结构不符:\n got  %s\n want %s", got, want)
	}
}

func TestFitRoundtripIdentity(t *testing.T) {
	l := layer.NewImageLayer(
		layer.WithSize(30, 20),
		layer.WithImage("https://example.com/a.png"),
	)
	l.SetFit(layer.FitContain)

	var node layer.Node
	if err := json.Unmarshal([]byte(jsonOf(t, l.Graph())), &node); err != nil {
		t.Fatalf("解码 graph 节点: %v", err)
	}
	rebuilt := layer.ImageFromGraph(node)

	if got := rebuilt.Fit(); got != layer.FitContain {
		t.Errorf("重建后 Fit() = %q, want contain", got)
	}
	if got, want := jsonOf(t, rebuilt.Graph()), jsonOf(t, l.Graph()); got != want {
		t.Errorf("往返 graph 不恒等:\n got  %s\n want %s", got, want)
	}
}

func TestExplicitCoverWireNormalizedToKeyless(t *testing.T) {
	// 解码宽容:第三方写入显式 cover 与缺省同归一(name="" 先例),再序列化整键不落
	l := layer.NewImageLayer(layer.WithSize(10, 10), layer.WithImage("x"))
	var node layer.Node
	if err := json.Unmarshal([]byte(jsonOf(t, l.Graph())), &node); err != nil {
		t.Fatalf("解码 graph 节点: %v", err)
	}
	cover := layer.FitCover
	node.Spec.Fit = &cover

	rebuilt := layer.ImageFromGraph(node)

	if got := rebuilt.Fit(); got != "" {
		t.Errorf("重建后 Fit() = %q, want 空串(cover 归一缺省)", got)
	}
	if rebuilt.Graph().Spec.Fit != nil {
		t.Errorf("再序列化 spec.fit = %v, want nil(整键不落)", *rebuilt.Graph().Spec.Fit)
	}
}

func TestUnknownFitValuePassesThrough(t *testing.T) {
	// 未知值契约面原样透传(往返恒等);渲染面按 cover 降级属渲染臂职责
	l := layer.NewImageLayer(layer.WithSize(10, 10), layer.WithImage("x"))
	var node layer.Node
	if err := json.Unmarshal([]byte(jsonOf(t, l.Graph())), &node); err != nil {
		t.Fatalf("解码 graph 节点: %v", err)
	}
	fill := "fill"
	node.Spec.Fit = &fill

	rebuilt := layer.ImageFromGraph(node)

	if got := rebuilt.Fit(); got != "fill" {
		t.Errorf("重建后 Fit() = %q, want fill(未知值透传)", got)
	}
	if got := rebuilt.Graph().Spec.Fit; got == nil || *got != "fill" {
		t.Errorf("再序列化 spec.fit = %v, want fill", got)
	}
}

func TestFitSurvivesSrcRewriteAndExpressionMarking(t *testing.T) {
	// fit 与 src 正交:改图、打标、解除标记均不扰动适配声明
	l := layer.NewImageLayer(layer.WithSize(10, 10))
	l.SetFit(layer.FitContain)
	l.SetImage("https://example.com/a.png")
	l.SetExpression("{{row.photo}}")

	if got := l.Fit(); got != layer.FitContain {
		t.Errorf("打标后 Fit() = %q, want contain", got)
	}

	var node layer.Node
	if err := json.Unmarshal([]byte(jsonOf(t, l.Graph())), &node); err != nil {
		t.Fatalf("解码 graph 节点: %v", err)
	}
	if got := layer.ImageFromGraph(node).Fit(); got != layer.FitContain {
		t.Errorf("wire 往返后 Fit() = %q, want contain", got)
	}

	l.SetImage("https://example.com/b.png")
	if got := l.Fit(); got != layer.FitContain {
		t.Errorf("改图后 Fit() = %q, want contain", got)
	}
}

// TestContainRectMatrix contain 收缩矩形纯计算矩阵(ADR 0023):等比收缩长边贴内容盒
// + align/padding 放置,公式镜像 ImageOrigin 同门(center 取整盒中点、right/bottom 贴整盒对边)
func TestContainRectMatrix(t *testing.T) {
	cases := []struct {
		name         string
		horizontal   string
		vertical     string
		padT, padR   float64
		padB, padL   float64
		imgW, imgH   int
		wantX, wantY int
		wantW, wantH int
	}{
		// 方图进横盒:高贴边、水平居中
		{"方图进横盒", "center", "center", 0, 0, 0, 0, 40, 40, 25, 0, 50, 50},
		// 4:1 宽图进横盒:宽贴边、垂直居中(y 取 int(12.5))
		{"宽图进横盒", "center", "center", 0, 0, 0, 0, 200, 50, 0, 12, 100, 25},
		// 1:4 高图进横盒:高贴边、宽收缩(w 取 int(12.5),居中按收缩后宽)
		{"高图进横盒", "center", "center", 0, 0, 0, 0, 100, 400, 44, 0, 12, 50},
		// 对齐分量:left/top 贴内容盒原点侧,right/bottom 贴对侧
		{"left-top 贴原点", "left", "top", 0, 0, 0, 0, 200, 50, 0, 0, 100, 25},
		{"left-bottom 贴对侧", "left", "bottom", 0, 0, 0, 0, 200, 50, 0, 25, 100, 25},
		// padding:收缩在内容盒(去 padding)上进行
		{"全边 padding", "center", "center", 10, 10, 10, 10, 40, 40, 35, 10, 30, 30},
		// 非对称 padding:center 仍按整盒中点(同门公式),left/top 取各自 padding 分量
		{"非对称 padding center", "center", "center", 20, 10, 5, 0, 100, 100, 37, 12, 25, 25},
		{"非对称 padding left", "left", "center", 20, 10, 5, 0, 100, 100, 0, 12, 25, 25},
		{"非对称 padding top", "center", "top", 20, 10, 5, 0, 100, 100, 37, 20, 25, 25},
	}

	for _, tc := range cases {
		l := layer.NewImageLayer(
			layer.WithSize(100, 50),
			layer.WithPaddingTRBL(tc.padT, tc.padR, tc.padB, tc.padL),
			layer.WithHorizontalAlign(tc.horizontal),
			layer.WithVerticalAlign(tc.vertical),
		)
		x, y, w, h := l.ContainRect(tc.imgW, tc.imgH)
		if x != tc.wantX || y != tc.wantY || w != tc.wantW || h != tc.wantH {
			t.Errorf("%s: ContainRect(%d, %d) = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
				tc.name, tc.imgW, tc.imgH, x, y, w, h, tc.wantX, tc.wantY, tc.wantW, tc.wantH)
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

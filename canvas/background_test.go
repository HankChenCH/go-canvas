package canvas_test

// 画布背景 wire(ADR 0022 / canvas-background spec §3):条件写键 + 键序钉死 +
// 往返恒等。对拍基准 = php-canvas-next v1.3.0 CanvasTest 背景三用例与
// CanvasBackground 值对象语义(空串与 null 同归缺省)。

import (
	"encoding/json"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
)

func strp(s string) *string { return &s }

func TestCanvasSectionWithoutBackgroundKeepsLegacyShape(t *testing.T) {
	// 两分量皆空/未设置 → background 整键不落:canvas 段与现状逐字节一致(name/visible 先例)
	plain := canvas.New(100, 80)
	if got, want := jsonOf(t, plain.Graph().Canvas), `{"width":100,"height":80}`; got != want {
		t.Errorf("无背景 canvas 段不符:\n got  %s\n want %s", got, want)
	}

	emptyComponents := canvas.New(100, 80)
	emptyComponents.SetBackground(canvas.NewCanvasBackground(strp(""), strp("")))
	if got, want := jsonOf(t, emptyComponents.Graph().Canvas), `{"width":100,"height":80}`; got != want {
		t.Errorf("空分量背景应与无背景同态:\n got  %s\n want %s", got, want)
	}

	nilBackground := canvas.New(100, 80)
	nilBackground.SetBackground(nil)
	if got, want := jsonOf(t, nilBackground.Graph().Canvas), `{"width":100,"height":80}`; got != want {
		t.Errorf("nil 背景应与无背景同态:\n got  %s\n want %s", got, want)
	}
}

func TestBackgroundKeysWrittenConditionallyInPinnedOrder(t *testing.T) {
	// 落键仅非空分量落键;键序钉死:canvas{width,height,background} / background{color,image}
	both := canvas.New(100, 80)
	both.SetBackground(canvas.NewCanvasBackground(strp("#ffffff"), strp("https://cdn.example.com/bg.png")))
	want := `{"width":100,"height":80,"background":{"color":"#ffffff","image":"https://cdn.example.com/bg.png"}}`
	if got := jsonOf(t, both.Graph().Canvas); got != want {
		t.Errorf("双分量 canvas 段不符:\n got  %s\n want %s", got, want)
	}

	colorOnly := canvas.New(100, 80)
	colorOnly.SetBackground(canvas.NewCanvasBackground(strp("#ffffff"), nil))
	if got, want := jsonOf(t, colorOnly.Graph().Canvas),
		`{"width":100,"height":80,"background":{"color":"#ffffff"}}`; got != want {
		t.Errorf("仅 color canvas 段不符:\n got  %s\n want %s", got, want)
	}

	imageOnly := canvas.New(100, 80)
	imageOnly.SetBackground(canvas.NewCanvasBackground(nil, strp("bg.png")))
	if got, want := jsonOf(t, imageOnly.Graph().Canvas),
		`{"width":100,"height":80,"background":{"image":"bg.png"}}`; got != want {
		t.Errorf("仅 image canvas 段不符:\n got  %s\n want %s", got, want)
	}
}

func TestBackgroundRoundtripIdentityOnBothStates(t *testing.T) {
	// 落键态:decode → encode 键级恒等,分量读回一致
	withBackground := canvas.New(100, 80,
		layer.NewImageLayer(layer.WithSize(10, 10), layer.WithBackground("#f00")),
	)
	withBackground.SetBackground(canvas.NewCanvasBackground(strp("#ffffff"), strp("https://cdn.example.com/bg.png")))

	var g canvas.Graph
	if err := json.Unmarshal([]byte(jsonOf(t, withBackground.Graph())), &g); err != nil {
		t.Fatalf("解码带背景 graph: %v", err)
	}
	rebuilt, err := canvas.FromGraph(g)
	if err != nil {
		t.Fatalf("FromGraph: %v", err)
	}

	if got, want := jsonOf(t, rebuilt.Graph()), jsonOf(t, withBackground.Graph()); got != want {
		t.Errorf("带背景画布往返 graph 不恒等:\n got  %s\n want %s", got, want)
	}
	bg := rebuilt.Background()
	if bg == nil {
		t.Fatal("重建画布应带背景")
	}
	if bg.Color == nil || *bg.Color != "#ffffff" {
		t.Errorf("背景色读回不符: got %v", bg.Color)
	}
	if bg.Image == nil || *bg.Image != "https://cdn.example.com/bg.png" {
		t.Errorf("背景图读回不符: got %v", bg.Image)
	}

	// 不落键态:宽容读缺键回填空,往返同样恒等
	var plain canvas.Graph
	if err := json.Unmarshal([]byte(`{"canvas":{"width":10,"height":10},"layers":[]}`), &plain); err != nil {
		t.Fatalf("解码无背景 graph: %v", err)
	}
	withoutBackground, err := canvas.FromGraph(plain)
	if err != nil {
		t.Fatalf("FromGraph: %v", err)
	}
	if withoutBackground.Background() != nil {
		t.Errorf("无背景画布 Background() = %v, want nil", withoutBackground.Background())
	}
	if got, want := jsonOf(t, withoutBackground.Graph()), `{"canvas":{"width":10,"height":10},"layers":[]}`; got != want {
		t.Errorf("无背景画布再序列化不符:\n got  %s\n want %s", got, want)
	}
}

func TestBackgroundTolerantRead(t *testing.T) {
	// 宽容读(spec §3):"background": null 与缺键同判;空对象两分量皆空归无背景
	// (与 PHP fromGraph isEmpty→null 合流同款)
	cases := []struct {
		name  string
		wire  string
		isNil bool
	}{
		{"缺键", `{"canvas":{"width":10,"height":10},"layers":[]}`, true},
		{"显式 null", `{"canvas":{"width":10,"height":10,"background":null},"layers":[]}`, true},
		{"空对象", `{"canvas":{"width":10,"height":10,"background":{}},"layers":[]}`, true},
		{"仅 color", `{"canvas":{"width":10,"height":10,"background":{"color":"#fff"}},"layers":[]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var g canvas.Graph
			if err := json.Unmarshal([]byte(tc.wire), &g); err != nil {
				t.Fatalf("解码: %v", err)
			}
			c, err := canvas.FromGraph(g)
			if err != nil {
				t.Fatalf("FromGraph: %v", err)
			}
			if tc.isNil && c.Background() != nil {
				t.Errorf("Background() = %v, want nil", c.Background())
			}
			if !tc.isNil && c.Background() == nil {
				t.Error("Background() = nil, want 非空背景")
			}
		})
	}

	// 空串分量与缺键同归缺省(PHP 构造器同款):解码 → 再编码不落该键,
	// 任意来源 wire 往返恒等
	var g canvas.Graph
	wire := `{"canvas":{"width":10,"height":10,"background":{"color":"","image":"bg.png"}},"layers":[]}`
	if err := json.Unmarshal([]byte(wire), &g); err != nil {
		t.Fatalf("解码: %v", err)
	}
	c, err := canvas.FromGraph(g)
	if err != nil {
		t.Fatalf("FromGraph: %v", err)
	}
	if got, want := jsonOf(t, c.Graph().Canvas),
		`{"width":10,"height":10,"background":{"image":"bg.png"}}`; got != want {
		t.Errorf("空串分量应归缺省不落键:\n got  %s\n want %s", got, want)
	}
}

package renderer_test

// 布局快照 fixture(工单10):PHP 导出脚本产出"graph + 绘制原语记录"JSON 作单一事实源,
// 本测试解码快照 graph 重建画布,驱动 Go 渲染模板(录制假后端),逐记录断言双端布局一致——
// 两端排版漂移在 CI 前被人眼可见的方式拦截(快照 diff 即布局行为 diff)。
//
// 断言规则:
//   - graph:快照解码 → 重建 → Go Graph() 再序列化,与 PHP 产物逐字段一致(wire 互通);
//   - 绘制记录:op/x/行数/行内容与盒子坐标逐字段严格一致;
//   - 文本行 y 是预期差异字段(ADR-0003):PHP getTextOrigin 的 GD 基线魔数
//     -round(fontSize*0.1) 不移植,Go 按 pure 对齐锚点语义断言 gotY == y + expectedYDiff
//     (expectedYDiff 由 PHP 导出侧按快照契约标注,其余分支恒 0);
//   - 图片绘制记录不比 src(机器本地缓存路径,非布局事实),只比盒子坐标。
//
// 快照由 `make layout-snapshot` 人工再生成(需本机 PHP 8.3 + ext-intl;Go CI 不装 PHP),
// 何时需要重导、双端各自改了什么时要同步,见 docs/layout-snapshot.md。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
)

// snapshotRecord 快照绘制记录(rect/text/image 三态,字段按 op 取用)
type snapshotRecord struct {
	Op            string `json:"op"`
	X             int    `json:"x"`
	Y             int    `json:"y"`
	W             int    `json:"w"`
	H             int    `json:"h"`
	Line          string `json:"line"`
	ExpectedYDiff int    `json:"expectedYDiff"`
}

// snapshotBackend 录制假后端:按快照记录的扁平 paint 序收录原语调用
type snapshotBackend struct {
	records []snapshotRecord
}

func (b *snapshotBackend) Begin(width, height int) error { return nil }

func (b *snapshotBackend) End() any { return nil }

func (b *snapshotBackend) DrawRect(x, y, width, height int, _ *string, _ layer.Border) error {
	b.records = append(b.records, snapshotRecord{Op: "rect", X: x, Y: y, W: width, H: height})
	return nil
}

func (b *snapshotBackend) DrawImage(_ string, x, y, width, height int) error {
	b.records = append(b.records, snapshotRecord{Op: "image", X: x, Y: y, W: width, H: height})
	return nil
}

func (b *snapshotBackend) DrawText(line string, x, y int, _ string, _ int, _, _, _ string, _ int) error {
	b.records = append(b.records, snapshotRecord{Op: "text", Line: line, X: x, Y: y})
	return nil
}

type snapshotCase struct {
	Name    string           `json:"name"`
	Graph   json.RawMessage  `json:"graph"`
	Records []snapshotRecord `json:"records"`
}

type layoutSnapshot struct {
	Cases []snapshotCase `json:"cases"`
}

func TestLayoutSnapshot(t *testing.T) {
	raw, err := os.ReadFile("testdata/layout-snapshot.json")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Fatal("布局快照缺失(testdata/layout-snapshot.json):运行 make layout-snapshot 再生成" +
				"(需本机 PHP 8.3 + ext-intl),流程见 docs/layout-snapshot.md")
		}
		t.Fatalf("读取布局快照失败: %v", err)
	}

	var snap layoutSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("解析布局快照失败: %v", err)
	}
	if len(snap.Cases) == 0 {
		t.Fatal("布局快照无用例,请检查导出脚本 php-canvas-next/scripts/export-layout-snapshot.php")
	}

	for _, c := range snap.Cases {
		t.Run(c.Name, func(t *testing.T) {
			assertSnapshotCase(t, c)
		})
	}
}

// assertSnapshotCase 单用例断言:graph wire 互通 + 绘制记录逐条比对
func assertSnapshotCase(t *testing.T, c snapshotCase) {
	t.Helper()

	// 1) graph 互通:解码快照 graph 重建画布,Go 侧 Graph() 须与 PHP 产物逐字段一致。
	// 两侧 JSON 规整为 any 深比:消除 PHP pretty-print 缩进与浮点字面(1.0 vs 1)差异,
	// 保留键集、键值与嵌套结构——键序由两侧结构体/数组写出序单独锁定(往返恒等测试)
	var phpWire canvas.Graph
	if err := json.Unmarshal(c.Graph, &phpWire); err != nil {
		t.Fatalf("解码快照 graph: %v", err)
	}
	cv, err := canvas.FromGraph(phpWire)
	if err != nil {
		t.Fatalf("从快照 graph 重建画布: %v", err)
	}
	phpAny, goAny := any(nil), any(nil)
	if err := json.Unmarshal(c.Graph, &phpAny); err != nil {
		t.Fatalf("规整 PHP graph: %v", err)
	}
	goGraph, err := json.Marshal(cv.Graph())
	if err != nil {
		t.Fatalf("序列化 Go graph: %v", err)
	}
	if err := json.Unmarshal(goGraph, &goAny); err != nil {
		t.Fatalf("规整 Go graph: %v", err)
	}
	if !reflect.DeepEqual(phpAny, goAny) {
		t.Fatalf("graph 双端不一致(wire 契约漂移):\n PHP  = %s\n Go   = %s", c.Graph, goGraph)
	}

	// 2) 布局一致:Go 渲染模板驱动录制后端,记录与快照 paint 序逐条比对
	// (newRenderer 复用模板测试的组装:缓存指向临时目录;用例集无远程引用、
	// 二维码内容为空,物化全程零 I/O,两端同款语义)
	backend := &snapshotBackend{}
	if _, err := newRenderer(t, backend).Render(context.Background(), cv); err != nil {
		t.Fatalf("渲染快照用例: %v", err)
	}

	if len(backend.records) != len(c.Records) {
		t.Fatalf("绘制记录数不符: got %d want %d(PHP %v / Go %v)",
			len(backend.records), len(c.Records), summarize(c.Records), summarize(backend.records))
	}
	for i, want := range c.Records {
		got := backend.records[i]
		if got.Op != want.Op {
			t.Fatalf("记录 %d op 不符: got %s want %s", i, got.Op, want.Op)
		}

		switch want.Op {
		case "text":
			if got.Line != want.Line {
				t.Errorf("记录 %d 行内容不符: got %q want %q", i, got.Line, want.Line)
			}
			if got.X != want.X {
				t.Errorf("记录 %d 行 %q x 不符: got %d want %d", i, want.Line, got.X, want.X)
			}
			// 预期差异字段(ADR-0003):Go 纯对齐锚点语义 = PHP y + GD 基线魔数标注
			if got.Y != want.Y+want.ExpectedYDiff {
				t.Errorf("记录 %d 行 %q y 不符: got %d want %d(快照 y=%d + expectedYDiff=%d)",
					i, want.Line, got.Y, want.Y+want.ExpectedYDiff, want.Y, want.ExpectedYDiff)
			}
		case "rect", "image":
			if got.X != want.X || got.Y != want.Y || got.W != want.W || got.H != want.H {
				t.Errorf("记录 %d %s 盒不符: got (%d,%d,%d,%d) want (%d,%d,%d,%d)",
					i, want.Op, got.X, got.Y, got.W, got.H, want.X, want.Y, want.W, want.H)
			}
		default:
			t.Fatalf("记录 %d 未知 op: %s", i, want.Op)
		}
	}
}

// summarize 记录序列摘要(数量不符时辅助人眼定位首个分叉点)
func summarize(records []snapshotRecord) string {
	out := ""
	for i, r := range records {
		if i > 0 {
			out += " "
		}
		switch r.Op {
		case "text":
			out += fmt.Sprintf("%d:text@%d,%d", i, r.X, r.Y)
		default:
			out += fmt.Sprintf("%d:%s@%d,%d", i, r.Op, r.X, r.Y)
		}
	}
	return out
}

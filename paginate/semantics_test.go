package paginate_test

// 分页/文档语义 fixture runner(工票 08 立预言机,工票 10 全绿并入 make test):
// 两份三端共享语义 fixture 逐用例驱动 Go 管线「解码 → paginate/compile 入口 →
// 比对 expect(页 graph 列表或 errorCode)」。
//
//   - paginate/testdata/paginate-semantics.json(paginate-semantics v1,12 用例)
//     管线 = graph → canvas.FromGraph → Paginator.Paginate → 页 graph 列表
//   - paginate/testdata/flow-semantics.json(flow-semantics v1,20 用例)
//     管线 = 模板态 frames graph → 逐帧 canvas.FromGraph →
//     DocumentCompiler.Compile(frames, dataset, flowChain) → 帧序×页序页 graph 列表
//
// 语义权威 = php-canvas-next(再生成 make paginate-fixtures / flow-fixtures,
// 人审 diff 即双端语义行为 diff)。

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/paginate"
)

// fixtureErrorCodes 稳定 code → sentinel 映射(spec §4.2.6/§10.4 两段全量;
// fixture errorCode 字符串经它对位 errors.Is)。
var fixtureErrorCodes = map[string]error{
	"content_overflow":            paginate.ErrContentOverflow,
	"paginate_target_invalid":     paginate.ErrPaginateTargetInvalid,
	"page_geometry_invalid":       paginate.ErrPageGeometryInvalid,
	"flow_chain_invalid":          paginate.ErrFlowChainInvalid,
	"flow_rows_path_inconsistent": paginate.ErrFlowRowsPathInconsistent,
}

// fixtureErrorCode 定位错误对应的稳定 code;不在 code 集内的错误直接失败
// (静默放行会让 fixture 失去三端一致性锚点作用)
func fixtureErrorCode(t *testing.T, err error) string {
	t.Helper()
	for code, sentinel := range fixtureErrorCodes {
		if errors.Is(err, sentinel) {
			return code
		}
	}
	t.Fatalf("错误 %v 不在任何稳定 code 集内(spec §5.2)", err)
	return ""
}

func loadFixture(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 fixture %s: %v", path, err)
	}
	return raw
}

// normalizeJSON 归一为 any 消除 int vs float64 字面差(1.0 vs 1),双端图深比用
func normalizeJSON(t *testing.T, v any) any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("归一序列化: %v", err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("归一反序列化: %v", err)
	}
	return out
}

func jsonDeepEqual(a, b any) bool {
	rawA, errA := json.Marshal(a)
	rawB, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(rawA) == string(rawB)
}

func mustJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "<序列化失败>"
	}
	return string(raw)
}

// compareCanvases 页 graph 序列深比(归一后),页序即帧序×页序
func compareCanvases(t *testing.T, got []*canvas.Canvas, want []json.RawMessage) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("页数 = %d, want %d", len(got), len(want))
	}
	for i, page := range got {
		gotAny := normalizeJSON(t, page.Graph())
		wantAny := normalizeJSON(t, json.RawMessage(want[i]))
		if !jsonDeepEqual(gotAny, wantAny) {
			t.Errorf("第 %d 页 graph 与预期不符:\n got  %s\n want %s", i, mustJSON(gotAny), mustJSON(wantAny))
		}
	}
}

// runPipelineCase 统一驱动一例的比对面:错误预期对稳定 code,正例对页 graph 列表
// (pagesKey = paginate 段 "pages" / flow 段 "canvases",同构管线共用比对面)
func runPipelineCase(t *testing.T, pagesKey string, expect json.RawMessage, canvases []*canvas.Canvas, err error) {
	t.Helper()
	var wantErr struct {
		ErrorCode string `json:"errorCode"`
	}
	if json.Unmarshal(expect, &wantErr) == nil && wantErr.ErrorCode != "" {
		if err == nil {
			t.Fatalf("预期错误 %s,但管线成功(产出 %d 页)", wantErr.ErrorCode, len(canvases))
		}
		if got := fixtureErrorCode(t, err); got != wantErr.ErrorCode {
			t.Fatalf("错误 code = %s, want %s", got, wantErr.ErrorCode)
		}
		return
	}

	if err != nil {
		t.Fatalf("管线意外报错: %v", err)
	}
	var want map[string]json.RawMessage
	if err := json.Unmarshal(expect, &want); err != nil {
		t.Fatalf("解码预期: %v", err)
	}
	raw, ok := want[pagesKey]
	if !ok {
		t.Fatalf("预期既无 errorCode 也无 %s 键: %s", pagesKey, string(expect))
	}
	var expected []json.RawMessage
	if err := json.Unmarshal(raw, &expected); err != nil {
		t.Fatalf("解码预期页序列: %v", err)
	}
	compareCanvases(t, canvases, expected)
}

func TestPaginateSemanticsFixtures(t *testing.T) {
	var doc struct {
		Meta struct {
			Contract string `json:"contract"`
		} `json:"meta"`
		Cases []struct {
			Name        string                   `json:"name"`
			Description string                   `json:"description"`
			Graph       json.RawMessage          `json:"graph"`
			Options     paginate.PaginateOptions `json:"options"`
			Expect      json.RawMessage          `json:"expect"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(loadFixture(t, "testdata/paginate-semantics.json"), &doc); err != nil {
		t.Fatalf("解码 fixture: %v", err)
	}
	if doc.Meta.Contract != "paginate-semantics v1" {
		t.Fatalf("contract = %q, want paginate-semantics v1", doc.Meta.Contract)
	}

	green := 0
	for _, tc := range doc.Cases {
		tc := tc
		// 绿数按 t.Run 返回值累计(父测试 t.Failed() 在首个失败后恒真,会漏计绿数)
		if t.Run(tc.Name, func(t *testing.T) {
			var wire canvas.Graph
			if err := json.Unmarshal(tc.Graph, &wire); err != nil {
				t.Fatalf("解码画布 graph: %v", err)
			}
			decoded, err := canvas.FromGraph(wire)
			if err != nil {
				t.Fatalf("重建画布: %v", err)
			}

			pages, pagErr := paginate.NewPaginator(tc.Options).Paginate(decoded)
			runPipelineCase(t, "pages", tc.Expect, pages, pagErr)
		}) {
			green++
		}
	}
	t.Logf("paginate-semantics v1:%d 用例,%d 绿 / %d 红", len(doc.Cases), green, len(doc.Cases)-green)
}

func TestFlowSemanticsFixtures(t *testing.T) {
	var doc struct {
		Meta struct {
			Contract string `json:"contract"`
		} `json:"meta"`
		Cases []struct {
			Name        string                   `json:"name"`
			Description string                   `json:"description"`
			Frames      []json.RawMessage        `json:"frames"`
			Dataset     json.RawMessage          `json:"dataset"`
			FlowChain   []paginate.FlowChainNode `json:"flowChain"`
			Expect      json.RawMessage          `json:"expect"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(loadFixture(t, "testdata/flow-semantics.json"), &doc); err != nil {
		t.Fatalf("解码 fixture: %v", err)
	}
	if doc.Meta.Contract != "flow-semantics v1" {
		t.Fatalf("contract = %q, want flow-semantics v1", doc.Meta.Contract)
	}

	compiler := paginate.NewDocumentCompiler(nil)
	green := 0
	for _, tc := range doc.Cases {
		tc := tc
		// 绿数按 t.Run 返回值累计(父测试 t.Failed() 在首个失败后恒真,会漏计绿数)
		if t.Run(tc.Name, func(t *testing.T) {
			frames := make([]*canvas.Canvas, 0, len(tc.Frames))
			for i, raw := range tc.Frames {
				var wire canvas.Graph
				if err := json.Unmarshal(raw, &wire); err != nil {
					t.Fatalf("解码帧 %d graph: %v", i, err)
				}
				frame, err := canvas.FromGraph(wire)
				if err != nil {
					t.Fatalf("重建帧 %d: %v", i, err)
				}
				frames = append(frames, frame)
			}

			var dataset any
			if len(tc.Dataset) > 0 && string(tc.Dataset) != "null" {
				if err := json.Unmarshal(tc.Dataset, &dataset); err != nil {
					t.Fatalf("解码数据集: %v", err)
				}
			}

			result, err := compiler.Compile(frames, dataset, tc.FlowChain)
			var canvases []*canvas.Canvas
			if result != nil {
				canvases = result.Canvases
			}
			runPipelineCase(t, "canvases", tc.Expect, canvases, err)
		}) {
			green++
		}
	}
	t.Logf("flow-semantics v1:%d 用例,%d 绿 / %d 红", len(doc.Cases), green, len(doc.Cases)-green)
}

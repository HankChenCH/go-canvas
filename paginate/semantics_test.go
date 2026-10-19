package paginate_test

// 分页/文档语义语料 runner(工票 07 消费翻转):语义权威 = submodule 语料仓
// canvas-semantics(本仓 pin 消费,镜像副本已删除),两份语料逐用例驱动 Go 管线
// 「解码 → paginate/compile 入口 → 比对 expect(页 graph 列表或 errorCode)」。
//
//   - corpus/paginate-semantics/v1.0/paginate.json(paginate-semantics v1.0,12 用例)
//     管线 = graph → canvas.FromGraph → Paginator.Paginate → 页 graph 列表
//   - corpus/flow-semantics/v1.1/flow.json(flow-semantics v1.1,23 用例,v1.1 增软诊断面:
//     expect.warnings 序列比对 = code + path + 序列序,工票 05)
//     管线 = 模板态 frames graph → 逐帧 canvas.FromGraph →
//     DocumentCompiler.Compile(frames, dataset, flowChain) → 帧序×页序页 graph 列表
//
// 语料 diff 即语义变更,升级 = 一次可审的 submodule 指针移动;fresh clone 先
// git submodule update --init。

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/corpus"
	"github.com/HankChenCH/go-canvas/paginate"
)

// fixtureErrorCodes 稳定 code → sentinel 映射(pipeline.md §4/§10 两段全量;
// 语料 errorCode 字符串经它对位 errors.Is)。
var fixtureErrorCodes = map[string]error{
	"content_overflow":            paginate.ErrContentOverflow,
	"paginate_target_invalid":     paginate.ErrPaginateTargetInvalid,
	"page_geometry_invalid":       paginate.ErrPageGeometryInvalid,
	"flow_chain_invalid":          paginate.ErrFlowChainInvalid,
	"flow_rows_path_inconsistent": paginate.ErrFlowRowsPathInconsistent,
}

// fixtureErrorCode 定位错误对应的稳定 code;不在 code 集内的错误直接失败
// (静默放行会让语料失去三端一致性锚点作用)
func fixtureErrorCode(t *testing.T, err error) string {
	t.Helper()
	for code, sentinel := range fixtureErrorCodes {
		if errors.Is(err, sentinel) {
			return code
		}
	}
	t.Fatalf("错误 %v 不在任何稳定 code 集内(pipeline.md §5)", err)
	return ""
}

// loadCorpus 读取 submodule 语料模块文件并钉死契约版本:meta.contract ≠ 期望串
// 即红(双写互检);未检出 submodule 或 pin 落后在此给出指引
func loadCorpus(t *testing.T, relPath, wantContract string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "canvas-semantics", "corpus", filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("读取语料 %s: %v(语料仓 submodule 未检出?先 git submodule update --init)", relPath, err)
	}
	var meta struct {
		Meta struct {
			Contract string `json:"contract"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("解码语料 %s meta: %v", relPath, err)
	}
	if meta.Meta.Contract != wantContract {
		t.Fatalf("语料 %s 契约 = %q, want %q(语料升版本须同步 runner 版本常量,语义变更可审)",
			relPath, meta.Meta.Contract, wantContract)
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
		Cases []struct {
			Name        string                   `json:"name"`
			Description string                   `json:"description"`
			Graph       json.RawMessage          `json:"graph"`
			Options     paginate.PaginateOptions `json:"options"`
			Expect      json.RawMessage          `json:"expect"`
		} `json:"cases"`
	}
	// 契约版本互检在 loadCorpus 内单点完成,此处不再重复断言
	if err := json.Unmarshal(loadCorpus(t, "paginate-semantics/v1.0/paginate.json", corpus.ContractPaginate), &doc); err != nil {
		t.Fatalf("解码语料: %v", err)
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
	t.Logf("paginate-semantics v1.0:%d 用例,%d 绿 / %d 红", len(doc.Cases), green, len(doc.Cases)-green)
}

func TestFlowSemanticsFixtures(t *testing.T) {
	var doc struct {
		Cases []struct {
			Name        string                   `json:"name"`
			Description string                   `json:"description"`
			Frames      []json.RawMessage        `json:"frames"`
			Dataset     json.RawMessage          `json:"dataset"`
			FlowChain   []paginate.FlowChainNode `json:"flowChain"`
			Expect      json.RawMessage          `json:"expect"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(loadCorpus(t, "flow-semantics/v1.1/flow.json", corpus.ContractFlow), &doc); err != nil {
		t.Fatalf("解码语料: %v", err)
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
			if err == nil && result != nil {
				// 软诊断比对(工票 05):warnings 断言面 = code + path + 序列序
				var want struct {
					Warnings []paginate.Diagnostic `json:"warnings"`
				}
				if err := json.Unmarshal(tc.Expect, &want); err != nil {
					t.Fatalf("解码预期 warnings: %v", err)
				}
				compareDiagnostics(t, "flow", result.Warnings, want.Warnings)
			}
			var canvases []*canvas.Canvas
			if result != nil {
				canvases = result.Canvases
			}
			runPipelineCase(t, "canvases", tc.Expect, canvases, err)
		}) {
			green++
		}
	}
	t.Logf("flow-semantics v1.1:%d 用例,%d 绿 / %d 红", len(doc.Cases), green, len(doc.Cases)-green)
}

// compareDiagnostics 软诊断期望序列比对(工票 05):断言面 = code + path +
// 序列序(消息人读可本地化,不入断言面;期望省略 warnings 键 = 空序列)
func compareDiagnostics(t *testing.T, contract string, got, want []paginate.Diagnostic) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s 诊断数 = %d, want %d\n got  %+v\n want %+v", contract, len(got), len(want), got, want)
	}
	for i := range want {
		if got[i].Code != want[i].Code || got[i].Path != want[i].Path {
			t.Errorf("%s 诊断[%d] = {code:%q path:%q}, want {code:%q path:%q}",
				contract, i, got[i].Code, got[i].Path, want[i].Code, want[i].Path)
		}
	}
}

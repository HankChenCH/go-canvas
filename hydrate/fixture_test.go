package hydrate_test

// 三端共享 fixture runner(go-canvas 工票 12,spec §7):
// hydrate/testdata/expression-eval.json(expression-eval v2)+ expand-semantics.json
// (expand-semantics v1.1,v1.1 增软诊断面:expect.warnings 序列比对 = code + path +
// 序列序,工票 05),由 php-canvas-next/scripts/export-hydrate-fixtures.php
// 产出(make hydrate-fixtures)。runner 管线与 PHP 导出端同构:
// graph → 反序列化重建 → hydrate(dataset, 收集器) → graph;解码期/填充期错误以
// 稳定 code 断言(spec §5.2)。canvas-web 适配后可复用本 fixture。

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/hydrate"
	"github.com/HankChenCH/go-canvas/layer"
)

// hydrateErrorCodes 稳定 code → sentinel 映射(跨包汇总解码期与填充期;
// fixture errorCode 字符串经它对位 errors.Is)
var hydrateErrorCodes = map[string]error{
	"unknown_layer_type":             layer.ErrUnknownLayerType,
	"template_rows_conflict":         layer.ErrTemplateRowsConflict,
	"rows_path_missing":              layer.ErrRowsPathMissing,
	"rows_path_invalid":              hydrate.ErrRowsPathInvalid,
	"expression_row_outside_loop":    hydrate.ErrExpressionRowOutsideLoop,
	"reserved_root_key":              hydrate.ErrReservedRootKey,
	"expression_empty_resource":      hydrate.ErrExpressionEmptyResource,
	"expression_resource_single_ref": hydrate.ErrExpressionSingleResourceRef,
	"expression_type_mismatch":       hydrate.ErrExpressionTypeMismatch,
	"expression_syntax_error":        hydrate.ErrExpressionSyntaxError,
}

// fixtureErrorCode 定位错误对应的稳定 code;不在 code 集内的错误直接失败
// (静默放行会让 fixture 失去一致性锚点作用)
func fixtureErrorCode(t *testing.T, err error) string {
	t.Helper()
	for code, sentinel := range hydrateErrorCodes {
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

func TestExpressionEvalFixture(t *testing.T) {
	var doc struct {
		Meta struct {
			Contract string `json:"contract"`
		} `json:"meta"`
		Cases []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Template    string          `json:"template"`
			Context     map[string]any  `json:"context"`
			Expect      json.RawMessage `json:"expect"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(loadFixture(t, "testdata/expression-eval.json"), &doc); err != nil {
		t.Fatalf("解码 fixture: %v", err)
	}
	if doc.Meta.Contract != "expression-eval v2" {
		t.Fatalf("contract = %q, want expression-eval v2", doc.Meta.Contract)
	}

	evaluator := hydrate.NewInterpolationEvaluator()
	for _, tc := range doc.Cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tc.Template, tc.Context)

			// expect 为串 = 求值成功预期;为对象 = {errorCode}
			var wantCode struct {
				ErrorCode string `json:"errorCode"`
			}
			if jsonErr := json.Unmarshal(tc.Expect, &wantCode); jsonErr == nil && wantCode.ErrorCode != "" {
				if err == nil {
					t.Fatalf("预期错误 %s,但求值成功(%q)", wantCode.ErrorCode, result)
				}
				if got := fixtureErrorCode(t, err); got != wantCode.ErrorCode {
					t.Fatalf("错误 code = %s, want %s", got, wantCode.ErrorCode)
				}
				return
			}
			var want string
			if err := json.Unmarshal(tc.Expect, &want); err != nil {
				t.Fatalf("解码预期: %v", err)
			}
			if err != nil {
				t.Fatalf("求值意外报错: %v", err)
			}
			if result != want {
				t.Errorf("求值结果 = %q, want %q", result, want)
			}
		})
	}
}

// TestDateTokensFixture 日期 token 转译表 runner(v2 增量,工票 06 ⑥ 双轨):
// PHP DateFormat 权威导出,Go 内置同构表经求值器端到端断言锁漂移——token 本身
// 作为格式串 {{date(row.d, 'TOKEN')}},与 PHP 导出端自检(DateFormat::format)
// 同构
func TestDateTokensFixture(t *testing.T) {
	var doc struct {
		DateTokens struct {
			Sample string `json:"sample"`
			Rows   []struct {
				Token     string `json:"token"`
				Semantics string `json:"semantics"`
				Expect    string `json:"expect"`
			} `json:"rows"`
		} `json:"dateTokens"`
	}
	if err := json.Unmarshal(loadFixture(t, "testdata/expression-eval.json"), &doc); err != nil {
		t.Fatalf("解码 fixture: %v", err)
	}

	evaluator := hydrate.NewInterpolationEvaluator()
	context := map[string]any{"row": map[string]any{"d": doc.DateTokens.Sample}}
	for _, row := range doc.DateTokens.Rows {
		row := row
		t.Run(row.Token, func(t *testing.T) {
			got, err := evaluator.Evaluate("{{date(row.d, '"+row.Token+"')}}", context)
			if err != nil {
				t.Fatalf("token %s 求值意外报错: %v", row.Token, err)
			}
			if got != row.Expect {
				t.Errorf("token %s 输出 = %q, want %q(内置表与 PHP 权威表漂移)", row.Token, got, row.Expect)
			}
		})
	}
}

func TestExpandSemanticsFixture(t *testing.T) {
	var doc struct {
		Meta struct {
			Contract string `json:"contract"`
		} `json:"meta"`
		Cases []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Graph       json.RawMessage `json:"graph"`
			Dataset     json.RawMessage `json:"dataset"`
			Expect      json.RawMessage `json:"expect"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(loadFixture(t, "testdata/expand-semantics.json"), &doc); err != nil {
		t.Fatalf("解码 fixture: %v", err)
	}
	if doc.Meta.Contract != "expand-semantics v1.1" {
		t.Fatalf("contract = %q, want expand-semantics v1.1", doc.Meta.Contract)
	}

	hydrator := hydrate.NewHydrator(nil)
	for _, tc := range doc.Cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			// 管线 = graph → canvas.FromGraph → hydrate(dataset, 收集器)(与 PHP 导出端同构)
			var wire canvas.Graph
			if err := json.Unmarshal(tc.Graph, &wire); err != nil {
				t.Fatalf("解码画布 graph: %v", err)
			}
			var ds any
			if err := json.Unmarshal(tc.Dataset, &ds); err != nil {
				t.Fatalf("解码数据集: %v", err)
			}

			collector := hydrate.NewDiagnosticCollector()
			decoded, err := canvas.FromGraph(wire)
			if err == nil {
				hydrated, hydErr := hydrator.Hydrate(decoded, ds, collector)
				err = hydErr
				if err == nil {
					// expect = {graph, warnings?} | {errorCode}
					var wantErr struct {
						ErrorCode string `json:"errorCode"`
					}
					if jsonErr := json.Unmarshal(tc.Expect, &wantErr); jsonErr == nil && wantErr.ErrorCode != "" {
						t.Fatalf("预期错误 %s,但填充成功", wantErr.ErrorCode)
					}
					var want struct {
						Graph    json.RawMessage      `json:"graph"`
						Warnings []hydrate.Diagnostic `json:"warnings"`
					}
					if err := json.Unmarshal(tc.Expect, &want); err != nil {
						t.Fatalf("解码预期: %v", err)
					}
					got := normalizeJSON(t, hydrated.Graph())
					wantAny := normalizeJSON(t, json.RawMessage(want.Graph))
					if !jsonDeepEqual(got, wantAny) {
						t.Errorf("填充产物 graph 与预期不符:\n got  %s\n want %s", mustJSON(got), mustJSON(wantAny))
					}
					compareDiagnostics(t, "expand", collector.All(), want.Warnings)
					return
				}
			}

			var wantErr struct {
				ErrorCode string `json:"errorCode"`
			}
			if jsonErr := json.Unmarshal(tc.Expect, &wantErr); jsonErr != nil || wantErr.ErrorCode == "" {
				t.Fatalf("管线意外报错: %v", err)
			}
			if got := fixtureErrorCode(t, err); got != wantErr.ErrorCode {
				t.Fatalf("错误 code = %s, want %s", got, wantErr.ErrorCode)
			}
		})
	}
}

// compareDiagnostics 软诊断期望序列比对(工票 05):断言面 = code + path +
// 序列序(消息人读可本地化,不入断言面;期望省略 warnings 键 = 空序列)
func compareDiagnostics(t *testing.T, contract string, got, want []hydrate.Diagnostic) {
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

// jsonDeepEqual 归一后的深比较
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

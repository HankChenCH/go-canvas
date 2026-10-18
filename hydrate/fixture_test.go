package hydrate_test

// 三端共享语义语料 runner(工票 07 消费翻转):语义权威 = submodule 语料仓
// canvas-semantics(本仓 pin 消费,镜像副本已删除),runner 直接读 corpus/ 下的
// expression-eval v2.0(shell/path/fn-date/fn-number 四模块,104 用例)与
// expand-semantics v1.1(29 用例,v1.1 增软诊断面:expect.warnings 序列比对 =
// code + path + 序列序,工票 05)。语料 diff 即语义变更,升级 = 一次可审的
// submodule 指针移动;fresh clone 先 git submodule update --init。
// runner 管线与 PHP 消费端同构:graph → 反序列化重建 → hydrate(dataset, 收集器)
// → graph;解码期/填充期错误以稳定 code 断言(spec/pipeline.md §5)。

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/hydrate"
	"github.com/HankChenCH/go-canvas/layer"
)

// runner 钉定的契约版本串,与语料 meta.contract 双写互检——语料升版本
// (次版本纯增/主版本语义变更)未同步常量即红,语义升级可审
const (
	expressionEvalContract = "expression-eval v2.0"
	expandContract         = "expand-semantics v1.1"
)

// evalModules 求值契约语义域拆分(语料仓模块注册表 v1):外壳/路径/日期/数字函数族
var evalModules = []string{"shell", "path", "fn-date", "fn-number"}

// hydrateErrorCodes 稳定 code → sentinel 映射(跨包汇总解码期与填充期;
// 语料 errorCode 字符串经它对位 errors.Is)
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
// (静默放行会让语料失去一致性锚点作用)
func fixtureErrorCode(t *testing.T, err error) string {
	t.Helper()
	for code, sentinel := range hydrateErrorCodes {
		if errors.Is(err, sentinel) {
			return code
		}
	}
	t.Fatalf("错误 %v 不在任何稳定 code 集内(spec/pipeline.md §5)", err)
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

func TestExpressionEvalCorpus(t *testing.T) {
	type evalCase struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Template    string          `json:"template"`
		Context     map[string]any  `json:"context"`
		Expect      json.RawMessage `json:"expect"`
	}
	type evalDocument struct {
		Cases []evalCase `json:"cases"`
	}

	evaluator := hydrate.NewInterpolationEvaluator()
	for _, module := range evalModules {
		var doc evalDocument
		rel := "expression-eval/v2.0/" + module + ".json"
		if err := json.Unmarshal(loadCorpus(t, rel, expressionEvalContract), &doc); err != nil {
			t.Fatalf("解码语料 %s: %v", rel, err)
		}
		for _, tc := range doc.Cases {
			tc := tc
			t.Run(module+":"+tc.Name, func(t *testing.T) {
				result, err := evaluator.Evaluate(tc.Template, tc.Context)

				// expect 为串 = 求值成功预期(空串合法);为对象 = {errorCode}
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
}

// TestEvalCorpusModulesWellFormed 模块文件齐备 + 用例名跨模块唯一(用例名唯一的
// 作用域 = 契约版本、跨模块文件,与 PHP 消费端 runner 同款)
func TestEvalCorpusModulesWellFormed(t *testing.T) {
	names := []string{}
	for _, module := range evalModules {
		var doc struct {
			Cases []struct {
				Name string `json:"name"`
			} `json:"cases"`
		}
		rel := "expression-eval/v2.0/" + module + ".json"
		if err := json.Unmarshal(loadCorpus(t, rel, expressionEvalContract), &doc); err != nil {
			t.Fatalf("解码语料 %s: %v", rel, err)
		}
		if len(doc.Cases) == 0 {
			t.Fatalf("模块 %s 用例集不可为空", module)
		}
		for _, tc := range doc.Cases {
			names = append(names, tc.Name)
		}
	}
	// 唯一性检查作用在裸用例名上(带模块前缀则恒真,失锚)
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			t.Errorf("语料用例名跨模块重复: %s", name)
		}
		seen[name] = true
	}
}

// TestDateTokensCorpus 日期 token 转译表 runner:语料为权威(fn-date.json 携带
// 转译表),Go 内置同构表经求值器端到端断言锁漂移——token 本身作为格式串
// {{date(row.d, 'TOKEN')}},与 PHP 消费端 DateFormat::format 自检同构
func TestDateTokensCorpus(t *testing.T) {
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
	if err := json.Unmarshal(loadCorpus(t, "expression-eval/v2.0/fn-date.json", expressionEvalContract), &doc); err != nil {
		t.Fatalf("解码语料 dateTokens: %v", err)
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
				t.Errorf("token %s 输出 = %q, want %q(内置表与语料权威表漂移)", row.Token, got, row.Expect)
			}
		})
	}
}

func TestExpandSemanticsCorpus(t *testing.T) {
	var doc struct {
		Cases []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Graph       json.RawMessage `json:"graph"`
			Dataset     json.RawMessage `json:"dataset"`
			Expect      json.RawMessage `json:"expect"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(loadCorpus(t, "expand-semantics/v1.1/expand.json", expandContract), &doc); err != nil {
		t.Fatalf("解码语料: %v", err)
	}

	hydrator := hydrate.NewHydrator(nil)
	for _, tc := range doc.Cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			// 管线 = graph → canvas.FromGraph → hydrate(dataset, 收集器)(与 PHP 消费端同构)
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

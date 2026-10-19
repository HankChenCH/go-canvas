package corpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSurface(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "support-surface.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestDeclaredVersionsExtractsContractStrings(t *testing.T) {
	path := writeSurface(t, `{"contracts":[{"id":"expression-eval","version":"v3.0"},{"id":"expand-semantics","version":"v1.1"}]}`)
	declared, err := DeclaredVersions(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"expression-eval":  "expression-eval v3.0",
		"expand-semantics": "expand-semantics v1.1",
	}
	for id, contract := range want {
		if declared[id] != contract {
			t.Fatalf("%s：得到 %s，期望 %s", id, declared[id], contract)
		}
	}
}

func TestCheckVersionsRejectsAllDriftDirections(t *testing.T) {
	declared := map[string]string{
		"expression-eval": "expression-eval v4.0",
		"paginate":        "paginate-semantics v1.0",
	}
	want := map[string]string{
		"expression-eval": "expression-eval v3.0",
		"expand":          "expand-semantics v1.1",
	}
	err := CheckVersions(declared, want)
	if err == nil {
		t.Fatal("漂移应当报错")
	}
	for _, fragment := range []string{
		"expression-eval：声明 expression-eval v4.0 ≠ runner expression-eval v3.0",
		"expand：声明缺席",
		"paginate：声明 paginate-semantics v1.0 无 runner 消费",
	} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("报错缺片段 %q：%v", fragment, err)
		}
	}
}

func TestCheckVersionsAcceptsExactMatch(t *testing.T) {
	declared := map[string]string{"expression-eval": "expression-eval v3.0"}
	want := map[string]string{"expression-eval": "expression-eval v3.0"}
	if err := CheckVersions(declared, want); err != nil {
		t.Fatal(err)
	}
}

// TestSupportSurfaceVersionsMatchRunnerContracts 仓级对表（互检补边）：实现仓钉定
// 的四契约版本串 ↔ 仓根 support-surface.json 声明。三边分工——常量↔语料归各
// runner 的 loadCorpus、声明↔目录归 canvasctl 三互检，本测试拦「pin/常量升级而
// 声明漏重签」。声明是仓级全集（四契约同仓两包消费），故对表在仓级一次做全。
func TestSupportSurfaceVersionsMatchRunnerContracts(t *testing.T) {
	declared, err := DeclaredVersions("../support-surface.json")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"expression-eval":    ContractExpressionEval,
		"expand-semantics":   ContractExpand,
		"paginate-semantics": ContractPaginate,
		"flow-semantics":     ContractFlow,
	}
	if err := CheckVersions(declared, want); err != nil {
		t.Fatal(err)
	}
}

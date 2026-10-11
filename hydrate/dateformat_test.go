package hydrate

// 内置 token 表与 PHP 权威导出(fixture dateTokens)逐行对位:表基数、顺序、
// token、语义串全等——工票 06 ⑥「Go 端内置同构表、fixture 断言锁漂移」的
// 表面锁定(fixture_test.go 的 TestDateTokensFixture 锁的是渲染行为面)。

import (
	"encoding/json"
	"os"
	"testing"
)

func TestDateTokenTableMatchesFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/expression-eval.json")
	if err != nil {
		t.Fatalf("读取 fixture: %v", err)
	}
	var doc struct {
		DateTokens struct {
			Rows []struct {
				Token     string `json:"token"`
				Semantics string `json:"semantics"`
			} `json:"rows"`
		} `json:"dateTokens"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("解码 fixture: %v", err)
	}

	if len(doc.DateTokens.Rows) != len(dateTokenTable) {
		t.Fatalf("token 表基数漂移: fixture %d 行,内置表 %d 行",
			len(doc.DateTokens.Rows), len(dateTokenTable))
	}
	for i, row := range doc.DateTokens.Rows {
		if row.Token != dateTokenTable[i].token || row.Semantics != dateTokenTable[i].semantics {
			t.Errorf("token 表第 %d 行漂移: fixture %q/%q vs 内置 %q/%q",
				i, row.Token, row.Semantics, dateTokenTable[i].token, dateTokenTable[i].semantics)
		}
	}
}

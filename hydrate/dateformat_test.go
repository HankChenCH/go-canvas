package hydrate

// 内置 token 表与语料权威导出(corpus fn-date.json 的 dateTokens 转译表)逐行
// 对位:表基数、顺序、token、语义串全等——「Go 端内置同构表、语料断言锁漂移」的
// 表面锁定(fixture_test.go 的 TestDateTokensCorpus 锁的是渲染行为面)。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDateTokenTableMatchesCorpus(t *testing.T) {
	// 语料仓 submodule 在包目录同级;未检出/pin 落后在此给出指引(与 fixture_test.go 同款)
	raw, err := os.ReadFile(filepath.Join("..", "canvas-semantics", "corpus", "expression-eval", "v2.0", "fn-date.json"))
	if err != nil {
		t.Fatalf("读取语料 dateTokens 转译表: %v(语料仓 submodule 未检出?先 git submodule update --init)", err)
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
		t.Fatalf("解码语料 dateTokens: %v", err)
	}

	if len(doc.DateTokens.Rows) != len(dateTokenTable) {
		t.Fatalf("token 表基数漂移: 语料 %d 行,内置表 %d 行",
			len(doc.DateTokens.Rows), len(dateTokenTable))
	}
	for i, row := range doc.DateTokens.Rows {
		if row.Token != dateTokenTable[i].token || row.Semantics != dateTokenTable[i].semantics {
			t.Errorf("token 表第 %d 行漂移: 语料 %q/%q vs 内置 %q/%q",
				i, row.Token, row.Semantics, dateTokenTable[i].token, dateTokenTable[i].semantics)
		}
	}
}

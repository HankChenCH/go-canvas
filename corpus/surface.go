// Package corpus 语料消费端的支持面对表辅助：从实现仓根 support-surface.json
// 提取各契约的声明版本，供 runner 测试与 runner 版本常量互检。GOVERNANCE §消费端
// pin 的三条边里，常量↔语料归各 runner 的 loadCorpus、声明↔语料目录归 canvasctl
// 三互检，本包补第三条「常量↔声明」——拦「pin/常量升级而声明漏重签」。
// 声明文件的形状权威在 canvasctl（严格 typed 解码）；本包只做版本提取，宽容解码。
package corpus

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// 实现仓消费的四契约 runner 钉定版本串（单一事实源）：与语料 meta.contract
// 双写互检——语料升版本（次版本纯增/主版本语义变更）未同步此处即红，语义升级
// 可审。hydrate/paginate 两包 runner 经此引用；仓级声明对表测试同源取值。
const (
	ContractExpressionEval = "expression-eval v3.0"
	ContractExpand         = "expand-semantics v1.1"
	ContractPaginate       = "paginate-semantics v1.0"
	ContractFlow           = "flow-semantics v1.1"
)

// DeclaredVersions 读入支持面声明，返回契约 id → 完整 contract 串（"expression-eval v3.0"）。
func DeclaredVersions(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("支持面声明读取失败：%w", err)
	}
	var doc struct {
		Contracts []struct {
			ID      string `json:"id"`
			Version string `json:"version"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("支持面声明解码失败：%w", err)
	}
	declared := make(map[string]string, len(doc.Contracts))
	for _, c := range doc.Contracts {
		declared[c.ID] = c.ID + " " + c.Version
	}

	return declared, nil
}

// CheckVersions 比对声明与 runner 常量，双向完备：任一侧缺席或版本漂移即报错，
// 差异聚合在一条消息里一次看全。
func CheckVersions(declared, want map[string]string) error {
	var drift []string
	for id, contract := range want {
		got, ok := declared[id]
		switch {
		case !ok:
			drift = append(drift, fmt.Sprintf("%s：声明缺席（runner 钉 %s，漏重签）", id, contract))
		case got != contract:
			drift = append(drift, fmt.Sprintf("%s：声明 %s ≠ runner %s", id, got, contract))
		}
	}
	ids := make([]string, 0, len(declared))
	for id := range declared {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, ok := want[id]; !ok {
			drift = append(drift, fmt.Sprintf("%s：声明 %s 无 runner 消费（多签或 runner 未翻转）", id, declared[id]))
		}
	}
	if len(drift) > 0 {
		return fmt.Errorf("支持面声明与 runner 常量漂移：%s", strings.Join(drift, "；"))
	}

	return nil
}

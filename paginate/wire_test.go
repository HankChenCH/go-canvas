package paginate

// paginate 契约面行为测试(工票 08):wire 类型解码行为 + 入口守卫。
// 置于包内测试(而非 paginate_test)——FlowChainNode 的原始键面是链校验(工票 09)
// 的裁决输入,封闭键捕获行为只能在包内断言。

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// FlowChainNode 解码:已知键逐字段落位,quota 双形态区分(缺席 nil / 0 显式零额)
func TestFlowChainNodeDecode(t *testing.T) {
	var node FlowChainNode
	src := `{"frame": 2, "mode": "paged", "quota": 0, "omitIfEmpty": true}`
	if err := json.Unmarshal([]byte(src), &node); err != nil {
		t.Fatalf("解码流链节点: %v", err)
	}
	if node.Frame != 2 {
		t.Errorf("Frame = %d, want 2", node.Frame)
	}
	if node.Mode != "paged" {
		t.Errorf("Mode = %q, want paged", node.Mode)
	}
	if node.Quota == nil || *node.Quota != 0 {
		t.Errorf("Quota = %v, want &0(显式零额与缺席必须可区分)", node.Quota)
	}
	if !node.OmitIfEmpty {
		t.Error("OmitIfEmpty = false, want true")
	}

	var bare FlowChainNode
	if err := json.Unmarshal([]byte(`{"frame": 0, "mode": "fixed"}`), &bare); err != nil {
		t.Fatalf("解码最小节点: %v", err)
	}
	if bare.Quota != nil {
		t.Errorf("Quota = %v, want nil(未声明)", bare.Quota)
	}
	if bare.OmitIfEmpty {
		t.Error("OmitIfEmpty 缺省应恒 false")
	}
}

// 键封闭是链校验的裁决面(flow_chain_invalid,工票 09),不是解码错误:
// 解码期宽松捕获未知键进原始键面,字段类型形态同理——否则 fixture 用例
// 落不到 errorCode 断言,预言机失效
func TestFlowChainNodeLenientCapture(t *testing.T) {
	var node FlowChainNode
	// unknown-key-rejected 用例的载荷形态:未知键 pageHeight 必须存活到校验期
	src := `{"frame": 0, "mode": "fixed", "pageHeight": 100}`
	if err := json.Unmarshal([]byte(src), &node); err != nil {
		t.Fatalf("未知键不得使解码失败: %v", err)
	}
	if _, ok := node.raw["pageHeight"]; !ok {
		t.Errorf("未知键未进原始键面, keys = %v", rawKeys(node.raw))
	}

	// 类型形态不符同样留存:frame 非整数时宽松解码(零值)但不报解码错误
	var typed FlowChainNode
	if err := json.Unmarshal([]byte(`{"frame": "x", "mode": 3}`), &typed); err != nil {
		t.Fatalf("字段类型形态不符不得使解码失败: %v", err)
	}
	if typed.Frame != 0 || typed.Mode != "" {
		t.Errorf("类型不符时字段应保持零值, got Frame=%d Mode=%q", typed.Frame, typed.Mode)
	}
	if string(typed.raw["frame"]) != `"x"` || string(typed.raw["mode"]) != `3` {
		t.Errorf("原始形态未留存, frame=%s mode=%s", typed.raw["frame"], typed.raw["mode"])
	}
}

// PaginateOptions 解码:PHP 空数组 [] 与 null 同义空选项(导出端惯态),
// 对象形态逐键落位、未知键忽略(CanvasPaginator::fromOptions 留演进空间同款)
func TestPaginateOptionsDecode(t *testing.T) {
	var empty PaginateOptions
	if err := json.Unmarshal([]byte(`[]`), &empty); err != nil {
		t.Fatalf("PHP 空数组编码不得使解码失败: %v", err)
	}
	if empty.PageHeight != nil || empty.Truncate {
		t.Errorf("空选项应得零值, got %+v", empty)
	}
	if err := json.Unmarshal([]byte(`null`), &empty); err != nil {
		t.Fatalf("null 不得使解码失败: %v", err)
	}

	var full PaginateOptions
	src := `{"pageHeight": 250, "truncate": true}`
	if err := json.Unmarshal([]byte(src), &full); err != nil {
		t.Fatalf("解码选项: %v", err)
	}
	if full.PageHeight == nil || *full.PageHeight != 250 {
		t.Errorf("PageHeight = %v, want &250", full.PageHeight)
	}
	if !full.Truncate {
		t.Error("Truncate = false, want true")
	}

	// pageHeight: 0 是合法解码值(几何校验 page_geometry_invalid 属分页期语义,
	// 裁决在 Paginate 入口)——选项类型不得吞掉 0 与缺席的区分
	var zero PaginateOptions
	if err := json.Unmarshal([]byte(`{"pageHeight": 0}`), &zero); err != nil {
		t.Fatalf("解码零页高: %v", err)
	}
	if zero.PageHeight == nil || *zero.PageHeight != 0 {
		t.Errorf("PageHeight = %v, want &0", zero.PageHeight)
	}

	var extra PaginateOptions
	if err := json.Unmarshal([]byte(`{"pageHeight": 250, "futureKey": 1}`), &extra); err != nil {
		t.Fatalf("未知键应忽略(fromOptions 同款): %v", err)
	}
	if extra.PageHeight == nil || *extra.PageHeight != 250 {
		t.Errorf("未知键在场时 PageHeight = %v, want &250", extra.PageHeight)
	}
}

// 稳定 code 是三端一致性锚点(spec §4.2.6/§10.4):sentinel 值即 code 本身,逐字断言
func TestStableErrorCodes(t *testing.T) {
	cases := []struct {
		sentinel error
		code     string
	}{
		{ErrContentOverflow, "content_overflow"},
		{ErrPaginateTargetInvalid, "paginate_target_invalid"},
		{ErrPageGeometryInvalid, "page_geometry_invalid"},
		{ErrFlowChainInvalid, "flow_chain_invalid"},
		{ErrFlowRowsPathInconsistent, "flow_rows_path_inconsistent"},
	}
	for _, tc := range cases {
		if tc.sentinel.Error() != tc.code {
			t.Errorf("sentinel = %q, want %q(code 不可改)", tc.sentinel.Error(), tc.code)
		}
	}
}

// frames_empty 入口守卫:调用方编程错误不立三端锚点 code(spec §10.4,
// PHP InvalidArgumentException 同构)——普通可读 error,不进稳定 code 集
func TestCompileFramesEmptyGuard(t *testing.T) {
	_, err := NewDocumentCompiler(nil).Compile(nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "frames_empty") {
		t.Errorf("Compile(nil frames) err = %v, want frames_empty 守卫错误", err)
	}
	for _, sentinel := range []error{ErrContentOverflow, ErrPaginateTargetInvalid, ErrPageGeometryInvalid,
		ErrFlowChainInvalid, ErrFlowRowsPathInconsistent} {
		if errors.Is(err, sentinel) {
			t.Errorf("frames_empty 不得落入稳定 code 集: %v", sentinel)
		}
	}
}

func rawKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

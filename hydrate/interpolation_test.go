package hydrate

// 解锁角落对齐单测:fixture 104 用例未覆盖、但 PHP 权威语义成立的行为,
// 在 Go 侧以单测锁定(镜像对齐记录,工票 10):
//   - 数组中途下钻 = 缺字段信号 + PHP 数字串键强制转换(php -r 实证钉定);
//   - 片段 trim = PHP trim 字符集(非 ASCII 空白属于键名字节)。

import "testing"

func TestDrillArrayMidPathPHPParity(t *testing.T) {
	evaluator := NewInterpolationEvaluator()
	context := map[string]any{"row": map[string]any{"tags": []any{"a", "b"}}}
	cases := []struct {
		template string
		want     string
		note     string
	}{
		{"{{row.tags.0}}", "a", "规范数字串键 → int 下标(PHP array_key_exists 强制转换)"},
		{"{{row.tags.1}}", "b", "第二个下标"},
		{"{{row.tags.2}}", "", "越界下标 = 缺字段信号"},
		{"{{row.tags.01}}", "", "前导零 = 字符串键,列表无字符串键 = 缺字段信号"},
		{"{{row.tags.007}}", "", "同上(多位前导零)"},
		{"{{row.tags.-1}}", "", "负键串不存在于列表"},
		{"{{row.tags.x}}", "", "非数字键 = 缺字段信号(PHP is_array 分支)"},
	}
	for _, tc := range cases {
		got, err := evaluator.Evaluate(tc.template, context)
		if err != nil {
			t.Errorf("%s(%s) 意外报错: %v", tc.template, tc.note, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s = %q, want %q(%s)", tc.template, got, tc.want, tc.note)
		}
	}
}

func TestDrillArrayTerminalStillTypeMismatch(t *testing.T) {
	// 终点容器 = 类型不匹配(fixture type-mismatch-array-at-scalar 语义,
	// 与 PHP drill 终点 is_array 分支一致)——中途信号对齐不放松终点
	evaluator := NewInterpolationEvaluator()
	if _, err := evaluator.Evaluate("{{row.tags}}", map[string]any{"row": map[string]any{"tags": []any{"a"}}}); err == nil {
		t.Error("数组落标量位应报错,却求值成功")
	}
}

func TestFragmentTrimPHPCharset(t *testing.T) {
	evaluator := NewInterpolationEvaluator()
	context := map[string]any{"row": map[string]any{"name": "张三"}}

	// NBSP(U+00A0)不属于 PHP trim 字符集(空格 \t \n \r \0 \v)→ 按字节属于
	// 键名 → 键不存在 = 空串;strings.TrimSpace 会误剥 NBSP 造成双端分叉
	got, err := evaluator.Evaluate("{{\u00A0row.name\u00A0}}", context)
	if err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if got != "" {
		t.Errorf("NBSP 垫片片段 = %q, want \"\"(NBSP 属于键名字节)", got)
	}

	// ASCII 空白照旧剥除(既有行为不回归)
	got, err = evaluator.Evaluate("{{\trow.name\t}}", context)
	if err != nil {
		t.Fatalf("意外报错: %v", err)
	}
	if got != "张三" {
		t.Errorf("ASCII 空白垫片片段 = %q, want %q", got, "张三")
	}
}

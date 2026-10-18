package paginate_test

// 文档编译软诊断单测(工票 05):CompileResult.Warnings 汇入 hydrate 全量发射
// (全帧共享单一收集器,帧序累积、不带帧标识);发射面 = hydrate 文案类兜底空串,
// 诊断 code/path 形态与 PHP 权威端逐字对齐(语义钉于语料 expand/flow v1.1)。

import (
	"reflect"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/hydrate"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/paginate"
)

// fallbackTemplateTable 单文本格模板表(rowsPath 可变),格内容表达式求值空串即兜底
func fallbackTemplateTable(rowsPath, expression string) *layer.TableLayer {
	content := layer.NewTextLayer(layer.WithSize(320, 0), layer.WithAutoHeight(), layer.WithFont("", 12, "#000"))
	content.SetExpression(expression)
	cell := layer.NewTableCellLayer(layer.WithSize(320, 0), layer.WithAutoHeight())
	cell.AddTemplateContentLayer(content)
	template := layer.NewTableRowTemplate(layer.WithAutoHeight())
	template.AddCell(cell)
	table := layer.NewTableLayer(layer.WithSize(320, 400))
	table.SetRowsPath(rowsPath)
	table.SetTemplate(template)
	return table
}

// nameRows 行序列:count 行,每行 name 缺席(文案求值全兜底)
func nameRows(count int) []any {
	rows := make([]any, count)
	for i := range rows {
		rows[i] = map[string]any{"filler": i}
	}
	return rows
}

func TestCompileCollectsHydrateWarnings(t *testing.T) {
	text := layer.NewTextLayer(layer.WithSize(100, 30), layer.WithFont("", 12, "#000"))
	text.SetExpression("{{missing}}")

	result, err := paginate.NewDocumentCompiler(nil).Compile(
		[]*canvas.Canvas{canvas.New(320, 400, text)}, map[string]any{"other": "x"}, nil)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	want := []hydrate.Diagnostic{{
		Code:    "expression_fallback_empty",
		Message: "文案表达式 {{missing}} 求值为空串，已兜底为空",
		Path:    "layers[0].data.value",
	}}
	if !reflect.DeepEqual(result.Warnings, want) {
		t.Errorf("Warnings = %+v, want %+v", result.Warnings, want)
	}
}

func TestCompileWithoutFallbackHasEmptyWarnings(t *testing.T) {
	// 未触发兜底 = 空序列(恒非 nil,JSON 序列化为 [] 不为 null)
	text := layer.NewTextLayer(layer.WithSize(100, 30), layer.WithFont("", 12, "#000"))
	text.SetExpression("{{orderNo}}")

	result, err := paginate.NewDocumentCompiler(nil).Compile(
		[]*canvas.Canvas{canvas.New(320, 400, text)}, map[string]any{"orderNo": "A2026-001"}, nil)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if result.Warnings == nil || len(result.Warnings) != 0 {
		t.Errorf("Warnings = %+v, want 空非 nil 序列", result.Warnings)
	}
}

func TestFlowCompileWarningsAreFullHydrateFace(t *testing.T) {
	// 流路径 warnings 反映分配前的全量 hydrate:每帧模板表吃全量行实例,
	// 两帧各发 2 条、按帧序累积,路径不带帧标识(帧标识升格走 spec 版本)
	frames := []*canvas.Canvas{
		canvas.New(320, 450, fallbackTemplateTable("order.items", "{{row.name}}")),
		canvas.New(320, 450, fallbackTemplateTable("order.items", "{{row.name}}")),
	}
	dataset := map[string]any{"order": map[string]any{"items": nameRows(2)}}
	chain := []paginate.FlowChainNode{
		{Frame: 0, Mode: "fixed", Quota: ptr(1)},
		{Frame: 1, Mode: "paged"},
	}

	result, err := paginate.NewDocumentCompiler(nil).Compile(frames, dataset, chain)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	wantPaths := []string{
		"layers[0].rows[0].cells[0].content.data.value", // 帧 0 全量 hydrate
		"layers[0].rows[1].cells[0].content.data.value",
		"layers[0].rows[0].cells[0].content.data.value", // 帧 1 全量 hydrate
		"layers[0].rows[1].cells[0].content.data.value",
	}
	if len(result.Warnings) != len(wantPaths) {
		t.Fatalf("Warnings 数 = %d, want %d(%+v)", len(result.Warnings), len(wantPaths), result.Warnings)
	}
	for i, wantPath := range wantPaths {
		if got := result.Warnings[i]; got.Code != "expression_fallback_empty" || got.Path != wantPath {
			t.Errorf("Warnings[%d] = {code=%q path=%q}, want expression_fallback_empty @ %s", i, got.Code, got.Path, wantPath)
		}
	}
}

func ptr(i int) *int { return &i }

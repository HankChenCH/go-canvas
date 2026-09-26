package expand_test

// 展开步骤单测(对拍基准 = php-canvas-next tests/Expand/CanvasExpanderTest.php 逐用例平移;
// 共享 fixture 语义由 fixture_test.go 的 runner 覆盖,本文件补 Go 特有面:
// 同对象恒等、源图不污染、struct 数据集归一、求值器注入)。

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/expand"
	"github.com/hankchen/go-canvas/layer"
)

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("序列化: %v", err)
	}
	return string(raw)
}

// templateTable 主模板表(与 PHP fixture fixtureTemplateTable 同构):
// auto 文本格(行上下文求值 + $index)+ 固定图片格,rowsPath = order.items
func templateTable() *layer.TableLayer {
	textContent := layer.NewTextLayer(layer.WithSize(160, 0), layer.WithAutoHeight(), layer.WithFont("", 12, "#000"))
	textContent.SetExpression("姓名：{{row.name}}（{{$index}}）")
	textCell := layer.NewTableCellLayer(layer.WithSize(160, 0), layer.WithAutoHeight())
	textCell.AddTemplateContentLayer(textContent)
	imageContent := layer.NewImageLayer(layer.WithSize(160, 0), layer.WithAutoHeight())
	imageContent.SetExpression("{{row.avatar}}")
	imageCell := layer.NewTableCellLayer(layer.WithSize(160, 40))
	imageCell.AddTemplateContentLayer(imageContent)

	row := layer.NewTableRowTemplate(layer.WithAutoHeight())
	row.AddCell(textCell)
	row.AddCell(imageCell)

	table := layer.NewTableLayer(layer.WithSize(320, 200), layer.WithBackground("#fff"))
	table.SetRowsPath("order.items")
	table.SetTemplate(row)
	return table
}

// dataset 主数据集(两行)
func dataset() map[string]any {
	return map[string]any{
		"orderNo": "A2026-001",
		"order": map[string]any{
			"items": []any{
				map[string]any{"name": "张三", "avatar": "/tmp/a.png"},
				map[string]any{"name": "李四", "avatar": "/tmp/b.png"},
			},
		},
	}
}

func firstTable(t *testing.T, expanded *canvas.Canvas) *layer.TableLayer {
	t.Helper()
	table, ok := expanded.GetLayers()[0].(*layer.TableLayer)
	if !ok {
		t.Fatalf("首图层类型 = %T, want *layer.TableLayer", expanded.GetLayers()[0])
	}
	return table
}

func TestExpandTemplateTableInstantiatesRows(t *testing.T) {
	expanded, err := expand.NewExpander(nil).Expand(
		canvas.New(320, 400, templateTable()), dataset())
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}

	table := firstTable(t, expanded)
	// 行实例化:两行数据,template/data 键消失,rows 键出现
	if rows := table.Rows(); len(rows) != 2 {
		t.Fatalf("行数 = %d, want 2", len(rows))
	}
	if table.Graph().Template != nil || table.Graph().Data != nil {
		t.Error("展开产物不得残留 template/data 键")
	}
	if table.Graph().Rows == nil {
		t.Error("展开产物 rows 键必须出现")
	}
}

func TestRowContextEvaluationAndOneBasedIndex(t *testing.T) {
	expanded, err := expand.NewExpander(nil).Expand(
		canvas.New(320, 400, templateTable()), dataset())
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}

	rows := firstTable(t, expanded).Rows()
	text1 := rows[0].Cells()[0].ContentLayer().(*layer.TextLayer)
	if got := text1.Text(); got != "姓名：张三（1）" {
		t.Errorf("行 1 文本 = %q, want 姓名：张三（1）($index 自 1 起)", got)
	}
	text2 := rows[1].Cells()[0].ContentLayer().(*layer.TextLayer)
	if got := text2.Text(); got != "姓名：李四（2）" {
		t.Errorf("行 2 文本 = %q, want 姓名：李四（2）", got)
	}
	image := rows[1].Cells()[1].ContentLayer().(*layer.ImageLayer)
	if got := *image.Image(); got != "/tmp/b.png" {
		t.Errorf("行 2 图片 = %q, want /tmp/b.png", got)
	}
	// 求值后标记解除(valueType → StaticValue)
	if text1.Graph().Data.ValueType != layer.ValueTypeStatic {
		t.Errorf("求值后 valueType = %q, want StaticValue", text1.Graph().Data.ValueType)
	}
}

func TestExpandedHeightsFinalizedByV1CouplingReplay(t *testing.T) {
	expanded, err := expand.NewExpander(nil).Expand(
		canvas.New(320, 400, templateTable()), dataset())
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}

	// 宽在声明态同步:行宽 = 表宽;高在展开态定稿:行高取最高格(V1 耦合重放)
	row := firstTable(t, expanded).Rows()[0]
	if got := row.Width(); got != 320 {
		t.Errorf("行宽 = %d, want 320", got)
	}
	if got := row.Height(); got != 40 {
		t.Errorf("行高 = %d, want 40(auto 文本格 12 与固定格 40 取最高)", got)
	}
	if row.Graph().Spec.Shape.AutoHeight {
		t.Error("定稿行高后 autoHeight 必须关闭")
	}
	// 固定格内容压平(V1 耦合重放)
	image := row.Cells()[1].ContentLayer().(*layer.ImageLayer)
	if got := image.Height(); got != 40 {
		t.Errorf("固定格内容高 = %d, want 40(压平)", got)
	}
	if image.Graph().Spec.Shape.AutoHeight {
		t.Error("压平后内容 autoHeight 必须关闭")
	}
}

func TestExpandDoesNotPolluteSourceCanvas(t *testing.T) {
	source := canvas.New(320, 400, templateTable())
	graphBefore := source.Graph()

	if _, err := expand.NewExpander(nil).Expand(source, dataset()); err != nil {
		t.Fatalf("Expand: %v", err)
	}

	// 展开产物与源图分离:源仍是声明态
	if got, want := jsonOf(t, source.Graph()), jsonOf(t, graphBefore); got != want {
		t.Errorf("源画布 graph 被污染:\n got  %s\n want %s", got, want)
	}
	if source.GetLayers()[0].(*layer.TableLayer).Template() == nil {
		t.Error("源画布模板丢失")
	}
}

func TestIndependentLayersEvaluateAtRootContext(t *testing.T) {
	text := layer.NewTextLayer(layer.WithSize(200, 30), layer.WithFont("", 12, "#000"))
	text.SetExpression("订单号：{{orderNo}}")
	image := layer.NewImageLayer(layer.WithSize(60, 60))
	image.SetExpression("{{orderNo}}.png")

	expanded, err := expand.NewExpander(nil).Expand(
		canvas.New(320, 100, text, image), dataset())
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}

	layers := expanded.GetLayers()
	if got := layers[0].(*layer.TextLayer).Text(); got != "订单号：A2026-001" {
		t.Errorf("独立文本 = %q, want 订单号：A2026-001", got)
	}
	if got := *layers[1].(*layer.ImageLayer).Image(); got != "A2026-001.png" {
		t.Errorf("独立图片 = %q, want A2026-001.png", got)
	}
}

func TestV1TableMarkedContentEvaluatesAtRootScope(t *testing.T) {
	cell := layer.NewTableCellLayer(layer.WithSize(320, 0), layer.WithAutoHeight())
	content := layer.NewTextLayer(layer.WithSize(320, 0), layer.WithAutoHeight(), layer.WithFont("", 12, "#000"))
	content.SetExpression("{{orderNo}}")
	cell.AddContentLayer(content)
	table := layer.NewTableLayer(layer.WithSize(320, 100), layer.WithBackground("#fff"))
	if err := table.AddRow(layer.NewTableRowLayer(layer.WithSize(320, 0), layer.WithAutoHeight(), layer.WithCells(cell))); err != nil {
		t.Fatalf("AddRow: %v", err)
	}

	expanded, err := expand.NewExpander(nil).Expand(canvas.New(320, 400, table), dataset())
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}

	rows := firstTable(t, expanded).Rows()
	if len(rows) != 1 {
		t.Fatalf("行数 = %d, want 1", len(rows))
	}
	if got := rows[0].Cells()[0].ContentLayer().(*layer.TextLayer).Text(); got != "A2026-001" {
		t.Errorf("V1 表标记内容 = %q, want A2026-001(根作用域)", got)
	}
}

func TestNullDatasetIsIdentityEvenWithBindings(t *testing.T) {
	// 未绑数据集:不跑求值器,全字面(标记字段按 value 镜像显示原文)
	source := canvas.New(320, 400, templateTable())

	expanded, err := expand.NewExpander(nil).Expand(source, nil)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if expanded != source {
		t.Error("dataset null 必须恒等直通(同一画布对象)")
	}
}

func TestCanvasWithoutBindingsIsIdentity(t *testing.T) {
	// 无标记无模板:恒等直通(存量文案里的字面 {{ 不误伤)
	source := canvas.New(320, 100, layer.NewTextLayer(
		layer.WithSize(100, 30), layer.WithText("字面 {{orderNo}} 文案")))

	expanded, err := expand.NewExpander(nil).Expand(source, dataset())
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if expanded != source {
		t.Error("无绑定画布必须恒等直通(同一画布对象)")
	}
}

func TestEmptyRowArrayYieldsEmptyShell(t *testing.T) {
	ds := map[string]any{"order": map[string]any{"items": []any{}}}
	expanded, err := expand.NewExpander(nil).Expand(canvas.New(320, 400, templateTable()), ds)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}

	table := firstTable(t, expanded)
	// 空数组 = 合法零行,空壳渲染(表壳按声明画,行区零高)
	if rows := table.Rows(); len(rows) != 0 {
		t.Fatalf("行数 = %d, want 0", len(rows))
	}
	if table.Graph().Rows == nil || len(*table.Graph().Rows) != 0 {
		t.Error("空壳 rows 键必须存在且为空数组")
	}
	if table.Template() != nil {
		t.Error("空壳不得残留模板")
	}
}

func TestNestedTemplateTableRowRelative(t *testing.T) {
	// 嵌套模板表:内层 rowsPath 行相对(以当前行数据为取数范围),$index 内层自 1 起
	innerContent := layer.NewTextLayer(layer.WithSize(200, 0), layer.WithAutoHeight(), layer.WithFont("", 12, "#000"))
	innerContent.SetExpression("{{row.name}}@{{$index}}")
	innerCell := layer.NewTableCellLayer(layer.WithSize(200, 0), layer.WithAutoHeight())
	innerCell.AddTemplateContentLayer(innerContent)
	innerTemplate := layer.NewTableRowTemplate(layer.WithAutoHeight())
	innerTemplate.AddCell(innerCell)
	innerTable := layer.NewTableLayer(layer.WithSize(200, 100))
	innerTable.SetRowsPath("sub")
	innerTable.SetTemplate(innerTemplate)

	outerCell := layer.NewTableCellLayer(layer.WithSize(200, 100))
	outerCell.AddTemplateContentLayer(innerTable)
	outerTemplate := layer.NewTableRowTemplate(layer.WithAutoHeight())
	outerTemplate.AddCell(outerCell)
	outerTable := layer.NewTableLayer(layer.WithSize(200, 200))
	outerTable.SetRowsPath("items")
	outerTable.SetTemplate(outerTemplate)

	ds := map[string]any{
		"items": []any{
			map[string]any{"sub": []any{map[string]any{"name": "甲"}}},
			map[string]any{"sub": []any{map[string]any{"name": "乙"}, map[string]any{"name": "丙"}}},
		},
	}
	expanded, err := expand.NewExpander(nil).Expand(canvas.New(240, 400, outerTable), ds)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}

	outerRows := firstTable(t, expanded).Rows()
	if len(outerRows) != 2 {
		t.Fatalf("外层行数 = %d, want 2", len(outerRows))
	}
	want := [][]string{{"甲@1"}, {"乙@1", "丙@2"}}
	for i, orow := range outerRows {
		innerTableNode := orow.Cells()[0].ContentLayer().(*layer.TableLayer)
		innerRows := innerTableNode.Rows()
		if len(innerRows) != len(want[i]) {
			t.Fatalf("外层行 %d 内表行数 = %d, want %d", i, len(innerRows), len(want[i]))
		}
		for j, irow := range innerRows {
			got := irow.Cells()[0].ContentLayer().(*layer.TextLayer).Text()
			if got != want[i][j] {
				t.Errorf("内表[%d][%d] 文本 = %q, want %q", i, j, got, want[i][j])
			}
		}
	}
}

func TestRowsPathErrors(t *testing.T) {
	cases := []struct {
		name    string
		dataset map[string]any
		wantErr error
	}{
		{"缺键", map[string]any{"orderNo": "A2026-001"}, expand.ErrRowsPathInvalid},
		{"非数组", map[string]any{"order": map[string]any{"items": "not-an-array"}}, expand.ErrRowsPathInvalid},
	}
	for _, tc := range cases {
		_, err := expand.NewExpander(nil).Expand(canvas.New(320, 400, templateTable()), tc.dataset)
		if !errors.Is(err, tc.wantErr) {
			t.Errorf("%s: 错误 = %v, want %v", tc.name, err, tc.wantErr)
		}
	}
}

func TestReservedRootKeyErrors(t *testing.T) {
	text := layer.NewTextLayer(layer.WithSize(100, 30), layer.WithFont("", 12, "#000"))
	text.SetExpression("{{orderNo}}")

	for name, ds := range map[string]map[string]any{
		"row 键": {"row": map[string]any{"name": "x"}},
		"$ 前缀键": {"$meta": 1, "orderNo": "A"},
	} {
		_, err := expand.NewExpander(nil).Expand(canvas.New(320, 100, text), ds)
		if !errors.Is(err, expand.ErrReservedRootKey) {
			t.Errorf("%s: 错误 = %v, want ErrReservedRootKey", name, err)
		}
	}
}

func TestEmptyResourceExpressionThrows(t *testing.T) {
	// 资源类字段(Image src)求值空串 = 报错
	ds := map[string]any{"order": map[string]any{"items": []any{map[string]any{"name": "张三"}}}}
	_, err := expand.NewExpander(nil).Expand(canvas.New(320, 400, templateTable()), ds)
	if !errors.Is(err, expand.ErrExpressionEmptyResource) {
		t.Errorf("错误 = %v, want ErrExpressionEmptyResource", err)
	}
}

func TestTypeMismatchInRowContextThrows(t *testing.T) {
	content := layer.NewTextLayer(layer.WithSize(320, 0), layer.WithAutoHeight(), layer.WithFont("", 12, "#000"))
	content.SetExpression("{{row.tags}}")
	cell := layer.NewTableCellLayer(layer.WithSize(320, 0), layer.WithAutoHeight())
	cell.AddTemplateContentLayer(content)
	template := layer.NewTableRowTemplate(layer.WithAutoHeight())
	template.AddCell(cell)
	table := layer.NewTableLayer(layer.WithSize(320, 200))
	table.SetRowsPath("items")
	table.SetTemplate(template)

	ds := map[string]any{"items": []any{map[string]any{"tags": []any{"a", "b"}}}}
	_, err := expand.NewExpander(nil).Expand(canvas.New(320, 400, table), ds)
	if !errors.Is(err, expand.ErrExpressionTypeMismatch) {
		t.Errorf("错误 = %v, want ErrExpressionTypeMismatch(数组落到标量位)", err)
	}
}

type recordingEvaluator struct {
	templates []string
	result    string
}

func (e *recordingEvaluator) Evaluate(template string, context map[string]any) (string, error) {
	e.templates = append(e.templates, template)
	return e.result, nil
}

func TestCustomEvaluatorInjection(t *testing.T) {
	// 求值器为可注入策略(spec §3.6):替换默认实现后展开器走注入面
	stub := &recordingEvaluator{result: "STUB"}
	text := layer.NewTextLayer(layer.WithSize(100, 30), layer.WithFont("", 12, "#000"))
	text.SetExpression("任意模板")

	expanded, err := expand.NewExpander(stub).Expand(canvas.New(320, 100, text), map[string]any{"k": "v"})
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if got := expanded.GetLayers()[0].(*layer.TextLayer).Text(); got != "STUB" {
		t.Errorf("注入求值结果 = %q, want STUB", got)
	}
	if len(stub.templates) != 1 || stub.templates[0] != "任意模板" {
		t.Errorf("求值器收到的模板 = %v, want [任意模板]", stub.templates)
	}
}

type pointDataset struct {
	Order struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	} `json:"order"`
}

func TestStructDatasetNormalization(t *testing.T) {
	// Go 形状数据集(struct)经 JSON 往返归一(PHP normalizeDataset 同精神)
	var ds pointDataset
	ds.Order.Items = []struct {
		Name string `json:"name"`
	}{{Name: "张三"}}

	content := layer.NewTextLayer(layer.WithSize(320, 0), layer.WithAutoHeight(), layer.WithFont("", 12, "#000"))
	content.SetExpression("{{row.name}}")
	cell := layer.NewTableCellLayer(layer.WithSize(320, 0), layer.WithAutoHeight())
	cell.AddTemplateContentLayer(content)
	template := layer.NewTableRowTemplate(layer.WithAutoHeight())
	template.AddCell(cell)
	table := layer.NewTableLayer(layer.WithSize(320, 200))
	table.SetRowsPath("order.items")
	table.SetTemplate(template)

	expanded, err := expand.NewExpander(nil).Expand(canvas.New(320, 400, table), ds)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if got := firstTable(t, expanded).Rows()[0].Cells()[0].ContentLayer().(*layer.TextLayer).Text(); got != "张三" {
		t.Errorf("struct 数据集行文本 = %q, want 张三", got)
	}
}

func TestSentinelMessagesCarryCode(t *testing.T) {
	// 稳定 code 是三端一致性抓手:消息以 code 前缀开头
	ds := map[string]any{"order": map[string]any{"items": "not-an-array"}}
	_, err := expand.NewExpander(nil).Expand(canvas.New(320, 400, templateTable()), ds)
	if err == nil || !strings.HasPrefix(err.Error(), "rows_path_invalid") {
		t.Errorf("错误消息 = %v, want rows_path_invalid 前缀", err)
	}
}

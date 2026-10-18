package hydrate_test

// 填充步骤单测(对拍基准 = php-canvas-next tests/Hydrate/CanvasHydratorTest.php 逐用例平移;
// 共享 fixture 语义由 fixture_test.go 的 runner 覆盖,本文件补 Go 特有面:
// 同对象恒等、源图不污染、struct 数据集归一、求值器注入)。

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/hydrate"
	"github.com/HankChenCH/go-canvas/layer"
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

func firstTable(t *testing.T, hydrated *canvas.Canvas) *layer.TableLayer {
	t.Helper()
	table, ok := hydrated.GetLayers()[0].(*layer.TableLayer)
	if !ok {
		t.Fatalf("首图层类型 = %T, want *layer.TableLayer", hydrated.GetLayers()[0])
	}
	return table
}

func TestHydrateTemplateTableInstantiatesRows(t *testing.T) {
	hydrated, err := hydrate.NewHydrator(nil).Hydrate(
		canvas.New(320, 400, templateTable()), dataset())
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	table := firstTable(t, hydrated)
	// 行实例化:两行数据,template/data 键消失,rows 键出现
	if rows := table.Rows(); len(rows) != 2 {
		t.Fatalf("行数 = %d, want 2", len(rows))
	}
	if table.Graph().Template != nil || table.Graph().Data != nil {
		t.Error("填充产物不得残留 template/data 键")
	}
	if table.Graph().Rows == nil {
		t.Error("填充产物 rows 键必须出现")
	}
}

func TestRowContextEvaluationAndOneBasedIndex(t *testing.T) {
	hydrated, err := hydrate.NewHydrator(nil).Hydrate(
		canvas.New(320, 400, templateTable()), dataset())
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	rows := firstTable(t, hydrated).Rows()
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

func TestHydratedHeightsFinalizedByV1CouplingReplay(t *testing.T) {
	hydrated, err := hydrate.NewHydrator(nil).Hydrate(
		canvas.New(320, 400, templateTable()), dataset())
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	// 宽在声明态同步:行宽 = 表宽;高在填充态定稿:行高取最高格(V1 耦合重放)
	row := firstTable(t, hydrated).Rows()[0]
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

func TestHydrateDoesNotPolluteSourceCanvas(t *testing.T) {
	source := canvas.New(320, 400, templateTable())
	graphBefore := source.Graph()

	if _, err := hydrate.NewHydrator(nil).Hydrate(source, dataset()); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	// 填充产物与源图分离:源仍是声明态
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
	image.SetExpression("{{orderNo}}")
	qr := layer.NewQrCodeLayer(layer.WithSize(60, 60))
	qr.SetExpression("{{$root.orderNo}}")

	hydrated, err := hydrate.NewHydrator(nil).Hydrate(
		canvas.New(320, 100, text, image, qr), dataset())
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	layers := hydrated.GetLayers()
	if got := layers[0].(*layer.TextLayer).Text(); got != "订单号：A2026-001" {
		t.Errorf("独立文本 = %q, want 订单号：A2026-001", got)
	}
	if got := *layers[1].(*layer.ImageLayer).Image(); got != "A2026-001" {
		t.Errorf("独立图片 = %q, want A2026-001", got)
	}
	if got := layers[2].(*layer.QrCodeLayer).Text(); got != "A2026-001" {
		t.Errorf("独立二维码 = %q, want A2026-001($root. 前缀放行)", got)
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

	hydrated, err := hydrate.NewHydrator(nil).Hydrate(canvas.New(320, 400, table), dataset())
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	rows := firstTable(t, hydrated).Rows()
	if len(rows) != 1 {
		t.Fatalf("行数 = %d, want 1", len(rows))
	}
	if got := rows[0].Cells()[0].ContentLayer().(*layer.TextLayer).Text(); got != "A2026-001" {
		t.Errorf("V1 表标记内容 = %q, want A2026-001(根作用域)", got)
	}
}

// TestFitPassesThroughHydrateUntouched fit 是静态声明(ADR 0023):不进求值面与
// 表达式白名单,随 graph 改写原样透传(平移 PHP testFitPassesThroughHydrateUntouched)
func TestFitPassesThroughHydrateUntouched(t *testing.T) {
	marked := layer.NewImageLayer(layer.WithSize(60, 60))
	marked.SetExpression("{{orderNo}}")
	marked.SetFit(layer.FitContain)
	static := layer.NewImageLayer(layer.WithSize(60, 60), layer.WithImage("/tmp/static.png"))
	static.SetFit("fill")

	hydrated, err := hydrate.NewHydrator(nil).Hydrate(
		canvas.New(320, 100, marked, static), dataset())
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	graph := hydrated.Graph()
	layers := hydrated.GetLayers()
	first := layers[0].(*layer.ImageLayer)
	if got := *first.Image(); got != "A2026-001" {
		t.Errorf("独立图片 = %q, want A2026-001", got)
	}
	if got := first.Fit(); got != layer.FitContain {
		t.Errorf("独立图片 Fit() = %q, want contain", got)
	}
	if got := graph.Layers[0].Spec.Fit; got == nil || *got != layer.FitContain {
		t.Errorf("hydrate 后 wire spec.fit = %v, want contain", got)
	}
	if got := graph.Layers[0].Data.ValueType; got != layer.ValueTypeStatic {
		t.Errorf("hydrate 后 data.valueType = %q, want StaticValue(求值结果不落 graph)", got)
	}

	second := layers[1].(*layer.ImageLayer)
	if got := second.Fit(); got != "fill" {
		t.Errorf("静态图片 Fit() = %q, want fill(未知值透传)", got)
	}
	if got := graph.Layers[1].Spec.Fit; got == nil || *got != "fill" {
		t.Errorf("hydrate 后 wire spec.fit = %v, want fill", got)
	}
}

// TestFitPassesThroughTemplateInstantiation 模板表实例化路径(expand 语义同门):
// 模板子树格内容层的 fit 随行实例原样透传(平移 PHP testFitPassesThroughTemplateInstantiation)
func TestFitPassesThroughTemplateInstantiation(t *testing.T) {
	content := layer.NewImageLayer(layer.WithSize(160, 40))
	content.SetExpression("{{row.avatar}}")
	content.SetFit(layer.FitContain)
	cell := layer.NewTableCellLayer(layer.WithSize(160, 40))
	cell.AddTemplateContentLayer(content)
	row := layer.NewTableRowTemplate(layer.WithSize(0, 40))
	row.AddCell(cell)
	table := layer.NewTableLayer(layer.WithSize(320, 100), layer.WithBackground("#fff"))
	table.SetRowsPath("order.items")
	table.SetTemplate(row)

	hydrated, err := hydrate.NewHydrator(nil).Hydrate(canvas.New(320, 100, table), dataset())
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	rows := hydrated.GetLayers()[0].(*layer.TableLayer).Rows()
	if len(rows) != 2 {
		t.Fatalf("实例化行数 = %d, want 2", len(rows))
	}
	for _, rowNode := range rows {
		image := rowNode.Cells()[0].ContentLayer().(*layer.ImageLayer)
		if got := image.Fit(); got != layer.FitContain {
			t.Errorf("行实例 Fit() = %q, want contain", got)
		}
	}
	// wire 面:行实例的 content 载荷(RawMessage)内 spec.fit 原样在场
	wireRows := hydrated.Graph().Layers[0].Rows
	if wireRows == nil || len(*wireRows) == 0 {
		t.Fatalf("hydrate 后 wire 缺 rows")
	}
	wireCells := (*wireRows)[0].Cells
	if wireCells == nil || len(*wireCells) == 0 {
		t.Fatalf("hydrate 后 wire 行实例缺 cells")
	}
	var contentNode layer.Node
	if err := json.Unmarshal((*wireCells)[0].Content, &contentNode); err != nil {
		t.Fatalf("解码行实例 content 载荷: %v", err)
	}
	if got := contentNode.Spec.Fit; got == nil || *got != layer.FitContain {
		t.Errorf("行实例 wire content.spec.fit = %v, want contain", got)
	}
}

func TestNullDatasetIsIdentityEvenWithBindings(t *testing.T) {
	// 未绑数据集:不跑求值器,全字面(标记字段按 value 镜像显示原文)
	source := canvas.New(320, 400, templateTable())

	hydrated, err := hydrate.NewHydrator(nil).Hydrate(source, nil)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if hydrated != source {
		t.Error("dataset null 必须恒等直通(同一画布对象)")
	}
}

func TestCanvasWithoutBindingsIsIdentity(t *testing.T) {
	// 无标记无模板:恒等直通(存量文案里的字面 {{ 不误伤)
	source := canvas.New(320, 100, layer.NewTextLayer(
		layer.WithSize(100, 30), layer.WithText("字面 {{orderNo}} 文案")))

	hydrated, err := hydrate.NewHydrator(nil).Hydrate(source, dataset())
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if hydrated != source {
		t.Error("无绑定画布必须恒等直通(同一画布对象)")
	}
}

func TestEmptyRowArrayYieldsEmptyShell(t *testing.T) {
	ds := map[string]any{"order": map[string]any{"items": []any{}}}
	hydrated, err := hydrate.NewHydrator(nil).Hydrate(canvas.New(320, 400, templateTable()), ds)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	table := firstTable(t, hydrated)
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
	hydrated, err := hydrate.NewHydrator(nil).Hydrate(canvas.New(240, 400, outerTable), ds)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	outerRows := firstTable(t, hydrated).Rows()
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
		{"缺键", map[string]any{"orderNo": "A2026-001"}, hydrate.ErrRowsPathInvalid},
		{"非数组", map[string]any{"order": map[string]any{"items": "not-an-array"}}, hydrate.ErrRowsPathInvalid},
	}
	for _, tc := range cases {
		_, err := hydrate.NewHydrator(nil).Hydrate(canvas.New(320, 400, templateTable()), tc.dataset)
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
		_, err := hydrate.NewHydrator(nil).Hydrate(canvas.New(320, 100, text), ds)
		if !errors.Is(err, hydrate.ErrReservedRootKey) {
			t.Errorf("%s: 错误 = %v, want ErrReservedRootKey", name, err)
		}
	}
}

func TestEmptyResourceExpressionThrows(t *testing.T) {
	// 资源类字段(Image src)求值空串 = 报错
	ds := map[string]any{"order": map[string]any{"items": []any{map[string]any{"name": "张三"}}}}
	_, err := hydrate.NewHydrator(nil).Hydrate(canvas.New(320, 400, templateTable()), ds)
	if !errors.Is(err, hydrate.ErrExpressionEmptyResource) {
		t.Errorf("错误 = %v, want ErrExpressionEmptyResource", err)
	}
}

// ─── 资源表达式单值门(ADR 0020,issue 02;对拍 = CanvasHydratorTest 单值门用例平移) ───

// singleImageCanvas 单独立图片图层画布(与 PHP 导出端 singleImageCanvas 同构):
// 资源单值门根上下文聚焦用例
func singleImageCanvas(expression string) *canvas.Canvas {
	image := layer.NewImageLayer(layer.WithSize(60, 60))
	image.SetExpression(expression)
	return canvas.New(320, 100, image)
}

// singleImageTemplateTable 单图片格模板表(与 PHP 导出端 singleImageTemplateTable 同构):
// 资源单值门行上下文聚焦用例
func singleImageTemplateTable(expression string) *layer.TableLayer {
	content := layer.NewImageLayer(layer.WithSize(160, 40))
	content.SetExpression(expression)
	cell := layer.NewTableCellLayer(layer.WithSize(160, 40))
	cell.AddTemplateContentLayer(content)
	template := layer.NewTableRowTemplate(layer.WithAutoHeight())
	template.AddCell(cell)
	table := layer.NewTableLayer(layer.WithSize(320, 200))
	table.SetRowsPath("items")
	table.SetTemplate(template)
	return table
}

func TestResourceSingleBarePathIsLegal(t *testing.T) {
	// 合法形态:恰一个 {{...}} 闭合片段 + 裸点路径,根上下文放行
	hydrated, err := hydrate.NewHydrator(nil).Hydrate(
		singleImageCanvas("{{orderNo}}"), map[string]any{"orderNo": "A2026-001"})
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	image := hydrated.GetLayers()[0].(*layer.ImageLayer)
	if got := *image.Image(); got != "A2026-001" {
		t.Errorf("图片 = %q, want A2026-001", got)
	}
	if image.Graph().Data.Expression != nil {
		t.Error("求值后 expression 键必须移除(Image/Qr 条件写键)")
	}
}

func TestResourceRootEscapePathIsLegal(t *testing.T) {
	// 合法形态:$root. 前缀同为裸点路径
	hydrated, err := hydrate.NewHydrator(nil).Hydrate(
		singleImageCanvas("{{$root.orderNo}}"), map[string]any{"orderNo": "A2026-001"})
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if got := *hydrated.GetLayers()[0].(*layer.ImageLayer).Image(); got != "A2026-001" {
		t.Errorf("图片 = %q, want A2026-001", got)
	}
}

func TestResourceTrailingNewlineToleratedLikePcreDollar(t *testing.T) {
	// PCRE $ 锚(无 D 修饰符)容忍串尾恰一个换行:PHP 权威端实证 {{orderNo}}\n
	// 门放行(trim 吸收),求值后片段外尾换行按字面直通(值面宽松,双端同判)——
	// \n\n 与 \r\n 两形态仍由违例表拦截
	hydrated, err := hydrate.NewHydrator(nil).Hydrate(
		singleImageCanvas("{{orderNo}}\n"), map[string]any{"orderNo": "A2026-001"})
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if got := *hydrated.GetLayers()[0].(*layer.ImageLayer).Image(); got != "A2026-001\n" {
		t.Errorf("图片 = %q, want A2026-001\\n(尾换行字面直通,PHP 同判)", got)
	}
}

func TestResourceIndexPathLegalInRowContext(t *testing.T) {
	// 合法形态:$index 保留名(行上下文,自 1 起)
	ds := map[string]any{"items": []any{map[string]any{"x": 1}, map[string]any{"x": 2}}}
	hydrated, err := hydrate.NewHydrator(nil).Hydrate(
		canvas.New(320, 400, singleImageTemplateTable("{{$index}}")), ds)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	rows := firstTable(t, hydrated).Rows()
	if got := *rows[0].Cells()[0].ContentLayer().(*layer.ImageLayer).Image(); got != "1" {
		t.Errorf("行 1 图片 = %q, want 1", got)
	}
	if got := *rows[1].Cells()[0].ContentLayer().(*layer.ImageLayer).Image(); got != "2" {
		t.Errorf("行 2 图片 = %q, want 2", got)
	}
}

func TestResourceExpressionViolationFailsFast(t *testing.T) {
	// 五类违例形态 + 边角(纯空白片段/未闭合片段)
	cases := []struct{ name, expression string }{
		{"多片段拼接", "{{orderNo}}{{orderNo}}"},
		{"片段外字面（尾缀）", "{{orderNo}}.png"},
		{"片段外字面（前缀）", "https://cdn/{{orderNo}}"},
		{"函数调用", "{{number(orderNo, 2)}}"},
		{"空片段", "{{}}"},
		{"纯空白片段", "{{  }}"},
		{"未闭合片段", "{{orderNo"},
		{"片段外字面 + 双换行尾缀", "{{orderNo}}\n\n"},
		{"片段外字面 + CRLF 尾缀", "{{orderNo}}\r\n"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := hydrate.NewHydrator(nil).Hydrate(singleImageCanvas(tc.expression), dataset())
			if !errors.Is(err, hydrate.ErrExpressionSingleResourceRef) {
				t.Fatalf("资源表达式「%s」违例必须报 ErrExpressionSingleResourceRef, got %v", tc.expression, err)
			}
		})
	}
}

func TestResourceViolationPrecedesEmptyResourceCheck(t *testing.T) {
	// 顺序钉定:校验在求值前——违例形态即使求值必空也报 single_ref,而非 empty_resource
	_, err := hydrate.NewHydrator(nil).Hydrate(
		singleImageCanvas("{{missing}}.png"), map[string]any{"orderNo": "A"})
	if !errors.Is(err, hydrate.ErrExpressionSingleResourceRef) {
		t.Fatalf("错误 = %v, want ErrExpressionSingleResourceRef(先于 empty_resource)", err)
	}
}

func TestQrCodeViolationFailsFast(t *testing.T) {
	// 二维码静默错编是该门的首要防线(渲染零报错、扫码才暴露的实测缺陷)
	qr := layer.NewQrCodeLayer(layer.WithSize(60, 60))
	qr.SetExpression("{{orderNo}}-qr")
	_, err := hydrate.NewHydrator(nil).Hydrate(canvas.New(320, 100, qr), dataset())
	if !errors.Is(err, hydrate.ErrExpressionSingleResourceRef) {
		t.Fatalf("错误 = %v, want ErrExpressionSingleResourceRef", err)
	}
}

func TestTextLayerInterpolationUnchangedBySingleRefGate(t *testing.T) {
	// 门只管资源两类:文本层多片段拼接 + 字面混排照旧插值(spec 裁决 3:插值语义不动)
	text := layer.NewTextLayer(layer.WithSize(200, 30), layer.WithFont("", 12, "#000"))
	text.SetExpression("单号 {{orderNo}} 与 {{orderNo}}")
	hydrated, err := hydrate.NewHydrator(nil).Hydrate(
		canvas.New(320, 100, text), map[string]any{"orderNo": "A2026-001"})
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if got := hydrated.GetLayers()[0].(*layer.TextLayer).Text(); got != "单号 A2026-001 与 A2026-001" {
		t.Errorf("文本 = %q, want 单号 A2026-001 与 A2026-001", got)
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
	_, err := hydrate.NewHydrator(nil).Hydrate(canvas.New(320, 400, table), ds)
	if !errors.Is(err, hydrate.ErrExpressionTypeMismatch) {
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
	// 求值器为可注入策略(spec §3.6):替换默认实现后填充器走注入面
	stub := &recordingEvaluator{result: "STUB"}
	text := layer.NewTextLayer(layer.WithSize(100, 30), layer.WithFont("", 12, "#000"))
	text.SetExpression("任意模板")

	hydrated, err := hydrate.NewHydrator(stub).Hydrate(canvas.New(320, 100, text), map[string]any{"k": "v"})
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if got := hydrated.GetLayers()[0].(*layer.TextLayer).Text(); got != "STUB" {
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

	hydrated, err := hydrate.NewHydrator(nil).Hydrate(canvas.New(320, 400, table), ds)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if got := firstTable(t, hydrated).Rows()[0].Cells()[0].ContentLayer().(*layer.TextLayer).Text(); got != "张三" {
		t.Errorf("struct 数据集行文本 = %q, want 张三", got)
	}
}

func TestSentinelMessagesCarryCode(t *testing.T) {
	// 稳定 code 是三端一致性抓手:消息以 code 前缀开头
	ds := map[string]any{"order": map[string]any{"items": "not-an-array"}}
	_, err := hydrate.NewHydrator(nil).Hydrate(canvas.New(320, 400, templateTable()), ds)
	if err == nil || !strings.HasPrefix(err.Error(), "rows_path_invalid") {
		t.Errorf("错误消息 = %v, want rows_path_invalid 前缀", err)
	}
}

func TestBackgroundPassesThroughHydrate(t *testing.T) {
	// 背景静态(ADR 0022 / spec §3):不进 hydrate 改写面,graph 层改写 + FromGraph
	// 重建全程透传,分量逐字保留;数据驱动底图走图层级,与背景并存合法
	c := canvas.New(320, 400, templateTable())
	color, image := "#ffffff", "https://cdn.example.com/bg.png"
	c.SetBackground(canvas.NewCanvasBackground(&color, &image))

	hydrated, err := hydrate.NewHydrator(nil).Hydrate(c, dataset())
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	bg := hydrated.Background()
	if bg == nil || bg.Color == nil || *bg.Color != color || bg.Image == nil || *bg.Image != image {
		t.Fatalf("hydrate 后背景不符: got %+v, want color=%s image=%s", bg, color, image)
	}
	// 再序列化键面:canvas 段 background 键原样透传
	want := `{"canvas":{"width":320,"height":400,"background":{"color":"#ffffff","image":"https://cdn.example.com/bg.png"}}`
	if got := jsonOf(t, hydrated.Graph()); !strings.HasPrefix(got, want) {
		t.Errorf("hydrate 产物 canvas 段背景键不符:\n got  %s\n want 前缀 %s", got, want)
	}
}

// ---- 软诊断发射缝(spec/pipeline.md §4):文案类表达式兜底空串发射
// expression_fallback_empty,code/message/path 与 PHP 权威端逐字对齐,
// 序列即发射序(文档序);未接收集器 = 静默兜底,存量行为零变化

func fallbackText(expression string) *layer.TextLayer {
	text := layer.NewTextLayer(layer.WithSize(100, 30), layer.WithFont("", 12, "#000"))
	text.SetExpression(expression)
	return text
}

func TestTextFallbackEmitsDiagnostic(t *testing.T) {
	collector := hydrate.NewDiagnosticCollector()
	hydrated, err := hydrate.NewHydrator(nil).Hydrate(
		canvas.New(320, 100, fallbackText("{{missing}}")),
		map[string]any{"other": "x"}, collector)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	want := []hydrate.Diagnostic{{
		Code:    "expression_fallback_empty",
		Message: "文案表达式 {{missing}} 求值为空串，已兜底为空",
		Path:    "layers[0].data.value",
	}}
	if !reflect.DeepEqual(collector.All(), want) {
		t.Errorf("诊断序列 = %+v, want %+v", collector.All(), want)
	}
	// 兜底行为本身零变化:求值结果落空串、标记解除
	if got := hydrated.GetLayers()[0].(*layer.TextLayer).Text(); got != "" {
		t.Errorf("兜底文本 = %q, want 空串", got)
	}
}

func TestTextFallbackSilentWithoutCollector(t *testing.T) {
	// 未接收集器 = 静默兜底(PHP 可空参数 null 同构),独立管线历史行为零变化
	hydrated, err := hydrate.NewHydrator(nil).Hydrate(
		canvas.New(320, 100, fallbackText("{{missing}}")), map[string]any{})
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if got := hydrated.GetLayers()[0].(*layer.TextLayer).Text(); got != "" {
		t.Errorf("兜底文本 = %q, want 空串", got)
	}
}

func TestFallbackMixedHitAndFallbackDocumentOrder(t *testing.T) {
	// 命中不发射、兜底才发射;多诊断序列 = 图层文档序
	collector := hydrate.NewDiagnosticCollector()
	_, err := hydrate.NewHydrator(nil).Hydrate(
		canvas.New(320, 100,
			fallbackText("{{orderNo}}"),
			fallbackText("{{missing}}")),
		map[string]any{"orderNo": "A2026-001"}, collector)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	want := []hydrate.Diagnostic{{
		Code:    "expression_fallback_empty",
		Message: "文案表达式 {{missing}} 求值为空串，已兜底为空",
		Path:    "layers[1].data.value",
	}}
	if !reflect.DeepEqual(collector.All(), want) {
		t.Errorf("诊断序列 = %+v, want %+v", collector.All(), want)
	}
}

func TestTemplateRowFallbackDiagnosticPaths(t *testing.T) {
	// 模板表实例路径 = 产物树落位 rows[实例序],逐行行序发射
	content := layer.NewTextLayer(layer.WithSize(320, 0), layer.WithAutoHeight(), layer.WithFont("", 12, "#000"))
	content.SetExpression("{{row.nickname}}")
	cell := layer.NewTableCellLayer(layer.WithSize(320, 0), layer.WithAutoHeight())
	cell.AddTemplateContentLayer(content)
	template := layer.NewTableRowTemplate(layer.WithAutoHeight())
	template.AddCell(cell)
	table := layer.NewTableLayer(layer.WithSize(320, 200))
	table.SetRowsPath("items")
	table.SetTemplate(template)

	collector := hydrate.NewDiagnosticCollector()
	_, err := hydrate.NewHydrator(nil).Hydrate(
		canvas.New(320, 400, table),
		map[string]any{"items": []any{
			map[string]any{"name": "甲"},
			map[string]any{"name": "乙"},
		}}, collector)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	all := collector.All()
	if len(all) != 2 {
		t.Fatalf("诊断数 = %d, want 2(%+v)", len(all), all)
	}
	wantPaths := []string{
		"layers[0].rows[0].cells[0].content.data.value",
		"layers[0].rows[1].cells[0].content.data.value",
	}
	for i, wantPath := range wantPaths {
		if all[i].Path != wantPath {
			t.Errorf("诊断[%d].Path = %q, want %q", i, all[i].Path, wantPath)
		}
		if all[i].Code != "expression_fallback_empty" {
			t.Errorf("诊断[%d].Code = %q, want expression_fallback_empty", i, all[i].Code)
		}
	}
}

func TestV1RowFallbackDiagnosticPath(t *testing.T) {
	// V1 表(静态 rows)路径形态 rows[i].cells[j]:命中行静默,兜底行发射
	row0 := layer.NewTableRowLayer(layer.WithSize(320, 0), layer.WithAutoHeight())
	cell0 := layer.NewTableCellLayer(layer.WithSize(320, 0), layer.WithAutoHeight())
	text0 := layer.NewTextLayer(layer.WithSize(320, 0), layer.WithAutoHeight(), layer.WithFont("", 12, "#000"))
	text0.SetExpression("{{orderNo}}")
	cell0.AddContentLayer(text0)
	row0.AddCell(cell0)

	row1 := layer.NewTableRowLayer(layer.WithSize(320, 0), layer.WithAutoHeight())
	cell1 := layer.NewTableCellLayer(layer.WithSize(320, 0), layer.WithAutoHeight())
	text1 := layer.NewTextLayer(layer.WithSize(320, 0), layer.WithAutoHeight(), layer.WithFont("", 12, "#000"))
	text1.SetExpression("{{missing}}")
	cell1.AddContentLayer(text1)
	row1.AddCell(cell1)

	table := layer.NewTableLayer(layer.WithSize(320, 200))
	table.AddRow(row0)
	table.AddRow(row1)

	collector := hydrate.NewDiagnosticCollector()
	_, err := hydrate.NewHydrator(nil).Hydrate(
		canvas.New(320, 400, table), map[string]any{"orderNo": "A2026-001"}, collector)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	want := []hydrate.Diagnostic{{
		Code:    "expression_fallback_empty",
		Message: "文案表达式 {{missing}} 求值为空串，已兜底为空",
		Path:    "layers[0].rows[1].cells[0].content.data.value",
	}}
	if !reflect.DeepEqual(collector.All(), want) {
		t.Errorf("诊断序列 = %+v, want %+v", collector.All(), want)
	}
}

func TestNullDatasetCollectsNoDiagnostics(t *testing.T) {
	// 未绑数据集恒等直通先于求值:收集器在场也无发射(PHP 同序)
	collector := hydrate.NewDiagnosticCollector()
	_, err := hydrate.NewHydrator(nil).Hydrate(canvas.New(320, 100, fallbackText("{{missing}}")), nil, collector)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if all := collector.All(); len(all) != 0 {
		t.Errorf("诊断序列 = %+v, want 空", all)
	}
}

func TestResourceEmptyStaysHardError(t *testing.T) {
	// 资源类求值空串 = 硬错误 fail-fast,不落软诊断(发射面唯一 = 文案类)
	collector := hydrate.NewDiagnosticCollector()
	image := layer.NewImageLayer(layer.WithSize(60, 60))
	image.SetExpression("{{missing}}")
	_, err := hydrate.NewHydrator(nil).Hydrate(
		canvas.New(320, 100, image), map[string]any{}, collector)
	if !errors.Is(err, hydrate.ErrExpressionEmptyResource) {
		t.Fatalf("错误 = %v, want ErrExpressionEmptyResource", err)
	}
	if all := collector.All(); len(all) != 0 {
		t.Errorf("诊断序列 = %+v, want 空", all)
	}
}

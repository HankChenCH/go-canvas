package layer_test

// TableLayer V2 模板态 wire 与解码（spec §2 / go-canvas 工票 12）:
// template 键 + data.rowsPath 条件写键（模板态不写 rows,V1 态不写 template/data）、
// XOR 解码校验、模板装配高度豁免、清空切换语义。对拍基准 = php-canvas-next
// tests/Layer/TableTemplateTest.php 与 layout-snapshot 的 table-template-wire 用例。

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hankchen/go-canvas/layer"
)

// templateTableFixture 主模板表:文本格（表达式标记,镜像 value）+ 固定图片格,
// rowsPath = order.items（与 PHP fixture fixtureTemplateTable 同构）
func templateTableFixture() *layer.TableLayer {
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

func TestTemplateStateGraphConditionalKeys(t *testing.T) {
	// 模板态:条件写键 data/template、不写 rows;data 形态 = 仅 rowsPath 键
	// （PHP TableLayer::graph() 同款,字节面 parity——Go graphdump ↔ PHP 结构级比对的前提）
	table := templateTableFixture()
	node := table.Graph()

	if node.Template == nil {
		t.Fatal("模板态 graph 必须携带 template 键")
	}
	if node.Rows != nil {
		t.Error("模板态 graph 不得写 rows 键")
	}
	if node.Data == nil {
		t.Fatal("已设 rowsPath 的模板态必须携带 data 键")
	}
	if got, want := jsonOf(t, node.Data), `{"rowsPath":"order.items"}`; got != want {
		t.Errorf("data 键字节面 = %s, want %s", got, want)
	}

	// 模板行类型标识与 PHP 类常量同名
	var templateNode layer.Node
	if err := json.Unmarshal(node.Template, &templateNode); err != nil {
		t.Fatalf("解码 template 载荷: %v", err)
	}
	if templateNode.Type != "TableRowTemplate" {
		t.Errorf("template.type = %q, want TableRowTemplate", templateNode.Type)
	}

	// V1 态:rows 恒写、无 template/data 键
	v1 := layer.NewTableLayer(layer.WithSize(320, 200))
	v1.AddRow(rowWithCell(320, 30, "#f00"))
	v1Node := v1.Graph()
	if v1Node.Template != nil || v1Node.Data != nil {
		t.Error("V1 态不得写 template/data 键")
	}
	if v1Node.Rows == nil {
		t.Error("V1 态 rows 键恒写（空表为 []）")
	}
}

func TestTemplateStateRowsPathOmittedKeepsDataKeyAbsent(t *testing.T) {
	// rowsPath 未设:模板态不写 data 键（不产出违反自身约束的中间 wire,PHP 同款）
	row := layer.NewTableRowTemplate()
	row.AddCell(layer.NewTableCellLayer(layer.WithSize(160, 40)))
	table := layer.NewTableLayer(layer.WithSize(320, 200))
	table.SetTemplate(row)

	node := table.Graph()
	if node.Data != nil {
		t.Errorf("未设 rowsPath 不得写 data 键, got %s", jsonOf(t, node.Data))
	}
	if node.Template == nil {
		t.Error("模板态必须携带 template 键")
	}
}

func TestTableTemplateStateRoundtrip(t *testing.T) {
	// 声明态往返恒等（spec §2.4）+ 模板装配高度豁免:解码不重放 V1 高度耦合,
	// 格/内容的声明高与 auto 标志原样保留（实例高度由展开定稿）
	table := templateTableFixture()

	var node layer.Node
	if err := json.Unmarshal([]byte(jsonOf(t, table.Graph())), &node); err != nil {
		t.Fatalf("解码 graph 节点: %v", err)
	}
	rebuilt, err := layer.TableFromGraph(node)
	if err != nil {
		t.Fatalf("TableFromGraph: %v", err)
	}

	if got, want := jsonOf(t, rebuilt.Graph()), jsonOf(t, table.Graph()); got != want {
		t.Errorf("模板态往返 graph 不恒等:\n got  %s\n want %s", got, want)
	}

	// 宽度耦合沿用:模板行宽 = 表宽
	template := rebuilt.Template()
	if template == nil {
		t.Fatal("重建后模板丢失")
	}
	if got := template.Width(); got != 320 {
		t.Errorf("模板行宽 = %d, want 320", got)
	}
	if got := rebuilt.RowsPath(); got != "order.items" {
		t.Errorf("rowsPath = %q, want order.items", got)
	}

	// 高度豁免:格/内容的声明高与 auto 标志原样保留(断言 wire 声明态;
	// 若重放 V1 耦合,auto 格会采纳内容高、固定格会把内容压平)
	textCell := template.Cells()[0]
	if !textCell.Graph().Spec.Shape.AutoHeight {
		t.Error("模板文本格 autoHeight 必须原样保留")
	}
	var textContent layer.Node
	if err := json.Unmarshal(textCell.Graph().Content, &textContent); err != nil {
		t.Fatalf("解码模板文本格内容: %v", err)
	}
	if shape := textContent.Spec.Shape; shape.Height != 0 || !shape.AutoHeight {
		t.Errorf("模板文本格内容声明态被改写: %v, want 高 0 + auto(豁免采纳)", shape)
	}
	imageCell := template.Cells()[1]
	var imageContent layer.Node
	if err := json.Unmarshal(imageCell.Graph().Content, &imageContent); err != nil {
		t.Fatalf("解码模板图片格内容: %v", err)
	}
	if shape := imageContent.Spec.Shape; shape.Height != 0 || !shape.AutoHeight {
		t.Errorf("模板图片格内容声明态被改写: %v, want 高 0 + auto(豁免压平)", shape)
	}
	if imageCell.Graph().Spec.Shape.AutoHeight {
		t.Error("模板图片格固定高必须原样保留")
	}
}

func TestTableFromGraphTemplateRowsConflict(t *testing.T) {
	// XOR（spec §2.2）:template 与 rows 键同现 = 非法 wire,稳定 code
	// template_rows_conflict（三端一致性抓手,errors.Is 判定）
	raw := `{"type":"TableLayer","priority":0,"spec":{"shape":{"width":320,"height":200,` +
		`"autoWidth":false,"autoHeight":false,"lineHeight":1,` +
		`"padding":{"top":0,"bottom":0,"left":0,"right":0},` +
		`"border":{"top":null,"bottom":null,"left":null,"right":null},"backgroundColor":null},` +
		`"align":{"horizontal":"left","vertical":"top"},` +
		`"position":{"x":0,"y":0,"position":"top-left"}},` +
		`"data":{"rowsPath":"order.items"},"rows":[],"template":{"type":"TableRowTemplate"}}`
	var node layer.Node
	if err := json.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatalf("解码非法 wire: %v", err)
	}

	_, err := layer.TableFromGraph(node)
	if !errors.Is(err, layer.ErrTemplateRowsConflict) {
		t.Errorf("双键同现错误 = %v, want ErrTemplateRowsConflict", err)
	}
}

func TestTableFromGraphTemplateMissingRowsPath(t *testing.T) {
	// 模板态缺 data.rowsPath = 非法 wire（先紧后松）,稳定 code rows_path_missing
	base := `{"type":"TableLayer","priority":0,"spec":{"shape":{"width":320,"height":200,` +
		`"autoWidth":false,"autoHeight":false,"lineHeight":1,` +
		`"padding":{"top":0,"bottom":0,"left":0,"right":0},` +
		`"border":{"top":null,"bottom":null,"left":null,"right":null},"backgroundColor":null},` +
		`"align":{"horizontal":"left","vertical":"top"},` +
		`"position":{"x":0,"y":0,"position":"top-left"}},"template":{"type":"TableRowTemplate"}}`

	for name, raw := range map[string]string{
		"无 data 键":   base,
		"data 缺键":    strings.Replace(base, `"template"`, `"data":{},"template"`, 1),
		"rowsPath 空": strings.Replace(base, `"template"`, `"data":{"rowsPath":""},"template"`, 1),
	} {
		var node layer.Node
		if err := json.Unmarshal([]byte(raw), &node); err != nil {
			t.Fatalf("%s: 解码 wire: %v", name, err)
		}
		if _, err := layer.TableFromGraph(node); !errors.Is(err, layer.ErrRowsPathMissing) {
			t.Errorf("%s: 错误 = %v, want ErrRowsPathMissing", name, err)
		}
	}
}

func TestTemplateNullKeyFallsBackToV1(t *testing.T) {
	// "template": null 与缺键同判（PHP array_key_exists + !== null 同款）:回落 V1 解码
	raw := `{"type":"TableLayer","priority":0,"spec":{"shape":{"width":100,"height":40,` +
		`"autoWidth":false,"autoHeight":false,"lineHeight":1,` +
		`"padding":{"top":0,"bottom":0,"left":0,"right":0},` +
		`"border":{"top":null,"bottom":null,"left":null,"right":null},"backgroundColor":null},` +
		`"align":{"horizontal":"left","vertical":"top"},` +
		`"position":{"x":0,"y":0,"position":"top-left"}},"template":null,"rows":[]}`
	var node layer.Node
	if err := json.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatalf("解码 wire: %v", err)
	}
	table, err := layer.TableFromGraph(node)
	if err != nil {
		t.Fatalf("template:null 应回落 V1 解码, got %v", err)
	}
	if table.Template() != nil {
		t.Error("template:null 不得产生模板")
	}
}

func TestAddRowRejectedInTemplateState(t *testing.T) {
	// API 误用防御（PHP addRow 抛 InvalidArgumentException 同款,不占解码期异常类）
	table := templateTableFixture()
	if err := table.AddRow(rowWithCell(320, 30, "#f00")); err == nil {
		t.Error("模板态 AddRow 必须报错")
	}
}

func TestSetTemplateSwitchingSemantics(t *testing.T) {
	// 清空切换:设置模板即清空既有行;传 nil 清模板与 rowsPath 回 V1 形态
	table := layer.NewTableLayer(layer.WithSize(320, 200))
	table.AddRow(rowWithCell(320, 30, "#f00"))
	table.AddRow(rowWithCell(320, 50, "#0f0"))
	// 累计 80:再加 130 高的行即超出表高 200
	if !table.IsOverHeight(rowWithCell(320, 130, "#00f")) {
		t.Fatal("前置断言失败:累计内容盒高应为 80")
	}

	template := layer.NewTableRowTemplate(layer.WithAutoHeight())
	template.AddCell(layer.NewTableCellLayer(layer.WithSize(160, 40)))
	table.SetRowsPath("order.items")
	table.SetTemplate(template)

	if rows := table.Rows(); len(rows) != 0 {
		t.Errorf("设置模板后既有行未清空: %d", len(rows))
	}
	if table.IsOverHeight(rowWithCell(320, 130, "#00f")) {
		t.Error("设置模板后内容盒高未清零")
	}

	table.SetTemplate(nil)
	if table.Template() != nil {
		t.Error("SetTemplate(nil) 未清模板")
	}
	if got := table.RowsPath(); got != "" {
		t.Errorf("SetTemplate(nil) 未清 rowsPath: %q", got)
	}
	if node := table.Graph(); node.Template != nil || node.Data != nil || node.Rows == nil {
		t.Error("清除模板后 graph 应回 V1 形态（rows 键、无 template/data）")
	}
}

func TestTableRowTemplateFactoryRegistration(t *testing.T) {
	// 工厂登记:TableRowTemplate 类型可解码（出现在非法位置属契约约束,工厂不拒绝——对齐 PHP）
	row := layer.NewTableRowTemplate(layer.WithAutoHeight())
	row.AddCell(layer.NewTableCellLayer(layer.WithSize(160, 40)))

	var node layer.Node
	if err := json.Unmarshal([]byte(jsonOf(t, row.Graph())), &node); err != nil {
		t.Fatalf("解码模板行节点: %v", err)
	}
	l, err := layer.FromGraph(node)
	if err != nil {
		t.Fatalf("FromGraph(TableRowTemplate): %v", err)
	}
	if got, want := jsonOf(t, l.Graph()), jsonOf(t, row.Graph()); got != want {
		t.Errorf("工厂解码往返不恒等:\n got  %s\n want %s", got, want)
	}
}

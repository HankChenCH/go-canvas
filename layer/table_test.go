package layer_test

// TableLayersTest 平移:表→行→单元格三层容器的构建期副作用(表 addRow 同步行宽并
// 累加内容盒高、行 addCell 行高取最高、单元格 addContentLayer 压平/采纳内容层高度)
// 与嵌套 graph 往返;另收口 base_test.go 头注遗留的基类默认对齐(left/top)断言。

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/HankChenCH/go-canvas/layer"
)

// rowWithCell PHP TableLayersTest::rowWithCell 助手:单单元格 auto 行
func rowWithCell(cellWidth, cellHeight int, bg string) *layer.TableRowLayer {
	row := layer.NewTableRowLayer(layer.WithSize(cellWidth, 0), layer.WithAutoHeight())
	row.AddCell(layer.NewTableCellLayer(layer.WithSize(cellWidth, cellHeight), layer.WithBackground(bg)))
	return row
}

func TestTableLayerKeepsBaseDefaultAlign(t *testing.T) {
	// 基类默认对齐(left/top)补齐断言:容器图层无 PHP 覆写,即基类默认态
	l := layer.NewTableLayer()
	if l.HorizontalAlign() != layer.AlignLeft || l.VerticalAlign() != layer.AlignTop {
		t.Errorf("表格图层默认对齐 = (%s, %s), want (left, top)", l.HorizontalAlign(), l.VerticalAlign())
	}
}

func TestAddRowSyncsRowWidthToTableWidth(t *testing.T) {
	// PHP TableLayersTest::testAddRowSyncsRowWidthToTableWidth
	table := layer.NewTableLayer(layer.WithSize(200, 100))
	row := rowWithCell(50, 10, "#fff")

	table.AddRow(row)

	if got := row.Width(); got != 200 {
		t.Errorf("行宽 = %d, want 200", got)
	}
}

func TestAutoWidthContentLayerWidthForceSynced(t *testing.T) {
	// 工单 02 回归:表格「宽同步 + 强关 autoWidth」耦合不受 TextLayer 宽度求值扰动
	// ——格内容层即使带 autoWidth 构造,包装时宽同步为格宽且标志被清除,
	// 求值不进入表格耦合(PHP TableLayer 强关同款)
	cell := layer.NewTableCellLayer(layer.WithSize(160, 40))
	content := layer.NewTextLayer(layer.WithAutoWidth(), layer.WithText("自动宽内容"))
	cell.AddContentLayer(content)

	if got := content.Width(); got != 160 {
		t.Errorf("内容层宽 = %d, want 160(同步格宽)", got)
	}
	if content.Graph().Spec.Shape.AutoWidth {
		t.Error("内容层 autoWidth 标志必须被强关")
	}
}

func TestIsOverHeightTracksAccumulatedRowHeights(t *testing.T) {
	// PHP TableLayersTest::testIsOverHeightTracksAccumulatedRowHeights:按累计行高判定
	table := layer.NewTableLayer(layer.WithSize(200, 25))
	table.AddRow(rowWithCell(200, 10, "#fff"))

	if table.IsOverHeight(rowWithCell(200, 15, "#fff")) {
		t.Errorf("10+15=25 不超 25, want false")
	}
	if !table.IsOverHeight(rowWithCell(200, 16, "#fff")) {
		t.Errorf("10+16=26 超 25, want true")
	}
}

func TestAddCellGrowsRowHeightToTallest(t *testing.T) {
	// PHP TableLayersTest::testAddCellGrowsRowHeightToTallest;行高落定即清除 auto 标志
	row := layer.NewTableRowLayer(layer.WithSize(100, 0), layer.WithAutoHeight())
	row.AddCell(layer.NewTableCellLayer(layer.WithSize(50, 12), layer.WithBackground("#fff")))
	row.AddCell(layer.NewTableCellLayer(layer.WithSize(50, 30), layer.WithBackground("#fff")))

	if got := row.Height(); got != 30 {
		t.Errorf("行高 = %d, want 30", got)
	}
	if row.Graph().Spec.Shape.AutoHeight {
		t.Errorf("行高经 setHeight 落定后 auto 标志应清除")
	}
}

func TestFixedCellFlattensContentHeight(t *testing.T) {
	// PHP TableLayersTest::testFixedCellFlattensContentHeight
	cell := layer.NewTableCellLayer(layer.WithSize(100, 40), layer.WithBackground("#fff"))
	text := layer.NewTextLayer(layer.WithSize(100, 0), layer.WithAutoHeight(), layer.WithText("内容"))

	cell.AddContentLayer(text)

	if got := text.Height(); got != 40 {
		t.Errorf("内容层高 = %d, want 40", got)
	}
	if text.Graph().Spec.Shape.AutoHeight {
		t.Errorf("固定单元格应关闭内容层 autoHeight")
	}
}

func TestAutoCellAdoptsContentHeight(t *testing.T) {
	// PHP TableLayersTest::testAutoCellAdoptsContentHeight
	cell := layer.NewTableCellLayer(
		layer.WithSize(100, 0),
		layer.WithAutoHeight(),
		layer.WithBackground("#fff"),
	)

	cell.AddContentLayer(layer.NewTextLayer(
		layer.WithSize(100, 0),
		layer.WithAutoHeight(),
		layer.WithText("内容"),
		layer.WithFont("", 12, "#000"),
	))

	if got := cell.Height(); got != 12 {
		t.Errorf("auto 单元格高 = %d, want 12", got)
	}
}

func bgOf(n layer.Node) string {
	if n.Spec.Shape.BackgroundColor == nil {
		return "<nil>"
	}
	return *n.Spec.Shape.BackgroundColor
}

func TestGraphContainsFullRowsNotOnlyFirst(t *testing.T) {
	// PHP TableLayersTest::testGraphContainsFullRowsNotOnlyFirst:无损,graph 携带全部行
	table := layer.NewTableLayer(layer.WithSize(100, 40))
	table.AddRow(rowWithCell(100, 10, "#f00"))
	table.AddRow(rowWithCell(100, 30, "#0f0"))

	rows := *table.Graph().Rows
	if len(rows) != 2 {
		t.Fatalf("graph 行数 = %d, want 2", len(rows))
	}
	if got := bgOf((*rows[0].Cells)[0]); got != "#f00" {
		t.Errorf("行 0 单元格背景 = %q, want #f00", got)
	}
	if got := bgOf((*rows[1].Cells)[0]); got != "#0f0" {
		t.Errorf("行 1 单元格背景 = %q, want #0f0", got)
	}
	if cells := *rows[0].Cells; len(cells) != 1 {
		t.Errorf("行 0 单元格数 = %d, want 1", len(cells))
	}
}

func TestTableFromGraphRoundtrip(t *testing.T) {
	// PHP TableLayersTest::testTableFromGraphRoundtrip:嵌套往返恒等,解码复现 add 路径
	table := layer.NewTableLayer(layer.WithSize(100, 40), layer.WithBackground("#fff"), layer.WithPriority(2))
	row := layer.NewTableRowLayer(layer.WithSize(100, 0), layer.WithAutoHeight())
	cell := layer.NewTableCellLayer(layer.WithSize(50, 20), layer.WithBackground("#eee"))
	cell.AddContentLayer(layer.NewTextLayer(
		layer.WithSize(50, 0),
		layer.WithAutoHeight(),
		layer.WithText("单元格"),
		layer.WithFont("", 10, "#000"),
	))
	row.AddCell(cell)
	table.AddRow(row)

	var node layer.Node
	if err := json.Unmarshal([]byte(jsonOf(t, table.Graph())), &node); err != nil {
		t.Fatalf("解码 graph 节点: %v", err)
	}
	rebuilt, err := layer.TableFromGraph(node)
	if err != nil {
		t.Fatalf("TableFromGraph: %v", err)
	}

	if got, want := jsonOf(t, rebuilt.Graph()), jsonOf(t, table.Graph()); got != want {
		t.Errorf("往返 graph 不恒等:\n got  %s\n want %s", got, want)
	}
	content := rebuilt.Rows()[0].Cells()[0].ContentLayer()
	text, ok := content.(*layer.TextLayer)
	if !ok {
		t.Fatalf("内容层类型 = %T, want *layer.TextLayer", content)
	}
	if got := text.Text(); got != "单元格" {
		t.Errorf("重建内容文本 = %q, want 单元格", got)
	}
	// 适配补充:解码复现 addRow 路径——重建后行宽已同步为表宽
	if got := rebuilt.Rows()[0].Width(); got != 100 {
		t.Errorf("重建后行宽 = %d, want 100", got)
	}
}

func TestEmptyContainerGraphKeepsPhpKeys(t *testing.T) {
	// 适配补充(wire 字节面):PHP 空表/空行/无内容单元格恒写 rows/cells/content 键
	// (空数组或 null),非容器节点不出现容器键;空容器 graph 解码→重建→graph 仍恒等
	spec := `"spec":{"shape":{"width":0,"height":0,"autoWidth":false,"autoHeight":false,` +
		`"lineHeight":1,"padding":{"top":0,"bottom":0,"left":0,"right":0},` +
		`"border":{"top":null,"bottom":null,"left":null,"right":null},"backgroundColor":null},` +
		`"align":{"horizontal":"left","vertical":"top"},` +
		`"position":{"x":0,"y":0,"position":"top-left"}}`

	cases := []struct {
		name string
		got  layer.Node
		want string
	}{
		{
			"空表恒写 rows:[]",
			layer.NewTableLayer().Graph(),
			`{"type":"TableLayer","priority":0,` + spec + `,"rows":[]}`,
		},
		{
			"空行恒写 cells:[]",
			layer.NewTableRowLayer().Graph(),
			`{"type":"TableRowLayer","priority":0,` + spec + `,"cells":[]}`,
		},
		{
			"无内容单元格恒写 content:null",
			layer.NewTableCellLayer().Graph(),
			`{"type":"TableCellLayer","priority":0,` + spec + `,"content":null}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := jsonOf(t, tc.got); got != tc.want {
				t.Errorf("graph 键面不符:\n got  %s\n want %s", got, tc.want)
			}
		})
	}

	if s := jsonOf(t, layer.NewTextLayer().Graph()); strings.Contains(s, `"rows"`) ||
		strings.Contains(s, `"cells"`) || strings.Contains(s, `"content"`) {
		t.Errorf("非容器节点不应含容器键: %s", s)
	}

	// 空容器往返:rows:[]/cells:[]/content:null 经解码重建后原样保留
	for _, tc := range cases {
		t.Run(tc.name+"往返", func(t *testing.T) {
			var node layer.Node
			if err := json.Unmarshal([]byte(tc.want), &node); err != nil {
				t.Fatalf("解码: %v", err)
			}
			rebuilt, err := layer.FromGraph(node)
			if err != nil {
				t.Fatalf("FromGraph: %v", err)
			}
			if got := jsonOf(t, rebuilt.Graph()); got != tc.want {
				t.Errorf("空容器往返不恒等:\n got  %s\n want %s", got, tc.want)
			}
		})
	}
}

func TestCellFromGraphRejectsUnknownContentType(t *testing.T) {
	// 适配补充:嵌套 content 未知类型报错且消息含类型名(对齐 PHP 异常传播,故事 14)
	var node layer.Node
	if err := json.Unmarshal(
		[]byte(jsonOf(t, layer.NewTableCellLayer(layer.WithSize(10, 10)).Graph())), &node,
	); err != nil {
		t.Fatalf("解码: %v", err)
	}
	node.Content = json.RawMessage(jsonOf(t, layer.Node{Type: "VideoLayer"}))

	if _, err := layer.TableCellFromGraph(node); err == nil {
		t.Fatal("嵌套未知类型未报错")
	} else if !strings.Contains(err.Error(), "VideoLayer") {
		t.Errorf("错误消息不含类型名: %v", err)
	}
}

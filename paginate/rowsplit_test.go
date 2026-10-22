package paginate

// 逐行拆分纯函数单测(提案 003,paginate-semantics v2.1;PHP RowSplitterTest 逐镜像):
// 判定/求界/片段构造的 Go 侧关怀——防线用例与 php-canvas-next tests/Compiler/
// RowSplitterTest.php 成对平移(AGENTS 跨端纪律:守卫是几何契约的一部分,防线不对称
// 会留死循环或静默面)。纯函数未导出,包内断言(先例 wire_test.go)。
// 拆分行为语义(页序列形状)由语料 v2.1 拆分 8 例钉定,本文件不手抄语料用例。

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/HankChenCH/go-canvas/layer"
)

// 行级 lineHeightPx(L)= fontSize × lineHeight 向上取整;测试统一 lineHeight=1(默认)
const testLineHeightPx = 20

// textRowGraph 多格 autowrap 文本行 graph(\n 硬断行给行数,内容盒足够宽不产生度量
// 折行——语料钉面同构:行数只依赖显式硬断行);构造走图层 API,graph() 产物即分片输入
func textRowGraph(cellValues []string, height, fontSize int) layer.Node {
	row := layer.NewTableRowLayer(layer.WithSize(320, height))
	for _, v := range cellValues {
		cell := layer.NewTableCellLayer(layer.WithSize(320, height))
		cell.AddContentLayer(layer.NewTextLayer(
			layer.WithSize(320, height),
			layer.WithText(v),
			layer.WithFont("", fontSize, "#000000"),
			layer.WithAutowrap(true),
		))
		row.AddCell(cell)
	}
	return row.Graph()
}

// imageRowGraph 单体格(图片)行 graph
func imageRowGraph(height int) layer.Node {
	cell := layer.NewTableCellLayer(layer.WithSize(320, height))
	cell.AddContentLayer(layer.NewImageLayer(layer.WithSize(320, height), layer.WithImage("/img/seal.png")))
	row := layer.NewTableRowLayer(layer.WithSize(320, height))
	row.AddCell(cell)
	return row.Graph()
}

// qrRowGraph 单体格(二维码)行 graph
func qrRowGraph(height int) layer.Node {
	cell := layer.NewTableCellLayer(layer.WithSize(320, height))
	cell.AddContentLayer(layer.NewQrCodeLayer(layer.WithSize(100, height), layer.WithQrText("https://example.com")))
	row := layer.NewTableRowLayer(layer.WithSize(320, height))
	row.AddCell(cell)
	return row.Graph()
}

// firstCellValue 首格内容值(data.value;分片构造与切片计数同一钉面)
func firstCellValue(t *testing.T, row layer.Node) string {
	t.Helper()
	var cell layer.Node
	if err := json.Unmarshal((*row.Cells)[0].Content, &cell); err != nil {
		t.Fatalf("解码首格 content: %v", err)
	}
	if cell.Data == nil || cell.Data.Value == nil {
		return ""
	}
	return *cell.Data.Value
}

// valueLineCount 值的内容行数(\n 硬断行切片计数,与片段构造同一钉面)
func valueLineCount(t *testing.T, row layer.Node) int {
	return strings.Count(firstCellValue(t, row), "\n") + 1
}

// jsonKeys 值 marshal 后的顶层键集(键形克隆断言用:分片不新增不删除键)
func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("序列化: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("反序列化取键集: %v", err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// decodeCellContent 解码格 content 键载荷(键缺省得零值 Node)
func decodeCellContent(t *testing.T, row layer.Node, cellIndex int) layer.Node {
	t.Helper()
	var content layer.Node
	raw := (*row.Cells)[cellIndex].Content
	if len(raw) == 0 {
		return content
	}
	if err := json.Unmarshal(raw, &content); err != nil {
		t.Fatalf("解码格 content: %v", err)
	}
	return content
}

func TestRowLineStatsCountsHardBreakLinesAcrossCells(t *testing.T) {
	row := textRowGraph([]string{"甲一", "乙一\n乙二\n乙三\n乙四\n乙五"}, 100, testLineHeightPx)
	maxLines, lineHeightPx, ok := rowLineStats(&row)

	if !ok {
		t.Fatal("全文本行应可拆统计")
	}
	if maxLines != 5 {
		t.Errorf("maxLines = %d, want 5", maxLines)
	}
	if lineHeightPx != testLineHeightPx {
		t.Errorf("lineHeightPx = %d, want %d", lineHeightPx, testLineHeightPx)
	}
}

func TestRowLineStatsRejectsMonolithicAndMissingContent(t *testing.T) {
	// 图片/QR 单体格 → 不可拆;content 缺失 → 不可拆;零格行 → 不可拆
	imageRow := imageRowGraph(100)
	if _, _, ok := rowLineStats(&imageRow); ok {
		t.Error("图片单体格行应不可拆")
	}
	qrRow := qrRowGraph(100)
	if _, _, ok := rowLineStats(&qrRow); ok {
		t.Error("二维码单体格行应不可拆")
	}

	noContent := textRowGraph([]string{"甲"}, 20, testLineHeightPx)
	(*noContent.Cells)[0].Content = nil
	if _, _, ok := rowLineStats(&noContent); ok {
		t.Error("content 缺失格行应不可拆")
	}

	noCells := textRowGraph([]string{"甲"}, 20, testLineHeightPx)
	empty := []layer.Node{}
	noCells.Cells = &empty
	if _, _, ok := rowLineStats(&noCells); ok {
		t.Error("零格行应不可拆")
	}
}

func TestSplitBoundIsZeroWhenLessThanOneLineFits(t *testing.T) {
	// K=0 整行下移:剩余不足一行高(19 < L=20)或零剩余
	row := textRowGraph([]string{"甲\n乙\n丙\n丁"}, 80, testLineHeightPx)

	if bound := splitBound(&row, 19); bound != 0 {
		t.Errorf("剩余 19 < L: bound = %d, want 0", bound)
	}
	if bound := splitBound(&row, 0); bound != 0 {
		t.Errorf("零剩余: bound = %d, want 0", bound)
	}
}

func TestSplitBoundIsZeroForSingleLineAndMonolithicRows(t *testing.T) {
	// M=1(含 autowrap 关的退化线:Lines 恒单行)→ M_max − 1 = 0;
	// 单体格行 rowLineStats 即不可拆
	single := textRowGraph([]string{"单行"}, 20, testLineHeightPx)

	degenerate := textRowGraph([]string{"甲\n乙"}, 40, testLineHeightPx)
	content := decodeCellContent(t, degenerate, 0)
	content.Spec.FontFamily.Autowrap = false
	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("重编码退化格: %v", err)
	}
	(*degenerate.Cells)[0].Content = raw

	if bound := splitBound(&single, 200); bound != 0 {
		t.Errorf("M=1: bound = %d, want 0", bound)
	}
	if bound := splitBound(&degenerate, 200); bound != 0 {
		t.Errorf("autowrap 关退化线: bound = %d, want 0", bound)
	}
	monolithic := imageRowGraph(100)
	if bound := splitBound(&monolithic, 200); bound != 0 {
		t.Errorf("单体格行: bound = %d, want 0", bound)
	}
}

func TestSplitBoundExactlyFillsRemaining(t *testing.T) {
	// K×L = remaining 恰满:remaining 40 / L 20 → K=2,2×20 = 40
	row := textRowGraph([]string{"甲\n乙\n丙\n丁"}, 80, testLineHeightPx)

	if bound := splitBound(&row, 40); bound != 2 {
		t.Errorf("bound = %d, want 2", bound)
	}
}

func TestSplitBoundCapsAtMaxLinesMinusOne(t *testing.T) {
	// 剩余再大也不把整行拆光:M_max − 1 封顶(续段至少留一行)
	row := textRowGraph([]string{"甲\n乙\n丙\n丁"}, 80, testLineHeightPx)

	if bound := splitBound(&row, 2000); bound != 3 {
		t.Errorf("bound = %d, want 3", bound)
	}
}

func TestSplitBoundGuardsNegativeDeclaredSurplus(t *testing.T) {
	// P < 0 防线(不一致 graph:行高 30 < 4 内容行 × L 20)→ 不可拆
	row := textRowGraph([]string{"甲\n乙\n丙\n丁"}, 30, testLineHeightPx)

	if bound := splitBound(&row, 200); bound != 0 {
		t.Errorf("P<0: bound = %d, want 0", bound)
	}
	if _, _, ok := splitRowGraph(&row, 1); ok {
		t.Error("P<0: splitRowGraph 应不可拆")
	}
}

func TestSplitBoundGuardsZeroLineHeight(t *testing.T) {
	// L ≤ 0 防线(非法字号 0):无从求界,不除零、视为不可拆(与 PHP 侧成对平移)
	row := textRowGraph([]string{"甲\n乙\n丙\n丁"}, 80, 0)

	if bound := splitBound(&row, 200); bound != 0 {
		t.Errorf("L=0: bound = %d, want 0", bound)
	}
	if _, _, ok := splitRowGraph(&row, 2); ok {
		t.Error("L=0: splitRowGraph 应不可拆")
	}
}

func TestSplitRowGraphSlicesHeightsValuesAndClonesKeyShape(t *testing.T) {
	source := textRowGraph([]string{"甲一\n甲二\n甲三\n甲四"}, 80, testLineHeightPx)
	front, cont, ok := splitRowGraph(&source, 2)
	if !ok {
		t.Fatal("k=2 应可拆")
	}

	// Σ 高恒等 = 原行高;片段行高 = K×L+P / 原行高 − 前段(P = 0)
	if front.Spec.Shape.Height != 40 || cont.Spec.Shape.Height != 40 {
		t.Errorf("片段行高 = %d/%d, want 40/40", front.Spec.Shape.Height, cont.Spec.Shape.Height)
	}

	// 格盒高与内容层 spec.height 同步片段行高(独立边框盒)
	for name, fragment := range map[string]layer.Node{"front": front, "cont": cont} {
		cell := (*fragment.Cells)[0]
		if cell.Spec.Shape.Height != 40 {
			t.Errorf("%s 格盒高 = %d, want 40", name, cell.Spec.Shape.Height)
		}
		content := decodeCellContent(t, fragment, 0)
		if content.Spec.Shape.Height != 40 {
			t.Errorf("%s 内容层高 = %d, want 40", name, content.Spec.Shape.Height)
		}
	}

	// 内容值按 \n 硬断行切片
	if got, want := firstCellValue(t, front), "甲一\n甲二"; got != want {
		t.Errorf("前段值 = %q, want %q", got, want)
	}
	if got, want := firstCellValue(t, cont), "甲三\n甲四"; got != want {
		t.Errorf("续段值 = %q, want %q", got, want)
	}

	// 其余字段逐字段克隆,不新增不删除键(wire 键形纪律)
	for name, fragment := range map[string]layer.Node{"front": front, "cont": cont} {
		if !slices.Equal(jsonKeys(t, source), jsonKeys(t, fragment)) {
			t.Errorf("%s 顶层键形漂移: %v vs %v", name, jsonKeys(t, source), jsonKeys(t, fragment))
		}
		if !slices.Equal(jsonKeys(t, source.Spec), jsonKeys(t, fragment.Spec)) {
			t.Errorf("%s spec 键形漂移", name)
		}
		sourceContent := decodeCellContent(t, source, 0)
		fragmentContent := decodeCellContent(t, fragment, 0)
		if !slices.Equal(jsonKeys(t, sourceContent), jsonKeys(t, fragmentContent)) {
			t.Errorf("%s content 键形漂移", name)
		}
		sourceFont, _ := json.Marshal(sourceContent.Spec.FontFamily)
		fragmentFont, _ := json.Marshal(fragmentContent.Spec.FontFamily)
		if string(sourceFont) != string(fragmentFont) {
			t.Errorf("%s fontFamily 漂移: %s vs %s", name, sourceFont, fragmentFont)
		}
	}

	// 源行 graph 不被改写(分片深拷贝纪律)
	if source.Spec.Shape.Height != 80 {
		t.Errorf("源行高被改写 = %d, want 80", source.Spec.Shape.Height)
	}
	if got, want := firstCellValue(t, source), "甲一\n甲二\n甲三\n甲四"; got != want {
		t.Errorf("源值被改写 = %q, want %q", got, want)
	}
}

func TestSplitRowGraphShortCellKeepsAllAndContinuationStaysEmpty(t *testing.T) {
	// 格独立分片:短格(M=1 < K=2)全内容留前段,续段 = 空串内容格且格盒画满续段高
	source := textRowGraph([]string{"短格", "乙一\n乙二\n乙三"}, 60, testLineHeightPx)
	front, cont, ok := splitRowGraph(&source, 2)
	if !ok {
		t.Fatal("k=2 应可拆")
	}

	if front.Spec.Shape.Height != 40 || cont.Spec.Shape.Height != 20 {
		t.Fatalf("片段行高 = %d/%d, want 40/20", front.Spec.Shape.Height, cont.Spec.Shape.Height)
	}

	if got, want := firstCellValue(t, front), "短格"; got != want {
		t.Errorf("前段短格值 = %q, want %q", got, want)
	}
	frontLong := firstCellValueAt(t, front, 1)
	if got, want := frontLong, "乙一\n乙二"; got != want {
		t.Errorf("前段长格值 = %q, want %q", got, want)
	}

	// 续段短格:空串值、格盒仍 = 续段行高 20
	for i := range *cont.Cells {
		cell := (*cont.Cells)[i]
		if cell.Spec.Shape.Height != 20 {
			t.Errorf("续段格[%d] 盒高 = %d, want 20", i, cell.Spec.Shape.Height)
		}
		content := decodeCellContent(t, cont, i)
		if content.Spec.Shape.Height != 20 {
			t.Errorf("续段格[%d] 内容层高 = %d, want 20", i, content.Spec.Shape.Height)
		}
	}
	if got := firstCellValueAt(t, cont, 0); got != "" {
		t.Errorf("续段短格值 = %q, want 空串", got)
	}
	if got, want := firstCellValueAt(t, cont, 1), "乙三"; got != want {
		t.Errorf("续段长格值 = %q, want %q", got, want)
	}
}

// firstCellValueAt 指定格内容值(多格行切片断言用)
func firstCellValueAt(t *testing.T, row layer.Node, cellIndex int) string {
	t.Helper()
	var cell layer.Node
	if err := json.Unmarshal((*row.Cells)[cellIndex].Content, &cell); err != nil {
		t.Fatalf("解码格[%d] content: %v", cellIndex, err)
	}
	if cell.Data == nil || cell.Data.Value == nil {
		return ""
	}
	return *cell.Data.Value
}

func TestSplitRowGraphGuardsReturnNotOk(t *testing.T) {
	row := textRowGraph([]string{"甲\n乙\n丙\n丁"}, 80, testLineHeightPx)

	// k < 1;续段 ≤ 0 防线(k = M_max 把整行拆光,零高续段不得产出)
	if _, _, ok := splitRowGraph(&row, 0); ok {
		t.Error("k=0 应不可拆")
	}
	if _, _, ok := splitRowGraph(&row, 4); ok {
		t.Error("k=M_max(零高续段)应不可拆")
	}
	// 非全文本格
	monolithic := imageRowGraph(100)
	if _, _, ok := splitRowGraph(&monolithic, 2); ok {
		t.Error("单体格行应不可拆")
	}
}

// 收敛结构断言(spec §0.4):片段内容行数严格递减、总片段数 ≤ Σ 行内容数——
// 装箱步进单调前进、无 0 步进(工票 01 死循环评估在拆分模式下的推广钉面;
// 多页行为钉面 = 语料 split-lines-row-across-three-pages)
func TestRepeatedSplitFragmentLineCountsStrictlyDecrease(t *testing.T) {
	segments := make([]string, 0, 24)
	for i := 1; i <= 24; i++ {
		segments = append(segments, "第"+strconv.Itoa(i)+"行内容")
	}
	row := textRowGraph([]string{strings.Join(segments, "\n")}, 480, testLineHeightPx)

	fragmentLines := []int{}
	for {
		bound := splitBound(&row, 200)
		if bound < 1 {
			break
		}
		front, cont, ok := splitRowGraph(&row, bound)
		if !ok {
			t.Fatalf("bound=%d 应可拆", bound)
		}
		fragmentLines = append(fragmentLines, valueLineCount(t, front))
		row = cont
	}

	// [10, 10, 3] 严格递减,终段余 1 行不再可拆
	if want := []int{10, 10, 3}; !slices.Equal(fragmentLines, want) {
		t.Errorf("片段行数 = %v, want %v", fragmentLines, want)
	}
	if remaining := valueLineCount(t, row); remaining != 1 {
		t.Errorf("终段行数 = %d, want 1", remaining)
	}
	if total := len(fragmentLines) + 1; total > 24 {
		t.Errorf("总片段数 %d > Σ行内容数 24", total)
	}
}

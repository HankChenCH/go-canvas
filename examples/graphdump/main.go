// 样例:构造带完整盒模型的画布(图片/文本/二维码/表格容器),输出 graph JSON。
// 键结构与 php-canvas-next Canvas::graph() 产物逐字段一致(工单 01/03 验收项);
// 数值字面有一处已知差异:Go 输出 0/1,PHP json_encode 输出 0.0/1.0(float 字面格式,
// JSON 语义相同),键名、键序、结构与嵌套关系完全一致。
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
)

func main() {
	// 表格:行 1 两个单元格(一个带文本内容层、一个空),行 2 单单元格
	row1 := layer.NewTableRowLayer(layer.WithSize(1080, 0), layer.WithAutoHeight())
	cell1 := layer.NewTableCellLayer(layer.WithSize(540, 100), layer.WithBackground("#f5f5f5"))
	cell1.AddContentLayer(layer.NewTextLayer(
		layer.WithSize(540, 0),
		layer.WithAutoHeight(),
		layer.WithPadding(12),
		layer.WithText("左侧单元格"),
		layer.WithFont("", 28, "#333333"),
	))
	row1.AddCell(cell1)
	row1.AddCell(layer.NewTableCellLayer(layer.WithSize(540, 100), layer.WithBackground("#eeeeee")))

	row2 := layer.NewTableRowLayer(layer.WithSize(1080, 0), layer.WithAutoHeight())
	cell2 := layer.NewTableCellLayer(layer.WithSize(1080, 150), layer.WithBackground("#f5f5f5"))
	cell2.AddContentLayer(layer.NewTextLayer(
		layer.WithSize(1080, 0),
		layer.WithAutoHeight(),
		layer.WithText("通栏单元格"),
		layer.WithFont("", 28, "#333333"),
	))
	row2.AddCell(cell2)

	c := canvas.New(1080, 1920,
		layer.NewImageLayer(
			layer.WithSize(1080, 608),
			layer.WithImage("https://example.com/bg.png"),
			layer.WithPriority(1),
		),
		layer.NewImageLayer(
			layer.WithSize(1000, 200),
			layer.WithAutoHeight(),
			layer.WithLineHeight(1.5),
			layer.WithPaddingVH(24, 40),
			layer.WithBorderTop(2, "#333333"),
			layer.WithBackground("#ffffff"),
			layer.WithPosition(40, 1600, layer.AnchorBottomLeft),
			layer.WithPriority(3),
		),
		layer.NewTextLayer(
			layer.WithSize(1080, 120),
			layer.WithText("标题文本"),
			layer.WithFont("", 48, "#333333"),
			layer.WithPriority(5),
		),
		layer.NewQrCodeLayer(
			layer.WithSize(160, 160),
			layer.WithQrText("https://example.com"),
			layer.WithPriority(2),
		),
		layer.NewTableLayer(
			layer.WithSize(1080, 300),
			layer.WithBackground("#ffffff"),
			layer.WithRows(row1, row2),
			layer.WithPriority(0),
		),
	)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(c.Graph()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// 样例:构造带完整盒模型的图片图层画布,输出 graph JSON。
// 键结构与 php-canvas-next Canvas::graph() 产物逐字段一致(工单 01 验收项);
// 数值字面有一处已知差异:Go 输出 0/1,PHP json_encode 输出 0.0/1.0(float 字面格式,
// JSON 语义相同),键名、键序、结构与嵌套关系完全一致。
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/layer"
)

func main() {
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
	)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(c.Graph()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

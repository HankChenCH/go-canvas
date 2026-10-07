// 目验脚本:一条命令渲染综合样图(中文禁则段落、表格嵌套单元格、二维码、
// 色块与边框、priority 叠加、宽自适应文本盒)供人工目验,对应
// php-canvas-image-renderer 的 scripts/visual-check.php——像素取样/布局快照
// 之外的人眼验收面。
//
// priority 语义与 PHP 一致:priority 越大越先渲染(视觉上越垫底)。
//
// 用法: go run ./cmd/visualcheck [输出路径]
// 缺省输出运行目录下的 visual-check.png(make visual-check 时即 image-renderer/,
// 已 gitignore)。用法与目验要点见 docs/visual-check.md。
package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/hydrate"
	"github.com/HankChenCH/go-canvas/image-renderer"
	"github.com/HankChenCH/go-canvas/image-renderer/typography"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/renderer"
)

// fontCandidates 字体候选:优先带 CJK 字形的字体,避免中文变豆腐块;
// 回退顺序与 PHP 版脚本逐项对齐。择取标准为「存在且可加载」——PHP(GD)仅查
// 存在即可,Go 侧 sfnt 解析不了部分候选(如实测 STHeiti Medium.ttc 的 cmap),
// 只查存在会把解析失败留到渲染中段爆出难定位的错误
var fontCandidates = []string{
	"/System/Library/Fonts/STHeiti Medium.ttc",
	"/System/Library/Fonts/Supplemental/Songti.ttc",
	"/System/Library/Fonts/Hiragino Sans GB.ttc",
	"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	"/System/Library/Fonts/Supplemental/Arial.ttf",
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "visual-check:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	font, err := pickFont(fontCandidates)
	if err != nil {
		return err
	}

	output := "visual-check.png"
	if len(args) > 0 {
		output = args[0]
	}

	c, dataset, err := buildSample(font)
	if err != nil {
		return err
	}

	// 标准管线(spec §0):渲染前独立填充步骤 + 渲染期惰性物化
	hydrated, err := hydrate.NewHydrator(nil).Hydrate(c, dataset)
	if err != nil {
		return fmt.Errorf("填充样图: %w", err)
	}

	img, err := renderer.RenderAs[*image.NRGBA](ctx, imagerenderer.NewRenderer(nil), hydrated)
	if err != nil {
		return fmt.Errorf("渲染样图: %w", err)
	}
	if err := imagerenderer.SavePNG(output, img); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "已输出: %s (%dx%d)\n", output, img.Bounds().Dx(), img.Bounds().Dy())
	return nil
}

// pickFont 取首个「存在且可加载」的字体候选;加载探针复用渲染端同一入口
// (typography.LoadFontFace),保证探测口径与实际渲染一致。全缺时报错逐条列明
// 落选原因并给出明确提示(内置点阵默认字体仅覆盖 ASCII,缺真实字体中文必然
// 渲染为方块,故不静默降级)
func pickFont(candidates []string) (string, error) {
	var tried []string
	for _, c := range candidates {
		if _, err := os.Stat(c); err != nil {
			tried = append(tried, "  - "+c+"(不存在)")
			continue
		}
		face, err := typography.LoadFontFace(c, 12)
		if err != nil {
			tried = append(tried, "  - "+c+"(不可加载: "+err.Error()+")")
			continue
		}
		_ = face // 探针 face 即用即弃,渲染端按(字体, 字号)自持缓存
		return c, nil
	}
	return "", fmt.Errorf(
		"未找到可用字体(候选按 CJK 字形优先排序,缺真实字体时中文会渲染为方块)。已依次检查:\n%s\n请安装其中一款字体,或按本机字体位置扩充 fontCandidates",
		strings.Join(tried, "\n"),
	)
}

// buildSample 构造综合样图:内容复刻 PHP 版 visual-check(版式几何逐项一致,
// 品牌字样与二维码内容按 go-canvas 改写,见 docs/visual-check.md 双端对照节)。
// V2 段 = 声明态模板表(工票 12),经标准管线 Render(hydrate(canvas, dataset)) 出图
func buildSample(font string) (*canvas.Canvas, map[string]any, error) {
	// 白色底(最垫底),避免透明区域
	bg := layer.NewImageLayer(
		layer.WithSize(400, 400), layer.WithBackground("#ffffff"), layer.WithPriority(11),
	)

	// 头图色块
	header := layer.NewImageLayer(
		layer.WithSize(400, 90), layer.WithBackground("#2d6cdf"),
		layer.WithPosition(0, 0), layer.WithPriority(10),
	)

	// 标题(画在色块上)
	title := layer.NewTextLayer(
		layer.WithSize(400, 90),
		layer.WithText("go-canvas 目验样图"),
		layer.WithFont(font, 24, "#ffffff"),
		layer.WithVerticalAlign(layer.AlignCenter), layer.WithHorizontalAlign(layer.AlignCenter),
		layer.WithPosition(0, 0), layer.WithPriority(5),
	)

	// 长中文段落:autowrap + 禁则效果(。不能出现在行首)
	paragraph := layer.NewTextLayer(
		layer.WithSize(360, 0), layer.WithAutoHeight(), layer.WithBackground("#f5f7fa"),
		layer.WithText("图层树与渲染器分离之后，同一份结构树可以交给不同后端渲染；中文断行内置禁则处理，行首不会出现句号、逗号等收尾标点。英文单词 hello world 优先在词边界断行。"),
		layer.WithFont(font, 14, "#333333"),
		layer.WithPadding(10), layer.WithAutowrap(true),
		layer.WithPosition(20, 110), layer.WithPriority(4),
	)

	// 表格:三行两列(宽 250,右侧留给二维码)
	table := buildTable(font)

	// 二维码(表格右侧)
	qrCode := layer.NewQrCodeLayer(
		layer.WithSize(90, 90), layer.WithQrText("https://github.com/HankChenCH/go-canvas"),
		layer.WithPosition(280, 250), layer.WithPriority(4),
	)

	// 图片图层:本地生成一张双色 PNG(#e8f0e8 底 + #6dc287 色块)
	stripPath := filepath.Join(os.TempDir(), "go-canvas-visual-strip.png")
	if err := writeStripPNG(stripPath); err != nil {
		return nil, nil, fmt.Errorf("生成条带图: %w", err)
	}
	strip := layer.NewImageLayer(
		layer.WithSize(360, 40), layer.WithBackground("#ffffff"), layer.WithImage(stripPath),
		layer.WithPosition(20, 340), layer.WithPriority(4),
	)

	footer := layer.NewTextLayer(
		layer.WithSize(400, 30),
		layer.WithText("HankChen/go-canvas"),
		layer.WithFont(font, 11, "#888888"),
		layer.WithHorizontalAlign(layer.AlignCenter), layer.WithVerticalAlign(layer.AlignCenter),
		layer.WithPosition(0, 370), layer.WithPriority(4),
	)

	// 宽自适应文本层(ADR 0014,对齐 PHP 版样图):盒宽 = 未断行自然宽 + 横向 padding,
	// 背景/边框随自然宽生效——盒宽贴合内容即目验通过(缺省启发式度量)
	autoWidthSingle := layer.NewTextLayer(
		layer.WithAutoWidth(), layer.WithAutoHeight(), layer.WithBackground("#fff7e6"),
		layer.WithText("自动宽度贴合内容"),
		layer.WithFont(font, 16, "#333333"),
		layer.WithPadding(8), layer.WithBorder(1, "#e6a23c"),
		layer.WithPosition(20, 465), layer.WithPriority(4),
	)

	// autoWidth + autowrap 组合退化为不折行:显式换行拆段取最大段宽
	autoWidthMultiline := layer.NewTextLayer(
		layer.WithAutoWidth(), layer.WithAutoHeight(), layer.WithBackground("#f0f9eb"),
		layer.WithText("第一段\n显式换行后的更长一段"),
		layer.WithFont(font, 14, "#333333"),
		layer.WithPadding(8), layer.WithBorder(1, "#67c23a"), layer.WithAutowrap(true),
		layer.WithPosition(220, 465), layer.WithPriority(4),
	)

	// V2 模板表(声明态,渲染前经 hydrate(dataset) 实例化)
	templateTable := buildTemplateTable(font)

	bgV2 := layer.NewImageLayer(
		layer.WithSize(400, 120), layer.WithBackground("#ffffff"),
		layer.WithPosition(0, 400), layer.WithPriority(11),
	)

	return canvas.New(400, 520, bg, bgV2, header, title, paragraph, table, qrCode, strip, footer,
		templateTable, autoWidthSingle, autoWidthMultiline), map[string]any{
		"items": []any{map[string]any{"name": "V2 模板行", "desc": "template × dataset → hydrate → render"}},
	}, nil
}

// buildTemplateTable V2 模板表样例:单行循环体声明(格表达式 {{row.*}}),
// 渲染前经 hydrate 实例化——目验面 = 行上下文求值与 V1 高度耦合重放
func buildTemplateTable(font string) *layer.TableLayer {
	nameContent := layer.NewTextLayer(
		layer.WithSize(120, 0), layer.WithAutoHeight(),
		layer.WithFont(font, 12, "#222222"), layer.WithPadding(6),
	)
	nameContent.SetExpression("{{row.name}}")
	nameCell := layer.NewTableCellLayer(
		layer.WithSize(120, 0), layer.WithAutoHeight(),
		layer.WithBackground("#eef3fd"), layer.WithBorder(1, "#dddddd"),
	)
	nameCell.AddTemplateContentLayer(nameContent)

	descContent := layer.NewTextLayer(
		layer.WithSize(240, 0), layer.WithAutoHeight(),
		layer.WithFont(font, 12, "#222222"), layer.WithPadding(6),
	)
	descContent.SetExpression("{{row.desc}}")
	descCell := layer.NewTableCellLayer(
		layer.WithSize(240, 40),
		layer.WithBackground("#ffffff"), layer.WithBorder(1, "#dddddd"),
	)
	descCell.AddTemplateContentLayer(descContent)

	template := layer.NewTableRowTemplate(layer.WithAutoHeight())
	template.AddCell(nameCell)
	template.AddCell(descCell)

	table := layer.NewTableLayer(
		layer.WithSize(360, 50),
		layer.WithBackground("#ffffff"), layer.WithPosition(20, 405), layer.WithPriority(4),
		layer.WithBorder(1, "#dddddd"),
	)
	table.SetRowsPath("items")
	table.SetTemplate(template)
	return table
}

// buildTable 表格:三行两列。行高 auto 取最高单元格、表高 = 行高累计——
// PHP 版先 addRow 再 setHeight(行高和),Go 构造面等价表达为
// "行先建好算出累计高,再连同 WithRows 一次性构造"(add 副作用同一条路径)
func buildTable(font string) *layer.TableLayer {
	specs := []struct {
		field, desc string
	}{
		{"字段", "说明"},
		{"Canvas", "纯结构容器"},
		{"Renderer", "可插拔渲染后端"},
	}

	var rows []*layer.TableRowLayer
	for i, spec := range specs {
		bg := "#ffffff"
		if i == 0 {
			bg = "#eef3fd" // 表头行底色
		}
		row := layer.NewTableRowLayer(layer.WithSize(250, 0), layer.WithAutoHeight())
		for _, cell := range []struct {
			text  string
			width int
		}{{spec.field, 80}, {spec.desc, 170}} {
			content := layer.NewTextLayer(
				layer.WithSize(cell.width, 0), layer.WithAutoHeight(),
				layer.WithText(cell.text), layer.WithFont(font, 12, "#222222"),
				layer.WithPadding(6),
			)
			cellLayer := layer.NewTableCellLayer(
				layer.WithSize(cell.width, 0), layer.WithAutoHeight(),
				layer.WithBackground(bg), layer.WithBorder(1, "#dddddd"),
			)
			cellLayer.AddContentLayer(content)
			row.AddCell(cellLayer)
		}
		rows = append(rows, row)
	}

	tableHeight := 0
	for _, r := range rows {
		tableHeight += r.Height()
	}
	return layer.NewTableLayer(
		layer.WithSize(250, tableHeight),
		layer.WithBackground("#ffffff"),
		layer.WithPosition(20, 250), layer.WithPriority(4),
		layer.WithBorder(1, "#dddddd"),
		layer.WithRows(rows...),
	)
}

// 条带图两色(对应 PHP 版 intervention 画布:#e8f0e8 填充 + #6dc287 矩形)
var (
	stripBg    = color.NRGBA{R: 0xe8, G: 0xf0, B: 0xe8, A: 0xff}
	stripBlock = color.NRGBA{R: 0x6d, G: 0xc2, B: 0x87, A: 0xff}
)

// writeStripPNG 本地生成 360×40 条带图:全幅底色 + (10,10) 起 120×20 色块
func writeStripPNG(path string) error {
	img := image.NewNRGBA(image.Rect(0, 0, 360, 40))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: stripBg}, image.Point{}, draw.Over)
	draw.Draw(img, image.Rect(10, 10, 130, 30), &image.Uniform{C: stripBlock}, image.Point{}, draw.Over)
	return imagerenderer.SavePNG(path, img)
}

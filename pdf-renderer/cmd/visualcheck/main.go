// 目验脚本:一条命令渲染综合样张(画布背景、矩形背景+四边异色边框、PNG 直嵌、
// 二维码、CJK/ASCII 文本对齐矩阵、旋转文本)出单页 .pdf,对应位图后端
// image-renderer/cmd/visualcheck 的 PDF 姊妹件——像素取样/结构断言之外的人眼
// 验收面。poppler 在场时顺带 pdftoppm 栅格化预览 png(缺工具则跳过预览,
// 只出 .pdf)。
//
// priority 语义与 PHP 一致:priority 越大越先渲染(视觉上越垫底)。
//
// 用法: go run ./cmd/visualcheck [输出基名]
// 缺省输出运行目录下的 visual-check.pdf 与 visual-check.png(make visual-check
// 时即 pdf-renderer/,已 gitignore)。CJK 字体探针候选见 fontCandidates,
// 可用 GO_CANVAS_VISUAL_FONT 覆写;无 CJK 单体 TTF 时降级 Helvetica 只排
// ASCII(样张保留可渲,中文位置以占位框标注)。
package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/hydrate"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/renderer"

	pdfrenderer "github.com/HankChenCH/go-canvas/pdf-renderer"
)

// fontCandidates CJK 字体候选:只认单体 TTF(fpdf 子集内嵌不支持 TTC 字体集);
// 探针标准为「存在且非 ttcf 魔数」——与 pdf-renderer 测试探针同一口径
var fontCandidates = []string{
	"/usr/share/fonts/truetype/fonts-ipafont-gothic/ipag.ttf",
	"/usr/share/fonts/opentype/ipafont-gothic/ipag.ttf",
	"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "visual-check:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	base := "visual-check"
	if len(args) > 0 {
		base = args[0]
	}
	font := pickFont()

	c, err := buildSample(font)
	if err != nil {
		return err
	}

	// 标准管线(spec §0):渲染前独立填充 + 渲染期惰性物化;NewRenderer
	// 默认接线(二维码缝在位),缓存根指向系统默认
	hydrated, err := hydrate.NewHydrator(nil).Hydrate(c, nil)
	if err != nil {
		return fmt.Errorf("填充样张: %w", err)
	}
	pdf, err := renderer.RenderAs[[]byte](ctx, pdfrenderer.NewRenderer(nil), hydrated)
	if err != nil {
		return fmt.Errorf("渲染样张: %w", err)
	}

	pdfPath := base + ".pdf"
	if err := os.WriteFile(pdfPath, pdf, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "已输出: %s (%d 字节)\n", pdfPath, len(pdf))

	// pdftoppm 预览:-r 96 恰好 1:1 还原像素网格
	pngPath := base + ".png"
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		fmt.Fprintf(stdout, "未找到 pdftoppm,跳过预览(安装 poppler 后重跑可得 %s)\n", pngPath)
		return nil
	}
	cmd := exec.Command("pdftoppm", "-r", "96", "-png", "-singlefile", pdfPath, strings.TrimSuffix(pngPath, ".png"))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pdftoppm 预览: %v\n%s", err, out)
	}
	fmt.Fprintf(stdout, "已输出: %s(pdftoppm -r 96 预览)\n", pngPath)
	return nil
}

// pickFont 取首个存在的候选(非 ttcf);全缺返回空串(内置兜底 = Helvetica,
// ASCII 可渲、CJK 缺字形——样张降级不中断)
func pickFont() string {
	if v := os.Getenv("GO_CANVAS_VISUAL_FONT"); v != "" {
		return v
	}
	for _, c := range fontCandidates {
		data, err := os.ReadFile(c)
		if err == nil && !strings.HasPrefix(string(data[:min(4, len(data))]), "ttcf") {
			return c
		}
	}
	return ""
}

// buildSample 构造综合样张:色块边框、PNG 直嵌、二维码、CJK/ASCII 文本、
// 旋转文本、表格行——覆盖五原语的主要分派分支
func buildSample(font string) (*canvas.Canvas, error) {
	// 头图色块 + 白字标题
	header := layer.NewImageLayer(
		layer.WithSize(400, 90), layer.WithBackground("#2d6cdf"),
		layer.WithPosition(0, 0), layer.WithPriority(10),
	)
	layers := []layer.Layer{header}

	title := layer.NewTextLayer(
		layer.WithSize(400, 90),
		layer.WithText("go-canvas PDF 目验样张"),
		layer.WithFont(font, 24, "#ffffff"),
		layer.WithHorizontalAlign(layer.AlignCenter), layer.WithVerticalAlign(layer.AlignCenter),
		layer.WithPosition(0, 0), layer.WithPriority(5),
	)
	layers = append(layers, title)

	// 本地生成双色 PNG 条带(左红右蓝)直嵌 + 一段白底红框
	stripPath := filepath.Join(os.TempDir(), "go-canvas-pdf-visual-strip.png")
	if err := writeStripPNG(stripPath); err != nil {
		return nil, fmt.Errorf("生成条带图: %w", err)
	}
	strip := layer.NewImageLayer(
		layer.WithSize(200, 50), layer.WithImage(stripPath), layer.WithBackground("#ffffff"),
		layer.WithPosition(20, 110), layer.WithPriority(4),
	)
	layers = append(layers, strip)

	// 边框盒:黄底 + 四边异色边框(盒内嵌色带目验)
	box := layer.NewImageLayer(
		layer.WithSize(120, 50), layer.WithBackground("#fff7e6"),
		layer.WithBorderTop(3, "#409eff"), layer.WithBorderBottom(3, "#67c23a"),
		layer.WithBorderLeft(3, "#e6a23c"), layer.WithBorderRight(3, "#f56c6c"),
		layer.WithPosition(240, 110), layer.WithPriority(4),
	)
	layers = append(layers, box)

	// 二维码
	qr := layer.NewQrCodeLayer(
		layer.WithSize(80, 80), layer.WithQrText("https://github.com/HankChenCH/go-canvas"),
		layer.WithPosition(300, 180), layer.WithPriority(4),
	)
	layers = append(layers, qr)

	// 文本对齐矩阵:CJK 左上 / ASCII 右下(有 CJK 字体时),
	// 逆时针 90° 旋转文本一条
	if font != "" {
		cjk := layer.NewTextLayer(
			layer.WithSize(260, 30),
			layer.WithText("证书批量生成:图层树 → PDF 直嵌"),
			layer.WithFont(font, 14, "#303133"),
			layer.WithHorizontalAlign(layer.AlignLeft), layer.WithVerticalAlign(layer.AlignTop),
			layer.WithPosition(20, 180), layer.WithPriority(4),
		)
		layers = append(layers, cjk)
	} else {
		fmt.Fprintln(os.Stderr, "visual-check: 未找到 CJK 单体 TTF,中文降级 Helvetica(缺字形)"+
			";可设 GO_CANVAS_VISUAL_FONT 指向单体 .ttf")
	}
	ascii := layer.NewTextLayer(
		layer.WithSize(260, 30),
		layer.WithText("px -> pt (96dpi): page 400px = 300pt"),
		layer.WithFont(font, 12, "#606266"),
		layer.WithHorizontalAlign(layer.AlignLeft), layer.WithVerticalAlign(layer.AlignTop),
		layer.WithPosition(20, 215), layer.WithPriority(4),
	)
	layers = append(layers, ascii)

	rotated := layer.NewTextLayer(
		layer.WithSize(20, 120),
		layer.WithText("rotated 90 CCW"),
		layer.WithFont(font, 12, "#909399"),
		layer.WithAngle(90),
		layer.WithHorizontalAlign(layer.AlignLeft), layer.WithVerticalAlign(layer.AlignTop),
		layer.WithPosition(385, 300), layer.WithPriority(4),
	)
	layers = append(layers, rotated)

	// 表格一行两列(行高 auto)
	cell := layer.NewTableCellLayer(
		layer.WithSize(180, 0), layer.WithAutoHeight(),
		layer.WithBackground("#eef3fd"), layer.WithBorder(1, "#dddddd"),
	)
	cell.AddContentLayer(layer.NewTextLayer(
		layer.WithSize(168, 0), layer.WithAutoHeight(),
		layer.WithText("表格行:单元格下钻"),
		layer.WithFont(font, 12, "#222222"), layer.WithPadding(6),
	))
	row := layer.NewTableRowLayer(layer.WithSize(360, 0), layer.WithAutoHeight())
	row.AddCell(cell)
	table := layer.NewTableLayer(
		layer.WithSize(360, 40), layer.WithBackground("#ffffff"),
		layer.WithPosition(20, 260), layer.WithPriority(4),
		layer.WithBorder(1, "#dddddd"), layer.WithRows(row),
	)
	layers = append(layers, table)

	// 画布背景白(ADR 0022:先色后图层)
	c := canvas.New(400, 310, layers...)
	white := "#ffffff"
	c.SetBackground(canvas.NewCanvasBackground(&white, nil))
	return c, nil
}

// writeStripPNG 生成 200×50 双色条带(左红右蓝)
func writeStripPNG(path string) error {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 50))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.NRGBA{R: 0xff, A: 0xff}}, image.Point{}, draw.Over)
	draw.Draw(img, image.Rect(100, 0, 200, 50), &image.Uniform{C: color.NRGBA{B: 0xff, A: 0xff}}, image.Point{}, draw.Over)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

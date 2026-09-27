package imagerenderer

// TableLayer V2 全链路像素断言(工票 12):声明态模板表 → hydrate(dataset) →
// 位图渲染,验收 = 用户可见像素——行实例化、行高定稿(V1 耦合重放)、表达式
// 求值、本地资源经惰性物化直画、空数据空壳。对拍基准 = php-canvas-image-renderer
// 的 V2 全链路像素断言;标准管线 = Render(hydrate(canvas, dataset))(spec §0)。

import (
	"context"
	"testing"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/hydrate"
	"github.com/hankchen/go-canvas/layer"
)

// v2TemplateTable 模板表:红格(auto 文本格,行上下文求值)+ 蓝格(固定 40,
// 图片格,表达式指向本地 PNG)。填充后行高 = max(文本 12, 格 40) = 40
func v2TemplateTable(t *testing.T, avatarPath string) *layer.TableLayer {
	t.Helper()
	textContent := layer.NewTextLayer(layer.WithSize(160, 0), layer.WithAutoHeight(), layer.WithFont("", 12, "#000"))
	textContent.SetExpression("订单 {{row.name}}")
	textCell := layer.NewTableCellLayer(layer.WithSize(160, 0), layer.WithAutoHeight(), layer.WithBackground("#ff0000"))
	textCell.AddTemplateContentLayer(textContent)

	imageContent := layer.NewImageLayer(layer.WithSize(160, 0), layer.WithAutoHeight())
	imageContent.SetExpression("{{row.avatar}}")
	imageCell := layer.NewTableCellLayer(layer.WithSize(160, 40), layer.WithBackground("#0000ff"))
	imageCell.AddTemplateContentLayer(imageContent)

	row := layer.NewTableRowTemplate(layer.WithAutoHeight())
	row.AddCell(textCell)
	row.AddCell(imageCell)

	table := layer.NewTableLayer(layer.WithSize(320, 200), layer.WithBackground("#ffffff"))
	table.SetRowsPath("order.items")
	table.SetTemplate(row)
	return table
}

func TestTemplateTableFullPipelineRendersRows(t *testing.T) {
	// 全链路:模板表声明态 → 填充两行 → 位图。行 1 y 0..40、行 2 y 40..80,
	// 红格(背景)与绿图(本地路径经惰性物化直画)按实例化位置落位
	avatarPath := solidPng(t, 160, 40, green)
	source := canvas.New(320, 200, v2TemplateTable(t, avatarPath))
	dataset := map[string]any{
		"orderNo": "A2026-001",
		"order": map[string]any{
			"items": []any{
				map[string]any{"name": "张三", "avatar": avatarPath},
				map[string]any{"name": "李四", "avatar": avatarPath},
			},
		},
	}

	hydrated, err := hydrate.NewHydrator(nil).Hydrate(source, dataset)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	product, err := newRenderer(t).Render(context.Background(), hydrated)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := productAsImage(t, product)

	// 行实例化与行高定稿:行 1/行 2 的红格、绿图各就各位
	assertPixel(t, img, 10, 5, red)     // 行 1 红格(auto 文本格,高 12)
	assertPixel(t, img, 10, 45, red)    // 行 2 红格(y 累加行高 40)
	assertPixel(t, img, 240, 20, green) // 行 1 图片(表达式求值 → 本地路径直画)
	assertPixel(t, img, 240, 60, green) // 行 2 图片
	// 行高定稿 = max(文本格 12, 固定格 40) = 40:行 2 红格只占 y 40..52
	assertPixel(t, img, 10, 55, white) // 行 2 红格以下(y > 52)为表白底
	// 行区外(两行共 80 高)为表白底
	assertPixel(t, img, 10, 90, white)
}

func TestTemplateTableEmptyDatasetRendersShell(t *testing.T) {
	// 空数组 = 合法零行:空壳渲染,表壳按声明画(bg 白),行区零高
	avatarPath := solidPng(t, 160, 40, green)
	source := canvas.New(320, 200, v2TemplateTable(t, avatarPath))
	dataset := map[string]any{"order": map[string]any{"items": []any{}}}

	hydrated, err := hydrate.NewHydrator(nil).Hydrate(source, dataset)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	product, err := newRenderer(t).Render(context.Background(), hydrated)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := productAsImage(t, product)

	assertPixel(t, img, 10, 5, white)   // 行区(零高)无格背景
	assertPixel(t, img, 240, 20, white) // 无图片
	assertPixel(t, img, 160, 100, white)
}

func TestTemplateTableNullDatasetRendersDeclarationShell(t *testing.T) {
	// dataset null = 未绑数据集恒等直通:模板表以声明态进入渲染,零行空壳
	avatarPath := solidPng(t, 160, 40, green)
	source := canvas.New(320, 200, v2TemplateTable(t, avatarPath))

	hydrated, err := hydrate.NewHydrator(nil).Hydrate(source, nil)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if hydrated != source {
		t.Fatal("dataset null 必须恒等直通")
	}
	product, err := newRenderer(t).Render(context.Background(), hydrated)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img := productAsImage(t, product)

	assertPixel(t, img, 10, 5, white)
	assertPixel(t, img, 240, 20, white)
}

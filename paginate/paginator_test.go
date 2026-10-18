package paginate_test

// 分页器单测(共享 fixture 之外的规定用例):画布背景随页透传(ADR 0022 易漏点 2
// ——split 手工构造页 canvas 键,背景必须显式携带;hydrate 消费前置由 fixture 面
// 覆盖,此处行高直接定稿)。

import (
	"encoding/json"
	"testing"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
	"github.com/HankChenCH/go-canvas/paginate"
)

func strp(s string) *string { return &s }

// driverTable 驱动表:两行各 60 高、无模板态(V1 具体行),页高 100 必切两页
func driverTable() *layer.TableLayer {
	rowA := layer.NewTableRowLayer(layer.WithSize(200, 60))
	rowB := layer.NewTableRowLayer(layer.WithSize(200, 60))
	return layer.NewTableLayer(layer.WithSize(200, 120), layer.WithRows(rowA, rowB))
}

func pageHeightPtr(h int) *int { return &h }

func TestSplitCarriesBackgroundOnEveryPage(t *testing.T) {
	c := canvas.New(200, 120, driverTable())
	c.SetBackground(canvas.NewCanvasBackground(strp("#ffffff"), strp("bg.png")))

	pages, err := paginate.NewPaginator(paginate.PaginateOptions{PageHeight: pageHeightPtr(100)}).Paginate(c)
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("页数 = %d, want 2", len(pages))
	}

	for i, page := range pages {
		raw, err := json.Marshal(page.Graph().Canvas)
		if err != nil {
			t.Fatalf("序列化页 %d canvas 段: %v", i, err)
		}
		want := `{"width":200,"height":100,"background":{"color":"#ffffff","image":"bg.png"}}`
		if string(raw) != want {
			t.Errorf("页 %d canvas 段不符:\n got  %s\n want %s", i, raw, want)
		}
	}
}

func TestSplitWithoutBackgroundKeepsLegacyCanvasSection(t *testing.T) {
	// 无背景画布:页 canvas 段与现状逐字节一致(background 整键不落)
	c := canvas.New(200, 120, driverTable())

	pages, err := paginate.NewPaginator(paginate.PaginateOptions{PageHeight: pageHeightPtr(100)}).Paginate(c)
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	for i, page := range pages {
		raw, err := json.Marshal(page.Graph().Canvas)
		if err != nil {
			t.Fatalf("序列化页 %d canvas 段: %v", i, err)
		}
		want := `{"width":200,"height":100}`
		if string(raw) != want {
			t.Errorf("页 %d canvas 段不符:\n got  %s\n want %s", i, raw, want)
		}
	}
}

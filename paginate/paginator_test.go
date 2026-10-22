package paginate_test

// 分页器单测(共享 fixture 之外的规定用例):画布背景随页透传(ADR 0022 易漏点 2
// ——split 手工构造页 canvas 键,背景必须显式携带;hydrate 消费前置由 fixture 面
// 覆盖,此处行高直接定稿)。

import (
	"encoding/json"
	"errors"
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

func TestSplitOptionInvalidRejectedAtPaginate(t *testing.T) {
	// 入口枚举守卫(提案 003,paginate_option_invalid):语料只钉字符串 "rows" 一例;
	// 本文件补锁空串/大小写/尾随空格(枚举闭集 v1 仅小写 "lines"),且守卫与分页
	// 开关正交——分页关同样报(Go 裁决在 Paginate 入口,PHP 构造期抛出的同构收敛)
	c := canvas.New(200, 120, driverTable())
	for _, bad := range []string{"rows", "", "Lines", "lines "} {
		_, err := paginate.NewPaginator(paginate.PaginateOptions{PageHeight: pageHeightPtr(250), Split: strp(bad)}).Paginate(c)
		if !errors.Is(err, paginate.ErrPaginateOptionInvalid) {
			t.Errorf("split %q: err = %v, want paginate_option_invalid", bad, err)
		}

		_, err = paginate.NewPaginator(paginate.PaginateOptions{Split: strp(bad)}).Paginate(c)
		if !errors.Is(err, paginate.ErrPaginateOptionInvalid) {
			t.Errorf("分页关 split %q: err = %v, want paginate_option_invalid(守卫正交)", bad, err)
		}
	}

	// 合法值:nil(缺席 = 原子模式)与 "lines" 均通过入口
	if _, err := paginate.NewPaginator(paginate.PaginateOptions{Split: strp("lines")}).Paginate(c); err != nil {
		t.Errorf("split=lines 合法值不得报错: %v", err)
	}
}

func TestSplitOptionIgnoredWhenPaginationOff(t *testing.T) {
	// 分页关时 split 键不改变行为(直通 + 溢出判定,与原子模式同果):
	// 语料 split-lines-key-ineffective-when-pagination-off 的对象级同构补锁
	c := canvas.New(200, 120, driverTable())

	pages, err := paginate.NewPaginator(paginate.PaginateOptions{Split: strp("lines")}).Paginate(c)
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if len(pages) != 1 || pages[0] != c {
		t.Fatalf("分页关应单页直通且与入参同实例, 得 %d 页", len(pages))
	}
}

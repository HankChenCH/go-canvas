// Package paginate 分页与文档编译(spec §4/§10):编译期纯结构管线,
// 消费 hydrate 后置不变量,无 I/O、无求值、零第三方依赖。
//
// 两个管线入口(PHP 权威端逐镜像):
//   - Paginator.Paginate:单画布分页 Canvas 1→n(装箱/溢出/几何语义)
//   - DocumentCompiler.Compile:N 帧 × 1 dataset × 流链声明的文档编译(ADR 0009)
//
// 语义由 submodule 语料仓钉死(canvas-semantics/corpus/,runner = semantics_test.go,
// 并入 make test):paginate-semantics v2.1 + flow-semantics v1.1,语料 diff 即语义变更。
package paginate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
)

// paginate 段稳定 code(spec §4.2.6,三端一致性锚点;消息可改、code 不可改)。
// 报错位:装箱溢出 / 驱动表目标在 Paginate 入口与 split,页几何在 Paginate 入口
// fail-fast(Go 构造器无错误返回口,PHP 构造期抛出的同构收敛)
var (
	ErrContentOverflow       = errors.New("content_overflow")
	ErrPaginateTargetInvalid = errors.New("paginate_target_invalid")
	ErrPageGeometryInvalid   = errors.New("page_geometry_invalid")
	ErrPaginateOptionInvalid = errors.New("paginate_option_invalid")
)

// splitModeLines 拆分模式枚举闭集 v1(提案 003):仅 "lines" = 按内容行逐行拆分
const splitModeLines = "lines"

// PaginateOptions 分页选项(wire 面 {pageHeight, truncate?, split?},PHP
// CanvasPaginator::fromOptions 承载形态):页宽恒 = 源画布宽,页高是唯一几何参数。
// PageHeight nil = 分页关(单页直通 + 溢出判定,页底 = 源画布高);非 nil ≤ 0 =
// page_geometry_invalid(裁决在 Paginate 入口)。0 与缺席必须可区分,故用指针。
// Split nil = 缺席 = 原子模式(行为零变化);指针区分缺席,枚举闭集 v1 仅 "lines",
// 非法值 paginate_option_invalid(裁决在 Paginate 入口,与分页开关正交恒执行)。
type PaginateOptions struct {
	PageHeight *int    `json:"pageHeight"`
	Truncate   bool    `json:"truncate"`
	Split      *string `json:"split"`
}

// UnmarshalJSON 容忍 PHP 导出端的空数组编码:fixture 里缺省 options 序列化为 []
// (PHP 空关联数组即 []),与 null 同义空选项;对象形态未知键忽略(fromOptions 留演进空间同款)
func (o *PaginateOptions) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if string(trimmed) == "null" || string(trimmed) == "[]" {
		*o = PaginateOptions{}
		return nil
	}
	type alias PaginateOptions
	return json.Unmarshal(trimmed, (*alias)(o))
}

// Paginator 分页器(PHP CanvasPaginator 镜像,spec §4.2):Canvas 1→n 编译期纯函数。
// 只拆驱动表的行序列,表壳每页按声明原位重画、非表图层逐页按源坐标复制;
// 顶层表 0 张 = 单页直通、≥ 2 张 = paginate_target_invalid;零行 = 1 页空壳、永不产空页。
//
// 原子模式(省略 split):行是唯一装箱原子、永不切开,装不下整行即整行下页;
// 拆分模式(split: "lines",提案 003):可拆行(全文本格且内容行数 ≥ 2)在页界即拆
// ——前段入当前页、封页,续段等价新行重入装箱(可再拆;片段内容行数严格递减 →
// 必然收敛),行退为最终原子;判定与片段构造在 rowsplit.go(纯函数,本类型只承载决策)
//
// 页几何与 options.split 枚举裁决收敛在 Paginate 入口(Go 构造器无错误返回口,
// PHP 构造期抛出的同构收敛;code 是唯一契约、在哪个方法抛出不是)
type Paginator struct {
	opts PaginateOptions
}

// NewPaginator 构造分页器
func NewPaginator(opts PaginateOptions) *Paginator {
	return &Paginator{opts: opts}
}

// Paginate 分页:页序列(分页关 = 单元素,与入参同一实例)。
// 输入前置 = hydrate 后置不变量(高度定稿);分页不重演填充、不依赖资源物化
func (p *Paginator) Paginate(c *canvas.Canvas) ([]*canvas.Canvas, error) {
	// 页几何 fail-fast(spec §4.2.4):非 nil 须为正整数;nil = 分页关直通
	if p.opts.PageHeight != nil && *p.opts.PageHeight <= 0 {
		return nil, fmt.Errorf("%w: pageHeight 须为正整数像素(int > 0),得到 %d", ErrPageGeometryInvalid, *p.opts.PageHeight)
	}
	// options.split 枚举守卫(提案 003):键在而值 ∉ 闭集 v1 = paginate_option_invalid,
	// 与分页开关正交恒执行;未知键仍忽略(开键面纪律不变)
	if p.opts.Split != nil && *p.opts.Split != splitModeLines {
		return nil, fmt.Errorf("%w: options.split 枚举闭集 v1 仅 %q,得到 %q", ErrPaginateOptionInvalid, splitModeLines, *p.opts.Split)
	}

	if p.opts.PageHeight == nil {
		// 分页关(默认直通):溢出判定仍执行,页底 = 源画布高(spec §2 表/§4.2.3)
		if err := assertRowSequencesWithinBottom(c); err != nil {
			return nil, err
		}

		return []*canvas.Canvas{c}, nil
	}

	return p.split(c)
}

// assertRowSequencesWithinBottom 分页关的溢出判定(spec §4.2.3):所有顶层表同规则
// ——行序列底边(表顶 y + Σ行高)越画布底即报错;装饰层盒子与表壳声明高不判
// (隐式裁剪现状不变),嵌套表不判(已折进行高)
func assertRowSequencesWithinBottom(c *canvas.Canvas) error {
	for i, l := range c.GetLayers() {
		table, ok := l.(*layer.TableLayer)
		if !ok {
			continue
		}

		anchor, _, y := table.Position()
		_, anchorY := resolveAnchor(anchor, c.Width(), c.Height(), table.Width(), table.Height())

		bottom := anchorY + y
		for _, row := range table.Rows() {
			bottom += row.Height()
		}

		if bottom > c.Height() {
			return fmt.Errorf("%w: layers[%d] 底边 %d > 页高 %d", ErrContentOverflow, i, bottom, c.Height())
		}
	}
	return nil
}

// split 分页开:行装箱切页(spec §4.2.2)。实现走 graph 层(与 hydrate 同思路):
// 源 graph 一次序列化,逐页替换驱动表的 rows 键后经 canvas.FromGraph 重建
func (p *Paginator) split(c *canvas.Canvas) ([]*canvas.Canvas, error) {
	pageHeight := *p.opts.PageHeight
	graph := c.Graph()

	driverIndexes := make([]int, 0, 1)
	for i := range graph.Layers {
		if graph.Layers[i].Type == layer.TypeTable {
			driverIndexes = append(driverIndexes, i)
		}
	}

	if len(driverIndexes) == 0 {
		return []*canvas.Canvas{c}, nil // 顶层表 0 张:无内容可分,单页直通不报错(spec §4.2.2)
	}
	if len(driverIndexes) > 1 {
		// 多表各自装箱页数不相容,静默任选属隐式裁剪语义(spec §4.2.2)
		return nil, fmt.Errorf("%w: 分页要求画布上唯一顶层 TableLayer,实际 %d 张", ErrPaginateTargetInvalid, len(driverIndexes))
	}

	driverIndex := driverIndexes[0]
	tableGraph := graph.Layers[driverIndex]
	if tableGraph.Template != nil {
		// 前置违反(paginate 消费 hydrate 产物,spec §2):模板态表行序列未定稿,无法装箱。
		// API 误用归调用期错误(先例 TableLayer::addRow),不占 wire 错误 code
		return nil, errors.New("paginate_requires_hydrated_canvas: 驱动表仍处模板态,须先经 hydrate 实例化")
	}

	rowGraphs := nodeRows(&tableGraph)
	// §4.2.5 结构预留:页首/页尾固定行落地时,在此扣减其行高
	// (可用区 = 页高 − 页首行 − 页尾行),装箱语义本身不变
	available := pageHeight - shellTop(&tableGraph, c.Width(), pageHeight)

	// 行装箱决策表(提案 003,按序判定、每行恰走一支):
	// ① 装得下 → 入当前页;② 拆分模式且可拆且 K ≥ 1 → 前段入当前页、封页,
	//    续段重入装箱(等价新行,可再拆);③ 不可拆残余超高 → truncate 独占页 / 报错;
	// ④ 否则整行移下页。拆分分支先于 truncate/overflow 判定(工票 04 决策 5)——
	// 拆分模式下 truncate 只达不可拆行,rowHeight > available 的可拆行照拆不报错。
	// 装箱单位改行 graph(片段无源下标),origin 保留源行号供错误消息定位;
	// 续段 unshift 回队首 = 保序消费。收敛:每次拆分 K ≥ 1、片段内容行数严格递减
	// → 步进必前进、总片段数 ≤ Σ行内容数,无 0 步进(rowsplit_test.go 钉结构,
	// 语料 split-lines-row-across-three-pages 钉多页行为)。
	// 行高读取与流装箱同源:rowHeight(spec §10.3 防数学漂移)
	type workItem struct {
		graph  layer.Node
		origin int
	}
	worklist := make([]workItem, 0, len(rowGraphs))
	for j := range rowGraphs {
		worklist = append(worklist, workItem{graph: rowGraphs[j], origin: j})
	}

	pageRows := [][]layer.Node{}
	current := []layer.Node{}
	currentHeight := 0
	for len(worklist) > 0 {
		item := worklist[0]
		worklist = worklist[1:]
		rowGraph := &item.graph
		rowH := rowHeight(rowGraph)

		// ① 装入当前页(与现役同)
		if currentHeight+rowH <= available {
			current = append(current, *rowGraph)
			currentHeight += rowH
			continue
		}

		// ② 拆分模式:可拆性/K=0/防线(P<0、续段≤0)都收拢在拆分纯函数返回值里
		if p.opts.Split != nil {
			if bound := splitBound(rowGraph, available-currentHeight); bound >= 1 {
				if front, cont, ok := splitRowGraph(rowGraph, bound); ok {
					// 前段高 = K×L + P ≤ 剩余(P ≠ 0 的非零声明差为未钉区,语料 v1 恒 0)
					current = append(current, front)
					currentHeight += rowHeight(&front)
					pageRows = append(pageRows, current)
					current = []layer.Node{}
					currentHeight = 0
					worklist = append([]workItem{{graph: cont, origin: item.origin}}, worklist...)
					continue
				}
			}
		}

		if rowH > available {
			// ③ 唯一残余溢出 = 不可拆行高 > 单页可用区(v2.0 收窄):默认报错,截断开则独占一页
			if !p.opts.Truncate {
				return nil, fmt.Errorf("%w: layers[%d] 行[%d] 高 %d > 单页可用区 %d",
					ErrContentOverflow, driverIndex, item.origin, rowH, available)
			}
			if len(current) > 0 {
				pageRows = append(pageRows, current)
				current = []layer.Node{}
				currentHeight = 0
			}
			pageRows = append(pageRows, []layer.Node{*rowGraph})
			continue
		}

		// ④ 整行移下页开新页(现役原子移动;truncate 不涉)
		if len(current) > 0 {
			pageRows = append(pageRows, current)
		}
		current = []layer.Node{*rowGraph}
		currentHeight = rowH
	}
	if len(current) > 0 {
		pageRows = append(pageRows, current)
	}
	if len(pageRows) == 0 {
		pageRows = [][]layer.Node{{}} // 零行驱动表 = 1 页空壳(spec §4.2.4)
	}

	// 每页组装:非驱动图层逐页原样复制(源坐标不动,页界外位图裁剪自理),
	// 驱动表壳保留声明、行切片替换——行 graph 值拷贝进新切片、各页 graph 隔离
	// (片段即普通行 graph,复用同一组装),fromGraph 重建保证页间零共享可变结构
	pages := make([]*canvas.Canvas, 0, len(pageRows))
	for _, rows := range pageRows {
		pageTableGraph := tableGraph
		pageRowsCopy := rows
		pageTableGraph.Rows = &pageRowsCopy

		pageLayers := slices.Clone(graph.Layers)
		pageLayers[driverIndex] = pageTableGraph

		page, err := canvas.FromGraph(canvas.Graph{
			// canvas 键手工重组:背景必须显式携带每页(ADR 0022 易漏点 2,与 PHP
			// CanvasPaginator 页 canvas 手工拼键同款);背景值对象不可变,各页共享实例
			Canvas: canvas.CanvasSize{Width: c.Width(), Height: pageHeight, Background: graph.Canvas.Background},
			Layers: pageLayers,
		})
		if err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}

	return pages, nil
}

package paginate

// 流链校验单测(工票 09,语义源 = PHP FlowChainValidator + DocumentCompilerFlowTest
// 负例逐条平移):十条链规则 + 跨帧一致性逐条对齐。端到端语义由 fixture 预言机
// (make semantics,flow-semantics v1)钉定,本文件补规则级边界:wire 原始形态裁决
// (非整数字面量/null/未知键)、程序化节点(raw=nil)裁决、规则先后次序
// (fail-fast 顺序决定哪个 code 浮出——code 是三端锚点,次序本身可观测)。

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/layer"
)

// ---- 输入构造 helpers(PHP DocumentCompilerFlowTest 同名 helper 同构,结构从简:
// 链校验只看模板态/rowsPath/顶层表数量,不触及行高装箱语义)----

// templateTable 模板态槽位表
func templateTable(rowsPath string) *layer.TableLayer {
	table := layer.NewTableLayer(layer.WithSize(320, 400))
	table.SetRowsPath(rowsPath)
	table.SetTemplate(layer.NewTableRowTemplate(layer.WithAutoHeight()))
	return table
}

// v1Table V1 固定行表(非模板态,rowsPath 空)
func v1Table(t *testing.T) *layer.TableLayer {
	t.Helper()
	table := layer.NewTableLayer(layer.WithSize(320, 80))
	cell := layer.NewTableCellLayer(layer.WithSize(320, 40))
	cell.AddContentLayer(layer.NewTextLayer(layer.WithSize(320, 40)))
	row := layer.NewTableRowLayer(layer.WithSize(320, 40))
	row.AddCell(cell)
	if err := table.AddRow(row); err != nil {
		t.Fatalf("构造 V1 表: %v", err)
	}
	return table
}

func frameOf(layers ...layer.Layer) *canvas.Canvas {
	return canvas.New(320, 450, layers...)
}

func intPtr(v int) *int { return &v }

// chainFromJSON wire 解码路径构造链(原始键面留存,校验走 raw 裁决)
func chainFromJSON(t *testing.T, src string) []FlowChainNode {
	t.Helper()
	var chain []FlowChainNode
	if err := json.Unmarshal([]byte(src), &chain); err != nil {
		t.Fatalf("解码流链 %s: %v", src, err)
	}
	return chain
}

// wantCode 断言校验错误对位稳定 code(errors.Is 判定;消息非契约不断言)
func wantCode(t *testing.T, err error, sentinel error) {
	t.Helper()
	if err == nil {
		t.Fatalf("预期错误 %v,校验通过", sentinel)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want errors.Is %v", err, sentinel)
	}
}

// ---- 十条链规则 + 跨帧一致性逐规则负例(含规则先后次序)----

func TestValidateFlowChainRuleViolations(t *testing.T) {
	cases := []struct {
		name   string
		frames func(t *testing.T) []*canvas.Canvas
		chain  string // wire JSON 载荷(宽松解码留存原始键面)
		want   error
	}{
		// 节点级规则(flow_chain_invalid)
		{
			name:   "未知键拒绝",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"frame":0,"mode":"fixed","pageHeight":100}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name:   "frame 缺失",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"mode":"fixed"}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name:   "frame 非整数字面量·字符串",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"frame":"x","mode":"fixed"}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name:   "frame 非整数字面量·小数",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"frame":1.5,"mode":"fixed"}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name:   "frame null 同非整数",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"frame":null,"mode":"fixed"}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name: "frame 越界",
			frames: func(t *testing.T) []*canvas.Canvas {
				return []*canvas.Canvas{frameOf(templateTable("order.items")), frameOf(templateTable("order.items"))}
			},
			chain: `[{"frame":2,"mode":"paged"}]`,
			want:  ErrFlowChainInvalid,
		},
		{
			name:   "frame 负数",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"frame":-1,"mode":"fixed"}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name: "frame 重复",
			frames: func(t *testing.T) []*canvas.Canvas {
				return []*canvas.Canvas{frameOf(templateTable("order.items")), frameOf(templateTable("order.items"))}
			},
			chain: `[{"frame":0,"mode":"fixed"},{"frame":0,"mode":"paged"}]`,
			want:  ErrFlowChainInvalid,
		},
		{
			name:   "mode 非法值",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"frame":0,"mode":"auto"}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name:   "mode 缺失",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"frame":0}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name:   "mode 非字符串",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"frame":0,"mode":3}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name:   "paged 携带 quota",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"frame":0,"mode":"paged","quota":2}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name:   "paged 携带 quota·null 字面量也算携带(键在场即裁决)",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"frame":0,"mode":"paged","quota":null}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name:   "quota 负数",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"frame":0,"mode":"fixed","quota":-1}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name:   "quota 非整数·字符串",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"frame":0,"mode":"fixed","quota":"3"}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name:   "quota 非整数·小数",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"frame":0,"mode":"fixed","quota":2.5}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name:   "quota 非整数·null",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"frame":0,"mode":"fixed","quota":null}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name:   "omitIfEmpty 非布尔",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(templateTable("order.items"))} },
			chain:  `[{"frame":0,"mode":"fixed","omitIfEmpty":"yes"}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name: "paged 非链尾",
			frames: func(t *testing.T) []*canvas.Canvas {
				return []*canvas.Canvas{frameOf(templateTable("order.items")), frameOf(templateTable("order.items"))}
			},
			chain: `[{"frame":0,"mode":"paged"},{"frame":1,"mode":"fixed"}]`,
			want:  ErrFlowChainInvalid,
		},
		{
			name: "双 paged(链尾判定同时封住)",
			frames: func(t *testing.T) []*canvas.Canvas {
				return []*canvas.Canvas{frameOf(templateTable("order.items")), frameOf(templateTable("order.items"))}
			},
			chain: `[{"frame":0,"mode":"paged"},{"frame":1,"mode":"paged"}]`,
			want:  ErrFlowChainInvalid,
		},
		{
			name: "链上帧无顶层 TableLayer",
			frames: func(t *testing.T) []*canvas.Canvas {
				return []*canvas.Canvas{frameOf(layer.NewTextLayer(layer.WithSize(200, 30)))}
			},
			chain: `[{"frame":0,"mode":"paged"}]`,
			want:  ErrFlowChainInvalid,
		},
		{
			name:   "链上帧顶层表非模板态(V1 表)",
			frames: func(t *testing.T) []*canvas.Canvas { return []*canvas.Canvas{frameOf(v1Table(t))} },
			chain:  `[{"frame":0,"mode":"paged"}]`,
			want:   ErrFlowChainInvalid,
		},
		{
			name: "链序乱序回跳",
			frames: func(t *testing.T) []*canvas.Canvas {
				return []*canvas.Canvas{frameOf(templateTable("order.items")), frameOf(templateTable("order.items"))}
			},
			chain: `[{"frame":1,"mode":"fixed"},{"frame":0,"mode":"paged"}]`,
			want:  ErrFlowChainInvalid,
		},

		// 跨帧一致性(flow_rows_path_inconsistent)
		{
			name: "链内 rowsPath 不一致",
			frames: func(t *testing.T) []*canvas.Canvas {
				return []*canvas.Canvas{frameOf(templateTable("order.items")), frameOf(templateTable("gift.items"))}
			},
			chain: `[{"frame":0,"mode":"fixed"},{"frame":1,"mode":"paged"}]`,
			want:  ErrFlowRowsPathInconsistent,
		},
		{
			name: "链外模板表同 rowsPath(幽灵行)",
			frames: func(t *testing.T) []*canvas.Canvas {
				return []*canvas.Canvas{frameOf(templateTable("order.items")), frameOf(templateTable("order.items"))}
			},
			chain: `[{"frame":0,"mode":"paged"}]`,
			want:  ErrFlowRowsPathInconsistent,
		},
		{
			name: "链外 V1 表标注同 rowsPath 同样拒绝(不限定模板态)",
			frames: func(t *testing.T) []*canvas.Canvas {
				ghost := v1Table(t)
				ghost.SetRowsPath("order.items")
				return []*canvas.Canvas{frameOf(templateTable("order.items")), frameOf(ghost)}
			},
			chain: `[{"frame":0,"mode":"paged"}]`,
			want:  ErrFlowRowsPathInconsistent,
		},

		// 任意帧 ≥2 顶层表(paginate_target_invalid,分页段 code 不换段)
		{
			name: "链上帧双表",
			frames: func(t *testing.T) []*canvas.Canvas {
				return []*canvas.Canvas{frameOf(templateTable("order.items"), v1Table(t))}
			},
			chain: `[{"frame":0,"mode":"paged"}]`,
			want:  ErrPaginateTargetInvalid,
		},
		{
			name: "链外帧双表",
			frames: func(t *testing.T) []*canvas.Canvas {
				return []*canvas.Canvas{
					frameOf(templateTable("order.items")),
					frameOf(templateTable("gift.items"), v1Table(t)),
				}
			},
			chain: `[{"frame":0,"mode":"paged"}]`,
			want:  ErrPaginateTargetInvalid,
		},

		// 规则先后次序:fail-fast 顺序决定哪个 code 浮出(对齐 PHP 校验器段序)
		{
			name: "节点级规则先于 paginate_target_invalid",
			frames: func(t *testing.T) []*canvas.Canvas {
				return []*canvas.Canvas{frameOf(templateTable("order.items"), v1Table(t))}
			},
			chain: `[{"frame":9,"mode":"paged"}]`,
			want:  ErrFlowChainInvalid,
		},
		{
			name: "paged 链尾判定先于 paginate_target_invalid",
			frames: func(t *testing.T) []*canvas.Canvas {
				return []*canvas.Canvas{frameOf(templateTable("order.items"), v1Table(t)), frameOf(templateTable("order.items"))}
			},
			chain: `[{"frame":0,"mode":"paged"},{"frame":1,"mode":"fixed"}]`,
			want:  ErrFlowChainInvalid,
		},
		{
			name: "paginate_target_invalid 先于链上无表",
			frames: func(t *testing.T) []*canvas.Canvas {
				return []*canvas.Canvas{frameOf(templateTable("order.items"), v1Table(t)), frameOf(layer.NewTextLayer(layer.WithSize(200, 30)))}
			},
			chain: `[{"frame":1,"mode":"paged"}]`,
			want:  ErrPaginateTargetInvalid,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateFlowChain(tc.frames(t), chainFromJSON(t, tc.chain))
			wantCode(t, err, tc.want)
		})
	}
}

// ---- 正例:校验通过返回 frame 下标 => 顶层槽位表图层下标(编译器回写定位用)----

func TestValidateFlowChainReturnsTableIndexByFrame(t *testing.T) {
	// 非表图层占位:槽位表下标按图层序列定位(与 Graph().Layers 同序),不是"第几张表"
	mixed := frameOf(layer.NewTextLayer(layer.WithSize(200, 30)), templateTable("order.items"))
	single := frameOf(templateTable("order.items"))

	slots, err := ValidateFlowChain([]*canvas.Canvas{mixed, single}, chainFromJSON(t,
		`[{"frame":0,"mode":"fixed","quota":0,"omitIfEmpty":true},{"frame":1,"mode":"paged"}]`))
	if err != nil {
		t.Fatalf("合法链校验失败: %v", err)
	}
	if len(slots) != 2 || slots[0] != 1 || slots[1] != 0 {
		t.Fatalf("槽位表定位 = %v, want map[0:1 1:0]", slots)
	}
}

// 显式空链 = 纯文档级管线,校验直通(链 rowsPath 未定,链外碰撞无从谈起——
// PHP $chainRowsPath 为 null、” !== null 恒不命中同构;管线侧由 Compile 分流,不经本校验)
func TestValidateFlowChainEmptyChain(t *testing.T) {
	slots, err := ValidateFlowChain([]*canvas.Canvas{frameOf(v1Table(t))}, nil)
	if err != nil {
		t.Fatalf("空链校验失败: %v", err)
	}
	if len(slots) != 0 {
		t.Fatalf("空链槽位表 = %v, want 空", slots)
	}
}

// ---- 程序化构造节点(raw=nil,工票 08 约定:按「恰好类型化键在场」裁决)----

func TestValidateFlowChainProgrammaticNodes(t *testing.T) {
	frame := frameOf(templateTable("order.items"))

	// 最小合法节点
	if _, err := ValidateFlowChain([]*canvas.Canvas{frame}, []FlowChainNode{{Frame: 0, Mode: "fixed"}}); err != nil {
		t.Fatalf("程序化最小节点校验失败: %v", err)
	}
	// 显式零额 quota:0 合法
	if _, err := ValidateFlowChain([]*canvas.Canvas{frame}, []FlowChainNode{{Frame: 0, Mode: "fixed", Quota: intPtr(0)}}); err != nil {
		t.Fatalf("quota 0 校验失败: %v", err)
	}
	// paged 不得携带 quota(Quota 指针在场即携带)
	_, err := ValidateFlowChain([]*canvas.Canvas{frame}, []FlowChainNode{{Frame: 0, Mode: "paged", Quota: intPtr(2)}})
	wantCode(t, err, ErrFlowChainInvalid)
	// 负 quota
	_, err = ValidateFlowChain([]*canvas.Canvas{frame}, []FlowChainNode{{Frame: 0, Mode: "fixed", Quota: intPtr(-1)}})
	wantCode(t, err, ErrFlowChainInvalid)
	// frame 越界(类型化值即裁决依据)
	_, err = ValidateFlowChain([]*canvas.Canvas{frame}, []FlowChainNode{{Frame: 5, Mode: "paged"}})
	wantCode(t, err, ErrFlowChainInvalid)
	// omitIfEmpty 类型化布尔恒合法
	if _, err := ValidateFlowChain([]*canvas.Canvas{frame}, []FlowChainNode{{Frame: 0, Mode: "fixed", OmitIfEmpty: true}}); err != nil {
		t.Fatalf("omitIfEmpty 校验失败: %v", err)
	}
}

// ---- Compile 接线:步骤 0 链校验(fail-fast 于入口,先于 hydrate,spec §10.3 时序)----

func TestCompileRunsChainValidation(t *testing.T) {
	compiler := NewDocumentCompiler(nil)
	frames := []*canvas.Canvas{frameOf(templateTable("order.items"))}

	// 非法链:校验错误先于 hydrate 直接浮出(spec §10.3 步骤 0)
	_, err := compiler.Compile(frames, nil, chainFromJSON(t, `[{"frame":7,"mode":"paged"}]`))
	wantCode(t, err, ErrFlowChainInvalid)

	// 合法链 + 未绑数据集:恒等直通——模板帧以声明态零行空壳单页直通(spec §10.3 第 5 条)
	result, err := compiler.Compile(frames, nil, chainFromJSON(t, `[{"frame":0,"mode":"fixed"}]`))
	if err != nil {
		t.Fatalf("合法链 err = %v, want nil", err)
	}
	if len(result.Canvases) != 1 {
		t.Fatalf("未绑数据集流路径页数 = %d, want 1(声明态空壳直通)", len(result.Canvases))
	}

	// 显式空链与 nil 同义 = 无流路径,不经链校验分流(spec §10.2)
	result, err = compiler.Compile(frames, nil, nil)
	if err != nil {
		t.Fatalf("空链 err = %v, want nil", err)
	}
	if len(result.Canvases) != 1 {
		t.Fatalf("空链页数 = %d, want 1(分页关单页直通)", len(result.Canvases))
	}
}

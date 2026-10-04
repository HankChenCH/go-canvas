package paginate

// 文档编译契约面(spec §10,ADR 0009,PHP DocumentCompiler 逐镜像):
// N 帧 × 1 dataset × 流链声明的文档管线——帧 = 设计期独立、版式各异的完整画布
// (模板态声明 graph,含 data.rowsPath + template),文档管线自声明态起跑、
// hydrate 在管线内(spec §10.3 时序,与 paginate 段「输入为 hydrate 产物」不同)。

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/hydrate"
	"github.com/hankchen/go-canvas/layer"
)

// document 段稳定 code(spec §10.4,三端一致性锚点;消息可改、code 不可改):
// 链校验报错位在 ValidateFlowChain(工票 09);分配期超容复用 paginate 段
// content_overflow(见 flow.go),document 段自身仅此两码
var (
	ErrFlowChainInvalid         = errors.New("flow_chain_invalid")
	ErrFlowRowsPathInconsistent = errors.New("flow_rows_path_inconsistent")
)

// FlowChainNode 流链声明节点(spec §10.2):封闭 4 键数据通道 {frame, mode,
// quota?, omitIfEmpty?},是管线输入(与 dataset 同类)而非 wire 图内容。
// frame = 帧数组下标(0 起);mode = "fixed"|"paged";quota 仅 fixed 合法、
// 允许 0(「显式不吃行」的声明位);omitIfEmpty 缺省 false。
// 显式空链与 nil 同义 = 纯文档级管线;paged 至多一个且在链尾、不得携带 quota。
type FlowChainNode struct {
	Frame       int    `json:"frame"`
	Mode        string `json:"mode"`
	Quota       *int   `json:"quota,omitempty"`
	OmitIfEmpty bool   `json:"omitIfEmpty,omitempty"`

	// raw 原始键面:链校验(工票 09)的裁决输入。解码期只宽松捕获——封闭键
	// (未知键拒绝)与字段类型形态都在文档期报 flow_chain_invalid,若解码即失败,
	// fixture 用例落不到 errorCode 断言、预言机失效(与 PHP 链节点以裸数组
	// 进入校验器同构)。工票 09 在包内读本字段,故不导出访问器。
	raw map[string]json.RawMessage
}

// UnmarshalJSON 宽松解码:已知键尽力落位(类型不符保持零值),未知键与原始
// 形态全部留存 raw;除「节点不是 JSON 对象」外不报解码错误。JSON null 节点
// 得零值节点(raw 空)——mode 缺失在链校验期即 flow_chain_invalid,无需解码期
// 特判;已知键字面 null 同理(如 quota:null 落 &0,原始 null 留存供校验)
func (n *FlowChainNode) UnmarshalJSON(data []byte) error {
	n.raw = nil
	if err := json.Unmarshal(data, &n.raw); err != nil {
		return err
	}
	_ = json.Unmarshal(n.raw["frame"], &n.Frame)
	_ = json.Unmarshal(n.raw["mode"], &n.Mode)
	if q, ok := n.raw["quota"]; ok {
		var quota int
		if err := json.Unmarshal(q, &quota); err == nil {
			n.Quota = &quota
		}
	}
	_ = json.Unmarshal(n.raw["omitIfEmpty"], &n.OmitIfEmpty)
	return nil
}

// Diagnostic 软诊断(spec §5.1):hydrate 兜底一类软失败观测,与硬失败
// (异常/sentinel)分通道。Path 空 = 未定位
type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

// CompileResult 文档编译产物(spec §5.1/§10.1):页序列按帧序 × 页序扁平拼接;
// warnings 全帧共享单一 collector、不带帧标识(帧标识升格走 spec 版本)
type CompileResult struct {
	Canvases []*canvas.Canvas
	Warnings []Diagnostic
}

// DocumentCompiler 文档编译器(PHP DocumentCompiler 镜像,spec §10.1):独立类,
// 与单画布管线共享内部件(hydrate.Hydrator / Paginator)、不共享模板方法。
// evaluator 注入链 = 构造器传入全量 hydrate(全库唯一可替换面,nil = 默认求值器)
type DocumentCompiler struct {
	evaluator hydrate.ExpressionEvaluator
}

// NewDocumentCompiler 构造文档编译器
func NewDocumentCompiler(evaluator hydrate.ExpressionEvaluator) *DocumentCompiler {
	return &DocumentCompiler{evaluator: evaluator}
}

// Compile 文档编译:逐帧 hydrate(全帧共享求值器)后按流链装箱再分页,
// 产物页序列按帧序 × 页序扁平拼接(flowChain 空 = 纯文档级管线,spec §10.2)。
//
// 两条路径(spec §10.3 时序):
//   - 无流:先全量 hydrate,后逐帧 paginate(分页关:单页 + 溢出判定,页底 = 帧画布高)
//   - 流:0 链校验(fail-fast 于入口,先于 hydrate)→ 1 全量 hydrate(行高定稿、
//     $index 即全局序号)→ 2 flow 分配(remaining = 链首帧槽位实例行序列,逐节点
//     take 切份额;未分派行丢弃)→ 3 回写 + 分页(槽位 rows 整体替换 → FromGraph
//     重建 → 既有 Paginator 逐帧分页;fixed/链外帧分页关,paged 帧页高 = 帧画布高)。
//     Paginator 零改动复用——流是 paginate 的调用者而非对它的改造(ADR 0009)
//
// 错误面(spec §10.4):空 frames = 调用方编程错误不立三端锚点 code(PHP
// InvalidArgumentException 同构);链校验 flow_chain_invalid /
// flow_rows_path_inconsistent / paginate_target_invalid(ValidateFlowChain);
// 分配期首行超容与帧内分页溢出复用 paginate 段 content_overflow(同一装箱数学
// 同一失败语义);既有 hydrate 错误语义逐帧原样适用,任一帧失败即抛
func (c *DocumentCompiler) Compile(frames []*canvas.Canvas, dataset any, flowChain []FlowChainNode) (*CompileResult, error) {
	if len(frames) == 0 {
		return nil, errors.New("frames_empty: 文档编译至少需要一帧,frames 不得为空数组")
	}

	// 显式空链与 nil 同义 = 纯文档级管线(spec §10.2),不经链校验分流
	if len(flowChain) == 0 {
		return c.compileWithoutFlow(frames, dataset)
	}
	return c.compileWithFlow(frames, dataset, flowChain)
}

// compileWithoutFlow 无流路径(spec §10.3):先全量 hydrate,后逐帧 paginate(分页关)
func (c *DocumentCompiler) compileWithoutFlow(frames []*canvas.Canvas, dataset any) (*CompileResult, error) {
	hydrated, err := c.hydrateAll(frames, dataset)
	if err != nil {
		return nil, err
	}

	paginator := NewPaginator(PaginateOptions{})
	canvases := make([]*canvas.Canvas, 0, len(hydrated))
	for _, frame := range hydrated {
		pages, err := paginator.Paginate(frame)
		if err != nil {
			return nil, err
		}
		canvases = append(canvases, pages...)
	}

	return &CompileResult{Canvases: canvases, Warnings: noDiagnostics()}, nil
}

// flowAllocation 帧分配结果:切得的行份额 + 源节点(回写定位与零行跳帧裁决用)
type flowAllocation struct {
	rows []layer.Node
	node *FlowChainNode
}

// compileWithFlow 流装箱路径(spec §10.3 时序:0 链校验 → 1 全量 hydrate → 2 分配 → 3 回写+分页)
func (c *DocumentCompiler) compileWithFlow(frames []*canvas.Canvas, dataset any, chain []FlowChainNode) (*CompileResult, error) {
	// 0 链校验(fail-fast 于入口,先于全量 hydrate):返回 frame 下标 => 槽位表图层下标
	tableIndexByFrame, err := ValidateFlowChain(frames, chain)
	if err != nil {
		return nil, err
	}

	// 1 全量 hydrate:$index 即全局序号、行高定稿——分配与聚合正确性的来源(ADR 0009)
	hydrated, err := c.hydrateAll(frames, dataset)
	if err != nil {
		return nil, err
	}

	// 2 flow 分配(graph 层,与分页同思路):remaining = 链首帧槽位实例行序列,
	// 逐节点 take 后切份额;未分派帧的实例行丢弃(回写 = 整体替换)
	graphs := make([]canvas.Graph, len(hydrated))
	for i, h := range hydrated {
		graphs[i] = h.Graph()
	}
	firstFrame := chain[0].Frame
	remaining := nodeRows(&graphs[firstFrame].Layers[tableIndexByFrame[firstFrame]])

	allocations := make(map[int]flowAllocation, len(chain))
	for i := range chain {
		node := &chain[i]
		frameIndex := node.Frame
		tableIndex := tableIndexByFrame[frameIndex]

		consumer := flowConsumerFromNode(node)
		capacity := frameCapacity(hydrated[frameIndex], &graphs[frameIndex].Layers[tableIndex])
		taken, err := consumer.take(capacity, remaining)
		if err != nil {
			return nil, err
		}
		share := make([]layer.Node, taken) // array_splice 切份额同构(恒非 nil,空份额写回 "rows":[])
		copy(share, remaining)
		allocations[frameIndex] = flowAllocation{rows: share, node: node}
		remaining = remaining[taken:]
	}

	// 3 回写 + 分页 + 4 帧序 × 页序扁平拼接:链上帧灌入分配结果重建后分页
	// (fixed 帧单页、paged 帧页高 = 帧画布高);链外帧 hydrate 产物直通分页。
	// Paginator 无状态,fixed/链外帧共用同一默认分页器
	paginator := NewPaginator(PaginateOptions{})
	canvases := make([]*canvas.Canvas, 0, len(frames))
	for frameIndex, frame := range frames {
		alloc, onChain := allocations[frameIndex]
		if !onChain {
			pages, err := paginator.Paginate(hydrated[frameIndex])
			if err != nil {
				return nil, err
			}
			canvases = append(canvases, pages...)
			continue
		}

		if len(alloc.rows) == 0 && alloc.node.OmitIfEmpty {
			continue // 零行跳帧:不产页、不进产物序列(spec §10.3)
		}

		graph := graphs[frameIndex]
		graph.Layers = slices.Clone(graph.Layers) // PHP 数组值语义同构:回写落副本,不扰动共享底层数组
		tableNode := &graph.Layers[tableIndexByFrame[frameIndex]]
		if tableNode.Template != nil {
			// 未绑数据集(spec §3 恒等直通在流路径原样适用):无实例行可分配,
			// 模板帧以声明态零行空壳单页直通——不写 rows(template ⊕ rows 互斥)、
			// 不重建不分页;观测几何与零行矩阵一致(spec §10.3 第 5 条)
			canvases = append(canvases, hydrated[frameIndex])
			continue
		}
		tableNode.Rows = &alloc.rows // 份额切片分配期已独立,此后只读(FromGraph 仅读)

		pageCanvas, err := canvas.FromGraph(graph)
		if err != nil {
			return nil, err
		}
		framePaginator := paginator
		if alloc.node.Mode == modePaged {
			pageHeight := frame.Height() // 页高 = 帧画布高(v1 无覆盖位)
			framePaginator = NewPaginator(PaginateOptions{PageHeight: &pageHeight})
		}
		pages, err := framePaginator.Paginate(pageCanvas)
		if err != nil {
			return nil, err
		}
		canvases = append(canvases, pages...)
	}

	return &CompileResult{Canvases: canvases, Warnings: noDiagnostics()}, nil
}

// hydrateAll 全帧共享单一求值器实例的全量 hydrate(spec §10.3 步骤 1):
// 既有 hydrate 错误语义逐帧原样适用,任一帧失败即抛
func (c *DocumentCompiler) hydrateAll(frames []*canvas.Canvas, dataset any) ([]*canvas.Canvas, error) {
	hydrator := hydrate.NewHydrator(c.evaluator)
	hydrated := make([]*canvas.Canvas, 0, len(frames))
	for _, frame := range frames {
		h, err := hydrator.Hydrate(frame, dataset)
		if err != nil {
			return nil, err
		}
		hydrated = append(hydrated, h)
	}
	return hydrated, nil
}

// noDiagnostics 软诊断空通道:PHP 侧 warnings 经 DiagnosticCollector 随 hydrate
// 发射(spec §3.3 文案兜底);Go hydrate 面尚无软诊断发射缝(文案兜底静默直通),
// warnings 恒空——发射缝落地时在 hydrateAll 接线,CompileResult 形状不变
func noDiagnostics() []Diagnostic {
	return []Diagnostic{}
}

// nodeRows 表节点的实例行序列(V1 形态 rows 键);模板态或未实例化无 rows 键 = 空序列
// (PHP $tableGraph['rows'] ?? [] 同构),恒非 nil
func nodeRows(tableNode *layer.Node) []layer.Node {
	if tableNode.Rows == nil {
		return []layer.Node{}
	}
	return *tableNode.Rows
}

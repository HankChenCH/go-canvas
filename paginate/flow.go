package paginate

// 流消费策略族(PHP Compiler\Flow 逐镜像,spec §10.3):按 mode/quota 封闭分发——
// fixed 无 quota = 容量型(CapacityTake)、fixed 带 quota = 声明配额(QuotaTake)、
// paged = 链尾终止(PagedTake)。前置 ValidateFlowChain 保证节点形状,非法 mode 到不了这里。

import (
	"fmt"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
)

// flowConsumer 流消费策略(spec §10.3):对剩余行序列取一份额,take 返回摄取行数。
// 剩余行 = 槽位实例行 graph 序列(行高已定稿),消费只读、不重排不复制
type flowConsumer interface {
	take(capacity int, remaining []layer.Node) (int, error)
}

// capacityTake 容量型 fixed(spec §10.3):可用区装得下的最大行前缀——行保序、不切开。
//
// 首行即超容(单行高 > 可用区)→ 分配期 fail-fast 抛 paginate 段 content_overflow
// (同一装箱数学的同一失败语义,由 DocumentCompiler 抛出,不换载体不换段,spec §10.4);
// 自然 0 行仅当剩余已空——首行超容已被 fail-fast 接管,不存在静默 0 行路径
type capacityTake struct{}

func (capacityTake) take(capacity int, remaining []layer.Node) (int, error) {
	if len(remaining) == 0 {
		return 0, nil
	}

	firstHeight := rowHeight(&remaining[0])
	if firstHeight > capacity {
		return 0, fmt.Errorf("%w: 流分配首行高 %d > 帧可用区 %d", ErrContentOverflow, firstHeight, capacity)
	}

	taken, used := 0, 0
	for i := range remaining {
		height := rowHeight(&remaining[i])
		if used+height > capacity {
			break
		}
		used += height
		taken++
	}

	return taken, nil
}

// quotaTake fixed + quota(spec §10.3):min(quota, count)——quota 是声明非许可,
// 超容不钳制(quota:0 = 显式不吃行的声明位);多装行由帧内 paginate 溢出语义裁决
type quotaTake struct {
	quota int
}

func (t quotaTake) take(_ int, remaining []layer.Node) (int, error) {
	return min(t.quota, len(remaining)), nil
}

// pagedTake paged 链尾终止(spec §10.3):吃尽剩余,页内切分全权交 paginate
// (页高 = 帧画布高,v1 无覆盖位,逃生舱 = fixed + quota)
type pagedTake struct{}

func (pagedTake) take(_ int, remaining []layer.Node) (int, error) {
	return len(remaining), nil
}

// flowConsumerFromNode 策略分发(PHP FlowConsumerFactory::fromNode 同构):
// paged → 链尾终止;fixed 携 quota(键在场即携带)→ 声明配额;其余 → 容量型。
// quota 在场判定以 Quota 指针为准:上游 ValidateFlowChain 已把 quota:null 与
// 非整数字面量按 flow_chain_invalid 拒之门外,走到这里「键在场 ⟺ 指针非 nil」
func flowConsumerFromNode(node *FlowChainNode) flowConsumer {
	if node.Mode == modePaged {
		return pagedTake{}
	}

	if node.Quota != nil {
		return quotaTake{quota: *node.Quota}
	}
	return capacityTake{}
}

// frameCapacity 帧装箱容量(PHP FrameCapacity 同构,spec §10.3):可用区 = 页底 − 表顶,
// 页底基准经 shellTop 与 paginate 共享同一数学,防漂移。
//
// fixed 帧单页页底 = 帧画布高(Paginator 分页关的溢出判定基准);
// paged 帧页高 = 帧画布高——两种帧的页底同值,容量计算不区分 mode
func frameCapacity(frame *canvas.Canvas, tableGraph *layer.Node) int {
	return frame.Height() - shellTop(tableGraph, frame.Width(), frame.Height())
}

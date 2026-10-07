package paginate

// 表几何共享纯函数(PHP Compiler\TableGeometry 逐镜像,spec §10.3):分页装箱
// (Paginator.split)与流装箱(FrameCapacity → 消费策略族)消费同一套「表顶 / 行高」
// 读取实现,防两处数学漂移;读取键位与 PHP 逐字一致(纯提取,非重写)。
//
// 锚点解析是 renderer.ResolveAnchor 的同构孪生(PHP 侧 TableGeometry 同样引用
// Renderer\PositionResolver):paginate 属结构侧、不得依赖渲染侧,故在此落本地副本
// ——两侧数学由各自语义 fixture(布局快照 / 分页语义)钉死,漂移即红。

import (
	"github.com/HankChenCH/go-canvas/layer"
)

// shellTop 表壳顶 y:锚点按给定页/画布尺寸解析 + 声明 y 偏移。
// 分页侧(页宽 = 源画布宽、页高 = pageHeight)与流侧(fixed 帧单页、
// paged 帧页高 = 帧画布高)页底基准不同,但锚点解析基准一致——
// 均与该页渲染期 paint 的锚点解析相同
//
// tableGraph 表图层 graph(spec.shape / spec.position),键缺省归零值(PHP ?? 0 同款)
func shellTop(tableGraph *layer.Node, pageWidth, pageHeight int) int {
	_, anchorY := resolveAnchor(
		tableGraph.Spec.Position.Position,
		pageWidth,
		pageHeight,
		tableGraph.Spec.Shape.Width,
		tableGraph.Spec.Shape.Height,
	)
	return anchorY + tableGraph.Spec.Position.Y
}

// rowHeight 行 graph 的定稿行高:hydrate 产物行高已定稿(声明态豁免在 hydrate 的
// fromGraph 耦合重放闭环),与装箱读取同一键位同一形态(PHP (int) 强转同款,
// wire 解码已把 spec.shape.height 强类型为 int)
func rowHeight(rowGraph *layer.Node) int {
	return rowGraph.Spec.Shape.Height
}

// resolveAnchor 九锚点定位解析(renderer.ResolveAnchor 同构副本,见包内注释):
// 把 top-left/center/bottom-right 等锚点串换算为子层在父盒内的偏移 (x, y)。
// 不做边界钳位——子层大于父盒时偏移为负,即溢出摆放;未知锚点归 (0, 0)
// (PHP match default 臂)。整除向零截断:Go 整除天然向零,与 PHP intval 一致
func resolveAnchor(anchor string, parentWidth, parentHeight, childWidth, childHeight int) (int, int) {
	var x, y int
	switch anchor {
	case layer.AnchorTopRight, layer.AnchorRight, layer.AnchorBottomRight:
		x = parentWidth - childWidth
	case layer.AnchorTop, layer.AnchorCenter, layer.AnchorBottom:
		x = (parentWidth - childWidth) / 2
	}

	switch anchor {
	case layer.AnchorBottomLeft, layer.AnchorBottom, layer.AnchorBottomRight:
		y = parentHeight - childHeight
	case layer.AnchorLeft, layer.AnchorCenter, layer.AnchorRight:
		y = (parentHeight - childHeight) / 2
	}

	return x, y
}

package renderer

import (
	"github.com/hankchen/go-canvas/layer"
)

// 九锚点定位解析:把 top-left/center/bottom-right 等锚点串换算为子层在
// 父盒内的偏移(纯函数,各渲染端共用)。

// Resolve 把锚点串解析为子层在父盒 (parentWidth×parentHeight) 内的偏移 (x, y)。
// 不做边界钳位——子层大于父盒时偏移为负,即溢出摆放(与旧库 intervention insert
// 语义一致);未知锚点归 (0, 0)(PHP match default 臂)。
// 整除向零截断:操作数均为整型,Go 整除天然向零,与 PHP intval((p-c)/2) 一致
func Resolve(anchor string, parentWidth, parentHeight, childWidth, childHeight int) (int, int) {
	var x, y int
	switch anchor {
	case layer.AnchorTopRight, layer.AnchorRight, layer.AnchorBottomRight:
		x = parentWidth - childWidth
	case layer.AnchorTop, layer.AnchorCenter, layer.AnchorBottom:
		x = (parentWidth - childWidth) / 2
	default:
		x = 0
	}

	switch anchor {
	case layer.AnchorBottomLeft, layer.AnchorBottom, layer.AnchorBottomRight:
		y = parentHeight - childHeight
	case layer.AnchorLeft, layer.AnchorCenter, layer.AnchorRight:
		y = (parentHeight - childHeight) / 2
	default:
		y = 0
	}

	return x, y
}

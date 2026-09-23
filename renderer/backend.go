package renderer

import (
	"github.com/hankchen/go-canvas/layer"
)

// Backend 绘制原语契约(扩展点):渲染后端作者只实现五个原语即可接入完整渲染管线
// ——物化、遍历、分派、容器下钻、定位全由渲染模板完成(对应 PHP AbstractRenderer
// 的五个 abstract 方法)。坐标一律为渲染面上的绝对像素。
type Backend interface {
	// Begin 创建渲染面(位图 / PDF 文档等)
	Begin(width, height int) error

	// End 收尾并返回渲染产物(类型由后端决定:位图、字节流……)
	End() any

	// DrawRect 绘制矩形盒:背景色 + 四边边框。
	// bgColor 为 nil 跳过填充;边框四边逐边可 nil(结构对齐 graph 的 shape.border)
	DrawRect(x, y, width, height int, bgColor *string, border layer.Border) error

	// DrawImage 绘制图片:src 为物化后的本地路径,缩放裁切至 width×height 后放置于 (x, y)
	DrawImage(src string, x, y, width, height int) error

	// DrawText 绘制单行文本,(x, y) 为 align 语义下的锚点;
	// fontFile 为物化后的字体路径,空串/纯数字表示渲染端内置默认字体
	DrawText(
		line string,
		x, y int,
		fontFile string,
		fontSize int,
		fontColor string,
		horizontalAlign string,
		verticalAlign string,
		angle int,
	) error
}

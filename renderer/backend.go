package renderer

import (
	"image"

	"github.com/HankChenCH/go-canvas/layer"
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

	// DrawImage 绘制图片(双矩形 blit,ADR 0024):把 src(物化后的本地路径)中
	// srcRect 矩形(源图内在尺寸坐标系)拉伸搬运至 dstRect 矩形。原语零 fit 语义、
	// 零内在尺寸读取——cover 裁窗 / contain 收缩 / fill 整图 / tile 网格全部是
	// 渲染模板分派层的纯几何(与 PHP 权威端 paintImage 同构)。
	// srcRect 为空矩形是整图哨兵:按解码后实际尺寸取整图(分派层内在尺寸不可用的
	// 退化形态,接口契约的组成部分);dstRect 空矩形后端防护不画
	DrawImage(src string, srcRect, dstRect image.Rectangle) error

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

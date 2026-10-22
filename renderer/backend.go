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

// ShapeCapableBackend 形状能力口(ADR 0026):渲染后端可选实现的加法原语对——
// 椭圆绘制(fill + stroke 一体)与椭圆掩码 blit(掩码 = 内切于给定矩形的椭圆,
// 分派层传正方形即圆)。原语零形状语义、零 fit 语义,几何全在分派层
// (ADR 0024 方向延续);分派层以类型断言探测本接口,未实现的后端对 clip 声明
// 降级矩形路径,既有 Backend 实现零破坏(接口面扩展携 CHANGELOG breaking note,
// ADR 0024 先例)
type ShapeCapableBackend interface {
	Backend

	// DrawEllipse 椭圆绘制原语:在 (x, y) 起的 width×height 外接矩形内画内切椭圆。
	// fillColor 填充(nil/空串跳过)、stroke 圆周描边(nil 不描,形态同边框边目)
	DrawEllipse(x, y, width, height int, fillColor *string, stroke *layer.BorderSide) error

	// DrawImageMasked 掩码 blit 原语(DrawImage 的掩码形态):双矩形 blit(语义与
	// DrawImage 完全一致,含 srcRect 空矩形整图哨兵)后,以 maskRect 外接矩形的
	// 内切椭圆为掩码裁切——掩码外透明。掩码矩形是画布绝对坐标,与目标矩形独立
	// (分派层传正方形即圆)
	DrawImageMasked(src string, srcRect, dstRect, maskRect image.Rectangle) error
}

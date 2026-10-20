package renderer

// 多页 walk 模板导出入口(工票 01):canvas-compiler 工票 07「AbstractRenderer
// 多页钩子」的 Go 同构物——每页一次 Begin/背景铺装/walk、单文档统一 End。
// 本文件只放包外可消费的入口函数;页内编排(renderPages/paintPage)在 renderer.go,
// 与单页 renderSurface 共享页内段。Renderer 接口不扩多页方法(非破坏红线:
// 既有接口签名零变化,外部实现者与 image-renderer 消费不受影响)。

import (
	"context"

	"github.com/HankChenCH/go-canvas/canvas"
)

// RenderPages 多页 walk 模板:消费已分页页序列(paginate.CompileResult.Canvases
// 同构形态,帧序×页序),逐页 建面→背景铺装(每页画布背景显式携带,ADR 0022)
// →逐层 paint,全部页完成后单文档统一 End 一次。产物 any(类型由后端 End 决定),
// 泛型断言辅助(RenderPagesAs 形态)由消费方按 renderas.go 模式自行组装。
// 复用 New(backend, nil) 模板路径(nil resolver → 默认物化器,后端自带优先,
// 与 RenderAs 同款复用模式)。
// ctx 检查点:页间一处新增(下一页 Begin 前)+ 页内既有检查点不动(ADR 0008);
// 失败语义同单页——任一页任一原语/物化失败即 error、End 不达、整个文档无产物逃逸;
// 空页序列属调用侧用法错误(不设 code)。
// 典型调用(单 canvas 退化为单页,与单页 Render 语义一致):
//
//	renderer.RenderPages(ctx, pdfBackend, []*canvas.Canvas{c1, c2})
func RenderPages(ctx context.Context, backend Backend, pages []*canvas.Canvas) (any, error) {
	return newTemplate(backend, nil).renderPages(ctx, pages)
}

package pdfrenderer

// 多页入口(工票 03):消费已分页页序列(paginate.CompileResult.Canvases 同构形态,
// 帧序×页序)输出单 PDF 文档字节。多页编排归核心 walk 模板(renderer.RenderPages,
// 工票 01:每页建面铺装 walk、n 页 = n 次 Begin + 1 次 End、页间 ctx 检查点、
// 任一页失败即 error 且 End 不达无产物逃逸),本文件只做后端接线与产物断言——
// 消费面形态镜像 renderer/renderas.go(泛型断言辅助 + 便捷具体入口)。

import (
	"context"
	"fmt"
	"reflect"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/renderer"
)

// RenderPages 多页便捷入口:本 module Renderer(经 NewRenderer 组装即完整管线,
// nil-resolver 归一同核心模板)消费页序列直出 PDF 文档字节。典型调用(文档管线
// 编译产物整卷渲染):
//
//	result, err := paginate.NewDocumentCompiler(nil).Compile(frames, dataset, chain)
//	pdf, err := pdfrenderer.RenderPages(ctx, pdfrenderer.NewRenderer(rs), result.Canvases)
//
// 错误面与核心多页模板一致:物化阶段沿用 resolver 既有 sentinel(errors.Is 判定);
// 绘制阶段普通包装 error;ctx 取消/超时 → ErrRenderCancelled / ErrRenderDeadlineExceeded;
// 空页序列属调用侧用法错误(不设 code)。渲染成功而产物非 []byte 不可能发生
// (本 module End 契约恒 []byte),泛型断言仅在误传外部 Backend 实现时兜底
func RenderPages(ctx context.Context, r *Renderer, pages []*canvas.Canvas) ([]byte, error) {
	return RenderPagesAs[[]byte](ctx, r, pages)
}

// RenderPagesAs 多页泛型入口(镜像 renderer.RenderAs 消费面):消费任意 Backend
// 的多页渲染产物并断言为期望类型 T。渲染错误原样透传(解码/填充/绘制三阶段
// 错误面不变);渲染成功后类型不符属调用侧用法错误——期望类型与后端产物约定
// 不一致,报普通 error(断言发生在渲染成功后,不设 code)
func RenderPagesAs[T any](ctx context.Context, backend renderer.Backend, pages []*canvas.Canvas) (T, error) {
	product, err := renderer.RenderPages(ctx, backend, pages)
	if err != nil {
		var zero T
		return zero, err
	}
	return assertProduct[T](backend, product)
}

// assertProduct 渲染成功后的产物断言:核心 renderer 的 assertProduct 未导出,
// 按 renderas.go 同一模式自组(工票 01 Answer 的分派)——消息三要素一致:
// 后端类型/实际产物类型/期望类型,直接定位改哪边
func assertProduct[T any](backend renderer.Backend, product any) (T, error) {
	if typed, ok := product.(T); ok {
		return typed, nil
	}
	var zero T
	return zero, fmt.Errorf("渲染产物类型不符: 后端 %T 产物为 %T, 期望 %s", backend, product, reflect.TypeFor[T]())
}

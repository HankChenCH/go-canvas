package renderer

// 渲染产物泛型辅助(工票13,ADR-0010):产物契约维持「类型由后端决定」,
// any 返回值的弱约束收敛为调用方显式声明期望产物类型——断言集中一处,
// 类型不符报可读错误,替代调用方散落的裸类型断言。

import (
	"context"
	"fmt"
	"reflect"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
)

// RenderAs 渲染整棵结构树并把产物断言为期望类型 T。内部复用
// New(backend, nil) 模板路径(nil resolver → 默认物化器),调用面形态:
//
//	renderer.RenderAs[*image.NRGBA](ctx, imagerenderer.NewRenderer(nil), c)
//
// 渲染错误原样透传(解码/填充/绘制三阶段错误面不变);渲染成功后类型不符
// 属调用侧用法错误,报普通 error。
func RenderAs[T any](ctx context.Context, backend Backend, c *canvas.Canvas) (T, error) {
	product, err := New(backend, nil).Render(ctx, c)
	if err != nil {
		var zero T
		return zero, err
	}
	return assertProduct[T](backend, product)
}

// RenderLayerAs 以图层自身尺寸为渲染面渲染单图层并把产物断言为期望类型 T,
// 语义同 RenderLayer(不做显隐过滤,物化时机同 Render 惰性);错误面同 RenderAs
func RenderLayerAs[T any](ctx context.Context, backend Backend, l layer.Layer) (T, error) {
	product, err := New(backend, nil).RenderLayer(ctx, l)
	if err != nil {
		var zero T
		return zero, err
	}
	return assertProduct[T](backend, product)
}

// assertProduct 渲染成功后的产物断言:类型不符说明期望类型与后端产物约定
// 不一致,属调用侧用法错误而非渲染失败——不进三阶段错误面(无 sentinel),
// 消息列明后端类型/实际产物类型/期望类型,直接定位改哪边
func assertProduct[T any](backend Backend, product any) (T, error) {
	if typed, ok := product.(T); ok {
		return typed, nil
	}
	var zero T
	return zero, fmt.Errorf("渲染产物类型不符: 后端 %T 产物为 %T, 期望 %s", backend, product, reflect.TypeFor[T]())
}

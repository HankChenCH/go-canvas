// Package paginate 分页与文档编译(spec §4/§10):编译期纯结构管线,
// 消费 hydrate 后置不变量,无 I/O、无求值、零第三方依赖。
//
// 两个管线入口(PHP 权威端逐镜像):
//   - Paginator.Paginate:单画布分页 Canvas 1→n(工票 10 实现装箱/溢出/几何语义)
//   - DocumentCompiler.Compile:N 帧 × 1 dataset × 流链声明的文档编译(ADR 0009)
//
// 工票 08 仅钉契约面(wire 类型 + 入口桩),把两份语义 fixture 变成红绿预言机;
// 链校验归工票 09,分页/文档语义归工票 10。fixture 见 testdata/(PHP 导出,
// runner = semantics_test.go,make semantics)。
package paginate

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/hankchen/go-canvas/canvas"
)

// ErrNotImplemented 契约面桩标记(工票 08):随工票 09/10 落地移除。
// 非三端契约 code——调用侧临时态,普通可读 error 即可(AGENTS.md 错误约定豁免条)
var ErrNotImplemented = errors.New("paginate: 管线未实现(工票 09/10 落地)")

// paginate 段稳定 code(spec §4.2.6,三端一致性锚点;消息可改、code 不可改)。
// 报错位随工票 10 落地:装箱溢出 / 驱动表目标 / 页几何(构造期 fail-fast)
var (
	ErrContentOverflow       = errors.New("content_overflow")
	ErrPaginateTargetInvalid = errors.New("paginate_target_invalid")
	ErrPageGeometryInvalid   = errors.New("page_geometry_invalid")
)

// PaginateOptions 分页选项(wire 面 {pageHeight, truncate?},PHP
// CanvasPaginator::fromOptions 承载形态):页宽恒 = 源画布宽,页高是唯一几何参数。
// PageHeight nil = 分页关(单页直通 + 溢出判定,页底 = 源画布高);非 nil ≤ 0 =
// page_geometry_invalid(裁决随工票 10)。0 与缺席必须可区分,故用指针。
type PaginateOptions struct {
	PageHeight *int `json:"pageHeight"`
	Truncate   bool `json:"truncate"`
}

// UnmarshalJSON 容忍 PHP 导出端的空数组编码:fixture 里缺省 options 序列化为 []
// (PHP 空关联数组即 []),与 null 同义空选项;对象形态未知键忽略(fromOptions 留演进空间同款)
func (o *PaginateOptions) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if string(trimmed) == "null" || string(trimmed) == "[]" {
		*o = PaginateOptions{}
		return nil
	}
	type alias PaginateOptions
	return json.Unmarshal(trimmed, (*alias)(o))
}

// Paginator 分页器(PHP CanvasPaginator 镜像,spec §4.2):Canvas 1→n 编译期纯函数。
// 只拆驱动表的行序列(行是唯一装箱原子、永不切开),表壳每页重画、非表图层逐页复制;
// 顶层表 0 张 = 单页直通、≥ 2 张 = paginate_target_invalid;零行 = 1 页空壳、永不产空页。
//
// 工票 08 仅契约面:装箱/溢出/几何语义归工票 10(届时 page_geometry_invalid
// 收敛为构造期 fail-fast,PHP 同款);页几何校验当前位置由工票 10 定,
// code 是唯一契约、在哪个方法抛出不是。
type Paginator struct {
	opts PaginateOptions
}

// NewPaginator 构造分页器
func NewPaginator(opts PaginateOptions) *Paginator {
	return &Paginator{opts: opts}
}

// Paginate 分页:页序列(分页关 = 单元素)。
// 输入前置 = hydrate 后置不变量(高度定稿);分页不重演填充、不依赖资源物化
func (p *Paginator) Paginate(c *canvas.Canvas) ([]*canvas.Canvas, error) {
	return nil, ErrNotImplemented
}

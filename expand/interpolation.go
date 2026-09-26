// Package expand 展开步骤(TableLayer V2,spec §4.2):渲染前的独立纯结构步骤,
// expand(Canvas, dataset) → Canvas——全画布标记字段求值 + 模板表实例化,
// 产出具体树(无模板、无表达式、高度定稿)。数据维度前置、资源维度后移
// (渲染期物化),两维度分离;本包为零依赖纯结构侧(不引渲染/图像库)。
//
// 实现走 graph 层改写 + canvas.FromGraph 重建:V1 尺寸耦合重放由解码路径承担
// (行高定稿/固定格压平免费来自 TableFromGraph→AddRow,PHP 权威实现同款策略)。
package expand

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// 展开期错误稳定 code(spec §5.2,三端一致性抓手:消息可本地化,code 稳定;
// PHP 侧对位 ExpandException.getErrorCode,errors.Is 判定)
var (
	// ErrRowsPathInvalid rowsPath 指向缺键/null/非数组(spec §4.4,结构性错误 fail-fast)
	ErrRowsPathInvalid = errors.New("rows_path_invalid")
	// ErrExpressionRowOutsideLoop row. 前缀出现在非行上下文(spec §3.2,
	// row 命名空间不存在;模板复制到表外是典型错误)
	ErrExpressionRowOutsideLoop = errors.New("expression_row_outside_loop: row. 前缀出现在非行上下文")
	// ErrReservedRootKey 数据集根级键名 row 或 $ 前缀(spec §3.2,{{row.x}} 的 row 段歧义)
	ErrReservedRootKey = errors.New("reserved_root_key")
	// ErrExpressionEmptyResource 资源类字段缺字段/null/求值空串
	// (spec §3.4,空 URL/空码无意义,fail-fast 优于错误后移到物化)
	ErrExpressionEmptyResource = errors.New("expression_empty_resource")
	// ErrExpressionTypeMismatch 数组/对象落到标量位或标量中途下钻(spec §3.4)
	ErrExpressionTypeMismatch = errors.New("expression_type_mismatch")
	// ErrExpressionSyntaxError 空表达式片段/非法路径(求值器内部 code,spec §5.2 注)
	ErrExpressionSyntaxError = errors.New("expression_syntax_error")
)

// ExpressionEvaluator 表达式求值器注入缝(spec §3.6,与 PHP
// Contracts/ExpressionEvaluatorInterface 同构):门控(valueType 标记)与字段类
// 错误策略留在展开器侧、不可被替换;上下文形状为契约层钉定(spec §3.2)——
// "row" 键 = 行命名空间、裸名 = 数据集根键、$index(自 1 起)/$root 保留名。
// 缺字段/null 信号 = 空串(展开器按字段类施加兜底/报错);
// 类型不匹配返回 expression_type_mismatch 错误
type ExpressionEvaluator interface {
	Evaluate(template string, context map[string]any) (string, error)
}

// InterpolationEvaluator 默认求值器:mustache 风格外壳 + 受限表达式子集
// (零依赖纯 Go,spec §3.6;可注入替换以服务更复杂场景)。
//
// v2 子集 = 变量点路径取值:{{row.x}} / {{row.user.city}} / {{根键}} /
// {{$index}} / {{$root.根键}};不含函数/条件/格式化(语义归求值器 spec effort,
// 届时以共享 fixture 钉死)。{{ }} 划定表达式片段,其余字面直通;
// 不含 {{ 的标记串 = 整串字面;未闭合的 {{ 按字面处理
type InterpolationEvaluator struct{}

// NewInterpolationEvaluator 构造默认求值器
func NewInterpolationEvaluator() *InterpolationEvaluator { return &InterpolationEvaluator{} }

// Evaluate 求值插值模板(spec §3.2 上下文形状契约见 ExpressionEvaluator)
func (e *InterpolationEvaluator) Evaluate(template string, context map[string]any) (string, error) {
	var out strings.Builder
	pos := 0
	for {
		open := strings.Index(template[pos:], "{{")
		if open < 0 {
			break
		}
		open += pos
		close := strings.Index(template[open+2:], "}}")
		if close < 0 {
			break // 未闭合:剩余全字面
		}
		close += open + 2

		out.WriteString(template[pos:open])
		result, err := resolveFragment(strings.TrimSpace(template[open+2:close]), context)
		if err != nil {
			return "", err
		}
		out.WriteString(result)
		pos = close + 2
	}
	out.WriteString(template[pos:])
	return out.String(), nil
}

// resolveFragment 解析单个表达式片段:按 . 切分,head 分派命名空间
func resolveFragment(expr string, context map[string]any) (string, error) {
	if expr == "" {
		return "", fmt.Errorf("%w: 空表达式片段", ErrExpressionSyntaxError)
	}
	segments := strings.Split(expr, ".")
	for _, segment := range segments {
		if segment == "" {
			return "", fmt.Errorf("%w: 非法路径 %s", ErrExpressionSyntaxError, expr)
		}
	}
	head, rest := segments[0], segments[1:]

	switch head {
	case "row":
		// row. 前缀 = 当前行命名空间;非行上下文(无 row 键)= 报错(spec §3.2)
		row, ok := context["row"]
		if !ok {
			return "", ErrExpressionRowOutsideLoop
		}
		return navigateValue(row, rest, expr)
	case "$root":
		// $root 显式取根,与裸名等价:下钻整个上下文
		return navigateValue(context, rest, expr)
	default:
		// 裸名(含 $index 等 $ 保留名)= 上下文根键
		value, ok := context[head]
		if !ok {
			value = nil
		}
		return navigateValue(value, rest, expr)
	}
}

// navigateValue 点路径下钻:缺键/null 记缺字段(空串信号);标量中途下钻或
// 数组/对象落到标量位记类型不匹配(fail-fast)
func navigateValue(value any, segments []string, expr string) (string, error) {
	for _, segment := range segments {
		if value == nil {
			return "", nil // 中途缺失与缺字段同信号
		}
		object, ok := value.(map[string]any)
		if !ok {
			return "", fmt.Errorf("%w: %s", ErrExpressionTypeMismatch, expr)
		}
		next, exists := object[segment]
		if !exists {
			return "", nil // 缺字段信号
		}
		value = next
	}
	switch value.(type) {
	case map[string]any, []any:
		return "", fmt.Errorf("%w: %s", ErrExpressionTypeMismatch, expr)
	}
	return stringify(value), nil
}

// stringify 字符串化(PHP (string) 语义):null → 空串、bool → 1/空串;
// 浮点按最短 f 格式——常规区间与 PHP 一致,极端浮点(科学计数法阈值)的
// 格式化语义归求值器 spec effort,不入共享 fixture(见 fixture meta.notes)
func stringify(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		if v {
			return "1"
		}
		return ""
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

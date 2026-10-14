// Package hydrate 数据填充步骤(TableLayer V2,spec §4.2):渲染前的独立纯结构步骤,
// hydrate(Canvas, dataset) → Canvas——全画布标记字段求值 + 模板表实例化,
// 产出具体树(无模板、无表达式、高度定稿)。数据维度前置、资源维度后移
// (渲染期物化),两维度分离;本包为零依赖纯结构侧(不引渲染/图像库)。
//
// 实现走 graph 层改写 + canvas.FromGraph 重建:V1 尺寸耦合重放由解码路径承担
// (行高定稿/固定格压平免费来自 TableFromGraph→AddRow,PHP 权威实现同款策略)。
package hydrate

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// 填充期错误稳定 code(spec §5.2,三端一致性抓手:消息可本地化,code 稳定;
// PHP 侧对位 HydrateException.getErrorCode,errors.Is 判定)
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
	// ErrExpressionSingleResourceRef 资源表达式单值门违例(ADR 0020):已标记 Image/Qr 的
	// expression 必须恰为一个 {{...}} 闭合片段 + 裸点路径(禁多片段拼接/片段外字面/
	// 内建函数调用/空片段;格式化标量不是数据端备好的资源引用)
	ErrExpressionSingleResourceRef = errors.New("expression_resource_single_ref")
	// ErrExpressionTypeMismatch 数组/对象落到标量位或标量中途下钻(spec §3.4)
	ErrExpressionTypeMismatch = errors.New("expression_type_mismatch")
	// ErrExpressionSyntaxError 空表达式片段/非法路径/未知函数/参数个数/调用结构错
	// (求值器内部 code,spec §5.2:签名与文法面确定性错误全部并入,零新 code)
	ErrExpressionSyntaxError = errors.New("expression_syntax_error")
)

// ExpressionEvaluator 表达式求值器注入缝(spec §3.6,与 PHP
// Contracts/ExpressionEvaluatorInterface 同构):门控(valueType 标记)与字段类
// 错误策略留在填充器侧、不可被替换;上下文形状为契约层钉定(spec §3.2)——
// "row" 键 = 行命名空间、裸名 = 数据集根键、$index(自 1 起)/$root 保留名。
// 缺字段/null 信号 = 空串(填充器按字段类施加兜底/报错);
// 类型不匹配返回 expression_type_mismatch 错误
type ExpressionEvaluator interface {
	Evaluate(template string, context map[string]any) (string, error)
}

// InterpolationEvaluator 默认求值器:mustache 风格外壳 + 受限表达式子集
// (零依赖纯 Go,spec §3.6;可注入替换以服务更复杂场景)。
//
// v2 子集 = 变量点路径取值 + 内建函数调用(v1 白名单 date/number/fixed/percent):
// {{row.x}} / {{row.user.city}} / {{根键}} / {{$index}} / {{$root.根键}} /
// {{date(row.d, 'YYYY年MM月DD日')}} / {{number(row.v, 2)}} / {{fixed(row.f)}} /
// {{percent(row.p)}}。语法与语义钉稿 = .scratch/expression-type-methods/spec.md,
// 三端共享 fixture 锁默认实现(函数语义面与 PHP DateFormat/NumberFormat 同构)。
//
// 分轴哲学(spec §0):值面宽松——缺字段/null = 空串信号、不可解析 = 原文直通
// (打印物场景数据不丢);文法/签名面 fail-fast——未知函数/参数个数/结构错是
// 数据无关的确定性错误,报错优于整批静默输出错误内容(expression_syntax_error,
// 零新 code)。{{ }} 划定表达式片段,其余字面直通;不含 {{ 的标记串 = 整串字面;
// 未闭合的 {{ 按字面处理;首个 }} 收口(引号不感知,片段内禁 }} 字面)
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
		result, err := resolveFragment(phpTrim(template[open+2:close]), context)
		if err != nil {
			return "", err
		}
		out.WriteString(result)
		pos = close + 2
	}
	out.WriteString(template[pos:])
	return out.String(), nil
}

// phpTrim PHP trim() 同字符集(空格 \t \n \r \0 \v):不用 Unicode 的
// strings.TrimSpace——非 ASCII 空白(如 NBSP)按字节属于键名,双端字节对齐
func phpTrim(s string) string {
	return strings.Trim(s, " \t\n\r\x00\x0B")
}

// fragmentHeadLen 片段首 token 字节长(PCRE [^()',.\s]+ 的字节面等价;
// \s = 空格 \t \n \v \f \r)。多字节字符(如中文函数名)按字节直通
func fragmentHeadLen(expr string) int {
	i := 0
	for i < len(expr) {
		switch expr[i] {
		case '(', ')', ',', '\'', '.', ' ', '\t', '\n', '\v', '\f', '\r':
			return i
		}
		i++
	}
	return i
}

// resolveFragment 解析单个表达式片段:片段分类(spec §1.2 ⑦)——trim 后首 token
// 其后紧跟可选空白与 ( 即调用,否则路径。函数白名单校验后置:foo(bar)/日期(bar)/
// foo (bar) 均按调用分类(未知函数 fail-fast,消灭「含 ( 的怪键名静默输出空串」),
// {{date}} 无 ( 恒为路径,数据集根键叫 date 合法
func resolveFragment(expr string, context map[string]any) (string, error) {
	if expr == "" {
		return "", fmt.Errorf("%w: 空表达式片段", ErrExpressionSyntaxError)
	}
	headLen := fragmentHeadLen(expr)
	if strings.HasPrefix(strings.TrimLeft(expr[headLen:], " \t\n\v\f\r"), "(") {
		return evaluateCall(expr, headLen, context)
	}
	return resolvePathFragment(expr, context)
}

// resolvePathFragment 路径片段:既有 v2 点路径取值,行为逐字节不变
func resolvePathFragment(expr string, context map[string]any) (string, error) {
	segments := strings.Split(expr, ".")
	for _, segment := range segments {
		if segment == "" {
			return "", fmt.Errorf("%w: 非法路径 %s", ErrExpressionSyntaxError, expr)
		}
	}
	base, rest, err := pathBase(segments, context)
	if err != nil {
		return "", err
	}
	value, err := drillRaw(base, rest, expr)
	if err != nil {
		return "", err
	}
	return stringify(value), nil
}

// pathBase head 三分类(既有钉稿):row. = 当前行命名空间(非行上下文报错)、
// $root = 显式取根、裸名(含 $index 等 $ 保留名)= 上下文根键。
// 片段路径与调用首参共用;返回 [基底值, 余下段]
func pathBase(segments []string, context map[string]any) (any, []string, error) {
	head, rest := segments[0], segments[1:]
	switch head {
	case "row":
		// row. 前缀 = 当前行命名空间;非行上下文(无 row 键)= 报错(spec §3.2)
		row, ok := context["row"]
		if !ok {
			return nil, nil, ErrExpressionRowOutsideLoop
		}
		return row, rest, nil
	case "$root":
		// $root 显式取根,与裸名等价:下钻整个上下文
		return context, rest, nil
	default:
		// 裸名(含 $index 等 $ 保留名)= 上下文根键;缺键 = nil 信号
		return context[head], rest, nil
	}
}

// drillRaw 点路径下钻(返回原始值,路径片段与调用首参共用):缺键/null 记缺字段
// (nil 信号);标量中途下钻记类型不匹配(fail-fast)。容器中途下钻 = PHP
// is_array 同语义缺字段信号:map 缺键 → nil;数组按 PHP 数字串键强制转换命中
// 下标取值,其余键一律缺字段(php -r 实证钉定)。容器落到终点 = 类型不匹配
func drillRaw(value any, segments []string, expr string) (any, error) {
	for _, segment := range segments {
		if value == nil {
			return nil, nil // 中途缺失与缺字段同信号
		}
		switch v := value.(type) {
		case map[string]any:
			next, ok := v[segment]
			if !ok {
				return nil, nil // 缺字段信号
			}
			value = next
		case []any:
			index, ok := phpArrayIndex(segment)
			if !ok || index < 0 || index >= len(v) {
				return nil, nil // 数组键不存在 = 缺字段信号
			}
			value = v[index]
		default:
			return nil, fmt.Errorf("%w: %s", ErrExpressionTypeMismatch, expr)
		}
	}
	switch value.(type) {
	case map[string]any, []any:
		return nil, fmt.Errorf("%w: %s", ErrExpressionTypeMismatch, expr)
	}
	return value, nil
}

// phpArrayIndex PHP 数字串键强制转换语义(数组下标):规范十进制串(无前导零、
// 非负)→ int 下标;前导零("007")/负串/超 int 位串 = 字符串键,PHP 不转换
func phpArrayIndex(segment string) (int, bool) {
	digits := segment
	if strings.HasPrefix(segment, "-") {
		digits = segment[1:]
	}
	if digits == "" {
		return 0, false
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return 0, false
		}
	}
	if strings.HasPrefix(segment, "-") {
		return 0, false // 负键串不存在于 JSON 列表("-0" 同为字符串键)
	}
	if len(digits) > 1 && digits[0] == '0' {
		return 0, false // 前导零 = 字符串键,PHP 不转换
	}
	index, err := strconv.Atoi(segment)
	if err != nil {
		return 0, false // 超出 int 范围 = 字符串键
	}
	return index, true
}

// ─── 内建函数调用(v1 白名单 date/number/fixed/percent,spec §1.2/§2) ───

// builtinFunctions v1 白名单:名 => [最小参, 最大参](大小写敏感、不保留字化:
// 函数名只在调用位生效,字段名只在路径位)
var builtinFunctions = map[string][2]int{
	"date":    {1, 2},
	"number":  {1, 2},
	"fixed":   {1, 2},
	"percent": {1, 2},
}

// callArg 实参解析产物:kind 'p' 路径 | 's' 字符串 | 'n' 数字;
// value = 路径原文 | 字符串内容(翻倍转义后)| 数字原文
type callArg struct {
	kind  byte
	value string
}

func callSyntaxErr(message string) error {
	return fmt.Errorf("%w: %s", ErrExpressionSyntaxError, message)
}

// evaluateCall 调用片段:白名单校验(未知函数 fail-fast)→ 实参表解析 →
// 签名校验 → 分派求值
func evaluateCall(expr string, nameLen int, context map[string]any) (string, error) {
	name := expr[:nameLen]
	arity, known := builtinFunctions[name]
	if !known {
		return "", callSyntaxErr("未知函数 " + name)
	}
	args, err := parseCallArgs(expr, nameLen)
	if err != nil {
		return "", err
	}
	if len(args) < arity[0] || len(args) > arity[1] {
		return "", callSyntaxErr("参数个数不符 " + name + "(...)")
	}
	// 首参文法层钉死为路径:'格式化常量' 是伪场景,误用更早一层被抓住(spec §1.2 ①)
	if args[0].kind != 'p' {
		return "", callSyntaxErr("首参必须为路径")
	}
	if name == "date" {
		return callDate(name, args, context)
	}
	return callNumber(name, args, context)
}

// parseCallArgs 实参表解析(spec §1.1):实参 := 路径 | 字符串 | 数字,实参间
// WS* ',' WS*;尾逗号与空参数位 = 语法错;字符串仅单引号、” 翻倍转义;数字无符号
// 无指数;零参调用文法合法(为 now() 留门,个数归签名层拒)
func parseCallArgs(expr string, nameLen int) ([]callArg, error) {
	pos := skipCallWs(expr, nameLen)
	if pos >= len(expr) || expr[pos] != '(' {
		return nil, callSyntaxErr("括号不平衡")
	}
	pos = skipCallWs(expr, pos+1)

	var args []callArg
	if pos < len(expr) && expr[pos] == ')' {
		pos++
	} else {
		for {
			if pos >= len(expr) {
				return nil, callSyntaxErr("括号不平衡")
			}
			arg, next, err := parseCallArg(expr, pos)
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			pos = skipCallWs(expr, next)
			if pos >= len(expr) {
				return nil, callSyntaxErr("括号不平衡")
			}
			if expr[pos] == ',' {
				pos = skipCallWs(expr, pos+1)
				if pos < len(expr) && expr[pos] == ')' {
					return nil, callSyntaxErr("空参数位")
				}
				continue
			}
			if expr[pos] == ')' {
				pos++
				break
			}
			return nil, callSyntaxErr("调用实参语法错")
		}
	}
	if skipCallWs(expr, pos) != len(expr) {
		return nil, callSyntaxErr("调用后有多余内容")
	}
	return args, nil
}

// parseCallArg 单个实参:按首字符三分派(' 串 / 数字 / 路径),返回消费终点。
// 路径吞至顶层 ',' 或 ')' 为止后 trim(段内空白属于键名,存量画布字节兼容;
// 缺逗号不构成结构错——date(row.d 'YYYY') 是键名 "d 'YYYY'" 的单参调用,
// fixture path-with-paren-stays-path 锁此边界)
func parseCallArg(expr string, pos int) (callArg, int, error) {
	ch := expr[pos]

	// 字符串字面量:仅单引号,'' 翻倍转义(ICU/SQL 风格),其余任意字符合法
	if ch == '\'' {
		pos++
		var buf []byte
		for {
			if pos >= len(expr) {
				return callArg{}, pos, callSyntaxErr("字符串字面量未闭合")
			}
			if expr[pos] == '\'' {
				if pos+1 < len(expr) && expr[pos+1] == '\'' {
					buf = append(buf, '\'')
					pos += 2
					continue
				}
				pos++
				break
			}
			buf = append(buf, expr[pos])
			pos++
		}
		return callArg{kind: 's', value: string(buf)}, pos, nil
	}

	// 数字字面量:无符号十进制 数字+ ['.' 数字+],无指数(负数/布尔字面量写了即语法错)
	if ch >= '0' && ch <= '9' {
		start := pos
		for pos < len(expr) && expr[pos] >= '0' && expr[pos] <= '9' {
			pos++
		}
		if pos < len(expr) && expr[pos] == '.' && pos+1 < len(expr) && expr[pos+1] >= '0' && expr[pos+1] <= '9' {
			pos++
			for pos < len(expr) && expr[pos] >= '0' && expr[pos] <= '9' {
				pos++
			}
		}
		return callArg{kind: 'n', value: expr[start:pos]}, pos, nil
	}

	// 路径:至顶层 ',' 或 ')' 止后 trim
	start := pos
	for pos < len(expr) && expr[pos] != ',' && expr[pos] != ')' {
		pos++
	}
	raw := phpTrim(expr[start:pos])
	if raw == "" {
		return callArg{}, pos, callSyntaxErr("空参数位")
	}
	return callArg{kind: 'p', value: raw}, pos, nil
}

// skipCallWs 调用文法空白(与 PHP skipWs 同集:空格 \t \n \r;注意片段级
// trim 与首 token 分类用集不同,见 phpTrim / fragmentHeadLen)
func skipCallWs(expr string, pos int) int {
	for pos < len(expr) {
		switch expr[pos] {
		case ' ', '\t', '\n', '\r':
			pos++
		default:
			return pos
		}
	}
	return pos
}

// callDate date 求值(spec §3):可解析输入按格式渲染;缺失/null/空串 = 空串;
// 不可解析/数值 = 原文直通;bool/容器落参数位 = type_mismatch
func callDate(name string, args []callArg, context map[string]any) (string, error) {
	format := dateDefaultFormat
	if len(args) > 1 {
		if args[1].kind != 's' {
			return "", callSyntaxErr(name + " 第二参须为字符串字面量")
		}
		format = dateExpandPreset(args[1].value)
	}

	value, err := resolveCallArgValue(args[0], context, name)
	if err != nil {
		return "", err
	}
	if value == nil {
		return "", nil
	}
	switch v := value.(type) {
	case string:
		if v == "" {
			return "", nil
		}
		dt, ok := parseDateISO(v)
		if !ok {
			return v, nil // 不可解析 = 原文直通
		}
		return formatDate(format, dt)
	default:
		// 数值:时间戳不收,按字符串化后原文直通(常规区间字符串化双端一致)
		return numberFromAny(v), nil
	}
}

// callNumber number/fixed/percent 求值(spec §4):严格十进制串与常规区间数值进
// 管线;缺失/null/空串 = 空串;非空不可解析 = 原文直通;bool/容器 = type_mismatch
func callNumber(name string, args []callArg, context map[string]any) (string, error) {
	var n *int
	if len(args) > 1 {
		if args[1].kind != 'n' {
			return "", callSyntaxErr(name + " 第二参须为数字字面量")
		}
		raw := args[1].value
		if !isAllDigits(raw) {
			return "", callSyntaxErr("数字参数须为整数 n=" + raw)
		}
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			parsed = math.MaxInt // 位串超 int:同 PHP int cast 饱和,按超界处理
		}
		if parsed > numberMaxN {
			return "", callSyntaxErr("数字参数超界 n=" + strconv.Itoa(parsed))
		}
		n = &parsed
	}

	value, err := resolveCallArgValue(args[0], context, name)
	if err != nil {
		return "", err
	}
	if value == nil {
		return "", nil
	}
	if text, ok := value.(string); ok {
		if text == "" {
			return "", nil
		}
		dec, ok := numberFromStrictString(text)
		if !ok {
			return text, nil // 不可解析 → 原文直通
		}
		return dispatchNumber(name, dec, n)
	}
	return dispatchNumber(name, numberFromAny(value), n)
}

// dispatchNumber 族内分派:number 缺省不定点(小数按已锁口径原样直出),
// fixed 缺省 n=2,percent 缺省 n=0(spec §2)
func dispatchNumber(name string, dec string, n *int) (string, error) {
	switch name {
	case "number":
		return numberGroup(dec, n), nil
	case "fixed":
		digits := 2
		if n != nil {
			digits = *n
		}
		return numberFixed(dec, digits), nil
	default: // percent
		digits := 0
		if n != nil {
			digits = *n
		}
		return numberPercent(dec, digits), nil
	}
}

// resolveCallArgValue 调用首参取原始值:复用路径遍历(缺键/null → nil 信号、
// 结构错同码),bool/数组/对象落函数参数位 = type_mismatch(bool 字符串化双端
// 不一致,直通无法对齐)
func resolveCallArgValue(arg callArg, context map[string]any, name string) (any, error) {
	path := arg.value
	segments := strings.Split(path, ".")
	for _, segment := range segments {
		if segment == "" {
			return nil, callSyntaxErr("非法路径 " + path)
		}
	}
	base, rest, err := pathBase(segments, context)
	if err != nil {
		return nil, err
	}
	value, err := drillRaw(base, rest, path)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, nil
	}
	switch value.(type) {
	case bool, map[string]any, []any:
		return nil, fmt.Errorf("%w: %s(%s)", ErrExpressionTypeMismatch, name, path)
	}
	return value, nil
}

// isAllDigits PHP ctype_digit 同义:全 ASCII 数字(含 '.' 即非整数 → 语法错)
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
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

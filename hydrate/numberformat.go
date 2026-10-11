package hydrate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// 数字族格式化:十进制字符串语义(spec §4)——舍入定义在值的最短往返十进制
// 表示上,不把二进制浮点喂给原生 round(2.675 → 2.68 而非 2.67);half-up 朝
// +∞(正数常规四舍五入,tie 时负数朝零)。全部运算在十进制字符串上进行:
// 乘 100 = 小数点右移,不引入二进制误差。零渲染依赖、纯 stdlib

// numberMaxN n 参数上界(0-8,spec §2;语义层校验在求值器,超界 fail-fast)
const numberMaxN = 8

// numberFromAny 数值 → 最短往返十进制串(常规区间,spec §4.3)。float64 经
// FormatFloat 'f' -1 取最短往返表示(与 PHP json_encode serialize_precision=-1
// 同为最短往返算法;'f' 不出指数形态,与 PHP 指数串经 normalize 展开结果一致);
// 极端浮点不入 contract、不承诺双端一致
func numberFromAny(value any) string {
	switch v := value.(type) {
	case float64:
		return numberNormalize(strconv.FormatFloat(v, 'f', -1, 64))
	case json.Number:
		return numberNormalize(v.String())
	case int:
		return numberNormalize(strconv.Itoa(v))
	case int64:
		return numberNormalize(strconv.FormatInt(v, 10))
	default:
		return numberNormalize(fmt.Sprint(v))
	}
}

var numberStrictRe = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

// numberFromStrictString 严格十进制串 → 归一十进制串(前导零/尾零按值归一,
// '007' → '7',spec §4.3);非严格形态(科学计数法/千分位/含空白等)→ false,
// 由求值器按原文直通
func numberFromStrictString(value string) (string, bool) {
	if !numberStrictRe.MatchString(value) {
		return "", false
	}
	return numberNormalize(value), true
}

// numberGroup number(千分位):只在整数部分插 , 分组符,小数点固定 .;
// n 给定先定点 n 位补零再分组,缺省小数原样直出不补零不舍入(spec §4.1②)
func numberGroup(dec string, n *int) string {
	neg, intPart, frac := numberSplit(dec)
	if n != nil {
		neg, intPart, frac = numberQuantize(neg, intPart, frac, *n)
	}
	return numberJoin(neg, groupInteger(intPart), frac)
}

// numberFixed fixed(定点补零):不分组,仅定点补零;n=0 无小数点(spec §4.1③)
func numberFixed(dec string, n int) string {
	neg, intPart, frac := numberSplit(dec)
	neg, intPart, frac = numberQuantize(neg, intPart, frac, n)
	return numberJoin(neg, intPart, frac)
}

// numberPercent percent(百分比):×100(小数点右移两位)后定点 n 位再追加 %
// (spec §4.1④;0.826 → 83%、percent(x,2): 0.826 → 82.60%)
func numberPercent(dec string, n int) string {
	neg, intPart, frac := numberSplit(dec)
	// 小数点右移两位:整数并入原小数前两位,余下小数位为新的小数部分
	digits := strings.TrimLeft(intPart+(frac + "00")[:2], "0")
	rest := ""
	if len(frac) > 2 {
		rest = frac[2:]
	}
	if digits == "" {
		digits = "0"
	}
	neg, intPart, frac = numberQuantize(neg, digits, rest, n)
	return numberJoin(neg, intPart, frac) + "%"
}

// numberQuantize 定点 n 位(half-up 朝 +∞ = floor(v·10ⁿ + 0.5)·10⁻ⁿ):丢弃首位
// >5 或(=5 且尾部有非零)进位;=5 精确 tie 时正数进位、负数朝零(朝 +∞);
// 小数位补零到恰好 n 位(spec §4.2)
func numberQuantize(neg bool, intPart, frac string, n int) (bool, string, string) {
	padded := frac + strings.Repeat("0", n)
	kept := padded[:n]
	rest := ""
	if len(frac) > n {
		rest = frac[n:]
	}

	if rest != "" {
		first := rest[0]
		tail := rest[1:]
		exactTie := first == '5' && !strings.ContainsAny(tail, "123456789")
		up := first > '5' || (first == '5' && (!exactTie || !neg))
		if up {
			combined := numberAddOne(intPart + kept)
			if n > 0 {
				intPart = combined[:len(combined)-n]
				kept = combined[len(combined)-n:]
			} else {
				intPart = combined
				kept = ""
			}
		}
	}
	return neg, intPart, kept
}

// numberSplit 拆解 canonical 十进制串 → [负号, 整数位, 小数位]
func numberSplit(dec string) (bool, string, string) {
	neg := strings.HasPrefix(dec, "-")
	body := strings.TrimPrefix(dec, "-")
	intPart, frac := body, ""
	if dot := strings.IndexByte(body, '.'); dot >= 0 {
		intPart, frac = body[:dot], body[dot+1:]
	}
	return neg, intPart, frac
}

// numberJoin 组装输出:整数位去前导零;定点结果为 0 时去符号保留补零形态
// (不输出 -0,spec §4.1③)
func numberJoin(neg bool, intPart, frac string) string {
	intPart = strings.TrimLeft(intPart, "0")
	if intPart == "" {
		intPart = "0"
	}
	if intPart == "0" && strings.TrimRight(frac, "0") == "" {
		if frac == "" {
			return "0"
		}
		return "0." + frac
	}
	out := intPart
	if neg {
		out = "-" + out
	}
	if frac != "" {
		out += "." + frac
	}
	return out
}

var numberExpRe = regexp.MustCompile(`^([+-]?)(\d+)(?:\.(\d+))?[eE]([+-]?\d+)$`)
var numberPlainRe = regexp.MustCompile(`^([+-]?)(\d+)(?:\.(\d+))?$`)

// numberNormalize 归一:指数展开(如有)→ 前导零/尾零剥离 → 零去符号;
// 非数值形态原样返回(防御出 contract 输入)
func numberNormalize(s string) string {
	if m := numberExpRe.FindStringSubmatch(s); m != nil {
		exp, _ := strconv.Atoi(m[4])
		s = numberExpandExponent(m[1], m[2], m[3], exp)
	}
	m := numberPlainRe.FindStringSubmatch(s)
	if m == nil {
		return s
	}
	intPart := strings.TrimLeft(m[2], "0")
	if intPart == "" {
		intPart = "0"
	}
	frac := ""
	if m[3] != "" {
		frac = strings.TrimRight(m[3], "0")
	}
	if intPart == "0" && frac == "" {
		return "0"
	}
	out := intPart
	if frac != "" {
		out += "." + frac
	}
	if m[1] == "-" {
		out = "-" + out
	}
	return out
}

// numberExpandExponent 科学计数法展开为普通十进制串(PHP json_encode 大/小值端的
// 指数形态;Go 'f' 格式不产生指数,此为 normalize 防御面)
func numberExpandExponent(sign, intPart, frac string, exp int) string {
	digits := intPart + frac
	point := len(intPart) + exp
	var body string
	switch {
	case point <= 0:
		body = "0." + strings.Repeat("0", -point) + digits
	case point >= len(digits):
		body = digits + strings.Repeat("0", point-len(digits))
	default:
		body = digits[:point] + "." + digits[point:]
	}
	return sign + body
}

// numberAddOne 十进制串 +1:保留宽度逐位进位,溢出时扩展一位
func numberAddOne(digits string) string {
	b := []byte(digits)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] != '9' {
			b[i]++
			return string(b)
		}
		b[i] = '0'
	}
	return "1" + string(b)
}

// groupInteger 整数部分千分位分组:自右每 3 位插 ,(负号由调用方在分组符之外处理)
func groupInteger(intPart string) string {
	if len(intPart) <= 3 {
		return intPart
	}
	var out strings.Builder
	head := len(intPart) % 3
	if head > 0 {
		out.WriteString(intPart[:head])
		out.WriteByte(',')
	}
	for i := head; i < len(intPart); i += 3 {
		out.WriteString(intPart[i : i+3])
		if i+3 < len(intPart) {
			out.WriteByte(',')
		}
	}
	return out.String()
}

package hydrate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// 日期格式化:moment 风格 token 子集(14 个,语义自含)+ 方括号字面转义 +
// 3 种无时区 ISO 输入的严格解析(wall clock 语义,不转时区)。
// 钉稿见 .scratch/expression-type-methods/spec.md §3;token 表是三端契约的
// 权威面——PHP DateFormat 为导出源、fixture dateTokens 承载,本表内置同构表、
// 由 fixture 断言锁漂移(工票 06 ⑥ 双轨)。零渲染依赖(纯算术周几,零时区)

// dateDefaultFormat 单参调用缺省格式(等价预设 date,spec §3.4)
const dateDefaultFormat = "YYYY-MM-DD"

// datePresets 具名预设 v1:第二参整体等值命中时先展开为 token 串再走统一管线
// (纯糖,spec §3.4;'datetime X' 不命中预设,按 token 串解析)
var datePresets = map[string]string{
	"date":     "YYYY-MM-DD",
	"datetime": "YYYY-MM-DD HH:mm:ss",
}

// dateTokenTable token 表 v1(权威面):[token, 语义一句话],同字母长 token 在先
// (最长匹配优先)。语义串与 spec §3.1 逐字对应,随 fixture dateTokens 断言
var dateTokenTable = []struct {
	token     string
	semantics string
}{
	{"YYYY", "四位年，零填充 4 位"},
	{"YY", "年后两位，零填充 2 位"},
	{"MM", "两位月 01-12"},
	{"M", "月 1-12"},
	{"DD", "两位日 01-31"},
	{"D", "日 1-31"},
	{"HH", "两位时 00-23"},
	{"H", "时 0-23"},
	{"mm", "两位分 00-59"},
	{"m", "分 0-59"},
	{"ss", "两位秒 00-59"},
	{"s", "秒 0-59"},
	{"dddd", "周几中文全称：星期日…星期六"},
	{"ddd", "周几中文简称：周日…周六"},
}

// dateWeekFull / dateWeekShort 周几中文输出,下标 = Sakamoto 周几(0 = 星期日)
var dateWeekFull = [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}
var dateWeekShort = [...]string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

// dateExpandPreset 预设是纯糖:第二参整体等值命中预设名时展开为 token 串,
// 非整体等值按 token 串解析(要输出字面 "date" 用转义段 '[date]')
func dateExpandPreset(format string) string {
	if expanded, ok := datePresets[format]; ok {
		return expanded
	}
	return format
}

// dateParts 严格解析产物(wall clock 六元组,零时区依赖)
type dateParts struct {
	y, mo, d int
	h, mi, s int
}

var dateISORe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})(?:[ T](\d{2}):(\d{2}):(\d{2}))?$`)

// parseDateISO 严格解析 3 种无时区形态(spec §3.3):YYYY-MM-DD ｜
// YYYY-MM-DD HH:mm:ss ｜ YYYY-MM-DDTHH:mm:ss(仅大写 T)。位数严格、越界即
// 不可解析(2026-13-45 / 2026-02-30 / 24:00:00 均不收);时间戳与带时区后缀不收
func parseDateISO(value string) (dateParts, bool) {
	m := dateISORe.FindStringSubmatch(value)
	if m == nil {
		return dateParts{}, false
	}
	atoi := func(s string) int {
		n, _ := strconv.Atoi(s)
		return n
	}
	dt := dateParts{y: atoi(m[1]), mo: atoi(m[2]), d: atoi(m[3])}
	if m[4] != "" {
		dt.h, dt.mi, dt.s = atoi(m[4]), atoi(m[5]), atoi(m[6])
	}
	if !dateCheckdate(dt.y, dt.mo, dt.d) || dt.h > 23 || dt.mi > 59 || dt.s > 59 {
		return dateParts{}, false
	}
	return dt, true
}

// dateCheckdate PHP checkdate 同语义:年 1-32767、月 1-12、日按公历月表(闰年规则)
func dateCheckdate(y, mo, d int) bool {
	if y < 1 || y > 32767 || mo < 1 || mo > 12 || d < 1 {
		return false
	}
	return d <= dateDaysInMonth(y, mo)
}

// dateDaysInMonth 公历月天数(2 月闰年:能被 4 整除且不被 100 整除,或被 400 整除)
func dateDaysInMonth(y, mo int) int {
	switch mo {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	default:
		if y%4 == 0 && (y%100 != 0 || y%400 == 0) {
			return 29
		}
		return 28
	}
}

// formatDate 按 token 串渲染:最长匹配优先、大小写敏感;非 ASCII 字母字面直通
// (多字节按字节直通);方括号转义段至首个 ] 止(段内字面直通,无嵌套无再次转义),
// 未配对 [ 按普通字符字面输出(含 [ 自身,单遍扫描无回溯),孤立 ] 字面输出;
// 白名单外 ASCII 字母 fail-fast(expression_syntax_error,格式串与数据无关)
func formatDate(format string, dt dateParts) (string, error) {
	var out strings.Builder
	i, n := 0, len(format)
	for i < n {
		ch := format[i]
		switch {
		case ch == '[':
			end := strings.IndexByte(format[i+1:], ']')
			if end < 0 {
				out.WriteByte(ch) // 未配对 [ 字面直通,后续字符继续常规扫描
				i++
				continue
			}
			out.WriteString(format[i+1 : i+1+end])
			i += end + 2
		case ch == ']':
			out.WriteByte(ch) // 孤立 ] 字面输出
			i++
		case (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z'):
			run := 0
			for i+run < n && format[i+run] == ch {
				run++
			}
			token, ok := dateMatchToken(ch, run)
			if !ok {
				return "", fmt.Errorf("%w: 格式串含未知 token %c", ErrExpressionSyntaxError, ch)
			}
			out.WriteString(dateRenderToken(token, dt))
			i += len(token)
		default:
			out.WriteByte(ch) // 中文/标点/数字字面直通
			i++
		}
	}
	return out.String(), nil
}

// dateMatchToken 同字母 run 上取可整段消费的最长白名单 token(表内同字母长者在先);
// run 短于最短 token = 白名单外
func dateMatchToken(letter byte, runLen int) (string, bool) {
	for _, row := range dateTokenTable {
		if row.token[0] == letter && len(row.token) <= runLen {
			return row.token, true
		}
	}
	return "", false
}

// dateRenderToken 单 token 渲染(零填充位数钉死;年份经解析层 checkdate 恒 ≥ 1)
func dateRenderToken(token string, dt dateParts) string {
	pad2 := func(v int) string {
		if v < 10 {
			return "0" + strconv.Itoa(v)
		}
		return strconv.Itoa(v)
	}
	switch token {
	case "YYYY":
		return fmt.Sprintf("%04d", dt.y)
	case "YY":
		return pad2(dt.y % 100)
	case "MM":
		return pad2(dt.mo)
	case "M":
		return strconv.Itoa(dt.mo)
	case "DD":
		return pad2(dt.d)
	case "D":
		return strconv.Itoa(dt.d)
	case "HH":
		return pad2(dt.h)
	case "H":
		return strconv.Itoa(dt.h)
	case "mm":
		return pad2(dt.mi)
	case "m":
		return strconv.Itoa(dt.mi)
	case "ss":
		return pad2(dt.s)
	case "s":
		return strconv.Itoa(dt.s)
	case "dddd":
		return dateWeekFull[dateWeekday(dt.y, dt.mo, dt.d)]
	default: // "ddd"
		return dateWeekShort[dateWeekday(dt.y, dt.mo, dt.d)]
	}
}

// dateWeekday 周几(Sakamoto 算法,纯算术零时区依赖):0 = 星期日
func dateWeekday(y, m, d int) int {
	t := [...]int{0, 3, 2, 5, 0, 3, 5, 1, 4, 6, 2, 4}
	if m < 3 {
		y--
	}
	return (y + y/4 - y/100 + y/400 + t[m-1] + d) % 7
}

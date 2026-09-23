package imagerenderer

import (
	"fmt"
	"image/color"
	"strings"
)

// 颜色解析:wire 面与图层选项只产出 CSS 十六进制写法(PHP intervention 还接受
// 颜色名与 rgb() 函数串,本项目不使用),故仅支持 hex 形态,其余报错中止渲染

// parseColor 解析十六进制颜色为直 alpha 的 NRGBA。
// 支持 #rgb / #rgba / #rrggbb / #rrggbbaa,前导 # 可省略;3/4 位逐位翻倍后走同一累积路径
func parseColor(s string) (color.NRGBA, error) {
	h := strings.TrimPrefix(s, "#")
	switch len(h) {
	case 3, 4:
		doubled := make([]byte, 0, len(h)*2)
		for i := 0; i < len(h); i++ {
			doubled = append(doubled, h[i], h[i]) // 逐位翻倍:#f00 → ff0000
		}
		h = string(doubled)
	case 6, 8:
	default:
		return color.NRGBA{}, fmt.Errorf("无法解析颜色 %q: 支持 #rgb/#rgba/#rrggbb/#rrggbbaa", s)
	}

	var c [4]int
	for i := 0; i < len(h); i++ {
		v, err := hexDigit(h[i])
		if err != nil {
			return color.NRGBA{}, fmt.Errorf("无法解析颜色 %q: %w", s, err)
		}
		c[i/2] = c[i/2]*16 + v
	}
	if len(h) == 6 {
		c[3] = 255
	}
	return color.NRGBA{R: uint8(c[0]), G: uint8(c[1]), B: uint8(c[2]), A: uint8(c[3])}, nil
}

func hexDigit(b byte) (int, error) {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0'), nil
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10, nil
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10, nil
	default:
		return 0, fmt.Errorf("非法十六进制位 %q", string(b))
	}
}

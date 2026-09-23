package imagerenderer

// 颜色解析的表驱动测试:四种位宽 × 带/不带 #、非法形态报错

import (
	"image/color"
	"testing"
)

func TestParseColorHexForms(t *testing.T) {
	cases := []struct {
		in   string
		want color.NRGBA
	}{
		{"#f00", red},
		{"f00", red}, // 前导 # 可省略
		{"#f00f", color.NRGBA{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}},
		{"#00ff00", green},
		{"#0000ff80", color.NRGBA{R: 0x00, G: 0x00, B: 0xFF, A: 0x80}}, // 8 位含 alpha
		{"fff", white},
	}
	for _, tc := range cases {
		got, err := parseColor(tc.in)
		if err != nil {
			t.Errorf("parseColor(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseColor(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestParseColorInvalidAborts(t *testing.T) {
	for _, in := range []string{"nope", "#12", "#12345", "#zzzzzz", ""} {
		if _, err := parseColor(in); err == nil {
			t.Errorf("parseColor(%q) 应报错", in)
		}
	}
}

package pdfrenderer

// 颜色解析测试:hex 四形态 + 非法输入防线(与 image-renderer color_test.go
// 同一用例集,几何/颜色契约两后端防线对称)

import "testing"

func TestParseColor(t *testing.T) {
	cases := []struct {
		in   string
		want rgba
	}{
		{"#000000", rgba{0, 0, 0, 255}},
		{"ffffff", rgba{255, 255, 255, 255}},
		{"#ff0000", rgba{255, 0, 0, 255}},
		{"#f00", rgba{255, 0, 0, 255}},
		{"#abc", rgba{170, 187, 204, 255}},
		{"#ff000080", rgba{255, 0, 0, 128}},
		{"#f008", rgba{255, 0, 0, 136}},
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

func TestParseColorInvalid(t *testing.T) {
	for _, in := range []string{"", "#", "#ff", "#fffff", "#ff00000", "red", "#zzz", "#ff00zz"} {
		if _, err := parseColor(in); err == nil {
			t.Errorf("parseColor(%q) 应报错", in)
		}
	}
}

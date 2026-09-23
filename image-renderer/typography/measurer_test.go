package typography_test

// OpenTypeMeasurerFactory 测试:真实字体度量(advance+kern 累加)经核心度量器
// 工厂接缝注入。关键不变量:度量口径与渲染端 font.Drawer.MeasureString 一致
// (同一加载配置 + 同一累加语义),布局断行用的宽度与绘内对齐用的宽度同源。

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/hankchen/go-canvas/image-renderer/typography"
	"github.com/hankchen/go-canvas/text"
)

// seam 锁定:工厂函数类型与核心接缝对齐
var _ text.MeasurerFactory = typography.OpenTypeMeasurerFactory

// writeGoRegular 把 x/image 自带的 Go 字体落盘为测试字体文件(无需仓库内字体资产)
func writeGoRegular(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "goregular.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0o600); err != nil {
		t.Fatalf("写测试字体: %v", err)
	}
	return path
}

func TestOpenTypeMeasurerMatchesDrawerMeasureString(t *testing.T) {
	// 度量口径 = 渲染端落笔度量(font.Drawer.MeasureString,advance+kern):
	// 布局断行宽度与后端绘内对齐宽度必须同源,否则对齐锚点漂移
	path := writeGoRegular(t)
	const fontSize = 20.0

	face, err := typography.LoadFontFace(path, fontSize)
	if err != nil {
		t.Fatalf("加载测试字体: %v", err)
	}
	drawer := &font.Drawer{Face: face}
	m := typography.OpenTypeMeasurerFactory(path, fontSize)

	for _, s := range []string{"", "Hello World", "iiii", "wwww", "mix i.w", "AVATAR",
		"a\uE000b", "中", "a中b"} {
		want := float64(drawer.MeasureString(s)) / 64
		if got := m.Measure(s); got != want {
			t.Errorf("Measure(%q) = %v, want 与 Drawer 同口径 %v", s, got, want)
		}
	}
}

func TestOpenTypeMeasurerRealFontProperties(t *testing.T) {
	path := writeGoRegular(t)
	m := typography.OpenTypeMeasurerFactory(path, 16)

	if got := m.Measure(""); got != 0 {
		t.Errorf("Measure(\"\") = %v, want 0", got)
	}
	// 真实字形宽度差异:i 窄于 w(启发式 0.55 等宽无此区分)
	if m.Measure("iiii") >= m.Measure("wwww") {
		t.Errorf("Measure(iiii)=%v 应小于 Measure(wwww)=%v", m.Measure("iiii"), m.Measure("wwww"))
	}
	// 缺字形(上表 PUA/CJK)按 notdef 字形宽计入,与渲染端对齐宽度同口径,不再单测
}

func TestOpenTypeMeasurerFactoryBuiltinFont(t *testing.T) {
	// 空串/纯数字字体 = 渲染端内置默认字体语义(basicfont.Face7x13 点阵),
	// 度量与绘制同源(仅 ASCII 覆盖,CJK 必须显式给真实字体文件)
	drawer := &font.Drawer{Face: basicfont.Face7x13}
	for _, fontFile := range []string{"", "12"} {
		m := typography.OpenTypeMeasurerFactory(fontFile, 16)
		for _, s := range []string{"a", "ab", "中"} {
			want := float64(drawer.MeasureString(s)) / 64
			if got := m.Measure(s); got != want {
				t.Errorf("内置字体 Measure(%q) = %v, want %v", s, got, want)
			}
		}
	}
}

func TestOpenTypeMeasurerFactoryLoadFailureFallsBackToHeuristic(t *testing.T) {
	// 加载失败(文件缺失/远程字体未物化等)降级 PHP 对齐启发式,布局不中断;
	// 降级路径 = 默认度量行为(ASCII 0.55 字宽 × 字号,其余 1.0)
	m := typography.OpenTypeMeasurerFactory(filepath.Join(t.TempDir(), "missing.ttf"), 10)

	if got, want := m.Measure("ab"), 0.55*2*10; got != want {
		t.Errorf("降级 Measure(ab) = %v, want 启发式 %v", got, want)
	}
	if got, want := m.Measure("中"), 10.0; got != want {
		t.Errorf("降级 Measure(中) = %v, want 启发式 %v", got, want)
	}
}

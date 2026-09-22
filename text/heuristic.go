package text

// HeuristicMeasurer 字符数启发式度量器(默认注入点):不看字体文件,按字符分类估算宽度。
// 半角(可打印 ASCII)计 0.55 个字宽,其余(CJK/全角/其他)计 1 个字宽,再乘以字号——
// 与旧库 php-canvas 的 autowrap 计权一致,保证迁移期观感接近。
// 启发式估算与具体字体文件无关,只依赖字号(对应 PHP HeuristicMeasurerFactory 忽略 fontFile)
type HeuristicMeasurer struct {
	fontSize float64
}

// NewHeuristicMeasurer 构造指定字号的启发式度量器
func NewHeuristicMeasurer(fontSize float64) *HeuristicMeasurer {
	return &HeuristicMeasurer{fontSize: fontSize}
}

// HeuristicMeasurerFactory 默认度量器工厂:忽略字体文件,按字号创建启发式度量器
func HeuristicMeasurerFactory(fontFile string, fontSize float64) TextMeasurer {
	return NewHeuristicMeasurer(fontSize)
}

// halfWidthUnit 可打印 ASCII(0x20-0x7e)的字宽计权(PHP HALF_WIDTH)
const halfWidthUnit = 0.55

// Measure implements TextMeasurer:逐码点按字符分类累加字宽(等价 PHP preg_split('//u'))。
// 非法 UTF-8 字节按 U+FFFD 计 1.0(PHP 解码失败计 0);JSON wire 面不可达,无互通影响
func (m *HeuristicMeasurer) Measure(text string) float64 {
	units := 0.0
	for _, r := range text {
		if r >= 0x20 && r <= 0x7e {
			units += halfWidthUnit
		} else {
			units++
		}
	}
	return units * m.fontSize
}

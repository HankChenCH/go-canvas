package typography

// 字体加载:扩展名不可信,按 magic bytes 分派 TTF/OTF 单体(opentype.Parse)与
// TTC/OTC 集合(ParseCollection 取首个 face);DPI 固定 72(1pt = 1px,字号即像素)。
// 本包是本 module 唯一的字体加载入口——布局度量(OpenTypeMeasurerFactory)与
// 位图绘制(imagerenderer)共用同一加载配置,保证度量与绘制口径同源。

import (
	"fmt"
	"os"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
)

// parsedFonts 解析结果缓存:同一字体文件进程内只解析一次。
// *opentype.Font 的方法并发安全(x/image 文档:持有不同 *sfnt.Buffer 或 nil 即可,
// NewFace 内部自持 buffer),共享安全;Face 仍每次新建。键为传入路径原文——
// 字体文件在进程生命周期内视为不可变(resolver 缓存物化后路径稳定)
var parsedFonts sync.Map

// LoadFontFace 加载字体文件为绘制/度量 Face。Face 非并发安全,调用方持有策略:
// 渲染端按渲染会话(Renderer 实例)缓存,度量端按度量器实例持有,均不跨 goroutine 共享
func LoadFontFace(path string, fontSize float64) (font.Face, error) {
	if fontSize <= 0 {
		return nil, fmt.Errorf("字号非法: %v", fontSize)
	}

	parsed, err := parsedFont(path)
	if err != nil {
		return nil, err
	}

	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil, fmt.Errorf("构建字体 Face %s: %w", path, err)
	}
	return face, nil
}

// parsedFont 读取并解析字体文件(magic bytes 分派),解析结果进程内缓存
func parsedFont(path string) (*opentype.Font, error) {
	if cached, ok := parsedFonts.Load(path); ok {
		return cached.(*opentype.Font), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取字体 %s: %w", path, err)
	}

	var parsed *opentype.Font
	if len(data) >= 4 && string(data[:4]) == "ttcf" {
		collection, err := opentype.ParseCollection(data)
		if err != nil {
			return nil, fmt.Errorf("解析字体集 %s: %w", path, err)
		}
		// 集合字体取首个 face(文档化语义,不暴露 face 选择)
		parsed, err = collection.Font(0)
		if err != nil {
			return nil, fmt.Errorf("取字体集首 face %s: %w", path, err)
		}
	} else {
		parsed, err = opentype.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("解析字体 %s: %w", path, err)
		}
	}

	actual, _ := parsedFonts.LoadOrStore(path, parsed)
	return actual.(*opentype.Font), nil
}

// BuiltinFace 内置默认字体:Go 侧没有 GD 内置字体的对应物,空字体/纯数字字体 id
// 的"内置默认字体语义"取 x/image 自带的 7×13 点阵 basicfont 兜底——零新增依赖;
// 仅覆盖 ASCII 且字号参数无效(点阵字体无缩放),CJK 文本必须显式提供真实字体文件。
// basicfont 的实现无内部状态,单例共享安全;度量与绘制经本入口取同一实例
func BuiltinFace() font.Face {
	return basicfont.Face7x13
}

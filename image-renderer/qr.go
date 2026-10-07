package imagerenderer

// 二维码物化的默认实现(工单 08,ADR-0002 缝的接线端):yeqown/go-qrcode v2
// 适配,固定选项逐项对齐 PHP endroid/qr-code v6 用法。缓存不在本层——命中
// 跳过/幂等由核心 resolver 统一负责,这里只做"内容+宽度 → PNG 字节"。

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"io"

	"github.com/hankchen/go-canvas/resolver"

	qr "github.com/yeqown/go-qrcode/v2"
	"github.com/yeqown/go-qrcode/writer/standard"
)

// QRMaterializer 二维码物化默认实现,固定选项逐项对齐 endroid 用法:
//   - 内容 UTF-8(Go string 即 UTF-8 字节);编码模式复刻 bacon chooseMode 的
//     三分(见 chooseEncMode)——endroid 传 UTF-8 时 bacon 从不选 Kanji 模式,
//     纯中文内容两端矩阵因此一致(库自带的 EncModeAuto 会为纯中日文选 Kanji,
//     故必须显式选模式);
//   - 纠错 = QR 'H' 30%(endroid ErrorCorrectionLevel::High;yeqown 常量名为
//     Highest,库默认 Quart 须显式覆写);
//   - size = max(图层宽,1);margin = 0(库默认 40px 静区,WithBorderWidth(0)
//     显式归零);纯黑前景/纯白背景(不透明);显式 PNG 编码器(库默认 JPEG)。
//
// size 语义差异:endroid RoundBlockSizeModeNone 按**精确** size 像素出图(块
// 尺寸允许小数,模块数超过 size 时抛 BlockSizeTooSmallException);yeqown 的
// WithQRWidth 是**整数**块像素宽,出图 = 模块数×块宽——对目标宽向下量化,而
// 目标宽小于矩阵边长时块宽钳到 1,出图反大于目标宽(endroid 同场景直接抛错)。
// 本项目规避方式与 PHP 一致:生成后由渲染原语内切正方形 cover 缩放铺放(ADR 0015),上述
// 差异在渲染面不可见,只影响缓存 PNG 的绝对尺寸。
type QRMaterializer struct{}

var _ resolver.QRMaterializer = QRMaterializer{}

// Materialize implements resolver.QRMaterializer。width 为生成基准宽度(图层宽);
// 库本身不支持 ctx,取消语义只在更外层(缓存/下载侧)生效
func (QRMaterializer) Materialize(_ context.Context, text string, width int) ([]byte, error) {
	size := max(width, 1) // 对齐 endroid size=max(图层宽,1)

	code, err := qr.NewWith(text, pickEncMode(text).option(),
		qr.WithErrorCorrectionLevel(qr.ErrorCorrectionHighest))
	if err != nil {
		return nil, fmt.Errorf("生成二维码矩阵(%s): %w", text, err)
	}

	// endroid None 模式的最近似映射:块宽 = size/模块数 向零截断,钳 [1,255]
	// (WithQRWidth 形参为 uint8,上限 255)
	blockWidth := min(max(size/code.Dimension(), 1), 255)

	var buf bytes.Buffer
	w := standard.NewWithWriter(nopWriteCloser{&buf},
		standard.WithQRWidth(uint8(blockWidth)),
		standard.WithBorderWidth(0),
		standard.WithFgColor(color.Black),
		standard.WithBgColor(color.White),
		standard.WithBuiltinImageEncoder(standard.PNG_FORMAT),
	)
	if err := code.Save(w); err != nil {
		return nil, fmt.Errorf("编码二维码 PNG(%s): %w", text, err)
	}
	return buf.Bytes(), nil
}

// baconEncMode 编码模式判定结果(本包枚举:yeqown 的 encMode 类型未导出,
// 跨包不能引用/转换,经此解耦)
type baconEncMode int

const (
	encModeByte baconEncMode = iota
	encModeNumeric
	encModeAlphaNum
)

// option 映射为 yeqown 编码模式选项
func (m baconEncMode) option() qr.EncodeOption {
	switch m {
	case encModeNumeric:
		return qr.WithEncodingMode(qr.EncModeNumeric)
	case encModeAlphaNum:
		return qr.WithEncodingMode(qr.EncModeAlphanumeric)
	default:
		return qr.WithEncodingMode(qr.EncModeByte)
	}
}

// pickEncMode 复刻 bacon Encoder::chooseMode 在 UTF-8 提示下的模式选择:
// 纯数字→Numeric、QR 字母数字集→Alphanumeric、其余→Byte,**无 Kanji 分支**
// (bacon 仅在 SHIFT-JIS 提示下才考虑 Kanji)。与 yeqown EncModeAuto 的差别
// 即在此:后者对纯中日文内容会升级到 Kanji(Shift-JIS 码),两端矩阵不一致
func pickEncMode(text string) baconEncMode {
	switch {
	case text == "": // bacon chooseMode 首行的空串守卫,忠实复刻(实际到不了这里,resolver 已拦空内容)
		return encModeByte
	case isAllDigits(text):
		return encModeNumeric
	case isAllAlphaNum(text):
		return encModeAlphaNum
	default:
		return encModeByte
	}
}

// isAllDigits 纯 ASCII 数字(PHP ctype_digit 语义,空串由调用方先行分流)
func isAllDigits(text string) bool {
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isAllAlphaNum QR 字母数字集(PHP/bacon 与 yeqown 的字符表一致:
// 0-9 A-Z 空格 $ % * + - . / :)
func isAllAlphaNum(text string) bool {
	for _, r := range text {
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') {
			switch r {
			case ' ', '$', '%', '*', '+', '-', '.', '/', ':':
			default:
				return false
			}
		}
	}
	return true
}

// nopWriteCloser bytes.Buffer 补 Close 以满足 standard.NewWithWriter 形参
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// NewDefaultResolver 构造带二维码物化的默认解析器:QR 缝固定接本 module 的
// QRMaterializer(ADR-0002 缝的默认接线),其余 resolver.Option 原样透传
// (后置项可覆写默认注入,如测试里注入假物化器/临时缓存根)
func NewDefaultResolver(opts ...resolver.Option) *resolver.ResourceResolver {
	return resolver.New(append([]resolver.Option{resolver.WithQRMaterializer(QRMaterializer{})}, opts...)...)
}

// NewRenderer 组装位图渲染器(PHP new ImageRenderer() 的对应物):位图后端 +
// 自带默认物化器。rs 为 nil 时取 NewDefaultResolver()(二维码缝已接线、缓存根
// 为系统默认);传入自建解析器时按原样使用——自定义根须经 NewDefaultResolver
// 构造。返回具体后端(实现 renderer.Backend),经 renderer.RenderAs/RenderLayerAs
// 消费即完整渲染管线:核心 New 的 nil-resolver 组装路径自动发现自带物化器,
// 泛型实参直接声明期望产物类型(工票13,ADR-0010)
func NewRenderer(rs *resolver.ResourceResolver) *Renderer {
	r := New()
	if rs == nil {
		rs = NewDefaultResolver()
	}
	r.defaultResolver = rs
	return r
}

package imagerenderer

// 文本绘制原语:字体加载(TTF/OTF/TTC)、Face 会话持有、对齐锚点落笔、角度旋转。
// (x, y) 为对齐语义锚点——水平按真实文本宽度(advance+kern)对齐,垂直用字体
// metrics(ascent/descent)把基线落到锚点语义对应位置;布局层只返回纯对齐锚点,
// 字体基线差统一在渲染端消化(ADR-0003,PHP 的 GD 基线魔数不移植)。

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/math/fixed"

	"github.com/hankchen/go-canvas/layer"
	"github.com/hankchen/go-canvas/resolver"
)

// fontKey Face 缓存键:字体文件路径 + 字号
type fontKey struct {
	path string
	size int
}

// sessionFace 取绘制用 Face:按(字体, 字号)在渲染会话内缓存。
// opentype.Face 非并发安全,处理策略是**按渲染会话(Renderer 实例)持有**——
// 同一实例串行服务一个渲染流程,不做跨实例共享也不加锁(渲染面位图本身同样
// 非并发安全);实例可跨多次 Render 复用,缓存随实例存活。
// fontFile 为空串/纯数字时返回内置默认字体
func (r *Renderer) sessionFace(fontFile string, fontSize int) (font.Face, error) {
	if fontFile == "" || resolver.IsNumeric(fontFile) {
		return builtinFontFace(), nil
	}

	key := fontKey{fontFile, fontSize}
	if f, ok := r.faces[key]; ok {
		return f, nil
	}

	f, err := loadFontFace(fontFile, fontSize)
	if err != nil {
		return nil, err
	}
	if r.faces == nil {
		r.faces = make(map[fontKey]font.Face)
	}
	r.faces[key] = f
	return f, nil
}

// builtinFontFace 内置默认字体:Go 侧没有 GD 内置字体的对应物,空字体/纯数字
// id 的"内置默认字体语义"取 x/image 自带的 7×13 点阵 basicfont 兜底——
// 零新增依赖;但仅覆盖 ASCII 且字号参数无效(点阵字体无缩放),CJK 文本必须
// 显式提供真实字体文件。basicfont 的实现无内部状态,单例共享安全
func builtinFontFace() font.Face {
	return basicfont.Face7x13
}

// loadFontFace 加载字体文件为绘制 Face:扩展名不可信,按 magic bytes 分派——
// "ttcf" 集合走 opentype.ParseCollection 取首个 face,TTF/OTF 单体走
// opentype.Parse;DPI 固定 72(1pt = 1px,字号即像素)
func loadFontFace(path string, fontSize int) (font.Face, error) {
	if fontSize <= 0 {
		return nil, fmt.Errorf("字号非法: %d", fontSize)
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

	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    float64(fontSize),
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil, fmt.Errorf("构建字体 Face %s: %w", path, err)
	}
	return face, nil
}

// DrawText implements renderer.Backend:空行零副作用(PHP 同款空串守卫);
// 水平对齐偏移 = 真实文本宽度(MeasureString,advance+kern,与落笔同一度量
// 路径)× 对齐语义;垂直基线 = 锚点 + metrics 偏移(top: +ascent /
// center: +(ascent-descent)/2 / bottom: -descent,未知取值归 top)。
// 角度为度,正值逆时针(GD angle 惯例),离屏渲染后绕锚点旋转移到渲染面
func (r *Renderer) DrawText(
	line string,
	x, y int,
	fontFile string,
	fontSize int,
	fontColor string,
	horizontalAlign string,
	verticalAlign string,
	angle int,
) error {
	surface, err := r.face()
	if err != nil {
		return err
	}
	if line == "" {
		return nil
	}
	c, err := parseColor(fontColor)
	if err != nil {
		return fmt.Errorf("解析字色: %w", err)
	}
	face, err := r.sessionFace(fontFile, fontSize)
	if err != nil {
		return err
	}

	drawer := &font.Drawer{Face: face}
	width := drawer.MeasureString(line)
	metrics := face.Metrics()

	// 水平:笔起点相对锚点的偏移(left: 0 / center: -宽一半 / right: -宽)
	var penX fixed.Int26_6
	switch horizontalAlign {
	case layer.AlignCenter:
		penX = -width / 2
	case layer.AlignRight:
		penX = -width
	}

	// 垂直:基线相对锚点的偏移,由字体 metrics 消化基线差(ADR-0003)
	baselineY := metrics.Ascent
	switch verticalAlign {
	case layer.AlignCenter:
		baselineY = (metrics.Ascent - metrics.Descent) / 2
	case layer.AlignBottom:
		baselineY = -metrics.Descent
	}

	dot := fixed.Point26_6{X: fixed.I(x) + penX, Y: fixed.I(y) + baselineY}
	if angle == 0 {
		drawer.Dst = surface
		drawer.Src = image.NewUniform(c)
		drawer.Dot = dot
		drawer.DrawString(line)
		return nil
	}
	drawRotatedText(surface, face, c, line, width, x, y, angle, penX, baselineY)
	return nil
}

// drawRotatedText 角度支持:文本先离屏渲染,再绕锚点 (x, y) 旋转移到渲染面
// (正值逆时针;face 非并发安全,离屏绘制同会话串行)。
// penX/baselineY 为锚点 → 笔起点的对齐偏移,width 为 DrawText 已度量的文本宽度
func drawRotatedText(
	surface *image.NRGBA,
	face font.Face,
	c color.NRGBA,
	line string,
	width fixed.Int26_6,
	x, y, angle int,
	penX, baselineY fixed.Int26_6,
) {
	metrics := face.Metrics()
	boxHeight := metrics.Height
	if boxHeight <= 0 {
		boxHeight = metrics.Ascent + metrics.Descent
	}

	// 字形可能越出 em box(负 bearing 等),四周留白防裁切;留白随字面高度比例
	// 放宽(大字号下溢出量也随之放大),下限 2px 兜底小字号
	margin := 2 + boxHeight.Ceil()/16
	offscreen := image.NewNRGBA(image.Rect(0, 0, width.Ceil()+2*margin, boxHeight.Ceil()+2*margin))
	offscreenDrawer := &font.Drawer{
		Dst:  offscreen,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(margin), Y: fixed.I(margin) + metrics.Ascent},
	}
	offscreenDrawer.DrawString(line)

	// 锚点在离屏图中的位置 = 离屏笔起点 - (笔偏移, 基线偏移)
	cos := math.Cos(float64(angle) * math.Pi / 180)
	sin := math.Sin(float64(angle) * math.Pi / 180)
	ax := float64(margin) - float64(penX)/64
	ay := float64(margin) + float64(metrics.Ascent)/64 - float64(baselineY)/64

	// 仿射矩阵按"源坐标 → 目标坐标"给出(x/image/draw.Transform 约定):
	// 目标 = R(θ)·(源 - 锚点离屏) + 锚点渲染面,实现绕锚点的逆时针旋转
	// (图像坐标系 y 轴向下,视觉逆时针的旋转矩阵为 [[cos,sin],[-sin,cos]])
	anchorX, anchorY := float64(x), float64(y)
	m := f64.Aff3{
		cos, sin, anchorX - (cos*ax + sin*ay),
		-sin, cos, anchorY - (-sin*ax + cos*ay),
	}
	xdraw.ApproxBiLinear.Transform(surface, m, offscreen, offscreen.Bounds(), xdraw.Over, nil)
}

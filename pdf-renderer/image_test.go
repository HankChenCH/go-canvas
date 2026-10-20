package pdfrenderer

// 图片原语测试:PNG/JPEG 直嵌、cover 裁窗、contain、fill、tile 网格与截断、
// EXIF 八向放置矩阵。经标准管线驱动(模板分派 fit 几何),poppler 栅格化后
// 实心区内部点采样。

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"os"
	"testing"

	"github.com/HankChenCH/go-canvas/layer"
)

func TestDrawImagePngDirectEmbed(t *testing.T) {
	// PNG 整图 fill:源红蓝两半 → 目标盒同比例映回(采样避开分界)
	src := twoColorPng(t, 20, 10, red, blue, 10)
	l := layer.NewImageLayer(layer.WithSize(20, 10), layer.WithImage(src))
	l.SetFit(layer.FitFill)
	img := rasterize(t, renderPdfLayer(t, l), 20, 10)
	assertPixel(t, img, 4, 5, red)
	assertPixel(t, img, 15, 5, blue)
}

func TestDrawImageJpegDirectEmbed(t *testing.T) {
	// JPEG 直嵌:有损编码用容差取样
	src := writeJpegFixture(t, solidImage(20, 10, green))
	l := layer.NewImageLayer(layer.WithSize(20, 10), layer.WithImage(src))
	l.SetFit(layer.FitFill)
	img := rasterize(t, renderPdfLayer(t, l), 20, 10)
	assertPixelNear(t, img, 10, 5, green, 24)
}

func TestDrawImageCoverLeftRightHalves(t *testing.T) {
	// cover 裁窗:40×10 双色图 cover 进 20×20 盒 → 裁中缝左右各半
	// (与位图后端 TestImageIsCoverCroppedIntoContentBox 同几何)
	wide := twoColorPng(t, 40, 10, red, blue, 20)
	l := layer.NewImageLayer(layer.WithSize(20, 20), layer.WithBackground("#ffffff"), layer.WithImage(wide))
	l.SetFit(layer.FitCover)
	img := rasterize(t, renderPdfLayer(t, l), 20, 20)
	assertPixel(t, img, 5, 10, red)
	assertPixel(t, img, 15, 10, blue)
}

func TestDrawImageContainLetterbox(t *testing.T) {
	// contain(ADR 0023):40×10 图按比例收缩进 20×20 盒 → 上下留白露背景
	src := twoColorPng(t, 40, 10, red, blue, 20)
	l := layer.NewImageLayer(layer.WithSize(20, 20), layer.WithBackground("#ffffff"),
		layer.WithImage(src))
	l.SetFit(layer.FitContain)
	img := rasterize(t, renderPdfLayer(t, l), 20, 20)
	// 收缩后图像占 y ∈ [7.5, 12.5],上下留白为背景白
	assertPixel(t, img, 3, 2, white)
	assertPixel(t, img, 17, 18, white)
	assertPixel(t, img, 5, 10, red)
	assertPixel(t, img, 15, 10, blue)
}

func TestDrawImageTileGridAndTruncation(t *testing.T) {
	// tile:20×10 源平铺 50×30 盒,默认居中对齐 → 网格起点 = 对齐锚点
	// ((50-20)/2, (30-10)/2) = (15,10)(PHP 同款,TileRects 语义);
	// 列 1 整块(x 15..35)、列 2 截断块(x 35..50,15px 宽);行 1(y 10..20)、
	// 行 2(y 20..30);网格未覆盖区露背景白
	src := solidPng(t, 20, 10, red)
	l := layer.NewImageLayer(layer.WithSize(50, 30), layer.WithBackground("#ffffff"),
		layer.WithImage(src))
	l.SetFit(layer.FitTile)
	img := rasterize(t, renderPdfLayer(t, l), 50, 30)
	// 块内部与截断块、右下缘:红
	for _, p := range []image.Point{{25, 15}, {25, 25}, {45, 15}, {45, 25}, {49, 29}} {
		assertPixel(t, img, p.X, p.Y, red)
	}
	// 网格未覆盖区(锚点左侧/上侧):背景白
	assertPixel(t, img, 5, 5, white)
	assertPixel(t, img, 25, 5, white)
	assertPixel(t, img, 5, 25, white)
}

func TestDrawImageGifDirectEmbed(t *testing.T) {
	// GIF 直嵌(fpdf parsegif):std lib 无 GIF 编码器,用最小非隔行 GIF 字节
	// (1×1 红,手工拼装)
	src := writeRawFixture(t, "fixture.gif", minimalRedGif)
	l := layer.NewImageLayer(layer.WithSize(10, 10), layer.WithImage(src))
	l.SetFit(layer.FitFill)
	img := rasterize(t, renderPdfLayer(t, l), 10, 10)
	assertPixelNear(t, img, 5, 5, red, 8)
}

func TestDrawImageEmptyDstRectSkipped(t *testing.T) {
	// dstRect 空矩形防护不画(原语级):渲染成功
	r := New()
	if err := r.Begin(10, 10); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := r.DrawImage(solidPng(t, 4, 4, red), image.Rectangle{}, image.Rectangle{}); err != nil {
		t.Fatalf("DrawImage 空 dstRect 应静默不画: %v", err)
	}
	product := r.End()
	pdf, ok := product.([]byte)
	if !ok || len(pdf) == 0 {
		t.Fatalf("产物 = %T, want []byte", product)
	}
}

func TestDrawImageSrcRectOutsideSilent(t *testing.T) {
	// 源窗与解码图不相交:静默不画(位图后端同款防御)
	r := New()
	if err := r.Begin(10, 10); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	src := solidPng(t, 8, 8, red)
	if err := r.DrawImage(src, image.Rect(100, 100, 200, 200), image.Rect(0, 0, 5, 5)); err != nil {
		t.Fatalf("不相交源窗应静默: %v", err)
	}
	if r.End() == nil {
		t.Fatal("静默路径也应有产物")
	}
}

func TestDrawImageGarbageAborts(t *testing.T) {
	// 垃圾字节报错中止(嗅探防线;与位图后端的 parity 适用域 =「不可解码字节
	// 必须报错中止、不得静默出图」这一失败结局,两后端防线对称;反例域 =
	// 错误文案与错误类型——两后端各出各的普通包装 error,不在契约内)
	r := New()
	if err := r.Begin(10, 10); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	path := writePngFixture(t, solidImage(2, 2, red))
	if err := os.WriteFile(path, []byte("not an image"), 0o644); err != nil {
		t.Fatalf("写垃圾测试图: %v", err)
	}
	if err := r.DrawImage(path, image.Rectangle{}, image.Rect(0, 0, 5, 5)); err == nil {
		t.Fatal("非法图片字节应报错")
	}
}

func TestDrawImageExifOrientationRotated(t *testing.T) {
	// EXIF 放置矩阵(开放点 3 定稿式的验收面):8×16 上红下蓝源图,
	// orientation 6(顺时针 90°)→ 观察方向 16×8 左蓝右红;整图哨兵 blit 进
	// 观察方向同尺寸盒(与位图后端 TestDrawImageOrientsExifRotatedJPEG 同几何)
	src := twoBandExifJpeg(t, 6)
	l := layer.NewImageLayer(layer.WithSize(16, 8), layer.WithBackground("#ffffff"),
		layer.WithImage(src))
	img := rasterize(t, renderPdfLayer(t, l), 16, 8)
	// JPEG 有损 + 色度抽样,取远离分界的点做容差取样
	assertPixelNear(t, img, 1, 4, blue, 60)
	assertPixelNear(t, img, 14, 4, red, 60)
}

func TestDrawImageExifOrientationAllEight(t *testing.T) {
	// 八向标定(JPEG 载体,poppler 渲染容差取样):8×16 上红下蓝源图,
	// 90° 族转置后观察方向 16×8 左右分色,镜像/180° 族仍 8×16 上下分色;
	// 期望色序按 orientAffine 的观察方向映射逐向手算(与位图后端
	// TestApplyOrientationAllDirections 的栅格标定同源)
	cases := []struct {
		orientation   int
		dstW, dstH    int
		splitVertical bool // 观察方向分界是否为竖线(左右分色)
		first, second color.NRGBA
	}{
		{6, 16, 8, true, blue, red},  // 顺时针 90°:下带(蓝)转到左
		{8, 16, 8, true, red, blue},  // 逆时针 90°:上带(红)转到左
		{5, 16, 8, true, red, blue},  // 转置:上行(红)到左
		{7, 16, 8, true, blue, red},  // 反转置:下行(蓝)到左
		{3, 8, 16, false, blue, red}, // 180°:上蓝下红(带序翻转)
		{2, 8, 16, false, red, blue}, // 水平镜像:上下带不变
		{4, 8, 16, false, blue, red}, // 垂直镜像:下带(蓝)翻上、红落底
	}
	for _, tc := range cases {
		src := twoBandExifJpeg(t, tc.orientation)
		l := layer.NewImageLayer(layer.WithSize(tc.dstW, tc.dstH), layer.WithBackground("#ffffff"),
			layer.WithImage(src))
		l.SetFit(layer.FitFill)
		img := rasterize(t, renderPdfLayer(t, l), tc.dstW, tc.dstH)
		if tc.splitVertical {
			assertPixelNear(t, img, 1, 4, tc.first, 60)
			assertPixelNear(t, img, tc.dstW-2, 4, tc.second, 60)
		} else {
			assertPixelNear(t, img, 4, 1, tc.first, 60)
			assertPixelNear(t, img, 4, tc.dstH-2, tc.second, 60)
		}
	}
}

func TestOrientAffineAllDirections(t *testing.T) {
	// orientAffine 八向纯单元标定:3×2 字母栅格 A B C / D E F 按内在格 → 观察格
	// 方向(DrawImage/transformRect 的实际使用方向)重排后逐格比对,期望栅格与
	// 位图后端 TestApplyOrientationAllDirections 逐向一致(矩阵式 = 位图像素
	// 搬运的连续化,两后端转正防线对称)
	letters := "ABCDEF"
	cases := []struct {
		orientation int
		want        string // 栅格行按 / 分隔;5–8 为 90° 族,宽高互换
	}{
		{1, "ABC/DEF"},
		{2, "CBA/FED"},  // 水平镜像
		{3, "FED/CBA"},  // 旋转 180°
		{4, "DEF/ABC"},  // 垂直镜像
		{5, "AD/BE/CF"}, // 转置(主对角镜像)
		{6, "DA/EB/FC"}, // 旋转 90° 顺时针
		{7, "FC/EB/DA"}, // 反转置(反对角镜像)
		{8, "CF/BE/AD"}, // 旋转 90° 逆时针
	}
	for _, tc := range cases {
		t.Run("orientation "+string(rune('0'+tc.orientation)), func(t *testing.T) {
			rows := bytesSplit(tc.want, '/')
			outW, outH := len(rows[0]), len(rows)
			grid := make([][]byte, outH)
			for i := range grid {
				grid[i] = bytes.Repeat([]byte{'.'}, outW)
			}
			m := orientAffine(tc.orientation, 3, 2)
			for y := 0; y < 2; y++ {
				for x := 0; x < 3; x++ {
					ox, oy := m.apply(float64(x)+0.5, float64(y)+0.5) // 格心采样,避边界歧义
					grid[int(oy)][int(ox)] = letters[y*3+x]
				}
			}
			for dy, row := range rows {
				if string(grid[dy]) != string(row) {
					t.Errorf("orientation %d 转正后第 %d 行 = %s, want %s",
						tc.orientation, dy, grid[dy], row)
				}
			}
		})
	}
}

// bytesSplit "ABC/DEF" → []string{"ABC", "DEF"}
func bytesSplit(s string, sep byte) []string {
	var out []string
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			out = append(out, s[last:i])
			last = i + 1
		}
	}
	return append(out, s[last:])
}

// ---- 小件 ----

// solidImage 纯色测试图(内存形态,配 JPEG 落盘)
func solidImage(width, height int, c color.NRGBA) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Over)
	return img
}

// minimalRedGif 1×1 红色非隔行 GIF(手工拼装:std lib 无 GIF 编码器)
var minimalRedGif = []byte{
	'G', 'I', 'F', '8', '9', 'a', // 署名
	0x01, 0x00, 0x01, 0x00, // 逻辑屏幕 1×1
	0x80, // 全局色表标志 + 2 色
	0x00, 0x00,
	0xFF, 0xFF, 0xFF, // 色表[0] 白
	0xFF, 0x00, 0x00, // 色表[1] 红
	0x2C, // 图像描述符
	0x00, 0x00, 0x00, 0x00,
	0x01, 0x00, 0x01, 0x00,
	0x00,             // 无局部色表、非隔行
	0x02,             // LZW 最小码长 2
	0x02, 0x4C, 0x01, // LZW 数据:清除码(100)+ 像素 1(001)+ 结束码(101),LSB 先行
	0x00, // 块终结
	0x3B, // 结尾
}

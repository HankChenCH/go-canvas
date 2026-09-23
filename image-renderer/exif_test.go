package imagerenderer

// EXIF orientation 的三段测试:字节流解析(手拼 JPEG/APP1/TIFF)、八向转正的
// 栅格标定、以及"带 EXIF 的 JPEG 解码即转正"集成(PHP intervention orient() 的对等面)

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"strconv"
	"testing"
)

// ---- 手拼字节流工具 ----

// tiffWithOrientation 组装仅含 orientation 标签的 TIFF 块(IFD0 单条目)
func tiffWithOrientation(bo binary.ByteOrder, typ, orientation uint16) []byte {
	var b bytes.Buffer
	if bo == binary.LittleEndian {
		b.WriteString("II")
	} else {
		b.WriteString("MM")
	}
	_ = binary.Write(&b, bo, uint16(42))     // 魔数
	_ = binary.Write(&b, bo, uint32(8))      // IFD0 偏移紧随头部
	_ = binary.Write(&b, bo, uint16(1))      // 条目数
	_ = binary.Write(&b, bo, uint16(0x0112)) // Orientation
	_ = binary.Write(&b, bo, typ)
	_ = binary.Write(&b, bo, uint32(1)) // 计数
	if typ == 3 {                       // SHORT 内联于值字段前 2 字节
		_ = binary.Write(&b, bo, orientation)
		_ = binary.Write(&b, bo, uint16(0))
	} else { // LONG
		_ = binary.Write(&b, bo, uint32(orientation))
	}
	_ = binary.Write(&b, bo, uint32(0)) // 下一 IFD 偏移
	return b.Bytes()
}

// jpegWithExifOrientation 以 img 为主体拼装 SOI + APP1(Exif) + 主体熵编码的 JPEG
func jpegWithExifOrientation(t *testing.T, img image.Image, bo binary.ByteOrder, typ, orientation uint16) []byte {
	t.Helper()
	var body bytes.Buffer
	if err := jpeg.Encode(&body, img, nil); err != nil {
		t.Fatalf("编码 JPEG 主体: %v", err)
	}
	payload := append([]byte("Exif\x00\x00"), tiffWithOrientation(bo, typ, orientation)...)
	segLen := len(payload) + 2 // 段长字段含自身 2 字节
	out := []byte{0xFF, 0xD8, 0xFF, 0xE1, byte(segLen >> 8), byte(segLen)}
	out = append(out, payload...)
	return append(out, body.Bytes()[2:]...) // 去掉主体自身的 SOI
}

// ---- 解析 ----

func TestExifOrientationFromJPEGBytes(t *testing.T) {
	// 8×16 上红下蓝:色度块上下分开,便于集成断言
	src := image.NewNRGBA(image.Rect(0, 0, 8, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 8; x++ {
			if y < 8 {
				src.SetNRGBA(x, y, red)
			} else {
				src.SetNRGBA(x, y, blue)
			}
		}
	}

	cases := []struct {
		name string
		data []byte
		want int
	}{
		{"无 APP1", jpegEncode(t, src), 1},
		{"II SHORT", jpegWithExifOrientation(t, src, binary.LittleEndian, 3, 6), 6},
		{"MM SHORT", jpegWithExifOrientation(t, src, binary.BigEndian, 3, 8), 8},
		{"MM LONG", jpegWithExifOrientation(t, src, binary.BigEndian, 4, 3), 3},
		{"越界值", jpegWithExifOrientation(t, src, binary.LittleEndian, 3, 9), 1},
		{"非 JPEG", pngEncode(t, src), 1},
		{"截断字节流", jpegWithExifOrientation(t, src, binary.LittleEndian, 3, 6)[:10], 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exifOrientation(tc.data); got != tc.want {
				t.Errorf("exifOrientation = %d, want %d", got, tc.want)
			}
		})
	}
}

func jpegEncode(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("编码 JPEG: %v", err)
	}
	return buf.Bytes()
}

func pngEncode(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("编码 PNG: %v", err)
	}
	return buf.Bytes()
}

// ---- 八向转正:3×2 标定栅格 A B C / D E F,期望栅格逐向手算 ----

func TestApplyOrientationAllDirections(t *testing.T) {
	letters := "ABCDEF"
	colors := map[byte]color.NRGBA{}
	for i := 0; i < len(letters); i++ {
		colors[letters[i]] = color.NRGBA{R: byte(i * 40), G: 0, B: 0, A: 0xFF}
	}
	letterOf := func(c color.NRGBA) byte { return letters[c.R/40] }

	src := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			src.SetNRGBA(x, y, colors[letters[y*3+x]])
		}
	}

	// 栅格行按 / 分隔;5–8 为 90° 族,宽高互换
	cases := []struct {
		orientation int
		want        string
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
		t.Run("orientation "+strconv.Itoa(tc.orientation), func(t *testing.T) {
			got := applyOrientation(src, tc.orientation)
			rows := bytes.Split([]byte(tc.want), []byte("/"))
			if got.Bounds().Dx() != len(rows[0]) || got.Bounds().Dy() != len(rows) {
				t.Fatalf("转正后尺寸 = %v, want %dx%d", got.Bounds(), len(rows[0]), len(rows))
			}
			for y, row := range rows {
				for x := 0; x < len(row); x++ {
					gotc := color.NRGBAModel.Convert(got.At(x, y)).(color.NRGBA)
					if letter := letterOf(gotc); letter != row[x] {
						t.Errorf("转正后 (%d, %d) = %c, want %c", x, y, letter, row[x])
					}
				}
			}
		})
	}
}

// ---- 集成:解码即转正 ----

func TestDecodeOrientedJPEGRotated(t *testing.T) {
	// orientation 6(旋转 90° 顺时针):8×16 上红下蓝 → 16×8 左蓝右红
	src := image.NewNRGBA(image.Rect(0, 0, 8, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 8; x++ {
			if y < 8 {
				src.SetNRGBA(x, y, red)
			} else {
				src.SetNRGBA(x, y, blue)
			}
		}
	}
	img, err := decodeOriented(jpegWithExifOrientation(t, src, binary.LittleEndian, 3, 6))
	if err != nil {
		t.Fatalf("decodeOriented: %v", err)
	}
	if img.Bounds().Dx() != 16 || img.Bounds().Dy() != 8 {
		t.Fatalf("转正后尺寸 = %dx%d, want 16x8", img.Bounds().Dx(), img.Bounds().Dy())
	}
	// JPEG 有损 + 色度抽样,取远离分界的点做容差取样
	assertPixelNear(t, img, 0, 1, blue, 60)
	assertPixelNear(t, img, 15, 6, red, 60)
}

func TestDecodeOrientedPNGUntouched(t *testing.T) {
	// PNG 不携带 EXIF:原样解码
	src := image.NewNRGBA(image.Rect(0, 0, 6, 3))
	draw.Draw(src, src.Bounds(), &image.Uniform{C: green}, image.Point{}, draw.Over)
	img, err := decodeOriented(pngEncode(t, src))
	if err != nil {
		t.Fatalf("decodeOriented: %v", err)
	}
	if img.Bounds().Dx() != 6 || img.Bounds().Dy() != 3 {
		t.Fatalf("尺寸 = %dx%d, want 6x3", img.Bounds().Dx(), img.Bounds().Dy())
	}
	assertPixel(t, img, 3, 1, green)
}

func TestDecodeOrientedGarbageAborts(t *testing.T) {
	if _, err := decodeOriented([]byte("not an image")); err == nil {
		t.Fatal("非法图片字节应报错")
	}
}

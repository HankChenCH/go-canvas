package imagerenderer

import (
	"encoding/binary"
	"image"
	"image/draw"
)

// EXIF orientation:JPEG 的 APP1(Exif) 段携带拍摄方向,渲染前需转正到观察方向
// (PHP intervention orient() 的对等职责)。标准库不解 EXIF,这里实现最小解析
// ——只找 IFD0 的 Orientation 标签(0x0112),其余结构一概不碰

// exifOrientation 从图片字节流解析 EXIF orientation(1–8)。
// 非 JPEG、无 EXIF、结构截断/非法一律归 1(无需转正)
func exifOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1 // 非 JPEG(PNG/WebP 等不携带该 EXIF 结构)
	}
	pos := 2
	for pos+4 <= len(data) && data[pos] == 0xFF {
		marker := data[pos+1]
		switch {
		case marker == 0xFF: // 标记前的填充字节,步进一位重新判读
			pos++
			continue
		case marker == 0xD8, marker == 0x01, marker == 0xD9: // SOI/TEM/EOI 无段载荷
			pos += 2
			continue
		}
		segLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		if segLen < 2 || pos+2+segLen > len(data) {
			return 1 // 截断或非法段长
		}
		if marker == 0xDA { // SOS:熵编码开始,EXIF 只会更靠前
			return 1
		}
		if marker == 0xE1 && segLen >= 8 && string(data[pos+4:pos+10]) == "Exif\x00\x00" {
			return parseTIFFOrientation(data[pos+10 : pos+2+segLen])
		}
		pos += 2 + segLen
	}
	return 1
}

// parseTIFFOrientation 从 TIFF 头起解析 IFD0 的 Orientation 值(SHORT/LONG 均兼容)
func parseTIFFOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	if bo.Uint16(tiff[2:4]) != 42 {
		return 1
	}
	ifd0 := int(bo.Uint32(tiff[4:8]))
	if ifd0 < 8 || ifd0+2 > len(tiff) {
		return 1
	}
	entries := int(bo.Uint16(tiff[ifd0 : ifd0+2]))
	for i := 0; i < entries; i++ {
		e := ifd0 + 2 + i*12
		if e+12 > len(tiff) {
			return 1
		}
		if bo.Uint16(tiff[e:e+2]) != 0x0112 { // Orientation
			continue
		}
		var v int
		switch bo.Uint16(tiff[e+2 : e+4]) { // 字段类型:3 SHORT / 4 LONG,值内联于条目
		case 3:
			v = int(bo.Uint16(tiff[e+8 : e+10]))
		case 4:
			v = int(bo.Uint32(tiff[e+8 : e+12]))
		default:
			return 1
		}
		if v >= 1 && v <= 8 {
			return v
		}
		return 1
	}
	return 1
}

// applyOrientation 按 EXIF orientation 把图像转正到观察方向:
// 2/4 镜像、3 旋转 180°、6/8 旋转 90°、5/7 沿对角镜像(宽高互换)。
// orientation 由 exifOrientation 保证在 1–8;逐像素搬运 NRGBA,零失真
func applyOrientation(src image.Image, orientation int) image.Image {
	n := toNRGBA(src)
	b := n.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dw, dh := sw, sh
	if orientation >= 5 { // 90° 族:宽高互换
		dw, dh = sh, sw
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for dy := 0; dy < dh; dy++ {
		for dx := 0; dx < dw; dx++ {
			sx, sy := dx, dy
			switch orientation {
			case 2:
				sx = sw - 1 - dx
			case 3:
				sx, sy = sw-1-dx, sh-1-dy
			case 4:
				sy = sh - 1 - dy
			case 5: // 转置
				sx, sy = dy, dx
			case 6: // 旋转 90° 顺时针
				sx, sy = dy, sh-1-dx
			case 7: // 反转置
				sx, sy = sw-1-dy, sh-1-dx
			case 8: // 旋转 90° 逆时针
				sx, sy = sw-1-dy, dx
			}
			dst.SetNRGBA(dx, dy, n.NRGBAAt(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	return dst
}

// toNRGBA 摊平为直 alpha 的 NRGBA(PNG 语义),已是的且零原点的直接返回引用
func toNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok && n.Bounds().Min == (image.Point{}) {
		return n
	}
	b := src.Bounds()
	n := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(n, n.Bounds(), src, b.Min, draw.Over)
	return n
}

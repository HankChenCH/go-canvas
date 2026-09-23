package imagerenderer

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
)

// PNG 编码出口:渲染产物落盘 / 测试取样共用

// PNGBytes 把位图编码为 PNG 字节
func PNGBytes(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("编码 PNG: %w", err)
	}
	return buf.Bytes(), nil
}

// SavePNG 把位图编码为 PNG 并落盘(对应 PHP $image->save())
func SavePNG(path string, img image.Image) error {
	data, err := PNGBytes(img)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入 PNG %s: %w", path, err)
	}
	return nil
}

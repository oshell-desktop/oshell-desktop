// Package imagedecode keeps OShell's TIFF support independent of codec registration order.
package imagedecode

import (
	"bytes"
	"image"
	"oshell-desktop/internal/tiff"
)

// Font parsing also registers x/image/tiff, which lacks our gray-alpha and planar support.
func Config(data []byte) (image.Config, string, error) {
	if bytes.HasPrefix(data, []byte("II\x2a\x00")) || bytes.HasPrefix(data, []byte("MM\x00\x2a")) {
		c, err := tiff.DecodeConfig(bytes.NewReader(data))
		return c, "tiff", err
	}
	return image.DecodeConfig(bytes.NewReader(data))
}
func Decode(data []byte) (image.Image, string, error) {
	if bytes.HasPrefix(data, []byte("II\x2a\x00")) || bytes.HasPrefix(data, []byte("MM\x00\x2a")) {
		im, err := tiff.Decode(bytes.NewReader(data))
		return im, "tiff", err
	}
	return image.Decode(bytes.NewReader(data))
}

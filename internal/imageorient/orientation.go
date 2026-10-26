// Package imageorient applies EXIF display orientation without copying decoded pixels.
package imageorient

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
)

// Orientation reads the primary TIFF orientation, including JPEG APP1, PNG eXIf
// and WebP EXIF containers. Missing or invalid metadata means normal orientation.
// It does not validate image data; callers must decode and bound it separately.
func Orientation(data []byte) int {
	tiff := data
	if bytes.HasPrefix(data, []byte{0xff, 0xd8}) {
		tiff = nil
		for i := 2; i < len(data); {
			if data[i] != 0xff {
				break
			}
			// JPEG allows any number of FF fill bytes before a marker.
			for i < len(data) && data[i] == 0xff {
				i++
			}
			if i >= len(data) {
				break
			}
			marker := data[i]
			i++
			if marker == 0xda || marker == 0xd9 || marker == 0 {
				break
			}
			if marker == 0x01 || marker >= 0xd0 && marker <= 0xd8 {
				continue
			}
			if len(data)-i < 2 {
				break
			}
			n := int(binary.BigEndian.Uint16(data[i:]))
			if n < 2 || n > len(data)-i {
				break
			}
			if marker == 0xe1 && n >= 8 && bytes.Equal(data[i+2:i+8], []byte("Exif\x00\x00")) {
				tiff = data[i+8 : i+n]
				break
			}
			i += n
		}
	} else if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		tiff = nil
		for i := 8; i+12 <= len(data); {
			size := uint64(binary.BigEndian.Uint32(data[i:]))
			if size > uint64(len(data)-i-12) {
				break
			}
			n := int(size)
			if string(data[i+4:i+8]) == "IEND" {
				break
			}
			if string(data[i+4:i+8]) == "eXIf" {
				tiff = data[i+8 : i+8+n]
				break
			}
			i += 12 + n
		}
	} else if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		tiff = nil
		size := uint64(binary.LittleEndian.Uint32(data[4:]))
		if size < 4 || size > uint64(len(data)-8) {
			return 1
		}
		data = data[:8+int(size)]
		for i := 12; i+8 <= len(data); {
			size := uint64(binary.LittleEndian.Uint32(data[i+4:]))
			if size+size%2 > uint64(len(data)-i-8) {
				break
			}
			n := int(size)
			if string(data[i:i+4]) == "EXIF" {
				tiff = bytes.TrimPrefix(data[i+8:i+8+n], []byte("Exif\x00\x00"))
				break
			}
			i += 8 + n + n%2
		}
	}
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:4]) {
	case "II*\x00":
		order = binary.LittleEndian
	case "MM\x00*":
		order = binary.BigEndian
	default:
		return 1
	}
	offset := uint64(order.Uint32(tiff[4:]))
	if offset < 8 || offset+2 > uint64(len(tiff)) {
		return 1
	}
	n := int(order.Uint16(tiff[offset:]))
	pos := int(offset) + 2
	for ; n > 0 && pos+12 <= len(tiff); n, pos = n-1, pos+12 {
		entry := tiff[pos : pos+12]
		if order.Uint16(entry) == 0x112 && order.Uint16(entry[2:]) == 3 && order.Uint32(entry[4:]) == 1 {
			v := int(order.Uint16(entry[8:]))
			if v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// Apply returns a read-only oriented view of decoded, unrotated pixels. The
// source and its encoded data must describe the same image. Decoders which
// already apply EXIF orientation must not use this a second time.
func Apply(im image.Image, data []byte) image.Image {
	return WithOrientation(im, Orientation(data))
}

// WithOrientation returns a read-only view using an already parsed EXIF value.
// Invalid values preserve the unrotated source.
func WithOrientation(im image.Image, orientation int) image.Image {
	if orientation >= 2 && orientation <= 8 {
		return orientedImage{im, orientation}
	}
	return im
}

type orientedImage struct {
	image.Image
	orientation int
}

// Keep the RGBA64 interface available to image/draw's optimized transforms.
func (im orientedImage) RGBA64At(x, y int) color.RGBA64 {
	r, g, b, a := im.At(x, y).RGBA()
	return color.RGBA64{uint16(r), uint16(g), uint16(b), uint16(a)}
}

func (im orientedImage) Bounds() image.Rectangle {
	b := im.Image.Bounds()
	w, h := b.Dx(), b.Dy()
	if im.orientation >= 5 {
		w, h = h, w
	}
	return image.Rect(0, 0, w, h)
}
func (im orientedImage) At(x, y int) color.Color {
	b := im.Image.Bounds()
	w, h := b.Dx(), b.Dy()
	sx, sy := x, y
	switch im.orientation {
	case 2:
		sx = w - 1 - x
	case 3:
		sx, sy = w-1-x, h-1-y
	case 4:
		sy = h - 1 - y
	case 5:
		sx, sy = y, x
	case 6:
		sx, sy = y, h-1-x
	case 7:
		sx, sy = w-1-y, h-1-x
	case 8:
		sx, sy = w-1-y, x
	}
	return im.Image.At(b.Min.X+sx, b.Min.Y+sy)
}

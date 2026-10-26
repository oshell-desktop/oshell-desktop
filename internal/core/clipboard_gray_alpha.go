package core

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
)

var errClipboardPNGSize = errors.New("转换后的 PNG 超过 20 MiB")

type clipboardPNGBuffer struct{ buffer bytes.Buffer }

func (b *clipboardPNGBuffer) Len() int      { return b.buffer.Len() }
func (b *clipboardPNGBuffer) Bytes() []byte { return b.buffer.Bytes() }

func (b *clipboardPNGBuffer) Write(p []byte) (int, error) {
	if len(p) > ClipboardImageLimit-b.Len() {
		return 0, errClipboardPNGSize
	}
	return b.buffer.Write(p)
}

// image/png decodes gray+alpha into NRGBA/NRGBA64, but cannot encode PNG
// color type 4. Retain the original gray samples and ICC space instead of
// attaching a gray profile to RGB output or dropping the profile/alpha.
// The caller has validated the source color type and dimensions.
func encodeClipboardGrayAlpha(im image.Image, depth byte) ([]byte, error) {
	if depth != 16 {
		depth = 8 // grayscale tRNS at 1/2/4 bits expands losslessly to 8 bits
	}
	b := im.Bounds()
	if b.Empty() || b.Dx() > 20000 || b.Dy() > 20000 || int64(b.Dx())*int64(b.Dy()) > 40000000 {
		return nil, errClipboardColor
	}
	bpp := int(depth) / 4
	row, previous := make([]byte, b.Dx()*bpp), make([]byte, b.Dx()*bpp)
	best, candidate := make([]byte, len(row)+1), make([]byte, len(row)+1)
	var compressed clipboardPNGBuffer
	w := zlib.NewWriter(&compressed)
	defer w.Close()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			var sample color.NRGBA64
			switch c := im.At(x, y).(type) {
			case color.NRGBA:
				// Avoid a premultiplied RGBA conversion, including at alpha 0.
				sample = color.NRGBA64{uint16(c.R) * 257, uint16(c.G) * 257, uint16(c.B) * 257, uint16(c.A) * 257}
			case color.NRGBA64:
				sample = c
			default:
				sample = color.NRGBA64Model.Convert(c).(color.NRGBA64)
			}
			if sample.R != sample.G || sample.R != sample.B {
				return nil, errClipboardColor
			}
			at := (x - b.Min.X) * bpp
			if depth == 8 {
				row[at], row[at+1] = byte(sample.R>>8), byte(sample.A>>8)
			} else {
				binary.BigEndian.PutUint16(row[at:], sample.R)
				binary.BigEndian.PutUint16(row[at+2:], sample.A)
			}
		}
		filterClipboardGrayRow(best, candidate, row, previous, bpp)
		if _, err := w.Write(best); err != nil {
			return nil, err
		}
		row, previous = previous, row
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header, uint32(b.Dx()))
	binary.BigEndian.PutUint32(header[4:], uint32(b.Dy()))
	header[8], header[9] = depth, 4
	out := appendClipboardPNGChunk([]byte("\x89PNG\r\n\x1a\n"), "IHDR", header)
	out = appendClipboardPNGChunk(out, "IDAT", compressed.Bytes())
	return appendClipboardPNGChunk(out, "IEND", nil), nil
}

// PNG's five lossless filters operate on bytes, with a two/four-byte pixel
// stride for gray+alpha. Choose the lowest signed-byte residual sum, retaining
// only the current/previous rows and two candidate buffers.
func filterClipboardGrayRow(best, candidate, row, previous []byte, bpp int) {
	bestScore := int(^uint(0) >> 1)
	for filter := 0; filter < 5; filter++ {
		candidate[0] = byte(filter)
		score := 0
		for i, v := range row {
			left, up, upperLeft := 0, int(previous[i]), 0
			if i >= bpp {
				left, upperLeft = int(row[i-bpp]), int(previous[i-bpp])
			}
			prediction := 0
			switch filter {
			case 1:
				prediction = left
			case 2:
				prediction = up
			case 3:
				prediction = (left + up) / 2
			case 4:
				p := left + up - upperLeft
				a, b, c := absClipboardFilter(p-left), absClipboardFilter(p-up), absClipboardFilter(p-upperLeft)
				switch {
				case a <= b && a <= c:
					prediction = left
				case b <= c:
					prediction = up
				default:
					prediction = upperLeft
				}
			}
			value := byte(int(v) - prediction)
			candidate[i+1] = value
			score += min(int(value), 256-int(value))
		}
		if score < bestScore {
			bestScore = score
			copy(best, candidate)
		}
	}
}

func absClipboardFilter(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

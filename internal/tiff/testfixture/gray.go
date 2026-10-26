// Package testfixture constructs synthetic TIFF samples for decoder and caller tests.
package testfixture

import (
	"bytes"
	"compress/lzw"
	"compress/zlib"
	"encoding/binary"
	"image"
	"image/color"
)

type GrayOptions struct {
	ReverseBlocks                                 bool
	Depth, Alpha, Photo, Compression, Orientation uint16
	BigEndian, Predictor, Tiled, Planar, RGB      bool
	Width, Height                                 int
	ICC                                           []byte
}

// Gray builds gray/alpha or RGB(A), in strips or padded 8x8 tiles. Expected pixels
// are derived from the original samples, before prediction and compression.
func Gray(o GrayOptions) ([]byte, image.Image) {
	var order interface {
		binary.ByteOrder
		binary.AppendByteOrder
	} = binary.LittleEndian
	header := []byte{'I', 'I', 42, 0, 8, 0, 0, 0}
	if o.BigEndian {
		order = binary.BigEndian
		header = []byte{'M', 'M', 0, 42, 0, 0, 0, 8}
	}
	var expected image.Image
	r := image.Rect(0, 0, o.Width, o.Height)
	if o.Alpha != 2 {
		if o.Depth == 16 {
			expected = image.NewRGBA64(r)
		} else {
			expected = image.NewRGBA(r)
		}
	} else {
		if o.Depth == 16 {
			expected = image.NewNRGBA64(r)
		} else {
			expected = image.NewNRGBA(r)
		}
	}
	bw, bh := o.Width, 2
	if o.Tiled {
		bw, bh = 8, 8
	}
	channels := 2
	if o.RGB {
		channels = 3
		if o.Alpha != 0 {
			channels = 4
		}
	}
	planes := 1
	if o.Planar {
		planes = channels
	}
	var blocks [][]byte
	for plane := 0; plane < planes; plane++ {
		for by := 0; by < o.Height; by += bh {
			for bx := 0; bx < o.Width; bx += bw {
				h := bh
				if !o.Tiled {
					h = min(h, o.Height-by)
				}
				var raw []byte
				for y := 0; y < h; y++ {
					row := make([]byte, bw*int(o.Depth)/8*channels)
					for x := 0; x < bw; x++ {
						xx, yy := bx+x, by+y
						v, a := uint16((xx*17+yy*53+71)%256), uint16([]int{0, 85, 170, 255}[(xx+yy)%4])
						if o.Depth == 16 {
							v = v*251 + 19
							a *= 257
						}
						values := []uint16{v, v, v, a}
						if o.RGB {
							values[1] = uint16((xx*37 + yy*11 + 29) % 256)
							values[2] = uint16((xx*3 + yy*89 + 167) % 256)
							if o.Depth == 16 {
								values[1] = values[1]*249 + 31
								values[2] = values[2]*247 + 43
							}
							if o.Alpha == 0 {
								values[3] = 255
								if o.Depth == 16 {
									values[3] = 65535
								}
							}
						}
						if o.Alpha == 1 {
							for k := 0; k < 3; k++ {
								values[k] = min(values[k], a)
							}
						}
						samples := values
						if !o.RGB {
							samples = []uint16{values[0], values[3]}
						}
						at := x * int(o.Depth) / 8 * channels
						for _, sample := range samples[:channels] {
							if o.Depth == 16 {
								order.PutUint16(row[at:], sample)
								at += 2
							} else {
								row[at] = byte(sample)
								at++
							}
						}
						if !o.RGB && o.Photo == 0 {
							maximum := uint16(255)
							if o.Depth == 16 {
								maximum = 65535
							}
							if o.Alpha == 1 {
								maximum = a
							}
							for k := 0; k < 3; k++ {
								values[k] = maximum - values[k]
							}
						}
						switch im := expected.(type) {
						case *image.RGBA64:
							im.SetRGBA64(xx, yy, color.RGBA64{values[0], values[1], values[2], values[3]})
						case *image.RGBA:
							im.SetRGBA(xx, yy, color.RGBA{byte(values[0]), byte(values[1]), byte(values[2]), byte(values[3])})
						case *image.NRGBA64:
							im.SetNRGBA64(xx, yy, color.NRGBA64{values[0], values[1], values[2], values[3]})
						case *image.NRGBA:
							im.SetNRGBA(xx, yy, color.NRGBA{byte(values[0]), byte(values[1]), byte(values[2]), byte(values[3])})
						}
					}
					step := int(o.Depth) / 8 * channels
					if o.Planar {
						sampleBytes := int(o.Depth) / 8
						part := make([]byte, bw*sampleBytes)
						for x := 0; x < bw; x++ {
							copy(part[x*sampleBytes:], row[(x*channels+plane)*sampleBytes:(x*channels+plane+1)*sampleBytes])
						}
						row = part
						step = sampleBytes
					}
					if o.Predictor {
						for at := len(row) - int(o.Depth)/8; at >= step; at -= int(o.Depth) / 8 {
							if o.Depth == 16 {
								order.PutUint16(row[at:], order.Uint16(row[at:])-order.Uint16(row[at-step:]))
							} else {
								row[at] -= row[at-step]
							}
						}
					}
					raw = append(raw, row...)
				}
				var compressed bytes.Buffer
				switch o.Compression {
				case 8, 32946:
					w := zlib.NewWriter(&compressed)
					w.Write(raw)
					w.Close()
				case 5:
					// The standard LZW writer differs at code-width transitions;
					// these small independent blocks stay below the first transition.
					w := lzw.NewWriter(&compressed, lzw.MSB, 8)
					w.Write(raw)
					w.Close()
				case 32773:
					for len(raw) > 0 {
						n := min(128, len(raw))
						compressed.WriteByte(byte(n - 1))
						compressed.Write(raw[:n])
						raw = raw[n:]
					}
				default:
					compressed.Write(raw)
				}
				blocks = append(blocks, compressed.Bytes())
			}
		}
	}
	type field struct {
		tag, kind uint16
		count     uint32
		data      []byte
	}
	var fields []field
	short := func(tag uint16, vals ...uint16) {
		b := []byte{}
		for _, v := range vals {
			b = order.AppendUint16(b, v)
		}
		fields = append(fields, field{tag, 3, uint32(len(vals)), b})
	}
	long := func(tag uint16, vals ...uint32) {
		b := []byte{}
		for _, v := range vals {
			b = order.AppendUint32(b, v)
		}
		fields = append(fields, field{tag, 4, uint32(len(vals)), b})
	}
	long(256, uint32(o.Width))
	long(257, uint32(o.Height))
	bits := make([]uint16, channels)
	for i := range bits {
		bits[i] = o.Depth
	}
	short(258, bits...)
	short(259, o.Compression)
	photo := o.Photo
	if o.RGB {
		photo = 2
	}
	short(262, photo)
	offTag, countTag := uint16(273), uint16(279)
	if o.Tiled {
		offTag, countTag = 324, 325
		long(322, 8)
		long(323, 8)
	} else {
		long(278, 2)
	}
	short(274, o.Orientation)
	short(277, uint16(channels))
	layout := uint16(1)
	if o.Planar {
		layout = 2
	}
	short(284, layout)
	if o.Alpha != 0 {
		short(338, o.Alpha)
	}
	if o.Predictor {
		short(317, 2)
	}
	if len(o.ICC) > 0 {
		fields = append(fields, field{34675, 7, uint32(len(o.ICC)), o.ICC})
	}
	long(offTag, make([]uint32, len(blocks))...)
	counts := make([]uint32, len(blocks))
	for i, b := range blocks {
		counts[i] = uint32(len(b))
	}
	long(countTag, counts...)
	// Sort IFD entries, then reserve their fixed-size table before external data.
	for i := 1; i < len(fields); i++ {
		for j := i; j > 0 && fields[j].tag < fields[j-1].tag; j-- {
			fields[j], fields[j-1] = fields[j-1], fields[j]
		}
	}
	data := append(header, make([]byte, 2+12*len(fields)+4)...)
	order.PutUint16(data[8:], uint16(len(fields)))
	offsets := make([]uint32, len(blocks))
	for position := range blocks {
		i := position
		if o.ReverseBlocks {
			i = len(blocks) - 1 - position
		}
		offsets[i] = uint32(len(data))
		data = append(data, blocks[i]...)
	}
	for i, f := range fields {
		if f.tag == offTag {
			f.data = nil
			for _, v := range offsets {
				f.data = order.AppendUint32(f.data, v)
			}
		}
		at := 10 + i*12
		order.PutUint16(data[at:], f.tag)
		order.PutUint16(data[at+2:], f.kind)
		order.PutUint32(data[at+4:], f.count)
		if len(f.data) <= 4 {
			copy(data[at+8:at+12], f.data)
		} else {
			if len(data)%2 != 0 {
				data = append(data, 0)
			}
			order.PutUint32(data[at+8:], uint32(len(data)))
			data = append(data, f.data...)
		}
	}
	return data, expected
}

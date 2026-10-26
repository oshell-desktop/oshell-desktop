package vt

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"math/bits"
)

// SwiftTerm starts at the first palette selector, ignores raster attributes,
// and sizes the image by painted pixels (not by complete six-pixel bands).
// Keep those rules, while bounding both allocation and repeated overdraw.
func decodeSixel(data []byte, r inlineImageRequest, defaults [16]uint32) *image.NRGBA {
	if len(data) > 8<<20 || r.width == 0 || r.height == 0 || r.cols == 0 || r.rows == 0 {
		return nil
	}
	start := bytes.IndexByte(data, '#')
	if start < 0 {
		return nil
	}
	data = data[start:]
	w, h, ok := scanSixel(data, defaults, nil)
	if !ok || !inlineImageSize(w, h) {
		return nil
	}
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	if _, _, ok := scanSixel(data, defaults, im); !ok {
		return nil
	}
	params := map[string]string{}
	if uint64(w) > uint64(r.width) {
		params["width"] = "100%"
	}
	return sizeInlineImage(im, params, r)
}

func scanSixel(data []byte, defaults [16]uint32, im *image.NRGBA) (width, height int, ok bool) {
	palette := make(map[int]color.NRGBA)
	x, y, index, repeat := 0, 0, 15, 1
	work := 0
	for p := 0; p < len(data); {
		b := data[p]
		p++
		switch {
		case b == '#':
			var values [5]int
			n := 0
			if p+1 < len(data) && data[p] == ';' {
				p++
			}
			for {
				value, found, valid := sixelNumber(data, &p)
				if !valid {
					return 0, 0, false
				}
				if !found {
					break
				}
				if n < len(values) {
					values[n] = value
				}
				n++
				if p == len(data) || data[p] != ';' {
					break
				}
				p++
			}
			if n == 0 {
				return width, height, true
			}
			index = values[0]
			if im != nil && n >= 5 {
				if len(palette) >= 65536 {
					return 0, 0, false
				}
				switch values[1] {
				case 1:
					palette[index] = sixelHLS(values[2], values[3], values[4])
				case 2:
					palette[index] = color.NRGBA{pctByte(values[2]), pctByte(values[3]), pctByte(values[4]), 255}
				}
			}
		case b == '!':
			value, found, valid := sixelNumber(data, &p)
			if !valid {
				return 0, 0, false
			}
			if found {
				repeat = value
			}
		case b == '$':
			x = 0
		case b == '-':
			x = 0
			y += 6
			if y > TerminalImageLimit/4 {
				return 0, 0, false
			}
		case b >= '?' && b <= '~':
			mask := b - '?'
			if repeat > TerminalImageLimit/4-x {
				return 0, 0, false
			}
			if repeat > 0 && mask != 0 {
				height = max(height, y+bits.Len8(mask))
			}
			width = max(width, x+repeat)
			if height > 0 && !inlineImageSize(width, height) {
				return 0, 0, false
			}
			if im != nil && mask != 0 {
				work += repeat * bits.OnesCount8(mask)
				if work > 32<<20 {
					return 0, 0, false
				}
				c, found := palette[index]
				if !found && index < len(defaults) {
					v := defaults[index]
					c = color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
				}
				for k := range 6 {
					if mask&(1<<k) == 0 {
						continue
					}
					for col := x; col < x+repeat; col++ {
						im.SetNRGBA(col, y+k, c)
					}
				}
			}
			x += repeat
			repeat = 1
		}
	}
	return width, height, true
}

func sixelNumber(data []byte, p *int) (n int, found, valid bool) {
	for *p < len(data) && data[*p] >= '0' && data[*p] <= '9' {
		d := int(data[*p] - '0')
		if n > (math.MaxInt32-d)/10 {
			return 0, false, false
		}
		n = n*10 + d
		*p += 1
		found = true
	}
	return n, found, true
}

func pctByte(n int) uint8 { return uint8(max(0, min(100, n)) * 255 / 100) }

// Sixel uses the HLS hue wheel with blue at 0°, red at 120°, green at
// 240°. Keep SwiftTerm's truncation to whole percentages before byte scaling.
func sixelHLS(hue, lightness, saturation int) color.NRGBA {
	h := float64((max(0, min(hue, 360)) + 240) % 360)
	l, s := float64(max(0, min(lightness, 100))), float64(max(0, min(saturation, 100)))
	// Evaluate the high and low endpoints independently, as SwiftTerm does.
	// Reconstructing high as low+chroma can round 100 down to 99 before the
	// integer percentage conversion (e.g. HLS 0;93;100 becomes 252, not 255).
	high := l + s*(1-math.Abs(2*l/100-1))/2
	low := l - s*(1-math.Abs(2*l/100-1))/2
	var r, g, b float64
	switch int(h / 60) {
	case 0:
		r, g, b = high, low+(high-low)*(h/60), low
	case 1:
		r, g, b = low+(high-low)*((120-h)/60), high, low
	case 2:
		r, g, b = low, high, low+(high-low)*((h-120)/60)
	case 3:
		r, g, b = low, low+(high-low)*((240-h)/60), high
	case 4:
		r, g, b = low+(high-low)*((h-240)/60), low, high
	case 5:
		r, g, b = high, low, low+(high-low)*((360-h)/60)
	}
	return color.NRGBA{pctByte(int(r)), pctByte(int(g)), pctByte(int(b)), 255}
}

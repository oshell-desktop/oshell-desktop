package tiff

import (
	"image"
	"image/color"
)

const (
	tPlanarConfiguration = 284
)

const (
	mGrayAlpha imageMode = mCMYK + 1 + iota
	mGrayAlphaPremultiplied
)

func (d *decoder) configureGrayAlpha() error {
	bits := d.features[tBitsPerSample]
	extra := d.features[tExtraSamples]
	if len(bits) != 2 || (bits[0] != 8 && bits[0] != 16) || bits[1] != bits[0] ||
		d.firstVal(tSamplesPerPixel) != 2 || len(extra) != 1 || (extra[0] != 1 && extra[0] != 2) {
		return UnsupportedError("gray alpha sample layout")
	}
	if planar := d.firstVal(tPlanarConfiguration); planar > 2 {
		return FormatError("planar configuration")
	}
	d.mode = mGrayAlpha
	d.config.ColorModel = color.NRGBAModel
	if d.bpp == 16 {
		d.config.ColorModel = color.NRGBA64Model
	}
	if extra[0] == 1 {
		d.mode = mGrayAlphaPremultiplied
		d.config.ColorModel = color.RGBAModel
		if d.bpp == 16 {
			d.config.ColorModel = color.RGBA64Model
		}
	}
	return nil
}

// Prediction and decompression are shared with the upstream decoder. Keep two
// samples per pixel when advancing over padded tiles and the end of each row.
func (d *decoder) decodeGrayAlpha(dst image.Image, xmin, ymin, xmax, ymax int) error {
	step := int(d.bpp) / 4
	rowBytes := (xmax - xmin) * step
	invert := d.firstVal(tPhotometricInterpretation) == pWhiteIsZero
	premultiplied := d.mode == mGrayAlphaPremultiplied
	for y := ymin; y < min(ymax, dst.Bounds().Max.Y); y++ {
		for x := xmin; x < min(xmax, dst.Bounds().Max.X); x++ {
			at := (y-ymin)*rowBytes + (x-xmin)*step
			if at+step > len(d.buf) {
				return errNoPixels
			}
			var v, a uint16
			if d.bpp == 16 {
				v, a = d.byteOrder.Uint16(d.buf[at:]), d.byteOrder.Uint16(d.buf[at+2:])
			} else {
				v, a = uint16(d.buf[at])*257, uint16(d.buf[at+1])*257
			}
			if invert {
				if premultiplied {
					v = a - min(v, a)
				} else {
					v = 65535 - v
				}
			}
			if premultiplied && a == 0 {
				v = 0
			}
			switch im := dst.(type) {
			case *image.NRGBA:
				im.SetNRGBA(x, y, color.NRGBA{byte(v >> 8), byte(v >> 8), byte(v >> 8), byte(a >> 8)})
			case *image.NRGBA64:
				im.SetNRGBA64(x, y, color.NRGBA64{v, v, v, a})
			case *image.RGBA:
				im.SetRGBA(x, y, color.RGBA{byte(v >> 8), byte(v >> 8), byte(v >> 8), byte(a >> 8)})
			case *image.RGBA64:
				im.SetRGBA64(x, y, color.RGBA64{v, v, v, a})
			}
		}
	}
	return nil
}

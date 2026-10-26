package tiff

import "image"

func (d *decoder) validatePlanar() error {
	bits := d.features[tBitsPerSample]
	if len(bits) == 1 {
		return nil // TIFF makes PlanarConfiguration irrelevant for one component.
	}
	layout := d.firstVal(tPlanarConfiguration)
	if layout > 2 {
		return FormatError("planar configuration")
	}
	if layout != 2 {
		return nil
	}
	if d.firstVal(tSamplesPerPixel) != uint(len(bits)) {
		return FormatError("planar sample count")
	}
	if len(bits) > 4 || (d.bpp != 8 && d.bpp != 16) {
		return UnsupportedError("planar samples")
	}
	for _, bit := range bits {
		if bit != d.bpp {
			return UnsupportedError("unequal planar sample depths")
		}
	}
	return nil
}

func (d *decoder) planes() int {
	if d.firstVal(tPlanarConfiguration) == 2 {
		return len(d.features[tBitsPerSample])
	}
	return 1
}

// TIFF indexes all blocks of the first channel, then all blocks of the next.
// Combine one block at a time, not entire channel images. Interleaving horizontal
// differences before applying the shared predictor is equivalent to reversing
// prediction in each plane: the predictor always steps by samples per pixel.
func (d *decoder) decodePlanarBlock(dst image.Image, x, y, w, h, index, blocksPerPlane int, offsets, counts []uint) error {
	planes, sampleBytes := d.planes(), int(d.bpp)/8
	// Dimensions were checked against eight bytes/pixel before this point.
	// Supported planar layouts use at most four two-byte samples per pixel.
	planeSize := w * h * sampleBytes
	combined := make([]byte, planeSize*planes)
	for plane := 0; plane < planes; plane++ {
		at := plane*blocksPerPlane + index
		if err := d.readBlock(int64(offsets[at]), int64(counts[at]), w, h, int64(w)*int64(h)*8); err != nil {
			return err
		}
		if len(d.buf) < planeSize {
			return errNoPixels
		}
		for src := 0; src < planeSize; src += sampleBytes {
			dst := (src/sampleBytes*planes + plane) * sampleBytes
			copy(combined[dst:dst+sampleBytes], d.buf[src:src+sampleBytes])
		}
	}
	// Keep the decompression buffer for the next block; it must not alias the
	// interleaved block while subsequent channel reads are in progress.
	scratch := d.buf
	d.buf = combined
	err := d.decode(dst, x, y, x+w, y+h)
	d.buf = scratch
	return err
}

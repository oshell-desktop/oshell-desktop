package tiff

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"testing"

	"oshell-desktop/internal/tiff/testfixture"
)

func TestGrayAlpha(t *testing.T) {
	for _, depth := range []uint16{8, 16} {
		for _, alpha := range []uint16{1, 2} {
			for _, photo := range []uint16{0, 1} {
				for _, big := range []bool{false, true} {
					for _, compression := range []uint16{1, 8, 32946, 32773, 5} {
						for _, predictor := range []bool{false, true} {
							for _, tiled := range []bool{false, true} {
								t.Run(fmt.Sprint(depth, "/", alpha, "/", photo, "/", big, "/", compression, "/", predictor, "/", tiled), func(t *testing.T) {
									data, want := testfixture.Gray(testfixture.GrayOptions{Depth: depth, Alpha: alpha, Photo: photo, BigEndian: big, Compression: compression, Predictor: predictor, Tiled: tiled, Width: 9, Height: 11, Orientation: 1})
									config, err := DecodeConfig(bytes.NewReader(data))
									if err != nil {
										t.Fatal(err)
									}
									if config.Width != 9 || config.Height != 11 {
										t.Fatal(config)
									}
									got, err := Decode(bytes.NewReader(data))
									if err != nil {
										t.Fatal(err)
									}
									for y := 0; y < 11; y++ {
										for x := 0; x < 9; x++ {
											if !reflect.DeepEqual(got.At(x, y), want.At(x, y)) {
												t.Fatalf("(%d,%d): got %#v want %#v", x, y, got.At(x, y), want.At(x, y))
											}
										}
									}
								})
							}
						}
					}
				}
			}
		}
	}
}

func TestGrayAlphaInvalidLayout(t *testing.T) {
	base, _ := testfixture.Gray(testfixture.GrayOptions{Depth: 16, Alpha: 2, Photo: 1, Width: 9, Height: 11, Compression: 1, Orientation: 1})
	for _, tc := range []struct {
		tag   uint16
		value uint32
	}{
		{tBitsPerSample, 16 | (8 << 16)}, {tSamplesPerPixel, 1}, {tExtraSamples, 0}, {tExtraSamples, 3}, {tPlanarConfiguration, 3},
	} {
		data := bytes.Clone(base)
		for pos := 10; pos < 10+12*int(binary.LittleEndian.Uint16(data[8:])); pos += 12 {
			if binary.LittleEndian.Uint16(data[pos:]) == tc.tag {
				binary.LittleEndian.PutUint32(data[pos+8:], tc.value)
			}
		}
		if _, err := DecodeConfig(bytes.NewReader(data)); err == nil {
			t.Fatal("invalid gray layout accepted", tc.tag)
		}
	}
	// A valid directory with a shortened pixel block must fail without a panic.
	for _, tiled := range []bool{false, true} {
		data, _ := testfixture.Gray(testfixture.GrayOptions{Depth: 16, Alpha: 2, Photo: 1, Width: 9, Height: 11, Compression: 1, Orientation: 1, Tiled: tiled})
		tag := uint16(tStripByteCounts)
		if tiled {
			tag = tTileByteCounts
		}
		for pos := 10; pos < 10+12*int(binary.LittleEndian.Uint16(data[8:])); pos += 12 {
			if binary.LittleEndian.Uint16(data[pos:]) == tag {
				at := binary.LittleEndian.Uint32(data[pos+8:])
				binary.LittleEndian.PutUint32(data[at:], 1)
			}
		}
		if _, err := Decode(bytes.NewReader(data)); err == nil {
			t.Fatal("short gray pixels accepted")
		}
	}
}

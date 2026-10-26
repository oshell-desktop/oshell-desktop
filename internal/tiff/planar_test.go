package tiff

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/color"
	"io"
	"reflect"
	"testing"

	"oshell-desktop/internal/tiff/testfixture"
)

func TestPlanarSamples(t *testing.T) {
	for _, rgb := range []bool{false, true} {
		for _, depth := range []uint16{8, 16} {
			for _, alpha := range []uint16{0, 1, 2} {
				if !rgb && alpha == 0 {
					continue
				}
				for _, photo := range []uint16{0, 1} {
					if rgb && photo == 0 {
						continue
					}
					for _, big := range []bool{false, true} {
						for _, compression := range []uint16{1, 8, 32946, 32773, 5} {
							for _, predictor := range []bool{false, true} {
								for _, tiled := range []bool{false, true} {
									t.Run(fmt.Sprint(rgb, "/", depth, "/", alpha, "/", photo, "/", big, "/", compression, "/", predictor, "/", tiled), func(t *testing.T) {
										data, want := testfixture.Gray(testfixture.GrayOptions{RGB: rgb, Depth: depth, Alpha: alpha, Photo: photo, BigEndian: big, Compression: compression, Predictor: predictor, Tiled: tiled, Width: 9, Height: 11, Orientation: 1, Planar: true, ReverseBlocks: big})
										var input io.Reader = bytes.NewReader(data)
										if big {
											input = ioReader{bytes.NewReader(data)}
										}
										got, err := Decode(input)
										if err != nil {
											t.Fatal(err)
										}
										for y := 0; y < 11; y++ {
											for x := 0; x < 9; x++ {
												if !reflect.DeepEqual(got.At(x, y), want.At(x, y)) {
													t.Fatalf("(%d,%d): %#v want %#v", x, y, got.At(x, y), want.At(x, y))
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
}

func TestSingleComponentPlanarIgnored(t *testing.T) {
	for _, layout := range []uint16{0, 1, 2, 3} {
		order := binary.BigEndian
		data := append(newTIFF(order), 123, 0)
		data = appendIFD(data, order, map[uint16]any{
			tImageWidth: uint32(1), tImageLength: uint32(1), tSamplesPerPixel: uint16(1),
			tBitsPerSample: uint16(8), tPhotometricInterpretation: uint16(pBlackIsZero),
			tCompression: uint16(cNone), tPlanarConfiguration: layout, tStripOffsets: uint32(8), tStripByteCounts: uint32(1),
		})
		got, err := Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(layout, err)
		}
		if got.At(0, 0) != (color.Gray{Y: 123}) {
			t.Fatal(layout, got.At(0, 0))
		}
	}
}

func TestPlanarMissingChannel(t *testing.T) {
	for _, tiled := range []bool{false, true} {
		for _, rgb := range []bool{false, true} {
			base, _ := testfixture.Gray(testfixture.GrayOptions{Planar: true, RGB: rgb, Depth: 16, Alpha: 2, Photo: 1, Width: 9, Height: 11, Tiled: tiled, Compression: 8, Orientation: 1})
			offTag, countTag := uint16(tStripOffsets), uint16(tStripByteCounts)
			if tiled {
				offTag, countTag = tTileOffsets, tTileByteCounts
			}
			for _, tag := range []uint16{offTag, countTag} {
				data := bytes.Clone(base)
				for at := 10; at < 10+12*int(binary.LittleEndian.Uint16(data[8:])); at += 12 {
					if binary.LittleEndian.Uint16(data[at:]) == tag {
						n := binary.LittleEndian.Uint32(data[at+4:])
						binary.LittleEndian.PutUint32(data[at+4:], n-1)
					}
				}
				if _, err := Decode(bytes.NewReader(data)); err == nil {
					t.Fatal("missing channel entry accepted", tiled, rgb, tag)
				}
			}
			for _, badOffset := range []uint32{0, 0xfffffffe} {
				data := bytes.Clone(base)
				for at := 10; at < 10+12*int(binary.LittleEndian.Uint16(data[8:])); at += 12 {
					if binary.LittleEndian.Uint16(data[at:]) == offTag {
						n := binary.LittleEndian.Uint32(data[at+4:])
						p := binary.LittleEndian.Uint32(data[at+8:])
						binary.LittleEndian.PutUint32(data[p+4*(n-1):], badOffset)
					}
				}
				if _, err := Decode(bytes.NewReader(data)); err == nil {
					t.Fatal("missing alpha plane data accepted", tiled, rgb, badOffset)
				}
			}
		}
	}
}

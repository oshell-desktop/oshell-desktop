package core

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"

	"oshell-desktop/internal/tiff/testfixture"
)

func TestClipboardPlanarTIFF(t *testing.T) {
	corners := [8][4]int{{0, 1, 2, 3}, {1, 0, 3, 2}, {3, 2, 1, 0}, {2, 3, 0, 1}, {0, 2, 1, 3}, {2, 0, 3, 1}, {3, 1, 2, 0}, {1, 3, 0, 2}}
	source := [4]image.Point{{0, 0}, {2, 0}, {0, 3}, {2, 3}}
	for _, rgb := range []bool{false, true} {
		profile := clipboardTestICC("GRAY")
		if rgb {
			profile = clipboardTestICC("RGB ")
		}
		for _, depth := range []uint16{8, 16} {
			for _, alpha := range []uint16{0, 1, 2} {
				if !rgb && alpha == 0 {
					continue
				}
				for orientation := uint16(1); orientation <= 8; orientation++ {
					t.Run(fmt.Sprint(rgb, "/", depth, "/", alpha, "/", orientation), func(t *testing.T) {
						data, want := testfixture.Gray(testfixture.GrayOptions{RGB: rgb, Planar: true, ReverseBlocks: true, Depth: depth, Alpha: alpha, Photo: 1, Width: 3, Height: 4, Compression: 8, Predictor: true, BigEndian: orientation%2 == 0, Tiled: orientation%2 == 1, Orientation: orientation, ICC: profile})
						out, err := NormalizeClipboardImage(data)
						if err != nil {
							t.Fatal(err)
						}
						assertPNGProfile(t, out, profile)
						if out[24] != byte(depth) {
							t.Fatal("bit depth lost", out[24])
						}
						got, err := png.Decode(bytes.NewReader(out))
						if err != nil {
							t.Fatal(err)
						}
						w, h := 3, 4
						if orientation >= 5 {
							w, h = h, w
						}
						if got.Bounds() != image.Rect(0, 0, w, h) {
							t.Fatal("orientation dimensions", got.Bounds())
						}
						points := [4]image.Point{{0, 0}, {w - 1, 0}, {0, h - 1}, {w - 1, h - 1}}
						for i, p := range points {
							s := source[corners[orientation-1][i]]
							if depth == 8 {
								actual := color.NRGBAModel.Convert(got.At(p.X, p.Y))
								expected := color.NRGBAModel.Convert(want.At(s.X, s.Y))
								if actual != expected {
									t.Fatalf("pixel %v: %v want %v", p, actual, expected)
								}
							} else if actual, expected := grayAlphaSample(got.At(p.X, p.Y)), grayAlphaSample(want.At(s.X, s.Y)); actual != expected {
								t.Fatalf("pixel %v: %v want %v", p, actual, expected)
							}
						}
					})
				}
			}
		}
	}
}

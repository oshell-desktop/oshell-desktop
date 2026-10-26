package core

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"testing"

	"oshell-desktop/internal/tiff/testfixture"
)

func TestClipboardTIFFGrayAlpha(t *testing.T) {
	corners := [8][4]int{{0, 1, 2, 3}, {1, 0, 3, 2}, {3, 2, 1, 0}, {2, 3, 0, 1}, {0, 2, 1, 3}, {2, 0, 3, 1}, {3, 1, 2, 0}, {1, 3, 0, 2}}
	source := [4]image.Point{{0, 0}, {2, 0}, {0, 3}, {2, 3}}
	for _, depth := range []uint16{8, 16} {
		for _, alpha := range []uint16{1, 2} {
			for _, photo := range []uint16{0, 1} {
				for orientation := uint16(1); orientation <= 8; orientation++ {
					t.Run(fmt.Sprint(depth, "/", alpha, "/", photo, "/", orientation), func(t *testing.T) {
						data, want := testfixture.Gray(testfixture.GrayOptions{Depth: depth, Alpha: alpha, Photo: photo, Width: 3, Height: 4, Compression: 8, Predictor: true, BigEndian: orientation%2 == 0, Orientation: orientation, ICC: clipboardTestICC("GRAY")})
						out, err := NormalizeClipboardImage(data)
						if err != nil {
							t.Fatal(err)
						}
						assertPNGProfile(t, out, clipboardTestICC("GRAY"))
						if out[24] != byte(depth) || out[25] != 4 {
							t.Fatal("gray bit depth/type changed", out[24:26])
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
							expected := grayAlphaSample(want.At(s.X, s.Y))
							if depth == 8 {
								expected.R = uint16(byte(expected.R>>8)) * 257
								expected.G = expected.R
								expected.B = expected.R
							}
							if actual := grayAlphaSample(got.At(p.X, p.Y)); actual != expected {
								t.Fatalf("pixel %v: %v want %v", p, actual, expected)
							}
						}
					})
				}
			}
		}
	}
	data, _ := testfixture.Gray(testfixture.GrayOptions{Depth: 8, Alpha: 2, Photo: 1, Width: 3, Height: 4, Compression: 1, Orientation: 1, ICC: clipboardTestICC("RGB ")})
	if _, err := NormalizeClipboardImage(data); err == nil {
		t.Fatal("RGB profile accepted on gray TIFF source")
	}
}

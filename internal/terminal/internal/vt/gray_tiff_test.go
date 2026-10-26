package vt

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"oshell-desktop/internal/tiff/testfixture"
)

func TestGrayTIFFTerminalImages(t *testing.T) {
	for _, planar := range []bool{false, true} {
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
						for _, protocol := range []string{"kitty", "osc1337"} {
							t.Run(fmt.Sprint(planar, "/", rgb, "/", depth, "/", alpha, "/", photo, "/", protocol), func(t *testing.T) {
								term, replies := kittyCacheTerminal(t)
								data, want := testfixture.Gray(testfixture.GrayOptions{Planar: planar, RGB: rgb, ReverseBlocks: true, Depth: depth, Alpha: alpha, Photo: photo, Width: 9, Height: 7, Compression: 8, Predictor: true, Tiled: true, Orientation: 1})
								var seq []byte
								if protocol == "kitty" {
									seq = kittyCommand("a=T,f=100,i=73,p=1,C=1", data)
								} else {
									seq = append([]byte("\x1b]1337;File="), inlinePayload("", data)...)
									seq = append(seq, 7)
								}
								for _, b := range seq {
									term.Write([]byte{b})
								}
								g := term.Graphics(10, 20)
								height := 7
								if protocol == "osc1337" {
									height = 20
								} // full-height transparent row stripe
								if len(g) != 1 || g[0].Image.Bounds() != image.Rect(0, 0, 9, height) {
									t.Fatal("gray TIFF not displayed", replies.String(), g)
								}
								for y := 7; y < height; y++ {
									for x := 0; x < 9; x++ {
										if g[0].Image.NRGBAAt(x, y) != (color.NRGBA{}) {
											t.Fatal("opaque stripe padding")
										}
									}
								}
								for y := 0; y < 7; y++ {
									for x := 0; x < 9; x++ {
										expected := color.NRGBAModel.Convert(want.At(x, y)).(color.NRGBA)
										if protocol == "osc1337" && expected.A == 0 {
											// Rasterization into a transparent row canvas discards
											// invisible RGB; straight encoded Kitty pixels retain it.
											expected = color.NRGBA{}
										}
										if got := g[0].Image.At(x, y); got != expected {
											t.Fatalf("(%d,%d): %#v want %#v", x, y, got, expected)
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

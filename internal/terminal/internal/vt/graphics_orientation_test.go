package vt

import (
	"fmt"
	"image"
	"image/color"
	"testing"
)

// Swift's PNG display uses NSImage orientation unless any crop is requested;
// cropRgba and the final protocol cursor grid use unrotated ImageIO dimensions.
func TestKittyPNGDisplayOrientation(t *testing.T) {
	corners := [8][4]int{{0, 1, 2, 3}, {1, 0, 3, 2}, {3, 2, 1, 0}, {2, 3, 0, 1}, {0, 2, 1, 3}, {2, 0, 3, 1}, {3, 1, 2, 0}, {1, 3, 0, 2}}
	sourcePoints := [4]image.Point{{0, 0}, {29, 0}, {0, 9}, {29, 9}}
	for orientation := byte(1); orientation <= 8; orientation++ {
		for _, tc := range []struct {
			control    string
			w, h, x, y int
		}{
			{"", 0, 0, 3, 1}, {"c=3", 30, 0, 3, 1}, {"r=3", 0, 60, 18, 3}, {"c=3,r=2", 30, 40, 3, 2},
		} {
			t.Run(fmt.Sprintf("%d/%s", orientation, tc.control), func(t *testing.T) {
				term, replies := graphicTerminal(t)
				data, raw := inlineOrientedImage(t, "png", orientation)
				term.Write(kittyCommand("a=T,f=100,i=7,p=1,"+tc.control, data))
				g := term.Graphics(10, 20)
				if len(g) != 1 {
					t.Fatalf("no image: %q", replies.String())
				}
				sw, sh := 30, 10
				if orientation >= 5 {
					sw, sh = sh, sw
				}
				w, h := tc.w, tc.h
				if w == 0 && h == 0 {
					w, h = sw, sh
				} else if w == 0 {
					w = h * sw / sh
				} else if h == 0 {
					h = w * sh / sw
				}
				if g[0].Image.Bounds() != image.Rect(0, 0, sw, sh) || g[0].Source != image.Rect(0, 0, sw, sh) || g[0].Width != w || g[0].Height != h {
					t.Fatalf("wrong oriented geometry: %+v", g[0])
				}
				points := [4]image.Point{{0, 0}, {sw - 1, 0}, {0, sh - 1}, {sw - 1, sh - 1}}
				for i, p := range points {
					s := sourcePoints[corners[orientation-1][i]]
					if got, want := g[0].Image.NRGBAAt(p.X, p.Y), color.NRGBAModel.Convert(raw.At(s.X, s.Y)); got != want {
						t.Fatalf("corner %d: %v want %v", i, got, want)
					}
				}
				if x, y := term.Cursor(); x != tc.x || y != tc.y {
					t.Fatalf("protocol cursor %d,%d want %d,%d", x, y, tc.x, tc.y)
				}
			})
		}
	}
}

func TestKittyPNGCropKeepsRawCoordinates(t *testing.T) {
	for orientation := byte(2); orientation <= 8; orientation++ {
		t.Run(fmt.Sprint(orientation), func(t *testing.T) {
			term, _ := graphicTerminal(t)
			data, raw := inlineOrientedImage(t, "png", orientation)
			// Preserve EXIF across protocol chunks, and retain both views of one
			// cached generation without sharing transformed pixels incorrectly.
			term.Write(kittyCommand("a=t,f=100,i=7,m=1", data[:len(data)/2/3*3]))
			term.Write(kittyCommand("m=0", data[len(data)/2/3*3:]))
			term.Write(kittyCommand("a=p,i=7,p=1,C=1", nil))
			term.Write(kittyCommand("a=p,i=7,p=2,C=1,w=30,h=10", nil))
			g := term.Graphics(10, 20)
			if len(g) != 2 || g[0].Image == g[1].Image || g[0].Generation != g[1].Generation {
				t.Fatal("raw/oriented views not isolated")
			}
			if g[1].Source != image.Rect(0, 0, 30, 10) || g[1].Width != 30 || g[1].Height != 10 || g[1].Image.NRGBAAt(0, 0) != color.NRGBAModel.Convert(raw.At(0, 0)) {
				t.Fatal("explicit full crop was rotated")
			}
			again := term.Graphics(10, 20)
			if again[0].Image != g[0].Image || again[1].Image != g[1].Image {
				t.Fatal("unchanged views were recopied")
			}
			term.Write(kittyCommand("a=p,i=7,p=2,C=1,x=2,y=1,w=6,h=3,c=3", nil))
			g = term.Graphics(10, 20)
			if len(g) != 2 || g[1].Source != image.Rect(2, 1, 8, 4) || g[1].Width != 30 || g[1].Height != 15 {
				t.Fatal("crop coordinates/ratio were rotated")
			}
			term.Write(kittyCommand("a=d,d=i,i=7,p=1", nil))
			if len(term.Graphics(10, 20)) != 1 || len(term.graphicsCache) != 1 {
				t.Fatal("obsolete oriented view not released")
			}
		})
	}
}

func TestKittyPNGOrientedRowsAndCursor(t *testing.T) {
	for _, preserve := range []bool{false, true} {
		t.Run(fmt.Sprint(preserve), func(t *testing.T) {
			term, _ := graphicTerminal(t)
			data, _ := inlineOrientedImage(t, "png", 6)
			term.Write([]byte("\x1b[10;1H"))
			control := "a=T,f=100,i=7,p=1"
			if preserve {
				control += ",C=1"
			}
			term.Write(kittyCommand(control, data))
			if total := term.Scrollbar().Total - 10; total != 2 {
				t.Fatalf("oriented raster should insert 2 rows, history=%d", total)
			}
			g := term.Graphics(10, 20)
			if len(g) != 1 || g[0].Y != 140 || g[0].Height != 30 {
				t.Fatalf("scrolled oriented geometry %+v", g)
			}
			x, y := term.Cursor()
			if preserve && (x != 0 || y != 9) || !preserve && (x != 3 || y != 8) {
				t.Fatalf("cursor %d,%d", x, y)
			}
		})
	}
}

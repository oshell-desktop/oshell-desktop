package vt

import (
	"bytes"
	"fmt"
	"image"
	"strings"
	"testing"
)

// Expected geometry follows SwiftTerm displayKittyImage, insertImage and
// cropRgba. Pixel offsets translate the canvas; they do not reduce its size.
func TestKittyCropAndOffsetCanvas(t *testing.T) {
	for _, format := range []string{"rgb", "png"} {
		for _, tc := range []struct {
			control string
			source  image.Rectangle
			w, h    int
		}{
			{"", image.Rect(0, 0, 54, 36), 54, 36},
			{"c=3", image.Rect(0, 0, 54, 36), 30, 20},
			{"r=1", image.Rect(0, 0, 54, 36), 30, 20},
			{"c=2,r=2", image.Rect(0, 0, 54, 36), 20, 40},
			{"x=6,y=4", image.Rect(6, 4, 54, 36), 48, 32},
			{"x=6,y=4,w=999,h=999", image.Rect(6, 4, 54, 36), 48, 32},
			{"x=6,y=4,w=6,h=9,c=2", image.Rect(6, 4, 12, 13), 20, 30},
			{"x=6,y=4,w=6,h=9,r=3", image.Rect(6, 4, 12, 13), 40, 60},
		} {
			t.Run(format+"/"+tc.control, func(t *testing.T) {
				term, replies := graphicTerminal(t)
				data := make([]byte, 54*36*3)
				control := "f=24,s=54,v=36"
				if format == "png" {
					data = inlinePNG(t, 54, 36)
					control = "f=100"
				}
				term.Write(kittyCommand("a=t,i=7,"+control, data))
				term.Write(kittyCommand("a=p,i=7,p=1,C=1,X=3,Y=4,"+tc.control, nil))
				g := term.Graphics(10, 20)
				if len(g) != 1 {
					t.Fatalf("placements %d, reply %q", len(g), replies.String())
				}
				if g[0].Source != tc.source || g[0].Width != tc.w || g[0].Height != tc.h || g[0].X != 3 || g[0].Y != 4 {
					t.Fatalf("geometry %v source %v, want %dx%d source %v", image.Rect(g[0].X, g[0].Y, g[0].X+g[0].Width, g[0].Y+g[0].Height), g[0].Source, tc.w, tc.h, tc.source)
				}
			})
		}
	}
}

func TestKittyInvalidCropKeepsPlacement(t *testing.T) {
	for _, retransmit := range []bool{false, true} {
		for _, crop := range []string{"x=3", "y=2", "x=4294967295,w=1", "y=4294967295,h=1"} {
			t.Run(fmt.Sprintf("%v/%s", retransmit, crop), func(t *testing.T) {
				term, replies := graphicTerminal(t)
				term.Write(kittyCommand("a=T,f=24,s=3,v=2,i=7,p=1,C=1", make([]byte, 18)))
				before := term.Graphics(10, 20)
				if len(before) != 1 {
					t.Fatal("missing initial image")
				}
				x, y := term.Cursor()
				replies.Reset()
				if retransmit {
					term.Write(kittyCommand("a=T,f=24,s=3,v=2,i=7,p=1,"+crop, bytes.Repeat([]byte{0, 0, 255}, 6)))
				} else {
					term.Write(kittyCommand("a=p,i=7,p=1,"+crop, nil))
				}
				after := term.Graphics(10, 20)
				if !strings.Contains(replies.String(), "EINVAL: bad crop") {
					t.Fatalf("wrong response %q", replies.String())
				}
				if len(after) != 1 || after[0].Generation != before[0].Generation || after[0].Image != before[0].Image || after[0].Source != before[0].Source {
					t.Fatal("invalid crop destroyed original placement")
				}
				if nx, ny := term.Cursor(); nx != x || ny != y {
					t.Fatal("invalid crop moved cursor")
				}
				// The cached source is still usable after the rejected placement.
				term.Write(kittyCommand("a=p,i=7,p=1,C=1,x=1,y=1,w=99,h=99", nil))
				after = term.Graphics(10, 20)
				if len(after) != 1 || after[0].Source != image.Rect(1, 1, 3, 2) {
					t.Fatal("valid retry failed", fmt.Sprint(after))
				}
				if retransmit && after[0].Image.Pix[2] != 255 {
					t.Fatal("rejected display lost new cached source")
				}
			})
		}
	}
}

package vt

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestKittyFractionalDimensionsKeepGridAndCanvasDistinct(t *testing.T) {
	for _, tc := range []struct {
		name, size string
		sw, sh     int
		w, h       int
		cursor     string
	}{
		{"height-above-row", "c=1", 101, 203, 10, 21, "\x1b[3;2R"},
		{"height-below-row", "c=1", 101, 201, 10, 20, "\x1b[2;2R"},
		{"width-above-cell", "r=1", 101, 200, 10, 20, "\x1b[2;3R"},
		{"width-truncates", "r=1", 49, 100, 9, 20, "\x1b[2;2R"},
		{"fixed-both", "c=1,r=1", 101, 203, 10, 20, "\x1b[2;2R"},
	} {
		for _, format := range []int{24, 100} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, format), func(t *testing.T) {
				term, replies := graphicTerminal(t)
				payload := make([]byte, tc.sw*tc.sh*3)
				if format == 100 {
					payload = inlinePNG(t, tc.sw, tc.sh)
				}
				term.Write(kittyCommand(fmt.Sprintf("a=T,f=%d,s=%d,v=%d,%s,i=91,p=1", format, tc.sw, tc.sh, tc.size), payload))
				g := term.Graphics(10, 20)
				if len(g) != 1 || g[0].Width != tc.w || g[0].Height != tc.h {
					t.Fatalf("canvas want %dx%d: %+v", tc.w, tc.h, g)
				}
				replies.Reset()
				term.Write([]byte("\x1b[6n"))
				if replies.String() != tc.cursor {
					t.Fatalf("cursor %q, want %q", replies.String(), tc.cursor)
				}
			})
		}
	}
}

func TestKittyFractionalRasterSurvivesChunksCropAndResize(t *testing.T) {
	for _, fragment := range []bool{false, true} {
		term, _ := graphicTerminal(t)
		data := inlinePNG(t, 11, 23)
		term.Write(kittyCommand("a=t,f=100,i=91,m=1", data[:len(data)/2/3*3]))
		term.Write(kittyCommand("m=0", data[len(data)/2/3*3:]))
		packet := kittyCommand("a=p,i=91,p=1,c=1,x=2,y=1,w=7,h=9,C=1", nil)
		if fragment {
			for _, b := range packet {
				term.Write([]byte{b})
			}
		} else {
			term.Write(packet)
		}
		g := term.Graphics(10, 20)
		if len(g) != 1 || g[0].Height != 13 || math.Abs(g[0].ExactHeight-90.0/7) > 1e-10 || !g[0].Clipped {
			t.Fatal("fractional crop lost its exact transform", g)
		}
		cached := g[0].Image
		if err := term.Resize(20, 10, 15, 30); err != nil {
			t.Fatal(err)
		}
		g = term.Graphics(15, 30)
		if len(g) != 1 || g[0].Image != cached || g[0].Width != 10 || g[0].Height != 13 || math.Abs(g[0].ExactHeight-90.0/7) > 1e-10 {
			t.Fatal("font change rescaled or recopied fractional raster", g)
		}
	}
}

func TestKittyFractionalPNGOrientationKeepsProtocolGrid(t *testing.T) {
	for orientation := byte(1); orientation <= 8; orientation++ {
		term, _ := graphicTerminal(t)
		data, _ := inlineOrientedImage(t, "png", orientation)
		term.Write(kittyCommand("a=T,f=100,c=1,i=91,p=1", data))
		wantHeight := 4
		if orientation >= 5 {
			wantHeight = 30
		}
		g := term.Graphics(10, 20)
		if len(g) != 1 || g[0].Height != wantHeight {
			t.Fatalf("orientation %d lost fractional sizing: %+v", orientation, g)
		}
		if x, y := term.Cursor(); x != 1 || y != 1 {
			t.Fatalf("orientation %d changed raw protocol cursor: %d,%d", orientation, x, y)
		}
	}
}

func TestKittyFractionalHeightInsertsPartialFinalRow(t *testing.T) {
	term, _ := graphicTerminal(t)
	term.Write([]byte("\x1b[10;2H"))
	term.Write(kittyCommand("a=T,f=100,c=1,C=1", inlinePNG(t, 101, 203)))
	if bar := term.Scrollbar(); bar.Total-bar.Len != 2 {
		t.Fatal("fractional final row did not insert its line feed", bar)
	}
	if x, y := term.Cursor(); x != 1 || y != 9 {
		t.Fatal("C=1 did not preserve cursor after fractional-row scrolling", x, y)
	}
}

func TestKittyCursorAndRasterKeepSeparateRoundingOrder(t *testing.T) {
	term, _ := graphicTerminal(t)
	if err := term.Resize(20, 80, 10, 20); err != nil {
		t.Fatal(err)
	}
	term.Write(kittyCommand("a=T,f=24,s=1,v=98,c=1", make([]byte, 98*3)))
	// insertImage computes 10*98/1 = 980 pixels (49 rows).
	// kittyPlacementGridSize computes 10/(1/98), one floating-point
	// step above 980. Its ceil produces 50 rows for the final cursor.
	if x, y := term.Cursor(); x != 1 || y != 50 {
		t.Fatal("cursor reused the raster's arithmetic", x, y)
	}
	if g := term.Graphics(10, 20); len(g) != 1 || g[0].Height != 980 || g[0].ExactHeight != 0 {
		t.Fatal("protocol rounding changed the raster", g)
	}
}

func TestKittyFractionalWidthKeepsLogicalDeleteExtent(t *testing.T) {
	for _, identity := range []string{"", ",i=91,p=1"} {
		term, _ := graphicTerminal(t)
		term.Write(kittyCommand("a=T,f=100,r=1,C=1"+identity, inlinePNG(t, 101, 200)))
		// The bitmap is 10 pixels wide, but 10.1 requested pixels occupy
		// two logical columns. Both raster metadata and records use two.
		term.Write(kittyCommand("a=d,d=x,x=2", nil))
		if len(term.Graphics(10, 20)) != 0 {
			t.Fatal("fractional logical column was lost", identity)
		}
	}
}

func TestKittySubpixelWidthSkipsRasterRowsButKeepsRecord(t *testing.T) {
	for _, preserve := range []int{0, 1} {
		term, replies := graphicTerminal(t)
		term.Write([]byte("\x1b[10;1H"))
		before := term.Scrollbar().Total
		term.Write(kittyCommand(fmt.Sprintf("a=T,f=100,r=2,i=91,p=1,C=%d", preserve), inlinePNG(t, 1, 100)))
		if len(term.Graphics(10, 20)) != 0 || term.Scrollbar().Total != before {
			t.Fatal("zero-width stripe painted or inserted line feeds")
		}
		replies.Reset()
		term.Write([]byte("\x1b[6n"))
		want := "\x1b[10;2R"
		if preserve == 1 {
			want = "\x1b[10;1R"
		}
		if replies.String() != want {
			t.Fatalf("zero-width cursor %q, want %q", replies.String(), want)
		}
		replies.Reset()
		term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=92,p=1,P=91,Q=1,C=1", []byte{255, 0, 0}))
		if !strings.Contains(replies.String(), ";OK") {
			t.Fatal("zero-width placement lost its registered origin", replies.String())
		}
	}
}

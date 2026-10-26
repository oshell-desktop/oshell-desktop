package vt

import (
	"fmt"
	"strings"
	"testing"
)

// SwiftTerm inserts every raster stripe using LF, then restores the saved
// cursor for C=1 or clamps (start column + grid width, start row + grid height).
// Reaching the right edge clamps the column rather than wrapping to column 1.
func TestKittyCursorPlacementPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, setup, control, cursor string
		history                      uint64
	}{
		{"normal", "\x1b[3;4H", "c=2,r=2", "\x1b[5;6R", 0},
		{"right-edge", "\x1b[3;20H", "c=2,r=2", "\x1b[5;20R", 0},
		{"preserve", "\x1b[3;4H", "c=2,r=2,C=1", "\x1b[3;4R", 0},
		{"bottom", "\x1b[10;4H", "c=2,r=2", "\x1b[10;6R", 2},
		{"bottom-preserve", "\x1b[10;4H", "c=2,r=2,C=1", "\x1b[10;4R", 2},
		{"lnm", "\x1b[20h\x1b[3;4H", "c=2,r=2", "\x1b[5;6R", 0},
		{"origin", "\x1b[3;8r\x1b[?6h\x1b[2;4H", "c=2,r=2", "\x1b[4;6R", 0},
		{"margin-bottom", "\x1b[3;8r\x1b[8;4H", "c=2,r=2", "\x1b[10;6R", 0},
		{"origin-bottom", "\x1b[3;8r\x1b[?6h\x1b[6;4H", "c=2,r=2", "\x1b[6;6R", 0},
		{"outside-margin", "\x1b[3;8r\x1b[9;4H", "c=2,r=2", "\x1b[10;6R", 0},
		{"offset-auto", "\x1b[3;4H", "X=3,Y=4", "\x1b[6;7R", 0},
		{"offset-fixed", "\x1b[3;4H", "c=2,r=2,X=3,Y=4", "\x1b[5;6R", 0},
	} {
		for _, format := range []string{"rgb", "png"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				term, replies := graphicTerminal(t)
				term.Write([]byte(tc.setup))
				control := "f=24,s=20,v=40"
				data := make([]byte, 20*40*3)
				if format == "png" {
					control = "f=100"
					data = inlinePNG(t, 20, 40)
				}
				term.Write(kittyCommand("a=T,i=7,p=1,q=2,"+control+","+tc.control, data))
				term.Write([]byte("\x1b[6n"))
				if replies.String() != tc.cursor {
					t.Fatalf("cursor %q want %q", replies.String(), tc.cursor)
				}
				if got := term.Scrollbar().Total - term.Scrollbar().Len; got != tc.history {
					t.Fatalf("history %d want %d", got, tc.history)
				}
			})
		}
	}
}

func TestKittyBottomPreserveRaster(t *testing.T) {
	for _, setup := range []string{"", "\x1b[3;8r"} {
		t.Run(fmt.Sprintf("%x", setup), func(t *testing.T) {
			term, replies := graphicTerminal(t)
			row := 10
			if setup != "" {
				row = 8
			}
			term.Write([]byte(setup + fmt.Sprintf("\x1b[%d;4H", row)))
			term.Write(kittyCommand("a=T,f=100,i=7,p=1,C=1,q=2", inlinePNG(t, 20, 40)))
			term.Write([]byte("\x1b[6nX"))
			if replies.String() != fmt.Sprintf("\x1b[%d;4R", row) || !strings.Contains(term.Text(), "X") {
				t.Fatal("cursor/text", replies.String())
			}
			g := term.Graphics(10, 20)
			if len(g) != 1 || g[0].Y != (row-3)*20 || g[0].Height != 40 || g[0].Source.Dy() != 40 {
				t.Fatalf("lost raster during display scroll: %+v", g)
			}
		})
	}
}

func TestKittyTallRasterPreservesVisibleTail(t *testing.T) {
	for _, mode := range []string{"primary", "alternate", "region"} {
		for _, rows := range []int{2, 6, 12} {
			t.Run(fmt.Sprintf("%s/%d", mode, rows), func(t *testing.T) {
				term, replies := graphicTerminal(t)
				bottom, top := 9, 0
				if mode == "alternate" {
					term.Write([]byte("\x1b[?1049h"))
				}
				if mode == "region" {
					term.Write([]byte("\x1b[3;8r"))
					top, bottom = 2, 7
				}
				term.Write([]byte(fmt.Sprintf("\x1b[%d;4H", bottom+1)))
				term.Write(kittyCommand("a=T,f=100,i=7,p=1,C=1,q=2", inlinePNG(t, 20, rows*20)))
				term.Write([]byte("\x1b[6n"))
				if replies.String() != fmt.Sprintf("\x1b[%d;4R", bottom+1) {
					t.Fatal("preserved cursor", replies.String())
				}
				visible, lost := rows, 0
				if rows > bottom-top {
					visible = bottom - top
					lost = rows - visible
				}
				g := term.Graphics(10, 20)
				if len(g) != 1 || g[0].Y != (bottom-visible)*20 || g[0].Height != visible*20 || g[0].Source.Min.Y != lost*20 || g[0].Source.Max.Y != rows*20 {
					t.Fatalf("visible tail: %+v; want y=%d height=%d source=%d..%d", g, (bottom-visible)*20, visible*20, lost*20, rows*20)
				}
				if mode == "primary" && term.Scrollbar().Total-term.Scrollbar().Len != uint64(rows) {
					t.Fatal("truncated image scrolling")
				}
				if mode == "primary" && lost > 0 {
					term.ScrollToTop()
					g = term.Graphics(10, 20)
					if len(g) != 1 || g[0].Source.Min.Y != 0 || g[0].Y != bottom*20 {
						t.Fatal("offscreen stripes were deleted instead of retained in history", g)
					}
				}
			})
		}
	}
}

func TestKittyCursorWrapAndDisplayBudget(t *testing.T) {
	for _, preserve := range []bool{false, true} {
		t.Run(fmt.Sprint(preserve), func(t *testing.T) {
			term, replies := graphicTerminal(t)
			term.Write([]byte("\x1b[3;20HQ")) // pending wrap
			c := "0"
			want := "\x1b[5;21R"
			if preserve {
				c = "1"
				want = "\x1b[4;2R"
			}
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,c=2,r=2,C="+c+",q=2", []byte{1, 2, 3}))
			term.Write([]byte("X\x1b[6n"))
			if replies.String() != want {
				t.Fatalf("wrap %q want %q", replies.String(), want)
			}
		})
	}
	term, replies := graphicTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,C=1", []byte{1, 2, 3}))
	before := term.Graphics(10, 20)
	x, y := term.Cursor()
	replies.Reset()
	term.Write(kittyCommand("a=p,i=7,p=1,c=1,r=4294967295", nil))
	after := term.Graphics(10, 20)
	if !strings.Contains(replies.String(), "EFBIG:") || len(after) != 1 || after[0].Generation != before[0].Generation {
		t.Fatal("oversized display replaced original", replies.String())
	}
	if nx, ny := term.Cursor(); nx != x || ny != y {
		t.Fatal("oversized display moved cursor")
	}
}

func TestKittySingleRowRegionReleasesDisplay(t *testing.T) {
	term, replies := graphicTerminal(t)
	if err := term.Resize(20, 1, 10, 20); err != nil {
		t.Fatal(err)
	}
	term.Write([]byte("\x1b[?1049h\x1b[1;3H"))
	term.Write(kittyCommand("a=T,f=100,i=7,p=1,C=1,q=2", inlinePNG(t, 20, 40)))
	term.Write([]byte("\x1b[6n"))
	if replies.String() != "\x1b[1;3R" {
		t.Fatal("single-row cursor", replies.String())
	}
	if g := term.Graphics(10, 20); len(g) != 0 {
		t.Fatal("single-row scroll retained deleted raster", g)
	}
	// Pruning a display must not discard its protocol cache or leave insertion
	// state active for a later placement and ordinary scroll.
	if err := term.Resize(20, 10, 10, 20); err != nil {
		t.Fatal(err)
	}
	term.Write([]byte("\x1b[r\x1b[H"))
	term.Write(kittyCommand("a=p,i=7,p=2,C=1,q=2", nil))
	if g := term.Graphics(10, 20); len(g) != 1 || g[0].Y != 0 || g[0].Height != 40 {
		t.Fatal("cached image lost after region scroll", g)
	}
}

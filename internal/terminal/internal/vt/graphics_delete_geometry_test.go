package vt

import (
	"fmt"
	"strings"
	"testing"
)

// SwiftTerm's kittyPlacementIntersects* uses saved insertion geometry, not
// the current stripe row, font size, pixel offsets, or visible source bounds.
// Anonymous placements exercise this without the separate record-table pass.
func TestKittyDeleteUsesSavedRasterGeometry(t *testing.T) {
	for _, identity := range []string{"", ",i=91", ",p=1", ",i=91,p=1"} {
		for _, tc := range []struct {
			name, setup, deletion string
			cw, ch                int
			removed               bool
		}{
			{"font-smaller-outside", "", "d=x,x=4", 5, 10, false},
			{"font-larger-inside", "", "d=x,x=3", 30, 40, true},
			{"pixel-offset-outside", "", "d=p,x=4,y=4", 10, 20, false},
			{"inserted-row-new-position", "\x1b[3;1H\x1b[2L", "d=y,y=5", 10, 20, false},
			{"inserted-row-old-position", "\x1b[3;1H\x1b[2L", "d=y,y=3", 10, 20, true},
			{"split-stripes-old-extent", "\x1b[4;1H\x1b[2L", "d=p,x=2,y=4", 10, 20, true},
			{"split-stripes-new-tail", "\x1b[4;1H\x1b[2L", "d=p,x=2,y=6", 10, 20, false},
			{"z-mismatch", "", "d=q,x=2,y=3,z=7", 10, 20, false},
			{"z-match", "", "d=q,x=2,y=3,z=6", 10, 20, true},
			{"cursor", "\x1b[3;2H", "d=c", 10, 20, true},
		} {
			t.Run(identity+"/"+tc.name, func(t *testing.T) {
				term, _ := graphicTerminal(t)
				term.Write([]byte("\x1b[3;2H"))
				term.Write(kittyCommand("a=T,f=100,C=1,X=9,Y=19,z=6"+identity, inlinePNG(t, 20, 40)))
				term.Write([]byte(tc.setup))
				if err := term.Resize(20, 10, tc.cw, tc.ch); err != nil {
					t.Fatal(err)
				}
				term.Write(kittyCommand("a=d,"+tc.deletion, nil))
				if g := term.Graphics(tc.cw, tc.ch); (len(g) == 0) != tc.removed {
					t.Fatalf("removed=%v, want %v; graphics %+v", len(g) == 0, tc.removed, g)
				}
			})
		}
	}
}

func TestKittyDeleteGeometryKeepsBufferScopeAndInlineImages(t *testing.T) {
	for _, identity := range []string{"", ",i=91", ",i=91,p=1"} {
		for _, mode := range []string{"x", "X", "y", "Y", "p", "P", "q", "Q", "c", "C"} {
			t.Run(identity+"/"+mode, func(t *testing.T) {
				term, replies := graphicTerminal(t)
				term.Write([]byte("\x1b[3;2H"))
				term.Write(kittyCommand("a=T,f=100,C=1,z=6"+identity, inlinePNG(t, 2, 20)))
				term.Write([]byte("\x1b[?47h\x1b[3;2H"))
				term.Write(kittyCommand("a=T,f=100,C=1,z=6", inlinePNG(t, 2, 20)))
				// OSC row images must not be selected by any Kitty geometry.
				term.Write([]byte("\x1b[3;2H"))
				term.Write(append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 3, 20))...), '\a'))
				term.Write([]byte("\x1b[3;2H"))
				replies.Reset()
				term.Write(kittyCommand("a=d,d="+mode+",x=2,y=3,z=6", nil))
				if g := term.Graphics(10, 20); len(g) != 1 || g[0].Width != 3 {
					t.Fatal("geometric deletion changed inline image or retained Kitty image", g)
				}
				if replies.Len() != 0 {
					t.Fatal("successful deletion must not respond", replies.String())
				}
				term.Write([]byte("\x1b[?47l"))
				// Full identities have global records, so Swift's separate
				// record pass also selects that record on the inactive screen.
				want := 1
				if strings.Contains(identity, "p=1") {
					want = 0
				}
				if got := len(term.Graphics(10, 20)); got != want {
					t.Fatalf("inactive screen has %d images, want %d", got, want)
				}
			})
		}
	}
}

func TestKittyDeleteGeometryTracksRelativeMoves(t *testing.T) {
	for _, row := range []int{2, 4} {
		t.Run(fmt.Sprint(row), func(t *testing.T) {
			term, _ := graphicTerminal(t)
			term.Write([]byte("\x1b[3;2H"))
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0}))
			term.Write(kittyCommand("a=T,f=100,i=8,p=1,P=7,Q=1,C=1", inlinePNG(t, 2, 20)))
			term.Write([]byte("\x1b[5;2H"))
			term.Write(kittyCommand("a=p,i=7,p=1,U=1", nil))
			term.Write([]byte("\x1b[10;1H\n"))
			if g := term.Graphics(10, 20); len(g) != 1 || g[0].Y != 60 {
				t.Fatal("relative move fixture failed", g)
			}
			term.Write(kittyCommand(fmt.Sprintf("a=d,d=y,y=%d", row), nil))
			if got := len(term.Graphics(10, 20)) == 0; got != (row == 4) {
				t.Fatalf("removed=%v for row %d", got, row)
			}
		})
	}
}

func TestKittyDeleteFrozenGeometryAfterScroll(t *testing.T) {
	for _, identity := range []string{"", ",i=91,p=1"} {
		for _, row := range []int{2, 3} {
			t.Run(fmt.Sprintf("%s/%d", identity, row), func(t *testing.T) {
				term, _ := graphicTerminal(t)
				term.Write([]byte("\x1b[3;2H"))
				term.Write(kittyCommand("a=T,f=100,C=1"+identity, inlinePNG(t, 2, 20)))
				// Explicit SU moves rows but does not run Swift's relative
				// record reconciliation. The hit metadata stays at row 3.
				term.Write([]byte("\x1b[S"))
				term.Write(kittyCommand(fmt.Sprintf("a=d,d=y,y=%d", row), nil))
				if got := len(term.Graphics(10, 20)) == 0; got != (row == 3) {
					t.Fatalf("removed=%v for row %d", got, row)
				}
			})
		}
	}
}

func TestKittyDeleteGeometryCanAddressBeyondScreen(t *testing.T) {
	for _, deletion := range []string{"d=x,x=22", "d=p,x=22,y=2", "d=q,x=22,y=2,z=-1", "d=y,y=11"} {
		t.Run(deletion, func(t *testing.T) {
			term, _ := graphicTerminal(t)
			term.Write([]byte("\x1b[?47h\x1b[2;19H"))
			// Freeze a ten-row, five-column rectangle before shrinking
			// the font. It extends beyond both edges of the active grid.
			term.Write(kittyCommand("a=T,f=100,C=1,z=-1", inlinePNG(t, 50, 200)))
			if err := term.Resize(20, 10, 5, 10); err != nil {
				t.Fatal(err)
			}
			if len(term.Graphics(5, 10)) == 0 {
				t.Fatal("missing fixture")
			}
			term.Write(kittyCommand("a=d,"+deletion, nil))
			if len(term.Graphics(5, 10)) != 0 {
				t.Fatal("saved geometry beyond the active grid was not selected")
			}
		})
	}
}

func TestKittyDeleteGeometryUsesActiveHistoryBase(t *testing.T) {
	term, _ := graphicTerminal(t)
	term.Write([]byte("\x1b[3;2H"))
	term.Write(kittyCommand("a=T,f=100,C=1", inlinePNG(t, 2, 40)))
	term.Write([]byte("\x1b[10;1H\n\n\n\n"))
	term.ScrollToTop()
	if len(term.Graphics(10, 20)) == 0 {
		t.Fatal("fixture history was not visible")
	}
	// Row 3 means active row 3, not row 3 of the scrolled viewport.
	term.Write(kittyCommand("a=d,d=y,y=3", nil))
	if len(term.Graphics(10, 20)) == 0 {
		t.Fatal("row delete used the scrolled viewport instead of active base")
	}
	term.Write(kittyCommand("a=d,d=x,x=2", nil))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("column deletion did not scan retained history stripes")
	}
}

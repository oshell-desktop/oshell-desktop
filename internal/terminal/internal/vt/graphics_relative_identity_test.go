package vt

import (
	"fmt"
	"testing"
)

func TestKittyRelativeTrackingRequiresCompleteIdentity(t *testing.T) {
	for _, alternate := range []bool{false, true} {
		for _, tc := range []struct {
			identity string
			tracked  bool
		}{
			{"i=8", false}, {"i=8,p=0", false}, {"p=1", false}, {"i=0,p=1", false},
			{"i=8,p=1", true}, {"I=8,p=1", true},
		} {
			t.Run(fmt.Sprintf("%s/alternate=%v", tc.identity, alternate), func(t *testing.T) {
				term, replies := graphicTerminal(t)
				if alternate {
					term.Write([]byte("\x1b[?47h"))
				}
				term.Write([]byte("\x1b[2;2H"))
				term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=7,U=1", []byte{255, 0, 0}))
				term.Write(kittyCommand("a=T,f=24,s=1,v=1,"+tc.identity+",P=7,Q=7,H=1,V=1", []byte{0, 0, 255}))
				g := term.Graphics(10, 20)
				if len(g) != 1 || g[0].X != 20 || g[0].Y != 40 {
					t.Fatalf("initial relative placement: %q %+v", replies.String(), g)
				}
				term.Write([]byte("\x1b[6;5H"))
				term.Write(kittyCommand("a=p,i=7,p=7,U=1", nil))
				term.Write([]byte("\x1b[10;1H\n"))
				x, y := 20, 20
				if tc.tracked {
					x, y = 50, 100
					if alternate {
						y = 120
					}
				}
				g = term.Graphics(10, 20)
				if len(g) != 1 || g[0].X != x || g[0].Y != y {
					t.Fatalf("incomplete identity must scroll independently: want %d,%d, got %+v", x, y, g)
				}
				term.Write(kittyCommand("a=d,d=A", nil))
				if len(term.Graphics(10, 20)) != 0 {
					t.Fatal("visible image deletion left an untracked child")
				}
			})
		}
	}
}

func TestKittyRelativeUnnumberedPlacementDoesNotBorrowIdentity(t *testing.T) {
	term, replies := graphicTerminal(t)
	term.Write([]byte("\x1b[2;2H"))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=7,U=1", []byte{255, 0, 0}))
	// Both placements share a logical image ID, but only p=1 is registered.
	// A generated internal placement number must not borrow that record.
	for _, identity := range []string{"i=8,p=1", "i=8"} {
		term.Write(kittyCommand("a=T,f=24,s=1,v=1,"+identity+",P=7,Q=7,H=1,V=1", []byte{0, 0, 255}))
	}
	if g := term.Graphics(10, 20); len(g) != 2 {
		t.Fatalf("initial placements: %q %+v", replies.String(), g)
	}
	term.Write([]byte("\x1b[6;5H"))
	term.Write(kittyCommand("a=p,i=7,p=7,U=1", nil))
	term.Write([]byte("\x1b[10;1H\n"))
	g := term.Graphics(10, 20)
	if len(g) != 2 {
		t.Fatalf("scroll lost a placement: %+v", g)
	}
	for _, placed := range g {
		x, y := 20, 20
		if placed.PlacementID == 1 {
			x, y = 50, 100
		}
		if placed.X != x || placed.Y != y {
			t.Fatalf("placement %d: want %d,%d, got %+v", placed.PlacementID, x, y, placed)
		}
	}
	term.Write(kittyCommand("a=d,d=i,i=8,p=1", nil))
	if g := term.Graphics(10, 20); len(g) != 1 || g[0].PlacementID != 0 {
		t.Fatalf("explicit deletion affected unnumbered placement: %+v", g)
	}
}

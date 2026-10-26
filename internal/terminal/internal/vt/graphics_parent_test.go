package vt

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// SwiftTerm resolves a complete P/Q reference before removing the old
// placement. The Swift parser normalizes zero IDs to nil, so either missing
// or zero field means cursor placement, never an arbitrary parent.
func TestKittyParentFailureKeepsPlacement(t *testing.T) {
	for _, retransmit := range []bool{false, true} {
		for _, ref := range []string{"P=999,Q=1", "P=8,Q=999", "P=999,Q=1,x=999"} {
			t.Run(fmt.Sprintf("%v/%s", retransmit, ref), func(t *testing.T) {
				term, replies := graphicTerminal(t)
				term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,C=1", []byte{0, 255, 0}))
				term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,C=1", []byte{255, 0, 0}))
				before := term.Graphics(10, 20)
				x, y := term.Cursor()
				replies.Reset()
				if retransmit {
					term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,"+ref, []byte{0, 0, 255}))
				} else {
					term.Write(kittyCommand("a=p,i=7,p=1,"+ref, nil))
				}
				if !strings.Contains(replies.String(), "ENOPARENT: parent placement not found") {
					t.Errorf("wrong rejection %q", replies.String())
				}
				after := term.Graphics(10, 20)
				if len(after) != len(before) {
					t.Fatalf("lost original image: %d -> %d", len(before), len(after))
				}
				for i := range before {
					if after[i].Generation != before[i].Generation || after[i].Image != before[i].Image || after[i].X != before[i].X || after[i].Y != before[i].Y {
						t.Fatal("changed old display")
					}
				}
				if nx, ny := term.Cursor(); nx != x || ny != y {
					t.Fatal("moved cursor")
				}
				term.Write(kittyCommand("a=p,i=7,p=1,C=1,P=8,Q=1,H=2,V=1", nil))
				after = term.Graphics(10, 20)
				if len(after) != 2 || after[0].logicalImageID != 7 || after[0].X != 20 || after[0].Y != 20 {
					t.Fatalf("valid retry failed: %+v", after)
				}
				if retransmit && !bytes.Equal(after[0].Image.Pix, []byte{0, 0, 255, 255}) {
					t.Fatal("new cached pixels were lost")
				}
			})
		}
	}
}

func TestKittyParentFieldPresence(t *testing.T) {
	for _, tc := range []struct {
		ref          string
		virtualError bool
	}{
		{"P=999", true}, {"Q=999", true}, {"P=0,Q=1", true}, {"P=8,Q=0", true},
		{"P=0", false}, {"Q=0", false}, {"P=0,Q=0", false},
	} {
		t.Run(tc.ref, func(t *testing.T) {
			term, replies := graphicTerminal(t)
			term.Write([]byte("\x1b[3;4H"))
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,C=1,"+tc.ref, []byte{255, 0, 0}))
			g := term.Graphics(10, 20)
			if !strings.Contains(replies.String(), ";OK") || len(g) != 1 || g[0].X != 30 || g[0].Y != 40 {
				t.Fatalf("incomplete parent must use cursor: %q %+v", replies.String(), g)
			}
			replies.Reset()
			term.Write(kittyCommand("a=p,i=7,p=1,U=1,"+tc.ref, nil))
			if tc.virtualError {
				if !strings.Contains(replies.String(), "EINVAL: virtual placement cannot refer to parent") || len(term.Graphics(10, 20)) != 1 {
					t.Fatalf("virtual reference accepted/destroyed old image: %q", replies.String())
				}
			} else if !strings.Contains(replies.String(), ";OK") || len(term.Graphics(10, 20)) != 0 {
				t.Fatalf("zero IDs must be ignored: %q", replies.String())
			}
		})
	}
}

func TestKittyParentScreenIsolation(t *testing.T) {
	term, replies := graphicTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,C=1", []byte{255, 0, 0}))
	term.Write([]byte("\x1b[?47h"))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,C=1", []byte{0, 255, 0}))
	replies.Reset()
	term.Write(kittyCommand("a=p,i=8,p=1,P=7,Q=1", nil))
	if !strings.Contains(replies.String(), "ENOPARENT: parent placement not in current buffer") || len(term.Graphics(10, 20)) != 1 {
		t.Fatalf("cross-screen parent changed display: %q", replies.String())
	}
	term.Write([]byte("\x1b[?47l"))
	if len(term.Graphics(10, 20)) != 1 {
		t.Fatal("other screen changed")
	}
}

func TestKittyParentReplacementKeepsDescendants(t *testing.T) {
	term, _ := graphicTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,C=1", []byte{255, 0, 0}))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=2,P=7,Q=1,H=2,V=1", []byte{0, 255, 0}))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=9,p=1,P=8,Q=2,H=1,V=1", []byte{0, 0, 255}))
	if len(term.Graphics(10, 20)) != 3 {
		t.Fatal("missing initial chain")
	}
	term.Write([]byte("\x1b[3;4H"))
	term.Write(kittyCommand("a=p,i=7,p=1,C=1", nil))
	g := term.Graphics(10, 20)
	if len(g) != 3 {
		t.Fatalf("parent replacement destroyed descendants: %d", len(g))
	}
	for _, p := range g {
		wantX, wantY := 30, 40
		if p.Image.Pix[1] == 255 {
			wantX, wantY = 20, 20
		}
		if p.Image.Pix[2] == 255 {
			wantX, wantY = 30, 40
		}
		if p.X != wantX || p.Y != wantY {
			t.Fatalf("replacement prematurely moved attached pixels: %d,%d want %d,%d", p.X, p.Y, wantX, wantY)
		}
	}
	// Terminal.scroll() is the synchronization point for existing raster rows.
	term.Write([]byte("\x1b[10;1H\n"))
	g = term.Graphics(10, 20)
	if len(g) != 3 {
		t.Fatalf("scroll lost descendants: %d", len(g))
	}
	for _, p := range g {
		wantX, wantY := 30, 20
		if p.logicalImageID == 8 {
			wantX, wantY = 50, 40
		}
		if p.logicalImageID == 9 {
			wantX, wantY = 60, 60
		}
		if p.X != wantX || p.Y != wantY {
			t.Fatalf("scroll reconciliation: %+v", p)
		}
	}
}

func TestKittyParentRejectsPrivateIDAndCycles(t *testing.T) {
	term, replies := graphicTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,C=1", []byte{255, 0, 0}))
	term.Write(kittyEncodedCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1,H=2,V=1,m=1", "A"))
	term.Write(kittyEncodedCommand("m=0", "P8A"))
	before := term.Graphics(10, 20)
	if len(before) != 2 {
		t.Fatal("missing chain")
	}
	for _, tc := range []struct{ control, message string }{
		{fmt.Sprintf("a=p,i=8,p=1,P=%d,Q=1", before[0].ImageID), "ENOPARENT: parent placement not found"},
		{"a=p,i=7,p=1,P=7,Q=1", "EINVAL: placement cannot be its own parent"},
		{"a=p,i=7,p=1,P=8,Q=1", "ECYCLE: parent chain creates a cycle"},
	} {
		replies.Reset()
		for _, b := range kittyCommand(tc.control, nil) {
			term.Write([]byte{b})
		}
		if !strings.Contains(replies.String(), tc.message) {
			t.Fatalf("%s: %q", tc.control, replies.String())
		}
		after := term.Graphics(10, 20)
		if len(after) != len(before) {
			t.Fatal("invalid request destroyed chain")
		}
		for i := range before {
			if after[i].ImageID != before[i].ImageID || after[i].Generation != before[i].Generation || after[i].X != before[i].X || after[i].Y != before[i].Y {
				t.Fatal("invalid request changed chain")
			}
		}
	}
	// Replacing an ancestor repeatedly must rebind children each time and
	// release only the obsolete snapshot. Deletion preserves attached children;
	// the logical reference may be resolved again after a parent is recreated.
	for n := 0; n < 20; n++ {
		term.Write([]byte(fmt.Sprintf("\x1b[%d;%dH", 2+n%3, 1+n%5)))
		term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,C=1", []byte{255, byte(n), 0}))
		term.Write([]byte("\x1b[10;1H\n"))
		g := term.Graphics(10, 20)
		if len(g) != 2 {
			t.Fatalf("lost descendant after replacement %d", n)
		}
		if g[1].logicalImageID != 8 || g[1].X != (n%5+2)*10 || g[1].Y != (n%3+1)*20 {
			t.Fatalf("stale child after replacement %d: %d,%d", n, g[1].X, g[1].Y)
		}
	}
	term.Write(kittyCommand("a=d,d=i,i=7,p=1", nil))
	if len(term.Graphics(10, 20)) != 1 {
		t.Fatal("parent deletion removed attached child")
	}
	term.Write(kittyCommand("a=p,i=8,p=1,C=1", nil))
	if len(term.Graphics(10, 20)) != 1 {
		t.Fatal("parent deletion lost child cache")
	}
}

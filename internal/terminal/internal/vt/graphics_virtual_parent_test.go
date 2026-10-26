package vt

import (
	"fmt"
	"strings"
	"testing"
)

// OShell's virtual records keep an absolute buffer coordinate. They don't
// paint pixels themselves, but a normal relative placement can use that
// coordinate without a Unicode placeholder ever appearing in the terminal.
func TestKittyVirtualParentOrigin(t *testing.T) {
	for _, alternate := range []bool{false, true} {
		t.Run(fmt.Sprint(alternate), func(t *testing.T) {
			term, replies := graphicTerminal(t)
			if alternate {
				term.Write([]byte("\x1b[?47h"))
			}
			term.Write([]byte("\x1b[3;4H"))
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=9,U=1,c=5,r=4", []byte{255, 0, 0}))
			if len(term.Graphics(10, 20)) != 0 {
				t.Fatal("virtual record painted pixels")
			}
			if x, y := term.Cursor(); x != 3 || y != 2 {
				t.Fatal("virtual record moved cursor")
			}
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=9,H=2,V=1,c=2,r=2", []byte{0, 255, 0}))
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=9,p=1,P=8,Q=1,H=-1,V=1", []byte{0, 0, 255}))
			check := func(rowOffset int) {
				t.Helper()
				g := term.Graphics(10, 20)
				if len(g) != 2 {
					t.Fatalf("relative images missing: count=%d replies=%q", len(g), replies.String())
				}
				if g[0].logicalImageID != 8 || g[0].X != 50 || g[0].Y != (3-rowOffset)*20 || g[1].X != 40 || g[1].Y != (4-rowOffset)*20 {
					t.Fatalf("wrong virtual origin/chain: %+v", g)
				}
			}
			check(0)
			if x, y := term.Cursor(); x != 3 || y != 2 {
				t.Fatal("relative display did not restore cursor")
			}
			// Refreshing the protocol pixels does not alter the displayed record.
			term.Write(kittyCommand("a=t,f=24,s=1,v=1,i=7", []byte{255, 255, 0}))
			term.Write([]byte("\x1b[10;1H\n"))
			if alternate {
				check(0)
			} else {
				check(1)
				term.ScrollToTop()
				check(0)
				term.ScrollToBottom()
			}
			term.Write(kittyCommand("a=d,d=i,i=7,p=9", nil))
			if len(term.Graphics(10, 20)) != 2 {
				t.Fatal("deleting virtual parent removed attached descendants")
			}
			term.Write(kittyCommand("a=d,d=i,i=8,p=1", nil))
			term.Write(kittyCommand("a=d,d=i,i=9,p=1", nil))
			term.Write([]byte("\x1b[1;1H"))
			term.Write(kittyCommand("a=p,i=8,p=1,C=1", nil))
			if len(term.Graphics(10, 20)) != 1 {
				t.Fatal("deletion lost child's protocol cache")
			}
		})
	}
}

func TestKittyVirtualParentBufferIdentity(t *testing.T) {
	term, replies := graphicTerminal(t)
	term.Write([]byte("\x1b[3;4H"))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=9,U=1", []byte{255, 0, 0}))
	term.Write([]byte("\x1b[?47h"))
	replies.Reset()
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=9", []byte{0, 255, 0}))
	if !strings.Contains(replies.String(), "ENOPARENT: parent placement not in current buffer") || len(term.Graphics(10, 20)) != 0 {
		t.Fatal("virtual reference crossed buffers", replies.String())
	}
	// Reusing the logical parent in this screen transfers the virtual record.
	term.Write([]byte("\x1b[5;6H"))
	term.Write(kittyCommand("a=p,i=7,p=9,U=1", nil))
	term.Write(kittyCommand("a=p,i=8,p=1,P=7,Q=9", nil))
	g := term.Graphics(10, 20)
	if len(g) != 1 || g[0].X != 50 || g[0].Y != 80 {
		t.Fatalf("new screen virtual origin: %+v", g)
	}
	term.Write([]byte("\x1b[?47l"))
	replies.Reset()
	term.Write(kittyCommand("a=p,i=8,p=2,P=7,Q=9", nil))
	if !strings.Contains(replies.String(), "ENOPARENT: parent placement not in current buffer") {
		t.Fatal("old screen retained transferred virtual record", replies.String())
	}
}

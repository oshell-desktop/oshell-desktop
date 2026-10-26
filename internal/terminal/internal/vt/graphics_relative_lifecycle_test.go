package vt

import (
	"bytes"
	"fmt"
	"image"
	"strings"
	"testing"
)

func TestKittyRelativeReconciliationClipsBufferRows(t *testing.T) {
	for _, rows := range []int{1, 3} {
		t.Run(fmt.Sprint(rows), func(t *testing.T) {
			term, _ := graphicTerminal(t)
			term.Write([]byte("\x1b[6;1H"))
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0}))
			pixels := []byte{255, 0, 0, 0, 255, 0, 0, 0, 255}
			term.Write(kittyCommand(fmt.Sprintf("a=T,f=24,s=1,v=%d,i=8,p=1,P=7,Q=1,V=-2,c=1,r=%d", rows, rows), pixels[:rows*3]))
			term.Write([]byte("\x1b[1;1H"))
			term.Write(kittyCommand("a=p,i=7,p=1,U=1", nil))
			term.Write([]byte("\x1b[10;1H\n"))
			term.ScrollToTop()
			g := term.Graphics(10, 20)
			if rows == 1 {
				if len(g) != 0 {
					t.Fatalf("out-of-buffer child stayed visible: %+v", g)
				}
			} else if len(g) != 1 || g[0].Y != -40 || g[0].Height != 60 || !g[0].Clipped || g[0].RasterClip != image.Rect(0, 0, 10, 20) || !bytes.Equal(g[0].Image.Pix[8:12], []byte{0, 0, 255, 255}) {
				t.Fatalf("surviving stripe not clipped: %+v", g)
			}
		})
	}
}

func TestKittyRelativeReconciliationClipsBottom(t *testing.T) {
	term, _ := graphicTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0}))
	term.Write(kittyCommand("a=T,f=24,s=1,v=3,i=8,p=1,P=7,Q=1,V=2,c=1,r=3", []byte{255, 0, 0, 0, 255, 0, 0, 0, 255}))
	term.Write([]byte("\x1b[8;1H"))
	term.Write(kittyCommand("a=p,i=7,p=1,U=1", nil))
	term.Write([]byte("\x1b[10;1H\n"))
	g := term.Graphics(10, 20)
	if len(g) != 1 || g[0].Y != 160 || g[0].Height != 60 || !g[0].Clipped || g[0].RasterClip != image.Rect(0, 160, 10, 200) {
		t.Fatalf("bottom stripes not clipped: %+v", g)
	}
}

func TestKittyRelativeReconcilesOnIndexButNotExplicitScroll(t *testing.T) {
	term, _ := graphicTerminal(t)
	term.Write([]byte("\x1b[4;1H"))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0}))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1,V=1", []byte{0, 255, 0}))
	term.Write([]byte("\x1b[6;1H"))
	term.Write(kittyCommand("a=p,i=7,p=1,U=1", nil))
	// Swift cmdScrollUp moves rows without calling Terminal.scroll().
	term.Write([]byte("\x1b[S"))
	g := term.Graphics(10, 20)
	if len(g) != 1 || g[0].Y != 60 {
		t.Fatalf("SU must move rows without reconciling: %+v", g)
	}
	term.Write([]byte("\x1b[T"))
	g = term.Graphics(10, 20)
	if len(g) != 1 || g[0].Y != 80 {
		t.Fatalf("SD must move rows without reconciling: %+v", g)
	}
	term.Write([]byte("\x1b[1;1H\x1bM"))
	g = term.Graphics(10, 20)
	if len(g) != 1 || g[0].Y != 100 {
		t.Fatalf("RI must move rows without reconciling: %+v", g)
	}
	term.Write([]byte("\x1b[2S"))
	// A soft wrap at the bottom does call the scroll reconciliation path.
	term.Write([]byte("\x1b[10;20Hxy"))
	g = term.Graphics(10, 20)
	if len(g) != 1 || g[0].Y != 100 {
		t.Fatalf("soft wrap failed to reconcile: %+v", g)
	}
}

func TestKittyRelativeOrphanVisibleDeleteAndScreenTransfer(t *testing.T) {
	term, _ := graphicTerminal(t)
	term.Write([]byte("\x1b[3;2H"))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0}))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1", []byte{0, 255, 0}))
	term.Write([]byte("\x1b[?47h"))
	term.Write(kittyCommand("a=p,i=7,p=1,U=1", nil))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("child transferred between screens")
	}
	term.Write([]byte("\x1b[?47l"))
	g := term.Graphics(10, 20)
	if len(g) != 1 || g[0].Y != 40 {
		t.Fatal("parent transfer removed old screen child")
	}
	term.Write(kittyCommand("a=d,d=a", nil))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("visible deletion did not select independent child")
	}
}

func TestKittyRelativeOrphanRetainsPixelsAndReattaches(t *testing.T) {
	for _, virtual := range []bool{false, true} {
		t.Run(fmt.Sprint(virtual), func(t *testing.T) {
			term, _ := graphicTerminal(t)
			parent := "a=T,f=24,s=1,v=1,i=7,p=1,C=1"
			if virtual {
				parent += ",U=1"
			}
			term.Write([]byte("\x1b[3;2H"))
			term.Write(kittyCommand(parent, []byte{255, 0, 0}))
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1,H=1,V=1", []byte{0, 255, 0}))
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=9,p=1,P=8,Q=1,H=1,V=1", []byte{0, 0, 255}))
			term.Write(kittyCommand("a=d,d=I,i=7,p=1", nil))
			g := term.Graphics(10, 20)
			if len(g) != 2 || g[0].Y != 60 || g[1].Y != 80 {
				t.Fatalf("parent delete changed children: %+v", g)
			}
			term.Write([]byte("\x1b[10;1H\n"))
			g = term.Graphics(10, 20)
			if len(g) != 2 || g[0].Y != 40 || g[1].Y != 60 {
				t.Fatalf("orphan did not scroll independently: %+v", g)
			}
			term.Write([]byte("\x1b[4;4H"))
			term.Write(kittyCommand(parent, []byte{255, 0, 0}))
			g = term.Graphics(10, 20)
			wantCount := 3
			if virtual {
				wantCount = 2
			}
			if len(g) != wantCount {
				t.Fatalf("recreation lost children: %d", len(g))
			}
			for _, p := range g {
				if p.logicalImageID == 8 && (p.X != 20 || p.Y != 40) {
					t.Fatal("recreation moved child before scroll")
				}
			}
			term.Write([]byte("\x1b[10;1H\n"))
			g = term.Graphics(10, 20)
			if len(g) != wantCount {
				t.Fatalf("reconciliation lost children: %d", len(g))
			}
			for _, p := range g {
				if p.logicalImageID == 8 && (p.X != 40 || p.Y != 60) {
					t.Fatalf("child did not reattach: %+v", p)
				}
				if p.logicalImageID == 9 && (p.X != 50 || p.Y != 80) {
					t.Fatalf("descendant did not reattach: %+v", p)
				}
			}
		})
	}
}

func TestKittyRelativeInsertionScrollsAndRestoresCursor(t *testing.T) {
	for _, alternate := range []bool{false, true} {
		t.Run(fmt.Sprint(alternate), func(t *testing.T) {
			term, replies := graphicTerminal(t)
			if alternate {
				term.Write([]byte("\x1b[?47h"))
			}
			term.Write([]byte("\x1b[10;2H"))
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0}))
			term.Write([]byte("\x1b[2;4H"))
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1,c=1,r=1", []byte{0, 255, 0}))
			if x, y := term.Cursor(); x != 3 || y != 1 {
				t.Fatalf("cursor changed: %d,%d", x, y)
			}
			g := term.Graphics(10, 20)
			wantY := 160
			if alternate {
				wantY = 180
			}
			if len(g) != 1 || g[0].X != 10 || g[0].Y != wantY {
				t.Fatalf("relative insertion omitted LF: %+v %q", g, replies.String())
			}
			term.ScrollToTop()
			g = term.Graphics(10, 20)
			if len(g) != 1 || g[0].Y != 180 {
				t.Fatalf("missing image in inserted history: %+v", g)
			}
		})
	}
}

func TestKittyRelativeInitialColumnClampsAndInvalidRowRemovesPlacement(t *testing.T) {
	for _, offset := range []int{-100, 100} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			term, replies := graphicTerminal(t)
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0}))
			term.Write(kittyCommand(fmt.Sprintf("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1,H=%d", offset), []byte{0, 255, 0}))
			g := term.Graphics(10, 20)
			want := 0
			if offset > 0 {
				want = 190
			}
			if len(g) != 1 || g[0].X != want {
				t.Fatalf("initial column not clamped: %+v %q", g, replies.String())
			}
			replies.Reset()
			term.Write(kittyCommand("a=p,i=8,p=1,P=7,Q=1,V=-1", nil))
			if !strings.Contains(replies.String(), "EINVAL: placement out of range") {
				t.Fatal(replies.String())
			}
			after := term.Graphics(10, 20)
			if len(after) != 0 {
				t.Fatal("invalid row retained old placement")
			}
		})
	}
}

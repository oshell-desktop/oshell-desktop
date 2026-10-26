package vt

import (
	"fmt"
	"strings"
	"testing"
)

func TestKittyVirtualFlagUsesSwiftIntegerEquality(t *testing.T) {
	for _, tc := range []struct {
		value   string
		virtual bool
	}{
		{"1", true}, {"01", true}, {"+1", true}, {strings.Repeat("0", 4096) + "1", true},
		{"0", false}, {"2", false}, {"-1", false}, {"4294967296", false}, {"", false}, {"x", false},
		{"++1", false}, {"1x", false}, {strings.Repeat("0", 4096), false},
		{"1,U=garbage", false}, {"2,U=1", true}, {"1,U=", false},
		{"1=garbage", false}, {"0+1", false}, {"+01", true},
		{"1,U=中", true}, {"中", false}, {"2,U=中", false},
	} {
		name := tc.value
		if len(name) > 30 {
			name = fmt.Sprintf("long-%d-%v", len(name), tc.virtual)
		}
		for _, fragmented := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fragmented=%v", name, fragmented), func(t *testing.T) {
				term, replies := graphicTerminal(t)
				term.Write([]byte("\x1b[3;2H"))
				command := kittyCommand("a=T,f=24,s=1,v=1,i=7,C=1,U="+tc.value, []byte{255, 0, 0})
				if fragmented {
					for _, b := range command {
						term.Write([]byte{b})
					}
				} else {
					term.Write(command)
				}
				if replies.String() != "\x1b_Gi=7;OK\x1b\\" {
					t.Fatalf("unexpected display reply: %q", replies.String())
				}
				wantCount, next := 1, 1
				if tc.virtual {
					wantCount, next = 0, 2
				}
				if len(term.Graphics(10, 20)) != wantCount {
					t.Fatalf("U=%q classified incorrectly", name)
				}
				term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=9,U=1", []byte{0, 0, 255}))
				term.Write(kittyCommand(fmt.Sprintf("a=T,f=24,s=1,v=1,i=8,p=8,P=9,Q=%d,V=1", next), []byte{0, 255, 0}))
				var found bool
				for _, p := range term.Graphics(10, 20) {
					if p.logicalImageID == 8 {
						found = p.X == 10 && p.Y == 60
					}
				}
				if !found {
					t.Fatalf("U classification consumed wrong counter: %q", replies.String())
				}
			})
		}
	}
}

func TestKittyVirtualPlacementAutomaticIdentity(t *testing.T) {
	for _, placement := range []string{"", ",p=0"} {
		t.Run(placement, func(t *testing.T) {
			term, replies := graphicTerminal(t)
			term.Write([]byte("\x1b[3;4H"))
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,U=1"+placement, []byte{255, 0, 0}))
			if replies.String() != "\x1b_Gi=7;OK\x1b\\" {
				t.Fatalf("generated p leaked into response: %q", replies.String())
			}
			if len(term.Graphics(10, 20)) != 0 {
				t.Fatal("virtual record painted pixels")
			}
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=8,P=7,Q=1,H=2,V=1", []byte{0, 255, 0}))
			g := term.Graphics(10, 20)
			if len(g) != 1 || g[0].X != 50 || g[0].Y != 60 {
				t.Fatalf("generated placement not addressable: %q %+v", replies.String(), g)
			}
			term.Write(kittyCommand("a=d,d=i,i=7,p=1", nil))
			replies.Reset()
			term.Write(kittyCommand("a=p,i=8,p=9,P=7,Q=1", nil))
			if !strings.Contains(replies.String(), "ENOPARENT") || len(term.Graphics(10, 20)) != 1 {
				t.Fatal("generated placement deletion changed child lifetime", replies.String())
			}
		})
	}
}

func TestKittyVirtualPlacementCounterScope(t *testing.T) {
	term, replies := graphicTerminal(t)
	for n, before := range []string{"", "\x1b[?47h", "\x1b[2J", "\x1bc", "\x1b[!p"} {
		term.Write([]byte(before + "\x1b[3;2H"))
		replies.Reset()
		term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,U=1", []byte{255, 0, 0}))
		if replies.String() != "\x1b_Gi=7;OK\x1b\\" {
			t.Fatal("bad automatic placement response", replies.String())
		}
		term.Write(kittyCommand(fmt.Sprintf("a=T,f=24,s=1,v=1,i=8,p=9,P=7,Q=%d,H=1,V=1", n+1), []byte{0, 255, 0}))
		g := term.Graphics(10, 20)
		if len(g) != 1 || g[0].X != 20 || g[0].Y != 60 {
			t.Fatalf("counter reset or split at step %d: %q %+v", n, replies.String(), g)
		}
	}
}

func TestKittyVirtualPlacementCounterOnlyConsumesEligibleDisplays(t *testing.T) {
	term, replies := graphicTerminal(t)
	// Explicit IDs, raster placements without p, anonymous virtual images,
	// queries, rejected references and failed/pending loads consume no ID.
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=4294967295,U=1", []byte{255, 0, 0}))
	term.Write(kittyCommand("a=p,i=7,C=1", nil))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,U=1", []byte{255, 0, 0}))
	term.Write(kittyCommand("a=q,f=24,s=1,v=1,i=7,U=1", []byte{255, 0, 0}))
	term.Write(kittyCommand("a=p,i=7,U=1,P=7", nil))
	term.Write(kittyCommand("a=p,i=999,U=1", nil))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,U=1", []byte{0}))
	term.Write(kittyEncodedCommand("a=T,f=24,s=1,v=1,i=7,U=1,m=1", "A"))
	// Complete the deferred display; this is the first generated position.
	term.Write(kittyEncodedCommand("m=0", "P8A"))
	term.Write([]byte("\x1b[3;2H"))
	replies.Reset()
	term.Write(kittyCommand("a=p,i=7,U=1", nil))
	if replies.String() != "\x1b_Gi=7;OK\x1b\\" {
		t.Fatal(replies.String())
	}
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=8,P=7,Q=2,H=1,V=1", []byte{0, 0, 255}))
	g := term.Graphics(10, 20)
	var found bool
	for _, p := range g {
		if p.logicalImageID == 8 {
			found = p.X == 20 && p.Y == 60
		}
	}
	if !found {
		t.Fatalf("unrelated operations consumed virtual counter: %q %+v", replies.String(), g)
	}
	term.Write([]byte("\x1b[5;5H"))
	replies.Reset()
	term.Write(kittyCommand("a=p,i=7,U=1,q=1", nil))
	if replies.Len() != 0 {
		t.Fatal("quiet virtual display replied", replies.String())
	}
	// Quiet display still creates position 3. The preceding generated
	// positions remain independently addressable after retransmission.
	for _, tc := range []struct{ id, x, y int }{{1, 0, 0}, {2, 10, 40}, {3, 40, 80}} {
		term.Write(kittyCommand(fmt.Sprintf("a=T,f=24,s=1,v=1,i=9,p=9,P=7,Q=%d", tc.id), []byte{255, 255, 0}))
		var matched bool
		for _, p := range term.Graphics(10, 20) {
			if p.logicalImageID == 9 {
				matched = p.X == tc.x && p.Y == tc.y
			}
		}
		if !matched {
			t.Fatalf("generated position %d was replaced or skipped: %q", tc.id, replies.String())
		}
	}
}

func TestKittyVirtualAutomaticPlacementReplacesExplicitAcrossBuffers(t *testing.T) {
	term, replies := graphicTerminal(t)
	term.Write([]byte("\x1b[3;2H"))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,C=1", []byte{255, 0, 0}))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1,V=1", []byte{0, 255, 0}))
	term.Write([]byte("\x1b[?47h\x1b[5;6H"))
	term.Write(kittyCommand("a=p,i=7,U=1", nil))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=9,p=1,P=7,Q=1", []byte{0, 0, 255}))
	g := term.Graphics(10, 20)
	if len(g) != 1 || g[0].X != 50 || g[0].Y != 80 {
		t.Fatalf("automatic ID skipped explicit collision: %+v", g)
	}
	term.Write([]byte("\x1b[?47l"))
	g = term.Graphics(10, 20)
	if len(g) != 1 || g[0].logicalImageID != 8 || g[0].Y != 60 {
		t.Fatalf("transfer removed child or retained old parent: %+v", g)
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=8,p=2,P=7,Q=1", nil))
	if !strings.Contains(replies.String(), "ENOPARENT: parent placement not in current buffer") {
		t.Fatal("old screen kept transferred automatic identity", replies.String())
	}
}

func TestKittyNumberedImageAutomaticPlacement(t *testing.T) {
	term, replies := graphicTerminal(t)
	term.Write([]byte("\x1b[2;3H"))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,I=70,U=1", []byte{255, 0, 0}))
	if replies.String() != "\x1b_Gi=1,I=70;OK\x1b\\" {
		t.Fatal("numbered response changed", replies.String())
	}
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=1,Q=1,H=1,V=1", []byte{0, 255, 0}))
	g := term.Graphics(10, 20)
	if len(g) != 1 || g[0].X != 30 || g[0].Y != 40 {
		t.Fatalf("numbered image has no generated placement: %q %+v", replies.String(), g)
	}
}

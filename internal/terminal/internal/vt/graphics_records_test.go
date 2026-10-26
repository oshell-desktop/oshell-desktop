package vt

import (
	"bytes"
	"strings"
	"testing"
)

func TestKittyRecordSurvivesCacheEvictionAndRasterPruning(t *testing.T) {
	term, replies := graphicTerminal(t)
	term.Write([]byte("\x1b[?47h\x1b[3;2H"))
	data := bytes.Repeat([]byte{20, 40, 60, 255}, 1024*1280)
	term.Write(kittyCommand("a=T,f=32,s=1024,v=1280,i=7,p=1,c=1,r=1,C=1", data))
	if len(term.Graphics(10, 20)) != 1 {
		t.Fatal("missing oversized display")
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=7,p=2,C=1", nil))
	if !strings.Contains(replies.String(), "ENOENT") {
		t.Fatal("fixture did not evict protocol cache")
	}
	term.Write([]byte("\x1b[10;1H\n\n\n"))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("fixture did not prune raster")
	}
	replies.Reset()
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1,V=2", []byte{0, 255, 0}))
	g := term.Graphics(10, 20)
	if !strings.Contains(replies.String(), ";OK") || len(g) != 1 || g[0].X != 10 || g[0].Y != 40 {
		t.Fatalf("record depended on pixel/cache lifetime: %q %+v", replies.String(), g)
	}
}

func TestKittyCompleteEraseClearsOtherBufferRecords(t *testing.T) {
	term, replies := graphicTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0}))
	term.Write([]byte("\x1b[?47h\x1b[2J\x1b[?47l"))
	replies.Reset()
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1", []byte{0, 255, 0}))
	if !strings.Contains(replies.String(), "ENOPARENT: parent placement not found") {
		t.Fatalf("ED2 retained global record: %q", replies.String())
	}
}

// Alternate-screen scrolling drops raster rows without clearing Swift's
// placement record. Its last observed row remains a valid parent origin.
func prunedKittyParent(t *testing.T) (*Terminal, *strings.Builder) {
	t.Helper()
	term, replies := graphicTerminal(t)
	term.Write([]byte("\x1b[?47h\x1b[3;2H"))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,C=1,z=55", []byte{255, 0, 0}))
	term.Write([]byte("\x1b[10;1H\n\n\n"))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("parent raster was not pruned")
	}
	replies.Reset()
	return term, replies
}

func TestKittyRecordSurvivesRasterPruning(t *testing.T) {
	term, replies := prunedKittyParent(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1,H=2,V=2", []byte{0, 255, 0}))
	g := term.Graphics(10, 20)
	if !strings.Contains(replies.String(), ";OK") || len(g) != 1 || g[0].X != 30 || g[0].Y != 40 {
		t.Fatalf("lost last recorded parent coordinate: %q %+v", replies.String(), g)
	}
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=9,p=1,P=8,Q=1,H=1,V=1", []byte{0, 0, 255}))
	g = term.Graphics(10, 20)
	if len(g) != 2 || g[1].X != 40 || g[1].Y != 60 {
		t.Fatalf("record-only ancestor broke nested reference: %+v", g)
	}
}

func TestKittyRecordKeepsCacheUsedAfterRasterPruning(t *testing.T) {
	term, replies := prunedKittyParent(t)
	// Uppercase deletes sweep all unreferenced cache images. The record is
	// still a use of image 7 even though it has no physical raster placement.
	term.Write(kittyCommand("a=d,d=I,i=9999", nil))
	term.Write([]byte("\x1b[2;2H"))
	term.Write(kittyCommand("a=p,i=7,p=2,C=1", nil))
	if !strings.Contains(replies.String(), ";OK") || len(term.Graphics(10, 20)) != 1 {
		t.Fatalf("record's cache reclaimed: %q", replies.String())
	}
}

func TestKittyRecordDeletionWithoutRaster(t *testing.T) {
	for _, deletion := range []string{
		"a=d,d=i,i=7,p=1", "a=d,d=I,i=7,p=1", "a=d,d=r,x=7,y=7", "a=d,d=z,z=55",
		"a=d,d=a", "a=d,d=x,x=2", "a=d,d=y,y=1", "a=d,d=p,x=2,y=1", "a=d,d=q,x=2,y=1,z=55", "a=d,d=c",
	} {
		t.Run(deletion, func(t *testing.T) {
			term, replies := prunedKittyParent(t)
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1,V=2", []byte{0, 255, 0}))
			if !strings.Contains(replies.String(), ";OK") {
				t.Fatal("missing setup record", replies.String())
			}
			term.Write(kittyCommand("a=d,d=i,i=8,p=1", nil))
			term.Write([]byte("\x1b[1;2H"))
			term.Write(kittyCommand(deletion, nil))
			replies.Reset()
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1", []byte{0, 255, 0}))
			if !strings.Contains(replies.String(), "ENOPARENT: parent placement not found") {
				t.Fatalf("deleted record still addressable: %q", replies.String())
			}
		})
	}
}

func TestKittyRecordBufferIdentityAndClear(t *testing.T) {
	for _, clear := range []string{"\x1b[2J", "\x1bc", "\x1b[?1047l\x1b[?47h"} {
		t.Run(clear, func(t *testing.T) {
			term, replies := prunedKittyParent(t)
			term.Write([]byte("\x1b[?47l"))
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1", []byte{0, 255, 0}))
			if !strings.Contains(replies.String(), "ENOPARENT: parent placement not in current buffer") {
				t.Fatalf("lost record's buffer identity: %q", replies.String())
			}
			term.Write([]byte("\x1b[?47h" + clear))
			replies.Reset()
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1", []byte{0, 255, 0}))
			if !strings.Contains(replies.String(), "ENOPARENT: parent placement not found") {
				t.Fatalf("clear retained old record: %q", replies.String())
			}
		})
	}
}

func TestKittyVirtualRecordGeometricDeletion(t *testing.T) {
	term, replies := graphicTerminal(t)
	term.Write([]byte("\x1b[3;2H"))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1,c=3,r=2,z=55", []byte{255, 0, 0}))
	term.Write(kittyCommand("a=d,d=q,x=4,y=4,z=55", nil))
	replies.Reset()
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1", []byte{0, 255, 0}))
	if !strings.Contains(replies.String(), "ENOPARENT: parent placement not found") {
		t.Fatalf("geometric delete ignored virtual record: %q", replies.String())
	}
}

func TestKittyRecordSpatialDeletionUsesActiveCoordinatesAcrossBuffers(t *testing.T) {
	term, replies := graphicTerminal(t)
	term.Write([]byte("\x1b[?47h\x1b[2;3H"))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1,c=2,r=2,z=55", []byte{255, 0, 0}))
	term.Write([]byte("\x1b[?47l"))
	// Swift's record predicates use active-buffer coordinates against its
	// global record table; they do not filter by record.isAlternateBuffer.
	term.Write(kittyCommand("a=d,d=q,x=3,y=2,z=55", nil))
	term.Write([]byte("\x1b[?47h"))
	replies.Reset()
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1", []byte{0, 255, 0}))
	if !strings.Contains(replies.String(), "ENOPARENT: parent placement not found") {
		t.Fatalf("global record selection differs from Swift: %q", replies.String())
	}
}

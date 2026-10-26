package vt

import (
	"bytes"
	"strings"
	"testing"
	"unsafe"
)

func TestKittyCacheSharedBetweenScreens(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=11,p=1,c=1,r=1,C=1", []byte{255, 0, 0}))
	term.Write([]byte("\x1b[?47h"))
	replies.Reset()
	term.Write(kittyCommand("a=p,i=11,p=2,c=1,r=1,C=1", nil))
	if g := term.Graphics(10, 20); !strings.Contains(replies.String(), ";OK") || len(g) != 1 || g[0].Image.Pix[0] != 255 {
		t.Fatal("alternate screen cannot display the primary cache entry", replies.String())
	}
	term.Write(kittyCommand("a=t,f=24,s=1,v=1,i=11", []byte{0, 0, 255}))
	if g := term.Graphics(10, 20); len(g) != 1 || g[0].Image.Pix[0] != 255 {
		t.Fatal("cache replacement changed the existing alternate display")
	}
	term.Write([]byte("\x1b[?47l"))
	if g := term.Graphics(10, 20); len(g) != 1 || g[0].Image.Pix[0] != 255 {
		t.Fatal("cache replacement changed the existing primary display")
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=11,p=3,c=1,r=1,C=1", nil))
	if g := term.Graphics(10, 20); !strings.Contains(replies.String(), ";OK") || len(g) != 2 || g[1].Image.Pix[2] != 255 {
		t.Fatal("primary screen cannot use the new global cache content", replies.String())
	}
}

func TestKittySharedPixelsAccountingAndRelease(t *testing.T) {
	term, _ := kittyCacheTerminal(t)
	pixels := func() uintptr {
		t.Helper()
		g := term.Graphics(10, 20)
		if len(g) != 1 {
			t.Fatal("expected one display", len(g))
		}
		var storage, data uintptr
		if err := term.get(30, unsafe.Pointer(&storage)); err != nil {
			t.Fatal(err)
		}
		native := call(fnGraphicsImage, storage, uintptr(g[0].ImageID))
		if native == 0 || !ok(call(fnImageGet, native, 7, uintptr(unsafe.Pointer(&data)))) || data == 0 {
			t.Fatal("missing native pixel data")
		}
		return data
	}
	checkBytes := func(want uint64) {
		t.Helper()
		var usage struct {
			size               uintptr
			compression        bool
			_                  [7]byte
			primary, alternate [6]uint64
		}
		usage.size = unsafe.Sizeof(usage)
		if err := term.get(42, unsafe.Pointer(&usage)); err != nil {
			t.Fatal(err)
		}
		if got := usage.primary[5] + usage.alternate[5]; got != want {
			t.Fatalf("pixel allocation bytes = %d, want %d", got, want)
		}
	}
	term.Write(kittyCommand("a=T,f=32,s=1,v=1,i=11,p=1,c=1,r=1,C=1", []byte{255, 0, 0, 255}))
	primary := pixels()
	checkBytes(4)
	term.Write([]byte("\x1b[?47h"))
	term.Write(kittyCommand("a=p,i=11,p=2,c=1,r=1,C=1", nil))
	if pixels() != primary {
		t.Fatal("cross-screen display copied pixel allocation")
	}
	checkBytes(4)
	term.Write([]byte("\x1b[2J"))
	checkBytes(4) // Main display still owns the allocation after global cache clear.
	term.Write([]byte("\x1b[?47l"))
	if pixels() != primary {
		t.Fatal("clearing alternate screen invalidated main pixels")
	}
	term.Write(kittyCommand("a=d,d=I,i=11", nil))
	checkBytes(0)
}

func TestKittyUnsupportedAnimationPreservesSharedPixels(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=11,p=1,c=1,r=1,C=1", []byte{255, 0, 0}))
	for _, command := range []string{"a=f,f=24,s=1,v=1,i=11", "a=a,i=11,s=3", "a=c,i=11,r=1,c=1", "a=d,d=f,i=11,r=1"} {
		replies.Reset()
		term.Write(kittyCommand(command, []byte{0, 0, 255}))
		if !strings.Contains(replies.String(), "EINVAL") {
			t.Fatal("unsupported action did not report error", command, replies.String())
		}
		if g := term.Graphics(10, 20); len(g) != 1 || g[0].Image.Pix[0] != 255 {
			t.Fatal("unsupported action changed display", command)
		}
	}
}

func TestKittyPlacementAndDeletionIdentitySpansScreens(t *testing.T) {
	for _, operation := range []string{"a=p,i=11,p=1,c=1,r=1,C=1", "a=d,d=i,i=11", "a=d,d=r,x=11,y=11", "a=d,d=z,z=2"} {
		t.Run(operation, func(t *testing.T) {
			term, _ := kittyCacheTerminal(t)
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=11,p=1,z=2,c=1,r=1,C=1", []byte{255, 0, 0}))
			term.Write([]byte("\x1b[?47h"))
			term.Write(kittyCommand(operation, nil))
			term.Write([]byte("\x1b[?47l"))
			if len(term.Graphics(10, 20)) != 0 {
				t.Fatal("primary placement survived a global identity operation")
			}
		})
	}
}

func TestKittyPendingUploadSurvivesScreenSwitchAndClear(t *testing.T) {
	for _, intervening := range []string{"\x1b[?47h", "\x1b[2J", "\x1bc"} {
		t.Run(intervening, func(t *testing.T) {
			term, replies := kittyCacheTerminal(t)
			term.Write(kittyEncodedCommand("a=T,f=24,s=1,v=1,i=11,p=1,c=1,r=1,C=1,m=1", "/"))
			term.Write([]byte(intervening))
			term.Write(kittyEncodedCommand("m=0", "wAA"))
			if g := term.Graphics(10, 20); !strings.Contains(replies.String(), ";OK") || len(g) != 1 || g[0].Image.Pix[0] != 255 {
				t.Fatal("terminal-wide pending transmission was lost", replies.String())
			}
		})
	}
}

func TestKittyCacheQuotaAndEvictionSpanScreens(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	data := bytes.Repeat([]byte{255, 0, 0, 255}, 1024*512)
	for _, id := range []string{"1", "2"} {
		term.Write(kittyCommand("a=T,f=32,s=1024,v=512,i="+id+",p=1,c=1,r=1,C=1", data))
	}
	term.Write([]byte("\x1b[?47h"))
	replies.Reset()
	term.Write(kittyCommand("a=p,i=1,p=2,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), ";OK") {
		t.Fatal("cache is not shared", replies.String())
	}
	term.Write(kittyCommand("a=t,f=32,s=1024,v=1024,i=2", bytes.Repeat(data, 2)))
	term.Write([]byte("\x1b[?47l"))
	if len(term.Graphics(10, 20)) != 2 {
		t.Fatal("global eviction removed old primary displays")
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=1,p=3,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), "ENOENT") {
		t.Fatal("eviction only applied to one screen", replies.String())
	}
}

func TestKittyAlternateClearDropsGlobalCacheButPreservesPrimaryDisplay(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=11,p=1,c=1,r=1,C=1", []byte{255, 0, 0}))
	term.Write([]byte("\x1b[?47h\x1b[2J\x1b[?47l"))
	if len(term.Graphics(10, 20)) != 1 {
		t.Fatal("alternate ED 2 removed primary display")
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=11,p=2,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), "ENOENT") || len(term.Graphics(10, 20)) != 1 {
		t.Fatal("alternate ED 2 retained global cache content", replies.String())
	}
}

func TestKittyEnteringAlternateCleansUnusedCacheAndOldDisplay(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyCommand("a=t,f=24,s=1,v=1,i=11", []byte{255, 0, 0}))
	term.Write([]byte("\x1b[?47h"))
	replies.Reset()
	term.Write(kittyCommand("a=p,i=11,p=1,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), "ENOENT") {
		t.Fatal("unused cache survived alternate activation", replies.String())
	}
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=12,p=1,c=1,r=1,C=1", []byte{0, 0, 255}))
	term.Write([]byte("\x1b[?47l\x1b[?47h"))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("alternate reactivation kept old Kitty placements")
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=12,p=1,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), "ENOENT") {
		t.Fatal("cleared alternate image remained cached", replies.String())
	}
}

func TestKittyNumberAliasAndUnusedCleanupAcrossScreens(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,I=7,p=1,c=1,r=1,C=1", []byte{255, 0, 0}))
	term.Write([]byte("\x1b[?47h"))
	replies.Reset()
	term.Write(kittyCommand("a=p,I=7,p=2,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), ";OK") || len(term.Graphics(10, 20)) != 1 {
		t.Fatal("number alias was not shared", replies.String())
	}
	term.Write(kittyCommand("a=t,f=24,s=1,v=1,i=22", []byte{0, 0, 255}))
	// Uppercase deletion cleans all unreferenced cache entries, including
	// unrelated IDs, but preserves images displayed on the inactive screen.
	term.Write(kittyCommand("a=d,d=I,i=999", nil))
	replies.Reset()
	term.Write(kittyCommand("a=p,i=22,p=3,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), "ENOENT") {
		t.Fatal("unrelated unused cache survived uppercase deletion", replies.String())
	}
	term.Write(kittyCommand("a=d,d=n,I=7,p=2", nil))
	replies.Reset()
	term.Write(kittyCommand("a=p,I=7,p=3,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), ";OK") {
		t.Fatal("inactive display failed to retain its global cache", replies.String())
	}
	term.Write(kittyCommand("a=d,d=N,I=7", nil))
	term.Write([]byte("\x1b[?47l"))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("number deletion did not remove primary display")
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,I=7,p=4,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), "ENOENT") {
		t.Fatal("freed image retained its number alias", replies.String())
	}
}

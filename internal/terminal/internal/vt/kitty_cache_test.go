package vt

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"unsafe"
)

func kittyCacheTerminal(t *testing.T) (*Terminal, *strings.Builder) {
	t.Helper()
	term, replies := graphicTerminal(t)
	if oshellABI < 9 || oshellABI > 62 {
		if os.Getenv("OSHELL_REQUIRE_WIDTH_EXTENSION") == "1" {
			t.Fatal("required OShell retained graphics extension absent")
		}
		t.Skip("requires ABI 9")
	}
	return term, replies
}

func TestKittyFailedAndPendingRetransmitPreserveDisplayedPixels(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	red := []byte{255, 0, 0}
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=11,p=1,c=1,r=1,C=1", red))
	before := term.Graphics(10, 20)
	term.Write(kittyCommand("a=t,f=24,s=1,v=1,i=11", []byte{0}))
	if after := term.Graphics(10, 20); len(after) != 1 || after[0].Image != before[0].Image {
		t.Fatal("invalid retransmission removed the old display")
	}
	replies.Reset()
	term.Write(kittyEncodedCommand("a=t,f=24,s=1,v=1,i=11,m=1", "A"))
	if after := term.Graphics(10, 20); len(after) != 1 || after[0].Image != before[0].Image {
		t.Fatal("incomplete retransmission removed the old display")
	}
	term.Write(kittyEncodedCommand("m=0", "AD/"))
	if !strings.Contains(replies.String(), ";OK") {
		t.Fatal("complete replacement failed", replies.String())
	}
	if after := term.Graphics(10, 20); len(after) != 1 || !bytes.Equal(after[0].Image.Pix, []byte{255, 0, 0, 255}) {
		t.Fatal("transmit-only replacement changed already displayed pixels")
	}
	term.Write(kittyCommand("a=p,i=11,p=2,c=1,r=1,C=1", nil))
	if after := term.Graphics(10, 20); len(after) != 2 {
		t.Fatal("old and new display generations cannot coexist")
	}
	term.Write(kittyCommand("a=p,i=11,p=1,c=1,r=1,C=1", nil))
	after := term.Graphics(10, 20)
	if len(after) != 2 {
		t.Fatal("replacing placement did not remove its old generation", len(after))
	}
	for _, g := range after {
		if !bytes.Equal(g.Image.Pix, []byte{0, 0, 255, 255}) {
			t.Fatal("old pixels survived explicit placement replacement")
		}
	}
	term.Write(kittyCommand("a=d,d=I,i=11", nil))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("ID delete left retained generations")
	}
}

func TestKittyEvictionRetainsDisplayButRemovesLookup(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	data := bytes.Repeat([]byte{200, 10, 20, 255}, 1024*512)
	for _, id := range []string{"21", "22"} {
		term.Write(kittyCommand("a=T,f=32,s=1024,v=512,i="+id+",p=1,c=1,r=1,C=1", data))
	}
	data = append(data, data...)
	term.Write(kittyCommand("a=t,f=32,s=1024,v=1024,i=22", data))
	if len(term.Graphics(10, 20)) != 2 {
		t.Fatal("protocol cache eviction removed displayed pixels")
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=21,p=2,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), "ENOENT") || len(term.Graphics(10, 20)) != 2 {
		t.Fatal("evicted entry remained addressable", replies.String())
	}
	term.Write(kittyCommand("a=d,d=i,i=21", nil))
	if len(term.Graphics(10, 20)) != 1 {
		t.Fatal("lowercase ID deletion did not release detached display")
	}
}

func TestKittyRetainedGenerationsDeleteByRangeAndNumber(t *testing.T) {
	for _, selector := range []string{"d=r,x=31,y=31", "d=n,I=71"} {
		term, replies := kittyCacheTerminal(t)
		id := "i=31"
		if strings.Contains(selector, "I=71") {
			id = "I=71"
		}
		term.Write(kittyCommand("a=T,f=24,s=1,v=1,"+id+",p=1,c=1,r=1,C=1", []byte{255, 0, 0}))
		term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=90,p=1,c=1,r=1,C=1", []byte{0, 0, 255}))
		// Grow an already used entry. A new unused ID would evict itself.
		term.Write(kittyCommand("a=t,f=32,s=1024,v=1024,i=90", bytes.Repeat([]byte{0, 0, 255, 255}, 1024*1024)))
		if len(term.Graphics(10, 20)) != 2 {
			t.Fatal("missing retained display", replies.String())
		}
		term.Write(kittyCommand("a=d,"+selector, nil))
		want := 1
		if strings.Contains(selector, "I=71") {
			// Swift removes the number mapping when that cache entry leaves.
			// The retained screen image alone does not restore the mapping.
			want = 2
		}
		if len(term.Graphics(10, 20)) != want {
			t.Fatal("selector used the wrong image identity", selector)
		}
	}
}

func TestKittyProtocolIDCanCollideWithPrivateImageStorage(t *testing.T) {
	for _, kind := range []string{"inline", "retained"} {
		t.Run(kind, func(t *testing.T) {
			term, replies := kittyCacheTerminal(t)
			if kind == "inline" {
				term.Write(append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 1, 1))...), '\a'))
			} else {
				term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=1,p=1,c=1,r=1,C=1", []byte{255, 0, 0}))
				term.Write(kittyCommand("a=t,f=24,s=1,v=1,i=1", []byte{0, 255, 0}))
			}
			before := term.Graphics(10, 20)
			if len(before) != 1 {
				t.Fatal("missing private image fixture")
			}
			privateID := before[0].ImageID
			replies.Reset()
			term.Write(kittyCommand(fmt.Sprintf("a=p,i=%d,p=2", privateID), nil))
			if !strings.Contains(replies.String(), "ENOENT") {
				t.Fatal("private record is not a protocol cache entry", replies.String())
			}
			term.Write(kittyCommand(fmt.Sprintf("a=d,d=I,i=%d", privateID), nil))
			if len(term.Graphics(10, 20)) != 1 {
				t.Fatal("deleting a colliding protocol ID removed private pixels")
			}
			replies.Reset()
			term.Write(kittyCommand(fmt.Sprintf("a=T,f=24,s=1,v=1,i=%d,p=1,c=1,r=1,C=1", privateID), []byte{0, 0, 255}))
			g := term.Graphics(10, 20)
			if !strings.Contains(replies.String(), ";OK") || len(g) != 2 {
				t.Fatal("explicit protocol ID was reserved by a private image", replies.String())
			}
			foundOld := false
			for _, image := range g {
				if image.Generation == before[0].Generation && image.Image == before[0].Image {
					foundOld = true
				}
			}
			if !foundOld {
				t.Fatal("relocating private storage replaced its pixels")
			}
			term.Write(kittyCommand(fmt.Sprintf("a=d,d=I,i=%d", privateID), nil))
			if g := term.Graphics(10, 20); len(g) != 1 || g[0].Generation != before[0].Generation {
				t.Fatal("client deletion affected the relocated private image")
			}
			if kind == "retained" {
				term.Write(kittyCommand("a=d,d=I,i=1", nil))
				if len(term.Graphics(10, 20)) != 0 {
					t.Fatal("relocation lost the original logical image ID")
				}
			}
		})
	}
}

func TestKittyNumberDeletionUsesOnlyCurrentMapping(t *testing.T) {
	term, _ := kittyCacheTerminal(t)
	for _, pixel := range [][]byte{{255, 0, 0}, {0, 0, 255}} {
		term.Write(kittyCommand("a=T,f=24,s=1,v=1,I=51,p=1,c=1,r=1,C=1", pixel))
	}
	if len(term.Graphics(10, 20)) != 2 {
		t.Fatal("number reuse must assign distinct image IDs")
	}
	term.Write(kittyCommand("a=d,d=n,I=51", nil))
	if g := term.Graphics(10, 20); len(g) != 1 || g[0].Image.Pix[0] != 255 {
		t.Fatal("number deletion removed an older image with the same number")
	}
}

func TestKittyNumberMappingSurvivesExplicitIDReplacement(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,I=51,p=1,c=1,r=1,C=1", []byte{255, 0, 0}))
	id := regexp.MustCompile(`i=(\d+)`).FindStringSubmatch(replies.String())
	if len(id) != 2 {
		t.Fatal("missing assigned image ID", replies.String())
	}
	term.Write(kittyCommand("a=t,f=24,s=1,v=1,i="+id[1], []byte{0, 0, 255}))
	replies.Reset()
	term.Write(kittyCommand("a=p,I=51,p=2,c=1,r=1,C=1", nil))
	g := term.Graphics(10, 20)
	if !strings.Contains(replies.String(), ";OK") || len(g) != 2 || g[1].Image.Pix[2] != 255 {
		t.Fatal("explicit ID replacement lost the number alias", replies.String())
	}
	term.Write(kittyCommand("a=d,d=n,I=51", nil))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("number alias did not delete every generation of its current ID")
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,I=51,p=1,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), ";OK") {
		t.Fatal("lowercase deletion removed the number/cache entry")
	}
	term.Write(kittyCommand("a=d,d=N,I=51", nil))
	replies.Reset()
	term.Write(kittyCommand("a=p,I=51,p=1,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), "ENOENT") {
		t.Fatal("uppercase deletion retained the number alias")
	}
}

func TestKittyNumberIDsFollowTerminalSequence(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=1,p=2,c=1,r=1,C=1", []byte{255, 0, 0}))
	for index, reset := range []string{"", "\x1b_Ga=d,d=I,i=1\x1b\\", "\x1b[2J", "\x1bc", "\x1b[?47h"} {
		term.Write([]byte(reset))
		// Invalid uploads and queries must not consume the sequence.
		term.Write(kittyCommand("a=t,f=24,s=2,v=2,I=99", []byte{0}))
		term.Write(kittyCommand("a=q,f=24,s=1,v=1,I=99", []byte{0, 0, 255}))
		replies.Reset()
		term.Write(kittyCommand(fmt.Sprintf("a=T,f=24,s=1,v=1,I=%d,p=1,c=1,r=1,C=1", 71+index), []byte{0, 0, 255}))
		want := fmt.Sprintf("i=%d,I=%d,p=1;OK", index+1, 71+index)
		if !strings.Contains(replies.String(), want) {
			t.Fatal("automatic IDs did not follow the terminal sequence", index, replies.String())
		}
		if index == 0 && len(term.Graphics(10, 20)) != 2 {
			t.Fatal("assigned ID collision did not preserve the old display")
		}
	}
}

func TestKittyAnonymousImagesNeverEnterProtocolCache(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyCommand("a=t,f=24,s=1,v=1,i=1", []byte{255, 0, 0}))
	data := bytes.Repeat([]byte{0, 0, 255, 255}, 1024*1024)
	for _, action := range []string{"t", "T"} {
		replies.Reset()
		term.Write(kittyCommand("a="+action+",f=32,s=1024,v=1024,c=1,r=1,C=1", data))
		if replies.Len() != 0 {
			t.Fatal("anonymous upload returned an identifier", replies.String())
		}
		want := 0
		if action == "T" {
			want = 1
		}
		if len(term.Graphics(10, 20)) != want {
			t.Fatal("anonymous display count", action)
		}
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=1,p=1,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), ";OK") || len(term.Graphics(10, 20)) != 2 {
		t.Fatal("anonymous data consumed quota and evicted a protocol image", replies.String())
	}
	term.Write(kittyCommand("a=d,d=R,x=1,y=4294967295", nil))
	if g := term.Graphics(10, 20); len(g) != 1 || g[0].Image.Pix[2] != 255 {
		t.Fatal("ID-range deletion selected an anonymous image")
	}
	term.Write(kittyCommand("a=d,d=A", nil))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("visible deletion did not remove anonymous image")
	}
}

func TestKittyCacheAccountsPNGEncodedBytes(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	png := inlinePNG(t, 1200, 1000)
	if len(png) >= KittyImageLimit {
		t.Fatal("fixture must fit the encoded cache quota")
	}
	term.Write(kittyCommand("a=T,f=100,i=61,p=1,c=1,r=1,C=1", png))
	if g := term.Graphics(10, 20); len(g) != 1 || len(g[0].Image.Pix) <= KittyImageLimit {
		t.Fatal("PNG cache quota used decoded pixels", replies.String())
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=61,p=2,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), ";OK") || len(term.Graphics(10, 20)) != 2 {
		t.Fatal("encoded PNG cache entry did not remain available", replies.String())
	}
}

func TestKittyCacheEvictsLeastRecentlyAccessed(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	data := bytes.Repeat([]byte{40, 80, 120, 255}, 512*512)
	for _, id := range []string{"1", "2", "3"} {
		term.Write(kittyCommand("a=T,f=32,s=512,v=512,i="+id+",p=1,c=1,r=1,C=1", data))
	}
	term.Write(kittyCommand("a=p,i=1,p=1,c=1,r=1,C=1", nil)) // Refresh entry 1.
	term.Write(kittyCommand("a=t,f=32,s=1536,v=512,i=3", bytes.Repeat(data, 3)))
	if len(term.Graphics(10, 20)) != 3 {
		t.Fatal("eviction removed one of the three displayed generations")
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=2,p=2,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), "ENOENT") {
		t.Fatal("cache did not evict the least recently accessed entry", replies.String())
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=1,p=1,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), ";OK") {
		t.Fatal("cache evicted the recently accessed entry", replies.String())
	}
}

func TestKittyIncomingUnusedCacheEntryCanEvictItself(t *testing.T) {
	for _, action := range []string{"t", "T"} {
		t.Run(action, func(t *testing.T) {
			term, replies := kittyCacheTerminal(t)
			data := bytes.Repeat([]byte{30, 60, 90, 255}, 1024*1024)
			term.Write(kittyCommand("a=T,f=32,s=1024,v=1024,i=1,p=1,c=1,r=1,C=1", data))
			replies.Reset()
			term.Write(kittyCommand("a="+action+",f=32,s=1024,v=1024,i=2,p=1,c=1,r=1,C=1", data))
			if !strings.Contains(replies.String(), ";OK") {
				t.Fatal("incoming transmission failed", replies.String())
			}
			want := 1
			if action == "T" {
				want = 2
			}
			if len(term.Graphics(10, 20)) != want {
				t.Fatal("cache self-eviction changed the requested display")
			}
			replies.Reset()
			term.Write(kittyCommand("a=p,i=2,p=2,c=1,r=1,C=1", nil))
			if !strings.Contains(replies.String(), "ENOENT") {
				t.Fatal("unused incoming cache entry was incorrectly protected", replies.String())
			}
			replies.Reset()
			term.Write(kittyCommand("a=p,i=1,p=1,c=1,r=1,C=1", nil))
			if !strings.Contains(replies.String(), ";OK") {
				t.Fatal("used old cache entry was evicted", replies.String())
			}
		})
	}
}

func TestKittyImageLargerThanCacheStillDisplays(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	data := bytes.Repeat([]byte{20, 40, 60, 255}, 1024*1280)
	term.Write(kittyCommand("a=T,f=32,s=1024,v=1280,i=1,p=1,c=1,r=1,C=1", data))
	g := term.Graphics(10, 20)
	if !strings.Contains(replies.String(), ";OK") || len(g) != 1 || len(g[0].Image.Pix) != len(data) {
		t.Fatal("image over cache quota did not display", replies.String())
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=1,p=2,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), "ENOENT") || len(term.Graphics(10, 20)) != 1 {
		t.Fatal("oversized image was cached or lost its display", replies.String())
	}
	term.Write(kittyCommand("a=d,d=i,i=1", nil))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("oversized retained image could not be deleted")
	}
}

func TestKittyRetainedPixelsReleasedAfterHistoryPruning(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	if err := term.SetHistoryLines(1); err != nil {
		t.Fatal(err)
	}
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=1,p=1,c=1,r=1,C=1", []byte{255, 0, 0}))
	term.Write(kittyCommand("a=t,f=24,s=1,v=1,i=1", []byte{0, 0, 255}))
	g := term.Graphics(10, 20)
	if len(g) != 1 {
		t.Fatal("missing retained pixels")
	}
	oldID := g[0].ImageID
	// Exact logical capacity can discard an image inside a partial page.
	term.Write([]byte(strings.Repeat("\r\n", 32)))
	if bar := term.Scrollbar(); bar.Total-bar.Len != 1 {
		t.Fatal("history fixture exceeded its line capacity", bar)
	}
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("pruned image still displayed")
	}
	var storage uintptr
	if err := term.get(30, unsafe.Pointer(&storage)); err != nil {
		t.Fatal(err)
	}
	if call(fnGraphicsImage, storage, uintptr(oldID)) != 0 {
		t.Fatal("pruned retained pixels remain in native storage")
	}
	// Pixel storage is gone, but Swift's logical record still remembers its
	// last absolute buffer row. It remains addressable after physical history removal.
	replies.Reset()
	vertical := int(term.Scrollbar().Total) - 10 + 2
	term.Write(kittyCommand(fmt.Sprintf("a=T,f=24,s=1,v=1,i=2,p=1,P=1,Q=1,H=2,V=%d", vertical), []byte{0, 255, 0}))
	if g := term.Graphics(10, 20); !strings.Contains(replies.String(), ";OK") || len(g) != 1 || g[0].X != 20 || g[0].Y != 40 {
		t.Fatalf("history removal lost logical parent origin: %q %+v", replies.String(), g)
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=1,p=1,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), ";OK") {
		t.Fatal("pruning the old display incorrectly removed the current cache entry")
	}
}

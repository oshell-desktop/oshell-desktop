package vt

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"unsafe"
)

func inlineCacheTerminal(t *testing.T) (*Terminal, *strings.Builder) {
	t.Helper()
	term, replies := inlineTerminal(t)
	if oshellABI < 7 || oshellABI > 62 {
		if os.Getenv("OSHELL_REQUIRE_WIDTH_EXTENSION") == "1" {
			t.Fatal("required OShell independent image-cache extension absent")
		}
		t.Skip("requires ABI 7")
	}
	// Tall cells keep large images visible without creating thousands of pins.
	if err := term.Resize(20, 10, 10, 2048); err != nil {
		t.Fatal(err)
	}
	return term, replies
}

func TestInlineImagesDoNotConsumeOrSufferKittyEviction(t *testing.T) {
	term, replies := inlineCacheTerminal(t)
	packet := append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 1000, 600))...), '\a')
	term.Write(packet)
	term.Write(packet) // Two row images alone exceed the 4 MiB protocol cache.
	if len(term.Graphics(10, 2048)) != 2 {
		t.Fatal("inline images evicted each other")
	}
	// A full Kitty cache must neither evict inline pixels nor be charged for them.
	raw := bytes.Repeat([]byte{10, 20, 30, 255}, 1024*1024)
	for index, id := range []string{"91", "92"} {
		replies.Reset()
		term.Write(kittyCommand("a=T,f=32,s=1024,v=1024,i="+id+",c=1,r=1,C=1", raw))
		if !strings.Contains(replies.String(), ";OK") {
			t.Fatal("Kitty transmission failed", replies.String())
		}
		g := term.Graphics(10, 2048)
		// Evicting a Kitty protocol cache entry retains its already displayed
		// generation, independently of the two ordinary row images.
		if len(g) != 3+index {
			t.Fatalf("Kitty eviction removed row images: %d", len(g))
		}
		inline := 0
		for _, image := range g {
			if image.Image.Bounds().Dx() == 1000 {
				inline++
			}
		}
		if inline != 2 {
			t.Fatal("inline images did not survive Kitty eviction")
		}
	}
	term.Write([]byte("\x1b[2J"))
	if len(term.Graphics(10, 2048)) != 0 || len(term.graphicsCache) != 0 {
		t.Fatal("ED 2 retained independent image storage")
	}
	term.Write(packet)
	if len(term.Graphics(10, 2048)) != 1 {
		t.Fatal("cleared storage could not accept new inline image")
	}
}

func TestInlineAndSixelPixelsCanExceedKittyCacheQuota(t *testing.T) {
	for _, protocol := range []string{"inline", "sixel"} {
		t.Run(protocol, func(t *testing.T) {
			term, _ := inlineCacheTerminal(t)
			if err := term.Resize(20, 10, 64, 2048); err != nil {
				t.Fatal(err)
			}
			var packet []byte
			if protocol == "inline" {
				packet = append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 1200, 1000))...), '\a')
			} else {
				packet = []byte("\x1bPq#1;2;100;0;0" + strings.Repeat("!1200~-", 167) + "\x1b\\")
			}
			term.Write(packet)
			g := term.Graphics(64, 2048)
			if len(g) != 1 || len(g[0].Image.Pix) <= KittyImageLimit || g[0].Image.Bounds().Dx() != 1200 {
				t.Fatal("valid image above protocol quota was rejected")
			}
			term.Write([]byte(strings.Repeat("\r\n", 12)))
			term.ScrollToTop()
			if len(term.Graphics(64, 2048)) != 1 {
				t.Fatal("large row image did not survive scroll into history")
			}
			term.Write([]byte("\x1bc"))
			if len(term.Graphics(64, 2048)) != 0 {
				t.Fatal("reset retained large row image")
			}
		})
	}
}

func TestInlineImageLargePacketFragmentsCancelAndOverflow(t *testing.T) {
	packet := append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 1000, 600))...), '\a')
	if len(packet) <= 2048 {
		t.Fatal("fixture must exceed the old OSC control buffer")
	}
	for _, chunk := range []int{1, 127, len(packet)} {
		term, _ := inlineCacheTerminal(t)
		for start := 0; start < len(packet); start += chunk {
			term.Write(packet[start:min(start+chunk, len(packet))])
		}
		if len(term.Graphics(10, 2048)) != 1 {
			t.Fatalf("large OSC lost at chunk size %d", chunk)
		}
	}
	for _, cancel := range []byte{0x18, 0x1a} {
		term, _ := inlineCacheTerminal(t)
		term.Write(packet[:len(packet)-1])
		term.Write([]byte{cancel})
		if len(term.Graphics(10, 2048)) != 0 {
			t.Fatal("cancelled OSC image displayed")
		}
		term.Write(packet)
		if len(term.Graphics(10, 2048)) != 1 {
			t.Fatal("valid image after cancellation was dropped")
		}
	}
	term, _ := inlineCacheTerminal(t)
	term.Write([]byte("\x1b]1337;File=inline=1:" + strings.Repeat("A", 8<<20) + "\a"))
	if len(term.Graphics(10, 2048)) != 0 {
		t.Fatal("oversized image packet displayed")
	}
	term.Write(packet)
	if len(term.Graphics(10, 2048)) != 1 {
		t.Fatal("OSC overflow poisoned following valid image")
	}
}

func TestInlinePixelsReleasedAfterLastHistoryRowIsRemoved(t *testing.T) {
	term, _ := inlineCacheTerminal(t)
	term.Write(append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 1000, 600))...), '\a'))
	g := term.Graphics(10, 2048)
	if len(g) != 1 {
		t.Fatal("missing row image")
	}
	id := g[0].ImageID
	var usage struct {
		size        uintptr
		compression bool
		_           [7]byte
		primary     [6]uint64
		alternate   [6]uint64
	}
	usage.size = unsafe.Sizeof(usage)
	if err := term.get(42, unsafe.Pointer(&usage)); err != nil || usage.primary[5] < 1000*600*4 {
		t.Fatal("native memory accounting omitted inline pixels", err, usage.primary[5])
	}
	term.Write([]byte(strings.Repeat("\r\n", 12)))
	term.ScrollToTop()
	if len(term.Graphics(10, 2048)) != 1 {
		t.Fatal("live history image was prematurely released")
	}
	term.Write([]byte("\x1b[3J"))
	if len(term.Graphics(10, 2048)) != 0 {
		t.Fatal("deleted history image still visible")
	}
	var storage uintptr
	if err := term.get(30, unsafe.Pointer(&storage)); err != nil {
		t.Fatal(err)
	}
	if call(fnGraphicsImage, storage, uintptr(id)) != 0 {
		t.Fatal("native pixels retained after their last row was removed")
	}
	if err := term.get(42, unsafe.Pointer(&usage)); err != nil || usage.primary[5] != 0 {
		t.Fatal("native memory accounting retained deleted pixels", err, usage.primary[5])
	}
}

func TestInlineHistoryErasePreservesRemainingStripes(t *testing.T) {
	term, _ := inlineCacheTerminal(t)
	if err := term.Resize(20, 2, 10, 20); err != nil {
		t.Fatal(err)
	}
	term.Write(append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 4, 80))...), '\a'))
	before := term.Graphics(10, 20)
	if len(before) != 1 {
		t.Fatal("expected one remaining active stripe", len(before))
	}
	term.Write([]byte("\x1b[3J"))
	after := term.Graphics(10, 20)
	if len(after) != 1 || after[0].Source != before[0].Source || after[0].ImageID != before[0].ImageID {
		t.Fatal("history erase deleted or relocated active stripes")
	}
	// Adding a long image can prune its own earlier stripes. The in-progress
	// native image must stay alive until the remaining stripes have been added.
	term.SetScrollback(0)
	term.Write(append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 4, 600))...), '\a'))
	if len(term.Graphics(10, 20)) == 0 {
		t.Fatal("pruning earlier stripes freed an image still being inserted")
	}
}

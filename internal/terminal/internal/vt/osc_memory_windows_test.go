//go:build windows

package vt

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Use the public allocator ABI to measure live requested bytes, independently
// of the process heap's caching policy. No private parser counters are needed.
func TestOShellOSCReleasesCapture(t *testing.T) {
	loadLib(t)
	if oshellABI < 25 || oshellABI > 62 {
		if os.Getenv("OSHELL_REQUIRE_WIDTH_EXTENSION") == "1" {
			t.Fatal("required OShell OSC extension is absent")
		}
		t.Skip("requires OShell native extension ABI 25 or later")
	}
	var live uintptr
	var failLarge bool
	blocks := map[uintptr]uintptr{}
	table := [4]uintptr{
		newCallback(func(_, size, alignment, _ uintptr) uintptr {
			if failLarge && size >= 4096 {
				return 0
			}
			// The ABI passes Zig's log2 alignment. VirtualAlloc is 64 KiB aligned.
			// Only the low byte is defined for the native uint8_t argument.
			alignment &= 0xff
			if alignment > 16 {
				t.Logf("unsupported native alignment=%d size=%d", alignment, size)
				return 0
			}
			p, err := windows.VirtualAlloc(0, size, windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
			if err != nil {
				t.Logf("VirtualAlloc size=%d: %v", size, err)
				return 0
			}
			blocks[p] = size
			live += size
			return p
		}),
		newCallback(func(_, _, _, _, _, _ uintptr) uintptr { return 0 }),
		newCallback(func(_, _, _, _, _, _ uintptr) uintptr { return 0 }),
		newCallback(func(_, p, size, _, _ uintptr) uintptr {
			if blocks[p] != size {
				t.Errorf("allocator free size=%d, allocated=%d", size, blocks[p])
			}
			live -= blocks[p]
			delete(blocks, p)
			if err := windows.VirtualFree(p, 0, windows.MEM_RELEASE); err != nil {
				t.Error(err)
			}
			return 0
		}),
	}
	allocator := [2]uintptr{0, uintptr(unsafe.Pointer(&table))}
	var pinned runtime.Pinner
	pinned.Pin(&table)
	pinned.Pin(&allocator)
	defer pinned.Unpin()
	term := &Terminal{}
	if err := result(call(fnTerminalNew, uintptr(unsafe.Pointer(&allocator)), uintptr(unsafe.Pointer(&term.h)), 80, 24)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		term.Free()
		if live != 0 || len(blocks) != 0 {
			t.Errorf("terminal destruction retained %d bytes in %d blocks", live, len(blocks))
		}
	}()
	compat := true
	if err := term.set(1002, unsafe.Pointer(&compat)); err != nil {
		t.Fatal(err)
	}
	baseline := live
	for _, end := range []string{"\a", "\x1b\\", "\x18", "\x1a"} {
		for _, chunk := range []int{1, 17, 4096} {
			writeControlChunks(term, []byte("\x1b]777;notify;title;"+strings.Repeat("x", 65536)), chunk)
			if live <= baseline+65536 {
				t.Fatal("unfinished OSC did not retain its reception buffer")
			}
			term.Write([]byte(end))
			if live != baseline {
				t.Errorf("end=%q chunk=%d retained %d reception bytes", end, chunk, live-baseline)
			}
		}
	}
	writeControlChunks(term, []byte("\x1b]2;"+strings.Repeat("x", 8<<20+1)+"\a"), 4096)
	if live != baseline {
		t.Errorf("overflow retained %d reception bytes", live-baseline)
	}
	failLarge = true
	writeControlChunks(term, []byte("\x1b]2;"+strings.Repeat("z", 6000)+"\a"), 17)
	failLarge = false
	if live != baseline || term.Title() != "" {
		t.Error("allocation failure retained capture or committed a partial title")
	}
	// Kitty keeps decoded data across completed APCs. Completion and both
	// deletion spellings must release that separate reception buffer too.
	term.Write(kittyEncodedCommand("a=q,f=24,s=1,v=1,i=71,m=1", strings.Repeat("!", 65536)))
	if live != baseline {
		t.Error("ignored Base64 characters consumed pending storage")
	}
	term.Write(kittyEncodedCommand("m=0", ""))
	for _, end := range []string{"m=0", "a=d", "a=D"} {
		term.Write(kittyEncodedCommand("a=q,f=24,s=1,v=1,i=71,m=1", strings.Repeat("A", 65536)))
		if live < baseline+49152 {
			t.Fatal("pending Kitty transmission did not retain decoded bytes")
		}
		term.Write(kittyEncodedCommand(end, ""))
		if live != baseline {
			t.Errorf("Kitty end=%s retained %d reception bytes", end, live-baseline)
		}
	}
	// Each fragment allocation fits; growing the aggregate buffer fails. Its
	// failure state must retain the first control but release all payload bytes.
	term.Write(kittyEncodedCommand("a=q,f=24,s=1,v=1,i=71,m=1", strings.Repeat("A", 1000)))
	failLarge = true
	for range 8 {
		term.Write(kittyEncodedCommand("m=1", strings.Repeat("A", 1000)))
	}
	failLarge = false
	if live != baseline {
		t.Errorf("Kitty aggregate allocation failure retained %d bytes", live-baseline)
	}
	term.Write(kittyEncodedCommand("m=0", "BwgJ"))
	if live != baseline {
		t.Error("failed Kitty transmission retained allocations after completion")
	}
	// Ending one message must not release a following unfinished message in the
	// same Write. Its prefix must survive until a later call completes it.
	prefix := strings.Repeat("y", 6000)
	term.Write([]byte("\x1b]777;notify;done;message\a\x1b]2;" + prefix))
	term.Write([]byte("suffix\a"))
	if term.Title() != prefix+"suffix" {
		t.Error("next partial OSC lost its prefix")
	}
	term.Write(kittyEncodedCommand("a=q,f=24,s=1,v=1,i=71,m=1", strings.Repeat("A", 65536)))
	term.Write([]byte("\x1b]2;" + prefix)) // destruction must free both unfinished inputs
}

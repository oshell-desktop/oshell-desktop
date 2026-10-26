package vt

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func writeControlChunks(term *Terminal, data []byte, chunk int) {
	for len(data) > 0 {
		n := min(chunk, len(data))
		term.Write(data[:n])
		data = data[n:]
	}
}

func TestOShellC1ExecutionWithinUnfinishedCommands(t *testing.T) {
	for _, prefix := range []string{"\x1b", "\x1b[12", "\x1b[1?", "\x1b[ "} {
		for _, c := range []byte{0x84, 0x85, 0x88, 0x8D, 0x8E, 0x8F, 0x99, 0x9A} {
			for _, chunk := range []int{1, 4096} {
				t.Run(fmt.Sprintf("%x/%x/%d", prefix, c, chunk), func(t *testing.T) {
					var replies strings.Builder
					term, _ := resetTerminal(t, Effects{WritePTY: func(p []byte) { replies.Write(p) }})
					term.Write([]byte("\x1b[3;6H"))
					writeControlChunks(term, append([]byte(prefix), c), chunk)
					x, y := 5, 2
					if c == 0x84 {
						y++
					}
					if c == 0x85 {
						x, y = 0, y+1
					}
					layoutCursor(t, term, "", x, y)
					if replies.Len() != 0 {
						t.Fatal("C1 unexpectedly replied", replies.String())
					}
					if c == 0x88 {
						layoutCursor(t, term, "\r\t", 5, 2)
					}
				})
			}
		}
	}
}

func TestOShellNextLineUsesOriginBeforeScrolling(t *testing.T) {
	for _, sequence := range []string{"\x1bE", "\x1b[\x85"} {
		for _, origin := range []bool{false, true} {
			term, _ := resetTerminal(t, Effects{})
			term.Write([]byte("\x1b[2;3HA\x1b[3;3HB\x1b[4;3HC\x1b[?69h\x1b[3;9s\x1b[2;4r"))
			if origin {
				term.Write([]byte("\x1b[?6h"))
			}
			position := "\x1b[4;12H"
			wantX := 0
			if origin {
				position, wantX = "\x1b[99;99H", 2
			}
			layoutCursor(t, term, position+sequence, wantX, 3)
			rows := strings.Split(term.FormattedText(false), "\n")
			if origin && (len(rows) < 3 || rows[1] != "  B" || rows[2] != "  C") {
				t.Fatalf("NEL did not enter margins before scroll: %q", rows)
			}
		}
	}
}

func TestOShellLinefeedModeUsesOriginForLineStart(t *testing.T) {
	for _, c := range []byte{'\n', '\v', '\f'} {
		for _, origin := range []bool{false, true} {
			term, _ := resetTerminal(t, Effects{})
			term.Write([]byte("\x1b[?69h\x1b[3;9s\x1b[20h"))
			wantX := 0
			if origin {
				term.Write([]byte("\x1b[?6h"))
				wantX = 2
			}
			term.Write([]byte("\x1b[2;6H"))
			layoutCursor(t, term, string(c), wantX, 2)
		}
	}
}

func TestOShellAPCBellAndIgnoredStrings(t *testing.T) {
	for _, chunk := range []int{1, 2, 7, 4096} {
		for _, intro := range []string{"\x1b_", "\x1bX", "\x1b^"} {
			t.Run(fmt.Sprintf("%x/%d", intro, chunk), func(t *testing.T) {
				term, replies := kittyCacheTerminal(t)
				command := kittyCommand("a=T,f=24,s=1,v=1,i=11,c=1,r=1,C=1", []byte{255, 0, 0})
				command = append([]byte(intro), command[2:len(command)-2]...)
				writeControlChunks(term, append(command, '\a'), chunk)
				if intro == "\x1b_" {
					if len(term.Graphics(10, 20)) != 1 || !strings.Contains(replies.String(), ";OK") {
						t.Fatal("BEL did not complete APC", replies.String())
					}
				} else {
					// BEL does not terminate SOS/PM. Their Kitty-looking content
					// must never reach the graphics decoder, even after ST.
					term.Write([]byte("hidden\x1b\\"))
					if len(term.Graphics(10, 20)) != 0 || replies.Len() != 0 {
						t.Fatal("ignored control string dispatched graphics", replies.String())
					}
				}
				term.Write([]byte("OK"))
				if got := strings.TrimSpace(term.Text()); got != "OK" {
					t.Fatal("parser did not recover", got)
				}
			})
		}
	}
}

func TestOShellSOSAndPMNeverDispatchAPC(t *testing.T) {
	for _, intro := range []string{"\x1bX", "\x1b^", "\x1b[\x98", "\x1b[\x9e"} {
		for _, end := range []string{"\x1b\\", "\x9c", "\x18", "\x1a"} {
			for _, chunk := range []int{1, 4096} {
				t.Run(fmt.Sprintf("%x/%x/%d", intro, end, chunk), func(t *testing.T) {
					term, replies := kittyCacheTerminal(t)
					command := kittyCommand("a=T,f=24,s=1,v=1,i=12,c=1,r=1,C=1", []byte{0, 255, 0})
					data := intro + string(command[2:len(command)-2]) + end + "OK"
					writeControlChunks(term, []byte(data), chunk)
					if len(term.Graphics(10, 20)) != 0 || replies.Len() != 0 || strings.TrimSpace(term.Text()) != "OK" {
						t.Fatalf("ignored string executed or failed to recover: text=%q reply=%q", term.Text(), replies.String())
					}
				})
			}
		}
	}
}

func TestOShellAPCIgnoresC0AndCancellationRecovers(t *testing.T) {
	for _, end := range []string{"\a", "\x1b\\", "\x9c", "\x18", "\x1a"} {
		for _, chunk := range []int{1, 2, 4096} {
			t.Run(fmt.Sprintf("%x/%d", end, chunk), func(t *testing.T) {
				term, replies := kittyCacheTerminal(t)
				command := kittyCommand("a=T,f=24,s=1,v=1,i=13,c=1,r=1,C=1", []byte{0, 0, 255})
				// Ordinary controls are ignored even in the middle of a key.
				data := string(command[:6]) + "\x00\n\r\t" + string(command[6:len(command)-2]) + end + "OK"
				writeControlChunks(term, []byte(data), chunk)
				cancelled := end == "\x18" || end == "\x1a"
				want := 1
				if cancelled {
					want = 0
				}
				if len(term.Graphics(10, 20)) != want || strings.TrimSpace(term.Text()) != "OK" {
					t.Fatalf("APC completion/cancellation wrong: text=%q reply=%q", term.Text(), replies.String())
				}
				if cancelled && replies.Len() != 0 {
					t.Fatal("cancelled APC replied", replies.String())
				}
			})
		}
	}
}

func TestOShellC1ShiftDoesNotChangeCharset(t *testing.T) {
	for _, c := range []byte{0x8E, 0x8F} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("\x1b*0\x1b+0"))
		term.Write([]byte{'\x1b', '[', c, 'q'})
		if got := strings.TrimSpace(term.Text()); got != "q" {
			t.Fatal("raw C1 changed charset", got)
		}
	}
}

func TestOShellCancelledAPCDoesNotCommitOrDestroyPendingUpload(t *testing.T) {
	for _, end := range []byte{0x18, 0x1A} {
		term, replies := kittyCacheTerminal(t)
		term.Write(kittyEncodedCommand("a=T,f=24,s=1,v=1,i=51,c=1,r=1,C=1,m=1", "/"))
		replies.Reset()
		cancelled := kittyEncodedCommand("m=0", "wAA")
		cancelled = append(cancelled[:len(cancelled)-2], end)
		term.Write(cancelled)
		if replies.Len() != 0 || len(term.Graphics(10, 20)) != 0 {
			t.Fatal("cancelled packet committed")
		}
		term.Write(kittyEncodedCommand("m=0", "wD/"))
		graphics := term.Graphics(10, 20)
		if len(graphics) != 1 || !bytes.Equal(graphics[0].Image.Pix, []byte{255, 0, 255, 255}) {
			t.Fatal("cancellation changed earlier accepted upload bytes", replies.String())
		}
	}
}

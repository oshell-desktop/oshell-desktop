package vt

import (
	"fmt"
	"strings"
	"testing"
)

func TestOShellHyperlinkWriteIdentity(t *testing.T) {
	for _, id := range []string{"", "id=same"} {
		for _, chunk := range []int{1, 17, 4096} {
			t.Run(fmt.Sprintf("%s/%d", id, chunk), func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				seq := "\x1b]8;" + id + ";https://example.test/a;b\x1b\\"
				writeControlChunks(term, []byte(seq+"ab界"+seq+"cde\x1b]8;;\x1b\\."), chunk)
				for x := 0; x < 7; x++ {
					a, b, ok := term.HyperlinkRange(x, 0)
					wantA, wantB := 0, 4
					if x >= 4 {
						wantA, wantB = 4, 7
					}
					if !ok || a != wantA || b != wantB {
						t.Fatalf("col %d range %d..%d/%v want %d..%d", x, a, b, ok, wantA, wantB)
					}
				}
				if _, _, ok := term.HyperlinkRange(7, 0); ok {
					t.Fatal("plain text acquired payload")
				}
				// Mutation splits the range, and a wide tail still uses its head.
				term.Write([]byte("\x1b[1;2H\x1b[X"))
				if a, b, ok := term.HyperlinkRange(3, 0); !ok || a != 2 || b != 4 {
					t.Fatalf("after erase %d..%d/%v", a, b, ok)
				}
			})
		}
	}
}

func TestOShellHyperlinkRangeHistoryReflowAndScreen(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	if err := term.Resize(12, 3, 0, 0); err != nil {
		t.Fatal(err)
	}
	term.Write([]byte("\x1b]8;id=wrap;https://example.test\x1b\\" + strings.Repeat("x", 24) + "\x1b]8;;\x1b\\\r\nend\r\n"))
	if term.Scrollbar().Offset == 0 {
		t.Fatal("no history")
	}
	before := term.Scrollbar()
	if a, b, ok := term.HyperlinkRange(5, 0); !ok || a != 0 || b != 12 {
		t.Fatalf("continued row %d..%d/%v", a, b, ok)
	}
	if term.Scrollbar() != before {
		t.Fatal("range read changed viewport")
	}
	term.ScrollToTop()
	if a, b, ok := term.HyperlinkRange(5, 0); !ok || a != 0 || b != 12 {
		t.Fatalf("history row %d..%d/%v", a, b, ok)
	}
	if err := term.Resize(8, 6, 0, 0); err != nil {
		t.Fatal(err)
	}
	term.ScrollToTop()
	for y := 0; y < 3; y++ {
		if a, b, ok := term.HyperlinkRange(5, y); !ok || a != 0 || b != 8 {
			t.Fatalf("reflow row %d: %d..%d/%v", y, a, b, ok)
		}
	}
	term.Write([]byte("\x1b[?1049h"))
	if _, _, ok := term.HyperlinkRange(1, 0); ok {
		t.Fatal("main screen payload leaked to alternate")
	}
	term.Write([]byte("\x1b[?1049l"))
	term.ScrollToTop()
	if a, b, ok := term.HyperlinkRange(1, 0); !ok || a != 0 || b != 8 {
		t.Fatal("return lost main payload")
	}
	for _, xy := range [][2]int{{-1, 0}, {0, -1}, {8, 0}, {0, 6}} {
		if _, _, ok := term.HyperlinkRange(xy[0], xy[1]); ok {
			t.Fatal("out-of-range lookup succeeded", xy)
		}
	}
}

func TestOShellEmptyHyperlinkRangeReflowAndActiveCursor(t *testing.T) {
	for _, params := range []string{"id=empty", "x=value", ":"} {
		t.Run(params, func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			if err := term.Resize(12, 3, 0, 0); err != nil {
				t.Fatal(err)
			}
			term.Write([]byte("\x1b]8;" + params + ";\x1b\\" + strings.Repeat("x", 24) + "\x1b[3;1H"))
			if err := term.Resize(8, 6, 0, 0); err != nil {
				t.Fatal(err)
			}
			term.Write([]byte("y\x1b]8;;\x1b\\."))
			term.ScrollToTop()
			for y := 0; y < 3; y++ {
				a, b, ok := term.HyperlinkRange(1, y)
				ref, found := term.CellAt(1, y)
				if !ok || a != 0 || b != 8 || !found || term.Hyperlink(ref) != "" {
					t.Fatalf("empty row %d: %d..%d/%v", y, a, b, ok)
				}
			}
			if a, b, ok := term.HyperlinkRange(0, 3); !ok || a != 0 || b != 1 {
				t.Fatalf("active cursor lost empty payload: %d..%d/%v", a, b, ok)
			}
			ref, _ := term.CellAt(0, 3)
			if term.Hyperlink(ref) != "" {
				t.Fatal("empty storage placeholder escaped")
			}
			if _, _, ok := term.HyperlinkRange(1, 3); ok {
				t.Fatal("closing marker retained empty payload")
			}
		})
	}
}

func TestOShellEmptyHyperlinkActiveCursorVT(t *testing.T) {
	for _, params := range []string{"id=empty", "x=value"} {
		a, _ := resetTerminal(t, Effects{})
		b, _ := resetTerminal(t, Effects{})
		a.Write([]byte("\x1b]8;" + params + ";\x1b\\x"))
		// This checks the active cursor only. Existing VT output omits body
		// cell hyperlinks, so it is not a complete hyperlink snapshot.
		b.Write(a.VT())
		b.Write([]byte("y"))
		ref, ok := b.CellAt(1, 0)
		if _, _, found := b.HyperlinkRange(1, 0); !ok || !found || b.Hyperlink(ref) != "" {
			t.Fatal("cursor replay lost empty URL or exposed its storage byte")
		}
	}
}

func TestOShellHyperlinkContinuesAfterResize(t *testing.T) {
	for _, params := range []string{"id=same", "x=value", ":"} {
		for _, uri := range []string{"", "https://example.test"} {
			t.Run(params+uri, func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				term.Write([]byte("\x1b]8;" + params + ";" + uri + "\x1b\\ABC"))
				term.Resize(16, 5, 8, 16)
				term.Write([]byte("D"))
				for x := 0; x < 4; x++ {
					a, b, ok := term.HyperlinkRange(x, 0)
					if !ok || a != 0 || b != 4 {
						t.Fatalf("col %d: %d..%d/%v", x, a, b, ok)
					}
				}
			})
		}
	}
}

package vt

import (
	"os"
	"strings"
	"testing"
)

func eraseImagesTerminal(t *testing.T) *Terminal {
	t.Helper()
	term, _ := graphicTerminal(t)
	if oshellABI < 6 || oshellABI > 62 {
		if os.Getenv("OSHELL_REQUIRE_WIDTH_EXTENSION") == "1" {
			t.Fatal("required OShell ED 2 extension absent")
		}
		t.Skip("requires ABI 6")
	}
	return term
}

// SwiftTerm's active buffer aliases altBuffer on the alternate screen. ED 2
// clears active+alt row images, not always both screens. Protocol cache sharing
// is a separate contract; these tests exercise the rendered row lifetimes.
func TestOShellEraseDisplayImageScope(t *testing.T) {
	packets := map[string][]byte{
		"kitty":  kittyCommand("a=T,f=24,s=1,v=1,i=1,c=1,r=1,C=1", []byte{255, 0, 0}),
		"sixel":  []byte("\x1bPq#1;2;100;0;0!2~\x1b\\"),
		"inline": append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 2, 6))...), '\a'),
	}
	for name, packet := range packets {
		t.Run(name+"/history", func(t *testing.T) {
			term := eraseImagesTerminal(t)
			term.Write(packet)
			term.Write([]byte("history-marker" + strings.Repeat("\r\n", 12)))
			term.ScrollToTop()
			if len(term.Graphics(10, 20)) != 1 {
				t.Fatal("missing history image before erase")
			}
			bar := term.Scrollbar()
			term.Write([]byte("\x1b[2J"))
			term.ScrollToTop()
			if len(term.Graphics(10, 20)) != 0 || len(term.graphicsCache) != 0 {
				t.Fatal("ED 2 retained history image")
			}
			if term.Scrollbar().Total != bar.Total || !strings.Contains(term.Text(), "history-marker") {
				t.Fatal("ED 2 changed text history")
			}
			term.ScrollToBottom()
			term.Write(packet)
			if len(term.Graphics(10, 20)) != 1 {
				t.Fatal("ED 2 disabled future graphics")
			}
		})
		t.Run(name+"/primary-clears-alternate", func(t *testing.T) {
			term := eraseImagesTerminal(t)
			term.Write([]byte("\x1b[?47h"))
			term.Write(packet)
			if len(term.Graphics(10, 20)) != 1 {
				t.Fatal("alternate image missing")
			}
			term.Write([]byte("\x1b[?47l\x1b[2J\x1b[?47h"))
			if len(term.Graphics(10, 20)) != 0 {
				t.Fatal("primary ED 2 retained alternate image")
			}
		})
		t.Run(name+"/alternate-preserves-primary", func(t *testing.T) {
			term := eraseImagesTerminal(t)
			term.Write(packet)
			term.Write([]byte("\x1b[?1049h"))
			term.Write(packet)
			term.Write([]byte("\x1b[2J"))
			if len(term.Graphics(10, 20)) != 0 {
				t.Fatal("alternate ED 2 retained active image")
			}
			term.Write([]byte("\x1b[?1049l"))
			if len(term.Graphics(10, 20)) != 1 {
				t.Fatal("alternate ED 2 or screen activation removed primary image")
			}
		})
		for _, erase := range []string{"\x1b[J", "\x1b[1J", "\x1b[K", "\x1b[1K", "\x1b[2K"} {
			t.Run(name+"/partial-"+erase, func(t *testing.T) {
				term := eraseImagesTerminal(t)
				term.Write(packet)
				term.Write([]byte("\x1b[H" + erase))
				if len(term.Graphics(10, 20)) != 1 {
					t.Fatal("partial text erase removed image")
				}
			})
		}
	}
}

func TestOShellEraseDisplayPromptDoesNotScroll(t *testing.T) {
	term := eraseImagesTerminal(t)
	term.Write([]byte("\x1b]133;A\aOShell > "))
	before := term.Scrollbar()
	term.Write([]byte("\x1b[H\x1b[2J"))
	if after := term.Scrollbar(); after.Total != before.Total {
		t.Fatalf("ED 2 scrolled prompt into history: before=%+v after=%+v", before, after)
	}
	term.ScrollToTop()
	if strings.Contains(term.Text(), "OShell >") {
		t.Fatal("prompt survived ED 2")
	}
}

package terminal

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestWordClickHigherCounts(t *testing.T) {
	for _, mods := range []ui.Modifiers{0, ui.Shift, ui.Ctrl, ui.Alt, ui.Super} {
		t.Run(fmt.Sprint(mods), func(t *testing.T) {
			term, tt := wordClickFixture(t, "/home/file.txt end", 500)
			x, y := cellCenter(term, 8, 0)
			for count := 1; count <= 8; count++ {
				tt.ClickAtWith(mods, x, y)
				if count < 3 {
					continue
				}
				want := "/home/file.txt end"
				if count == 4 && mods == 0 {
					want = "/home/file.txt"
				}
				if got := tt.Clipboard(); got != want {
					t.Fatalf("click %d copied %q, want %q", count, got, want)
				}
			}
		})
	}
}

func TestWordClickUnrelatedOutputKeepsSeed(t *testing.T) {
	term, tt := wordClickFixture(t, "/home/file.txt end", 500)
	for range 2 {
		clickWord(term, tt, 8, 0)
	}
	term.Feed([]byte("\x1b[2;1Hunrelated output"))
	tt.Frame()
	for range 2 {
		clickWord(term, tt, 8, 0)
	}
	if got := tt.Clipboard(); got != "/home/file.txt" {
		t.Fatalf("unchanged word after output copied %q", got)
	}
}

// Feed the same production word handler an explicit monotonic event clock.
// Clipboard/capture routing is separately covered by the real Tester clicks.
func timedWordClick(t *testing.T, term *Terminal, col, row int, mods ui.Modifiers, at time.Duration) string {
	t.Helper()
	v := term.v
	x, y := cellCenter(term, col, row)
	term.mu.Lock()
	defer term.mu.Unlock()
	ref, ok := term.term.CellAt(col, row)
	if !ok {
		t.Fatal("missing hit")
	}
	s, selected := v.wordPress(ref, ui.InputEvent{X: x, Y: y, Mods: mods}, float64(x*v.scale), float64(y*v.scale), uint64(at))
	if selected {
		term.term.SetSelection(&s)
	} else {
		term.term.SetSelection(nil)
	}
	v.gesture.Release(term.term, &ref)
	text, _ := term.term.SelectionText()
	return text
}

func timedWordFixture(t *testing.T) (*Terminal, *ui.Tester) {
	term, tt := wordClickFixture(t, "/home/file.txt end", 500)
	// Initialize the reusable native events, then clear only the host series.
	clickWord(term, tt, 8, 0)
	term.v.wordClicks = wordClickState{}
	return term, tt
}

func TestWordClickSeedPauseExpiryAndConsumption(t *testing.T) {
	for _, tc := range []struct {
		name  string
		pause time.Duration
		want  string
	}{
		{"short pause", 700 * time.Millisecond, "/home/file.txt"},
		{"inclusive deadline", 1400 * time.Millisecond, "/home/file.txt"},
		{"expired", 1400*time.Millisecond + time.Nanosecond, "file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, _ := timedWordFixture(t)
			timedWordClick(t, term, 8, 0, 0, time.Second)
			timedWordClick(t, term, 8, 0, 0, 1100*time.Millisecond)
			at := 1100*time.Millisecond + tc.pause
			timedWordClick(t, term, 8, 0, 0, at)
			if got := timedWordClick(t, term, 8, 0, 0, at+100*time.Millisecond); got != tc.want {
				t.Fatalf("selected %q want %q", got, tc.want)
			}
			if tc.want != "file" && term.v.wordClicks.seed != nil {
				t.Fatal("expanded word kept its seed")
			}
		})
	}
	term, _ := timedWordFixture(t)
	for _, ms := range []int{1000, 1100, 1800, 1900} {
		timedWordClick(t, term, 8, 0, 0, time.Duration(ms)*time.Millisecond)
	}
	timedWordClick(t, term, 8, 0, 0, 2600*time.Millisecond)
	if got := timedWordClick(t, term, 8, 0, 0, 2700*time.Millisecond); got != "file" {
		t.Fatal("third double click should start a new seed", got)
	}
}

func TestWordClickSeedIdentityAndFreshContent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Terminal, *ui.Tester)
		want   string
	}{
		{"unrelated output", func(term *Terminal, tt *ui.Tester) { term.Feed([]byte("\x1b[2;1Hother")); tt.Frame() }, "/home/file.txt"},
		{"same word rewrite", func(term *Terminal, tt *ui.Tester) { term.Feed([]byte("\x1b[1;7Hfile")); tt.Frame() }, "/home/file.txt"},
		{"changed selected text", func(term *Terminal, tt *ui.Tester) { term.Feed([]byte("\x1b[1;7Hname")); tt.Frame() }, "name"},
		{"changed settings", func(term *Terminal, tt *ui.Tester) { term.opts.ShiftWhitespace = false }, "file"},
		{"pixel-only resize", func(term *Terminal, tt *ui.Tester) { term.v.overlay.width++ }, "file"},
		{"main reset same text", func(term *Terminal, tt *ui.Tester) { term.Feed([]byte("\x1bc/home/file.txt end")); tt.Frame() }, "file"},
		{"screen switch same text", func(term *Terminal, tt *ui.Tester) {
			term.Feed([]byte("\x1b[?1049h\x1b[H/home/file.txt end"))
			tt.Frame()
		}, "file"},
		// MacTerminalView.bufferActivated cancels selection on both switches;
		// the next single click therefore clears the otherwise matching seed.
		{"screen round trip", func(term *Terminal, tt *ui.Tester) { term.Feed([]byte("\x1b[?1049h\x1b[?1049l")); tt.Frame() }, "file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, tt := timedWordFixture(t)
			timedWordClick(t, term, 8, 0, 0, time.Second)
			timedWordClick(t, term, 8, 0, 0, 1100*time.Millisecond)
			tc.mutate(term, tt)
			// A second double click, not a synthetic fourth press, tests seed reuse.
			timedWordClick(t, term, 8, 0, 0, 1800*time.Millisecond)
			if got := timedWordClick(t, term, 8, 0, 0, 1900*time.Millisecond); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestWordClickSeedNoFocusOrKeyReuse(t *testing.T) {
	for _, action := range []string{"copy key", "unhandled key", "focus"} {
		t.Run(action, func(t *testing.T) {
			term, tt := timedWordFixture(t)
			timedWordClick(t, term, 8, 0, 0, time.Second)
			timedWordClick(t, term, 8, 0, 0, 1100*time.Millisecond)
			switch action {
			case "copy key":
				term.v.keyDown(ui.InputEvent{Mods: ui.Ctrl | ui.Shift, Key: ui.KeyC})
			case "unhandled key":
				term.v.keyDown(ui.InputEvent{})
			case "focus":
				term.v.focused = false
				tt.Frame()
			}
			if term.v.wordClicks.seed != nil {
				t.Fatal("seed survived", action)
			}
		})
	}
}

func TestWordClickSeedRectangularTolerance(t *testing.T) {
	term, _ := timedWordFixture(t)
	timedWordClick(t, term, 8, 0, 0, time.Second)
	timedWordClick(t, term, 8, 0, 0, 1100*time.Millisecond)
	s := term.v.wordClicks.seed
	for _, tc := range []struct {
		dx, dy float32
		want   bool
	}{{4, 4, true}, {-4, -4, true}, {4.01, 0, false}, {0, -4.01, false}} {
		if got := term.v.continuesWordClick(s.x+tc.dx, s.y+tc.dy, 1200*uint64(time.Millisecond), s.buffer); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	// The viewport is part of identity, independently of output revisions.
	term.Feed([]byte(strings.Repeat("\r\n", 50)))
	if term.v.continuesWordClick(s.x, s.y, 1200*uint64(time.Millisecond), s.buffer) {
		t.Fatal("scroll row ignored")
	}
}

func TestWordClickSeedTextLimit(t *testing.T) {
	for _, size := range []int{65536, 65537} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			term, tt := wordClickFixture(t, strings.Repeat("a", size)+" end", 2400)
			clickWord(term, tt, 8, 0)
			term.v.wordClicks = wordClickState{}
			timedWordClick(t, term, 8, 0, 0, time.Second)
			text := timedWordClick(t, term, 8, 0, 0, 1100*time.Millisecond)
			if len(text) != size {
				t.Fatalf("selected %d bytes want %d", len(text), size)
			}
			if got := term.v.wordClicks.seed != nil; got != (size <= 65536) {
				t.Fatalf("seed present=%v for %d bytes", got, size)
			}
		})
	}
}

func TestWordClickExpandedDragKeepsWordAnchor(t *testing.T) {
	term, tt := wordClickFixture(t, "/home/file.txt /etc/name.conf", 500)
	for range 3 {
		clickWord(term, tt, 8, 0)
	}
	x, y := cellCenter(term, 8, 0)
	tt.Press(x, y) // the fourth press widens and leaves the word gesture active
	if got := term.SearchSelection(); got != "/home/file.txt" {
		t.Fatal(got)
	}
	x, y = cellCenter(term, 21, 0)
	tt.Move(x, y)
	tt.Release(x, y)
	if got := tt.Clipboard(); got != "/home/file.txt /etc/name.conf" {
		t.Fatal(got)
	}
}

func TestWordClickMenuCommandsKeepSeed(t *testing.T) {
	for _, command := range []string{"copy", "selectAll", "paste"} {
		t.Run(command, func(t *testing.T) {
			term, _ := timedWordFixture(t)
			timedWordClick(t, term, 8, 0, 0, time.Second)
			timedWordClick(t, term, 8, 0, 0, 1100*time.Millisecond)
			term.opts.BeforePaste = func(string) bool { return false }
			term.v.services.WriteClipboard("cancelled paste")
			term.v.input(ui.InputEvent{Kind: ui.InputCommand, Text: command})
			if term.v.wordClicks.seed == nil {
				t.Fatal("menu command cleared word seed")
			}
			timedWordClick(t, term, 8, 0, 0, 1800*time.Millisecond)
			if got := timedWordClick(t, term, 8, 0, 0, 1900*time.Millisecond); got != "/home/file.txt" {
				t.Fatal(got)
			}
		})
	}
}

func TestWordClickRightPasteClearsSeedBeforeImageHook(t *testing.T) {
	term, tt := timedWordFixture(t)
	timedWordClick(t, term, 8, 0, 0, time.Second)
	timedWordClick(t, term, 8, 0, 0, 1100*time.Millisecond)
	term.opts.RightClickPaste = true
	term.opts.BeforeClipboardPaste = func() bool { return true }
	x, y := cellCenter(term, 8, 0)
	tt.RightClickAt(x, y)
	if term.v.wordClicks.seed != nil {
		t.Fatal("right click image hook retained word seed")
	}
}

func TestWordClickSeedCanonicalTextEquality(t *testing.T) {
	term, tt := wordClickFixture(t, "/home/é.txt end", 500)
	clickWord(term, tt, 6, 0)
	term.v.wordClicks = wordClickState{}
	timedWordClick(t, term, 6, 0, 0, time.Second)
	if got := timedWordClick(t, term, 6, 0, 0, 1100*time.Millisecond); got != "é" {
		t.Fatal(got)
	}
	term.Feed([]byte("\x1b[1;7He\u0301"))
	tt.Frame()
	timedWordClick(t, term, 6, 0, 0, 1800*time.Millisecond)
	if got := timedWordClick(t, term, 6, 0, 0, 1900*time.Millisecond); got != "/home/e\u0301.txt" {
		t.Fatalf("canonical word failed to expand: %q", got)
	}
}

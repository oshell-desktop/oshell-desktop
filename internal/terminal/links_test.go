package terminal

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTerminalLinkSchemesAndModifierAtRelease(t *testing.T) {
	for _, tc := range []struct {
		link    string
		allowed bool
	}{
		{"https://example.test/a", true}, {"http://example.test/", true},
		{"HTTPS://example.test/", true}, {"mailto:person@example.test", true},
		{"file:///C:/example.txt", false}, {"ftp://example.test/", false},
		{"ssh://example.test/", false}, {"custom:open", false},
		{"javascript:alert(1)", false}, {"data:text/plain,hello", false},
		{"/relative/path", false},
	} {
		for _, modifiers := range [][2]ui.Modifiers{{0, 0}, {ui.Cmd, 0}, {0, ui.Cmd}, {ui.Cmd, ui.Cmd}} {
			t.Run(fmt.Sprintf("%s/%v", tc.link, modifiers), func(t *testing.T) {
				term, tt, _ := bidiViewFixture(t)
				term.Feed([]byte("\x1b]8;;" + tc.link + "\x1b\\label\x1b]8;;\x1b\\"))
				tt.Frame()
				x, y := cellCenter(term, 1, 0)
				term.v.input(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y, Mods: modifiers[0]})
				term.v.input(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y, Mods: modifiers[1]})
				want := 0
				if tc.allowed && modifiers[1]&ui.Cmd != 0 {
					want = 1
				}
				if got := tt.OpenedURLs(); len(got) != want || want == 1 && got[0] != tc.link {
					t.Fatalf("opened %v", got)
				}
			})
		}
	}
}

func TestTerminalLinkBidiWideCellsAndMouseTracking(t *testing.T) {
	for _, tc := range []struct {
		text          string
		logical, tail int
	}{
		{"界", 0, 0}, {"界", 0, 1}, {"אב", 0, 0}, {"אב界", 2, 0}, {"אב界", 2, 1}, {"א\u05b0ב", 0, 0},
	} {
		for _, tracking := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%d/%d/%v", tc.text, tc.logical, tc.tail, tracking), func(t *testing.T) {
				term, tt, _ := bidiViewFixture(t)
				term.Feed([]byte("\x1b]8;;https://example.test/target\x1b\\" + tc.text + "\x1b]8;;\x1b\\"))
				if tracking {
					term.Feed([]byte("\x1b[?1000h\x1b[?1006h"))
				}
				tt.Frame()
				x, y := cellCenter(term, term.v.visualColumn(0, tc.logical)+tc.tail, 0)
				tt.ClickAtWith(ui.Cmd, x, y)
				if got := tt.OpenedURLs(); len(got) != 1 || got[0] != "https://example.test/target" {
					t.Fatalf("visual link: %v", got)
				}
				if term.v.reporting || term.v.selecting || term.v.ticking {
					t.Fatal("link release retained pointer gesture")
				}
			})
		}
	}
}

func TestTerminalLinkReleaseUsesCurrentOutputAndCancelsOnBlur(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	write := func(target string) {
		term.Feed([]byte("\x1b[H\x1b]8;;https://example.test/" + target + "\x1b\\label\x1b]8;;\x1b\\"))
	}
	write("old")
	tt.Frame()
	x, y := cellCenter(term, 1, 0)
	press := ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y, Mods: ui.Cmd}
	release := ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y, Mods: ui.Cmd}
	term.v.input(press)
	write("new") // Resolve again at release, before another paint.
	term.v.input(release)
	if got := tt.OpenedURLs(); len(got) != 1 || got[0] != "https://example.test/new" {
		t.Fatalf("stale link: %v", got)
	}
	term.v.input(press)
	tt.SetFocused(false)
	term.v.input(release)
	if len(tt.OpenedURLs()) != 1 {
		t.Fatal("blurred press opened a link")
	}
}

func TestTerminalLinkActivationOnRelease(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte("\x1b]8;;https://example.test/\x1b\\link\x1b]8;;\x1b\\"))
	tt.Frame()
	x, y := cellCenter(term, 1, 0)
	term.v.input(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y, Mods: ui.Cmd})
	if got := tt.OpenedURLs(); len(got) != 0 {
		t.Fatalf("opened on press: %v", got)
	}
	term.v.input(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y, Mods: ui.Cmd})
	if got := tt.OpenedURLs(); len(got) != 1 || got[0] != "https://example.test/" {
		t.Fatalf("release: %v", got)
	}
}

func TestTerminalLinkDraggingDoesNotOpen(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte("https://example.test/"))
	tt.Frame()
	x, y := cellCenter(term, 2, 0)
	term.v.input(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y, Mods: ui.Cmd})
	term.v.input(ui.InputEvent{Kind: ui.InputPointerMove, X: x + 1, Y: y, Mods: ui.Cmd}) // Same cell.
	term.v.input(ui.InputEvent{Kind: ui.InputPointerUp, X: x + 1, Y: y, Mods: ui.Cmd})
	if got := tt.OpenedURLs(); len(got) != 0 {
		t.Fatalf("drag opened link: %v", got)
	}
}

func TestURLAt(t *testing.T) {
	row := "see https://example.com/a_(b)?q=1. and (http://x.org/y), mailto:me@x.org!"
	term, _, _ := bidiViewFixture(t)
	term.Resize(100, 6)
	term.Feed([]byte(row))
	term.mu.Lock()
	defer term.mu.Unlock()
	for col, want := range map[int]string{
		0:  "",
		4:  "https://example.com/a_(b)?q=1",
		20: "https://example.com/a_(b)?q=1",
		33: "", // the period after it
		40: "http://x.org/y",
		58: "mailto:me@x.org!",
	} {
		if got := term.v.linkAt(col, 0); got != want {
			t.Errorf("linkAt(%d) = %q, want %q", col, got, want)
		}
	}
}

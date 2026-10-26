package terminal

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

// MacTerminalView.mouseUp refreshes the highlight before deciding whether
// the gesture can open a link. It does not call previewUrl.
func TestLinkReleaseRefreshesStationaryHighlight(t *testing.T) {
	for _, replacement := range []string{
		"\x1b]8;;https://new.test/\x1b\\new\x1b]8;;\x1b\\",
		"plain text",
	} {
		t.Run(replacement, func(t *testing.T) {
			term, tt, _ := bidiViewFixture(t)
			term.Feed([]byte("\x1b]8;;https://old.test/\x1b\\long label\x1b]8;;\x1b\\"))
			tt.Frame()
			x, y := cellCenter(term, 1, 0)
			tt.Move(x, y)
			tt.HoldModifiers(ui.Cmd)
			old := slices.Clone(term.v.linkHover.match.ranges)
			term.v.input(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y, Mods: ui.Cmd})
			term.Feed([]byte("\x1b[H\x1b[2K" + replacement))
			tt.Frame()
			if !slices.Equal(term.v.linkHover.match.ranges, old) {
				t.Fatal("output alone changed hover")
			}
			term.v.input(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y, Mods: ui.Cmd})
			tt.Frame()
			var want []linkRowRange
			if replacement != "plain text" {
				want = []linkRowRange{{0, 0, 3}}
			}
			if got := term.v.linkHover.match.ranges; !slices.Equal(got, want) {
				t.Fatalf("released highlight %v, want %v", got, want)
			}
			if got := term.v.linkHover.preview; got != "https://old.test/" {
				t.Fatalf("release refreshed preview to %q", got)
			}
			if replacement == "plain text" && len(term.v.lines[0].decos) != 0 {
				t.Fatal("old underline still painted")
			}
		})
	}
}

func TestLinkReleaseAfterDragDoesNotRefreshPreview(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte("\x1b]8;;https://old.test/\x1b\\first\x1b]8;;\x1b\\ \x1b]8;;https://new.test/\x1b\\second\x1b]8;;\x1b\\"))
	tt.Frame()
	x, y := cellCenter(term, 1, 0)
	tt.Move(x, y)
	tt.HoldModifiers(ui.Cmd)
	term.v.input(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y, Mods: ui.Cmd})
	x, y = cellCenter(term, 8, 0)
	term.v.input(ui.InputEvent{Kind: ui.InputPointerMove, X: x, Y: y, Mods: ui.Cmd})
	term.mu.Lock()
	term.v.requestLinkHover(x, y, true)
	term.v.refreshLinkHover(false)
	term.mu.Unlock()
	if got := term.v.linkHover.match.ranges; !slices.Equal(got, []linkRowRange{{0, 0, 5}}) {
		t.Fatalf("drag changed hover: %v", got)
	}
	term.v.input(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y, Mods: ui.Cmd})
	term.mu.Lock()
	term.v.refreshLinkHover(false)
	term.mu.Unlock()
	if got := term.v.linkHover.match.ranges; !slices.Equal(got, []linkRowRange{{0, 6, 12}}) {
		t.Fatalf("release failed to highlight drag endpoint: %v", got)
	}
	if got := term.v.linkHover.preview; got != "https://old.test/" {
		t.Fatalf("drag release refreshed preview: %q", got)
	}
	if got := tt.OpenedURLs(); len(got) != 0 {
		t.Fatalf("drag opened a URL: %v", got)
	}
	// A later mouse-moved event really does refresh the explicit preview.
	x, y = cellCenter(term, 9, 0)
	tt.Move(x, y)
	tt.HoldModifiers(ui.Cmd)
	if got := term.v.linkHover.preview; got != "https://new.test/" {
		t.Fatalf("subsequent movement did not refresh preview: %q", got)
	}
}

func TestLinkReleaseDuringSynchronizedOutput(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte("\x1b]8;;https://old.test/\x1b\\long label\x1b]8;;\x1b\\"))
	tt.Frame()
	x, y := cellCenter(term, 1, 0)
	tt.Move(x, y)
	tt.HoldModifiers(ui.Cmd)
	term.Feed([]byte("\x1b[?2026h\x1b[H\x1b[2K\x1b]8;;https://new.test/\x1b\\new\x1b]8;;\x1b\\"))
	// Even a release without a locally retained press updates the hover;
	// it must not activate the link. Painting waits for synchronized output.
	term.v.input(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y, Mods: ui.Cmd})
	tt.Frame()
	if got := term.v.linkHover.match.ranges; !slices.Equal(got, []linkRowRange{{0, 0, 10}}) {
		t.Fatalf("held frame changed highlight: %v", got)
	}
	term.Feed([]byte("\x1b[?2026l"))
	tt.Frame()
	if got := term.v.linkHover.match.ranges; !slices.Equal(got, []linkRowRange{{0, 0, 3}}) {
		t.Fatalf("completed frame retained old highlight: %v", got)
	}
	if got := term.v.linkHover.preview; got != "https://old.test/" {
		t.Fatalf("release changed preview: %q", got)
	}
	if got := tt.OpenedURLs(); len(got) != 0 {
		t.Fatal("release without press opened link", got)
	}
	tt.HoldModifiers(0)
	if len(term.v.linkHover.match.ranges) != 0 || term.v.linkHover.previewSet {
		t.Fatal("modifier release retained highlight or preview")
	}
}

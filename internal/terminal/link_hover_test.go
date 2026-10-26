package terminal

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/terminal/internal/vt"
)

func TestLinkHoverModifierRangesAndStyle(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte("\x1b[4:3;58:2::255:0:0mhttps://example.test/\x1b[0m"))
	tt.Frame()
	x, y := cellCenter(term, 3, 0)
	tt.Move(x, y)
	if len(term.v.linkHover.match.ranges) != 0 {
		t.Fatal("unmodified hover highlighted link")
	}
	tt.HoldModifiers(ui.Cmd)
	want := []linkRowRange{{0, 0, 21}}
	if got := term.v.linkHover.match.ranges; !slices.Equal(got, want) {
		t.Fatalf("ranges %v want %v", got, want)
	}
	if term.v.linkHover.preview != "" {
		t.Fatal("implicit URL showed explicit payload preview")
	}
	decos := term.v.lines[0].decos
	if len(decos) != 1 || decos[0].kind != vt.UnderlineDashed || decos[0].c != color(term.v.colors.Foreground) {
		t.Fatalf("hover decorations %+v", decos)
	}
	tt.HoldModifiers(0)
	decos = term.v.lines[0].decos
	if len(decos) != 1 || decos[0].kind != vt.UnderlineCurly || decos[0].c.R < 230 || decos[0].c.G > 25 {
		t.Fatalf("original underline not restored: %+v", decos)
	}
	tt.HoldModifiers(ui.Cmd)
	tt.Move(-1, -1)
	if len(term.v.linkHover.match.ranges) != 0 {
		t.Fatal("pointer exit retained underline")
	}
	tt.Move(x, y)
	tt.HoldModifiers(ui.Cmd)
	tt.SetFocused(false)
	if len(term.v.linkHover.match.ranges) != 0 || term.v.linkHover.preview != "" {
		t.Fatal("blur retained link hover")
	}
}

func TestLinkHoverExplicitWriteAndSemicolon(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	start := "\x1b]8;id=same;https://example.test/a;b\x1b\\"
	term.Feed([]byte(start + "first" + start + "second\x1b]8;;\x1b\\ plain"))
	tt.Frame()
	x, y := cellCenter(term, 2, 0)
	tt.Move(x, y)
	tt.HoldModifiers(ui.Cmd)
	if got := term.v.linkHover.match.ranges; !slices.Equal(got, []linkRowRange{{0, 0, 5}}) {
		t.Fatalf("first write: %v", got)
	}
	if got := term.v.linkHover.preview; got != "https://example.test/a" {
		t.Fatalf("preview %q", got)
	}
	save(t, tt, "link-hover")
	tt.ClickAtWith(ui.Cmd, x, y)
	if got := tt.OpenedURLs(); len(got) != 1 || got[0] != "https://example.test/a" {
		t.Fatalf("explicit click %q", got)
	}
	x, y = cellCenter(term, 8, 0)
	tt.Move(x, y)
	tt.HoldModifiers(ui.Cmd)
	if got := term.v.linkHover.match.ranges; !slices.Equal(got, []linkRowRange{{0, 5, 11}}) {
		t.Fatalf("second write: %v", got)
	}
	tt.HoldModifiers(0)
	if term.v.linkHover.preview != "" {
		t.Fatal("modifier release retained preview")
	}
}

func TestLinkHoverWrappedAndBidi(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	url := "https://example.test/" + strings.Repeat("a", term.v.cols+7)
	term.Feed([]byte(url))
	tt.Frame()
	x, y := cellCenter(term, 2, 1)
	tt.Move(x, y)
	tt.HoldModifiers(ui.Cmd)
	if got := term.v.linkHover.match; got.text != url || len(got.ranges) != 2 {
		t.Fatalf("wrapped hover %+v", got)
	}
	for row := 0; row < 2; row++ {
		if len(term.v.lines[row].decos) == 0 {
			t.Fatalf("wrapped row %d missing underline", row)
		}
	}
	term.Feed([]byte("\x1b[2J\x1b[Hאב https://界.test/"))
	tt.Frame()
	x, y = cellCenter(term, term.v.visualColumn(0, 11)+1, 0)
	tt.Move(x, y)
	tt.HoldModifiers(ui.Cmd)
	if got := term.v.linkHover.match; got.text != "https://界.test/" {
		t.Fatalf("wide BiDi hover %+v", got)
	}
	for _, d := range term.v.lines[0].decos {
		if d.kind != vt.UnderlineDashed {
			t.Fatal("BiDi underline style", d)
		}
	}
}

func TestLinkHoverPlainCellRetainsExplicitPreview(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte("\x1b]8;;https://example.test/\x1b\\label\x1b]8;;\x1b\\ text"))
	tt.Frame()
	term.mu.Lock()
	x, y := cellCenter(term, 1, 0)
	term.v.requestLinkHover(x, y, true)
	term.v.refreshLinkHover(false)
	x, y = cellCenter(term, 8, 0)
	term.v.requestLinkHover(x, y, true)
	term.v.refreshLinkHover(false)
	term.mu.Unlock()
	if len(term.v.linkHover.match.ranges) != 0 || term.v.linkHover.preview != "https://example.test/" {
		t.Fatal("plain cell preview lifetime differs from source")
	}
}

func TestLinkHoverHiddenViewReentry(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte("\x1b]8;;https://old.test/\x1b\\old\x1b]8;;\x1b\\"))
	tt.Frame()
	x, y := cellCenter(term, 1, 0)
	tt.Move(x, y)
	tt.HoldModifiers(ui.Cmd)
	if term.v.linkHover.preview != "https://old.test/" {
		t.Fatal("missing initial preview")
	}
	// Hidden tabs need not build a frame, so use the visibility epoch rather
	// than expecting a focus transition through View.build.
	term.v.input(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y, Mods: ui.Cmd})
	term.SetVisible(false)
	term.Feed([]byte("\x1b[2J\x1b[Hhttps://new.test/"))
	term.SetVisible(true)
	tt.Frame()
	if term.v.linkHover.preview != "" || term.v.linkHover.match.text != "https://new.test/" {
		t.Fatalf("stale hidden hover: %+v", term.v.linkHover)
	}
	term.v.input(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y, Mods: ui.Cmd})
	if got := tt.OpenedURLs(); len(got) != 0 {
		t.Fatal("hidden press later opened a link", got)
	}
}

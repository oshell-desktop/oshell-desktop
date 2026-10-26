package vt

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"oshell-desktop/internal/terminal/internal/bidilayout"
)

func TestOShellBidiPresentationWholeParagraph(t *testing.T) {
	term, _ := bidiTerminal(t, Effects{})
	if err := term.Resize(6, 3, 8, 16); err != nil {
		t.Fatal(err)
	}
	term.Write([]byte("A.....אב...Z\r\nnew\r\nlast"))
	bar := term.Scrollbar()
	if bar.Offset == 0 {
		t.Fatal("fixture has no history")
	}
	whole := term.BidiRows(0, 2)
	part := term.BidiRows(1, 1)
	if len(part) != 1 || part[0].RTL || len(part[0].Row.Cells) == 0 {
		t.Fatal("history paragraph context lost", part)
	}
	if !slices.Equal(whole[1].Row.Cells, part[0].Row.Cells) {
		t.Fatal("partial viewport changed paragraph order")
	}
	if term.Scrollbar() != bar {
		t.Fatal("layout moved the viewport")
	}
	term.ScrollToTop()
	if got := term.BidiRows(0, 2); !slices.Equal(got[1].Row.Cells, part[0].Row.Cells) {
		t.Fatal("scroll changed layout")
	}
}

func TestOShellBidiBulkRowOwnership(t *testing.T) {
	term, _ := bidiTerminal(t, Effects{})
	if oshellABI < 31 {
		t.Skip("requires bulk row ABI 31")
	}
	term.Write([]byte("\x1b[2 kאב"))
	raw, cells, ok := term.bidiRawRow(0)
	if !ok || len(cells) != 12 || !rawBidiState(raw).RTL || rune(cell.codepoint.of(cells[0])) != 'א' {
		t.Fatal("bulk row", ok, len(cells), raw)
	}
	term.Write([]byte("\x1b[HA\x1b[1 k"))
	if rune(cell.codepoint.of(cells[0])) != 'א' || !rawBidiState(raw).RTL {
		t.Fatal("owned snapshot changed after native output")
	}
	if _, _, ok := term.bidiRawRow(10000); ok {
		t.Fatal("out-of-range row accepted")
	}
}

func TestOShellBidiContentCacheValidation(t *testing.T) {
	term, _ := bidiTerminal(t, Effects{})
	rows := [][]bidilayout.Cell{{{Column: 0, Width: 1, Text: "א"}, {Column: 1, Width: 1, Text: "ב"}}}
	first, ok := term.bidiParagraph(rows, 12, bidilayout.Default)
	if !ok || len(term.bidiCache) != 1 {
		t.Fatal("cache not populated")
	}
	second, _ := term.bidiParagraph(rows, 12, bidilayout.Default)
	if &first.Rows[0].Cells[0] != &second.Rows[0].Cells[0] {
		t.Fatal("identical paragraph did not reuse layout")
	}
	// Force the hash-key collision path, as Swift's tests do: a hash hit is
	// never enough to accept a different paragraph's stored result.
	for key, entry := range term.bidiCache {
		entry.rows[0][0].Text = "wrong"
		entry.layout.RTL = false
		term.bidiCache[key] = entry
	}
	third, _ := term.bidiParagraph(rows, 12, bidilayout.Default)
	if !third.RTL || &first.Rows[0].Cells[0] == &third.Rows[0].Cells[0] {
		t.Fatal("hash hit bypassed exact content comparison")
	}
	if rows[0][0].Text != "א" {
		t.Fatal("cache mutated caller-owned cells")
	}
	rows[0][0].Text = "a"
	fourth, _ := term.bidiParagraph(rows, 12, bidilayout.Default)
	if fourth.RTL {
		t.Fatal("changed first letter reused stale direction")
	}
	term.SetBidiEnabled(false)
	if len(term.bidiCache) != 0 || term.bidiCacheColumns != 0 {
		t.Fatal("disabled bridge retained cache")
	}
}

func TestOShellBidiPresentationWideGraphemeAndPadding(t *testing.T) {
	term, _ := bidiTerminal(t, Effects{})
	term.Write([]byte("א\u05b0ב界 "))
	cells := term.bidiCells(0, 12)
	if len(cells) != 4 || cells[0].Text != "א\u05b0" || cells[2].Width != 2 || cells[3].Text != " " {
		t.Fatal("grapheme geometry", cells)
	}
	row := term.BidiRows(0, 1)[0].Row
	if row.LogicalToVisual[0] != 11 || row.LogicalToVisual[2] != 8 || row.VisualToLogical[8] != 2 || row.VisualToLogical[9] != 2 {
		t.Fatal("wide map", row)
	}
}

func TestOShellBidiPresentationOversizedAndDisabled(t *testing.T) {
	term, _ := bidiTerminal(t, Effects{})
	term.Write([]byte("\x1b[2 kא" + strings.Repeat("x", 12*501)))
	bar := term.Scrollbar()
	for _, row := range term.BidiRows(int(bar.Offset), int(bar.Len)) {
		if len(row.Row.Cells) != 0 || !row.RTL {
			t.Fatal("oversized paragraph must retain logical presentation", row)
		}
	}
	term.SetBidiEnabled(false)
	if rows := term.BidiRows(0, 5); rows != nil {
		t.Fatal("disabled bridge returned presentation")
	}
}

func bidiTerminal(t *testing.T, effects Effects) (*Terminal, *RenderState) {
	t.Helper()
	term, rs := resetTerminal(t, effects)
	if !term.BidiEnabled() {
		t.Fatal("normal terminal creation did not enable BiDi")
	}
	return term, rs
}

func TestOShellBidiProtocolState(t *testing.T) {
	for _, chunk := range []int{1, 7, 4096} {
		var reply bytes.Buffer
		term, _ := bidiTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }})
		write := func(s string) { writeControlChunks(term, []byte(s), chunk) }
		write("\x1b[8$p\x1b[?2500$p\x1b[?2501$p\x1b[?1243$p")
		if reply.String() != "\x1b[8;1$y\x1b[?2500;2$y\x1b[?2501;1$y\x1b[?1243;2$y" {
			t.Fatal(reply.String())
		}
		write("\x1b[?2500;2501;1243s\x1b[8l\x1b[?2500;1243h\x1b[?2501l\x1b[2;99 k")
		if state, arrows := term.BidiState(); state != (bidilayout.State{Explicit: true, RTL: true, MirrorBoxes: true}) || !arrows {
			t.Fatal(state, arrows)
		}
		write("\x1b[?2500;2501;1243r")
		if state, arrows := term.BidiState(); state != (bidilayout.State{Explicit: true, Auto: true, RTL: true}) || arrows {
			t.Fatal(state, arrows)
		}
		write("\x1b[8;20h\x1b[0 S")
		if state, _ := term.BidiState(); state != bidilayout.Default || !term.Mode(0x8000|20) {
			t.Fatal("mixed mode list lost state", state)
		}
		write("\x1b[3 S\x1b[1 k\x1b[2 k\x1b[0 k\x1b[99 k\x1b[1 S")
		if state, _ := term.BidiState(); state != bidilayout.Default {
			t.Fatal("direction parameter defaults", state)
		}
		write("\x1b[?1243h\x1b[?1243s\x1b[!p\x1b[?1243r")
		if state, arrows := term.BidiState(); state != bidilayout.Default || arrows {
			t.Fatal("DECSTR retained saved modes", state, arrows)
		}
		reply.Reset()
		write("\x1b G\x1b[8$p\x1b[?2501$p")
		if reply.String() != "\x9b8;1$y\x9b?2501;1$y" {
			t.Fatal(reply.String())
		}
	}
}

func TestOShellBidiParagraphPropertyUpdates(t *testing.T) {
	term, rs := bidiTerminal(t, Effects{})
	term.Write([]byte("abc\x1b[2 k"))
	if term.BidiRowAt(0) != bidilayout.Default {
		t.Fatal("mid-paragraph changed existing text")
	}
	term.Write([]byte("\r\n\x1b[?2500habcdefghijklm\x1b[?2501l"))
	want := bidilayout.State{Auto: true, RTL: true, MirrorBoxes: true}
	if term.BidiRowAt(1) != want || term.BidiRowAt(2) != want {
		t.Fatal("wrap did not inherit paragraph state", term.BidiRowAt(1), term.BidiRowAt(2))
	}
	term.Write([]byte("\x1b[2;1H\x1b[8l"))
	want.Explicit = true
	if term.BidiRowAt(1) != want || term.BidiRowAt(2) != want {
		t.Fatal("property update lost old paragraph's auto mode")
	}
	if state, _ := term.BidiState(); state.Auto {
		t.Fatal("global default failed to retain mode change")
	}
	if err := rs.Update(term); err != nil {
		t.Fatal(err)
	}
	rs.Rows()
	rs.NextRow()
	rs.NextRow()
	if rs.RowBidiState() != want {
		t.Fatal("render snapshot lost state", rs.RowBidiState())
	}
	term.Write([]byte("\x1b[3;1H\x1b[1 k")) // continuation's column zero is not paragraph start
	if term.BidiRowAt(1) != want || term.BidiRowAt(2) != want {
		t.Fatal("continuation changed existing paragraph")
	}
}

func TestOShellBidiReflowAndHistory(t *testing.T) {
	term, _ := bidiTerminal(t, Effects{})
	term.Write([]byte("\x1b[2 kabcdefghijklmnopqrstuvwxY\r\n\x1b[1 kLTR"))
	rtl := bidilayout.State{Auto: true, RTL: true}
	if err := term.Resize(8, 5, 8, 16); err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 4; y++ {
		if term.BidiRowAt(y) != rtl {
			t.Fatalf("reflow row %d=%+v", y, term.BidiRowAt(y))
		}
	}
	if term.BidiRowAt(4) != bidilayout.Default {
		t.Fatal("reflow merged next paragraph")
	}
	term.Write([]byte("\r\nnew\r\nmore\r\nlast"))
	term.ScrollToTop()
	if term.BidiRowAt(0) != rtl {
		t.Fatal("scrollback forgot paragraph state")
	}
}

func TestOShellBidiEraseAndLineMoves(t *testing.T) {
	term, _ := bidiTerminal(t, Effects{})
	term.Write([]byte("\x1b[2 kRTL\r\n\x1b[1 kLTR\x1b[1;1H\x1b[L"))
	if term.BidiRowAt(0) != bidilayout.Default || !term.BidiRowAt(1).RTL || term.BidiRowAt(2).RTL {
		t.Fatal("inserted/moved row state", term.BidiRowAt(0), term.BidiRowAt(1), term.BidiRowAt(2))
	}
	term.Write([]byte("\x1b[M"))
	if !term.BidiRowAt(0).RTL || term.BidiRowAt(1).RTL {
		t.Fatal("deleted row state")
	}
	term.Write([]byte("\x1b[1;2H\x1b[?2500h\x1b[K"))
	if term.BidiRowAt(0).MirrorBoxes {
		t.Fatal("partial erase replaced paragraph state")
	}
	term.Write([]byte("\x1b[2J"))
	want, _ := term.BidiState()
	for y := 0; y < 5; y++ {
		if term.BidiRowAt(y) != want {
			t.Fatal("ED2 row default", y, term.BidiRowAt(y), want)
		}
	}
}

func TestOShellBidiDisabledBoundary(t *testing.T) {
	var reply bytes.Buffer
	term, _ := resetTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }})
	if !term.SetBidiEnabled(false) {
		t.Fatal("could not disable legacy-host bridge")
	}
	term.Write([]byte("\x1b[8l\x1b[2 k\x1b[8$p\x1b[?2501$p"))
	if reply.String() != "\x1b[8;0$y\x1b[?2501;0$y" {
		t.Fatal("disabled bridge advertised support", reply.String())
	}
	if state, _ := term.BidiState(); state != bidilayout.Default {
		t.Fatal("disabled state changed", state)
	}
}

func TestOShellBidiResizeNewRows(t *testing.T) {
	term, _ := bidiTerminal(t, Effects{})
	term.Write([]byte("old\x1b[2 k\x1b[?2500h"))
	want, _ := term.BidiState()
	if err := term.Resize(12, 8, 8, 16); err != nil {
		t.Fatal(err)
	}
	if term.BidiRowAt(0) != bidilayout.Default {
		t.Fatal("resizing changed existing paragraph")
	}
	for y := 5; y < 8; y++ {
		if got := term.BidiRowAt(y); got != want {
			t.Fatalf("new row %d = %+v, want %+v", y, got, want)
		}
	}
	term.Write([]byte("\x1b[?47h"))
	if err := term.Resize(12, 10, 8, 16); err != nil {
		t.Fatal(err)
	}
	for y := 8; y < 10; y++ {
		if got := term.BidiRowAt(y); got != want {
			t.Fatalf("alternate new row %d = %+v, want %+v", y, got, want)
		}
	}
	term.Write([]byte("\x1b[?47l"))
	for y := 8; y < 10; y++ {
		if got := term.BidiRowAt(y); got != want {
			t.Fatalf("inactive primary new row %d = %+v, want %+v", y, got, want)
		}
	}
}

func TestOShellBidiResetAndAlternate(t *testing.T) {
	term, _ := bidiTerminal(t, Effects{})
	term.Write([]byte("\x1b[2 kRTL\x1b[!p"))
	if state, arrows := term.BidiState(); state != bidilayout.Default || arrows || !term.BidiRowAt(0).RTL {
		t.Fatal("soft reset changed existing paragraph", state, arrows, term.BidiRowAt(0))
	}
	term.Write([]byte("\r\x1b[!p"))
	if term.BidiRowAt(0) != bidilayout.Default {
		t.Fatal("reset at paragraph start did not update state")
	}
	term.Write([]byte("\x1b[2 k\x1b[?1049h\x1b[2JALT\x1b[?1049l\x1b[?47h"))
	if !term.BidiRowAt(0).RTL {
		t.Fatal("cleared alternate lost current default")
	}
	term.Write([]byte("\x1bc"))
	if state, _ := term.BidiState(); state != bidilayout.Default {
		t.Fatal("RIS did not reset global state", state)
	}
	for y := 0; y < 5; y++ {
		if term.BidiRowAt(y) != bidilayout.Default {
			t.Fatal("RIS retained row state", y)
		}
	}
}

func TestOShellBidiBlockedLineFeed(t *testing.T) {
	term, _ := bidiTerminal(t, Effects{})
	// At the bottom margin, LF outside the horizontal margins does not
	// advance. It must not replace the current paragraph's old direction.
	term.Write([]byte("\x1b[2;3r\x1b[?69h\x1b[3;8s\x1b[3;2H\x1b[2 k\n"))
	if term.BidiRowAt(2) != bidilayout.Default {
		t.Fatal("blocked LF changed paragraph direction", term.BidiRowAt(2))
	}
}

func TestOShellBidiNarrowReverseIndex(t *testing.T) {
	term, _ := bidiTerminal(t, Effects{})
	term.Write([]byte("\x1b[2 kRTL\r\n\x1b[1 kLTR\x1b[?69h\x1b[3;8s\x1b[1;3H\x1bM"))
	if !term.BidiRowAt(0).RTL || term.BidiRowAt(1).RTL {
		t.Fatal("narrow reverse scroll changed row states", term.BidiRowAt(0), term.BidiRowAt(1))
	}
}

func TestOShellBidiPartialEditsAndForwardScroll(t *testing.T) {
	for _, edit := range []string{"\x1b[L", "\x1b[M"} {
		term, _ := bidiTerminal(t, Effects{})
		term.Write([]byte("\x1b[2 kRTL\r\n\x1b[1 kLTR\x1b[?69h\x1b[3;8s\x1b[1;3H" + edit))
		if !term.BidiRowAt(0).RTL || term.BidiRowAt(1).RTL {
			t.Fatal("partial line edit moved row state", edit, term.BidiRowAt(0), term.BidiRowAt(1))
		}
	}
	term, _ := bidiTerminal(t, Effects{})
	term.Write([]byte("\x1b[2 kRTL\r\n\x1b[1 kLTR\r\n\x1b[2 kRTL\r\n\x1b[1 kLTR\r\n\x1b[2 kRTL\x1b[1 k\x1b[?69h\x1b[3;8s\x1b[5;3H\n"))
	for y, rtl := range []bool{false, true, false, true, false} {
		if term.BidiRowAt(y).RTL != rtl {
			t.Fatal("narrow forward scroll lost source state", y, term.BidiRowAt(y))
		}
	}
}

func TestOShellBidiGrowthAcrossNativePages(t *testing.T) {
	term, _ := bidiTerminal(t, Effects{})
	term.Write([]byte("\x1b[2 k"))
	if err := term.Resize(12, 2000, 8, 16); err != nil {
		t.Fatal(err)
	}
	for y := 5; y < 2000; y++ {
		if !term.BidiRowAt(y).RTL {
			t.Fatal("new native page lost current default", y)
		}
	}
}

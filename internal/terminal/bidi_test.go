package terminal

import (
	imagecolor "image/color"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func bidiViewFixture(t *testing.T) (*Terminal, *ui.Tester, *pipe) {
	t.Helper()
	loadLib(t)
	conn := newPipe()
	term, err := New(Options{Conn: conn, NoBlink: true, Theme: &Theme{Foreground: ui.Hex("#eeeeee"), Background: ui.Hex("#000000"), Selection: ui.Hex("#0000ff"), SelectionText: ui.Hex("#ffffff")}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	term.mu.Lock()
	enabled := term.term.BidiEnabled()
	term.mu.Unlock()
	if !enabled {
		t.Fatal("normal terminal creation did not enable BiDi")
	}
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 640, 260)
	return term, tt, conn
}

func TestBidiProductionBeforeFirstViewAndHiddenOutput(t *testing.T) {
	for _, tc := range []struct {
		name, modes, text string
		explicit, right   bool
		// First two logical columns' visual positions, counted from either edge.
		first, second int
	}{
		{"implicit-LTR", "\x1b[?2501l\x1b[1 k", "אב abc", false, false, 1, 0},
		{"implicit-RTL", "\x1b[?2501l\x1b[2 k", "abc", false, true, 3, 2},
		{"implicit-auto-LTR", "\x1b[1 k", "אב abc", false, true, 1, 2},
		{"implicit-auto-RTL", "\x1b[2 k", "abc", false, false, 0, 1},
		{"explicit-LTR", "\x1b[8l\x1b[1 k", "אב abc", true, false, 0, 1},
		{"explicit-RTL", "\x1b[8l\x1b[2 k", "abc", true, true, 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loadLib(t)
			conn := newPipe()
			term, err := New(Options{Conn: conn, NoBlink: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { term.Close() })
			// Programs can output modes and text before attaching their first view.
			// Do not call the private bridge setter anywhere in this path.
			term.Feed([]byte(tc.modes + tc.text + "\x1b[8$p"))
			wantReply := "\x1b[8;1$y"
			if tc.explicit {
				wantReply = "\x1b[8;2$y"
			}
			if got := conn.take(len(wantReply)); got != wantReply {
				t.Fatalf("pre-view reply %q want %q", got, wantReply)
			}
			visible := true
			tt := ui.NewTester(func(c *ui.Context) {
				if visible {
					View(c, term).Fill().AutoFocus()
				} else {
					ui.Text(c, "other tab")
				}
			}, 640, 260)
			tt.Frame()
			a, b := tc.first, tc.second
			if tc.right {
				a, b = term.v.cols-a, term.v.cols-b
			}
			if term.v.visualColumn(0, 0) != a || term.v.visualColumn(0, 1) != b {
				t.Fatalf("first frame map %d,%d want %d,%d", term.v.visualColumn(0, 0), term.v.visualColumn(0, 1), a, b)
			}
			// Reset and output while hidden must replace cached maps when shown.
			visible = false
			term.SetVisible(false)
			tt.Frame()
			term.Feed([]byte("\x1bcאב\x1b[8$p"))
			if got := conn.take(len("\x1b[8;1$y")); got != "\x1b[8;1$y" {
				t.Fatalf("reset disabled production BiDi: %q", got)
			}
			visible = true
			term.SetVisible(true)
			tt.Frame()
			if term.v.visualColumn(0, 0) != term.v.cols-1 || term.v.visualColumn(0, 1) != term.v.cols-2 {
				t.Fatal("hidden reset/output retained previous paragraph map")
			}
			if got := term.Text(); got != "אב" {
				t.Fatalf("presentation changed logical text: %q", got)
			}
		})
	}
}

func TestBidiVisualStylesCursorAndHit(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte("\x1b[8l\x1b[2 k\x1b[31m█\x1b[32m█\x1b[0m界\x1b[H\x1b[?25l"))
	tt.Frame()
	v := term.v
	cols := v.cols
	line := &v.lines[0]
	if len(line.boxes) != 2 || line.boxes[0].x != cols-2 || line.boxes[1].x != cols-1 {
		t.Fatal("boxes not in visual order", line.boxes)
	}
	// Bright literal colors avoid dependence on the theme's ANSI palette.
	term.Feed([]byte("\x1b[H\x1b[38;2;255;0;0m█\x1b[38;2;0;255;0m█"))
	tt.Frame()
	graphicColor(t, term, tt, cols-1, 0, imagecolor.RGBA{255, 0, 0, 255})
	graphicColor(t, term, tt, cols-2, 0, imagecolor.RGBA{0, 255, 0, 255})
	term.mu.Lock()
	for _, pair := range [][2]int{{cols - 1, 0}, {cols - 2, 1}, {cols - 4, 2}, {cols - 3, 2}} {
		x, y := cellCenter(term, pair[0], 0)
		col, row := v.cellAt(x, y)
		if col != pair[1] || row != 0 {
			t.Errorf("hit %v = %d,%d", pair, col, row)
		}
	}
	term.mu.Unlock()
	term.Feed([]byte("\x1b[?25h\x1b[1;3H"))
	tt.Frame()
	cw, ch := float32(v.cellW)/v.scale, float32(v.cellH)/v.scale
	r, _, _, _ := v.cursorRect(0, 0, cw, ch)
	if r.X != float32(cols-4)*cw || r.W != 2*cw {
		t.Fatal("wide cursor", r)
	}
	if caret := v.caret(); caret.X != float32(v.ox)/v.scale+float32(cols-4)*cw {
		t.Fatal("IME caret not mapped", caret)
	}
}

func TestBidiLogicalSelectionAndSearch(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte("\x1b[?25lab אב cd"))
	term.mu.Lock()
	if !term.term.SelectSearchRange(0, 0, 0, 4) {
		t.Fatal("search selection failed")
	}
	term.mu.Unlock()
	tt.Frame()
	if got := term.SearchSelection(); got != "ab א" {
		t.Fatal("logical copy", got)
	}
	if got := term.v.lines[0].selected; !slices.Equal(got, []columnSpan{{0, 3}, {4, 5}}) {
		t.Fatal("selection must split across visual runs", got)
	}
	graphicColor(t, term, tt, 2, 0, imagecolor.RGBA{0, 0, 255, 255})
	// Hebrew's alef is at visual column 4; double-click selects the logical word.
	x, y := cellCenter(term, 4, 0)
	tt.ClickAt(x, y)
	tt.ClickAt(x, y)
	tt.Frame()
	if got := term.SearchSelection(); got != "אב" {
		t.Fatal("double-click selection", got)
	}
	tt.Command("copy")
	if tt.Clipboard() != "אב" {
		t.Fatal("clipboard reordered", tt.Clipboard())
	}
}

func TestBidiInputAndPixelMouse(t *testing.T) {
	term, tt, conn := bidiViewFixture(t)
	term.Feed([]byte("אב\x1b[?1243h"))
	tt.Frame()
	tt.Key(0, ui.KeyLeft)
	if got := conn.take(3); got != "\x1b[D" {
		t.Fatalf("physical arrow %q", got)
	}
	term.Feed([]byte("\x1b[?1h"))
	tt.Key(0, ui.KeyRight)
	if got := conn.take(3); got != "\x1bOC" {
		t.Fatalf("application physical arrow %q", got)
	}
	v := term.v
	x, y := cellCenter(term, v.cols-1, 0)
	term.mu.Lock()
	px, py := v.mousePosition(x, y)
	if int((px-float32(v.ox))/float32(v.cellW)) != 0 || py != y*v.scale {
		t.Error("cell mouse not mapped", px, py)
	}
	term.term.Write([]byte("\x1b[?1016h"))
	px, py = v.mousePosition(x, y)
	if px != x*v.scale || py != y*v.scale {
		t.Error("pixel mouse should stay physical", px, py)
	}
	term.mu.Unlock()
}

func TestBidiSynchronizedFrameAndHostShaping(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte("\x1b[?25lאב (abc)"))
	tt.Frame()
	v := term.v
	before := slices.Clone(v.lines[0].bidi.LogicalToVisual)
	term.Feed([]byte("\x1b[?2026h\x1b[H\x1b[8l\x1b[1 k"))
	tt.Frame()
	if !slices.Equal(before, v.lines[0].bidi.LogicalToVisual) {
		t.Fatal("held frame applied newer map")
	}
	term.Feed([]byte("\x1b[?2026l"))
	tt.Frame()
	if len(v.lines[0].bidi.LogicalToVisual) != 0 {
		t.Fatal("finished frame retained old map")
	}
	// A pre-mirrored punctuation cell must not be mirrored again by DirectWrite.
	plain := v.shape("(", 0, 1)
	visual := v.shape("(", 8, 1)
	if len(plain) != len(visual) {
		t.Fatal("visual override changed glyph count", len(plain), len(visual))
	}
	for i := range plain {
		a, b := plain[i], visual[i]
		a.X, b.X = 0, 0
		if !reflect.DeepEqual(a, b) {
			t.Fatal("visual override changed punctuation glyph", a, b)
		}
	}
	for _, punctuation := range []string{"(", ")", "<"} {
		want := v.shape(punctuation, 0, 1)
		var got []ui.Glyph
		for _, glyph := range v.shape("א"+punctuation, 8, 1) {
			if glyph.Cluster == 1 {
				glyph.Cluster = 0
				got = append(got, glyph)
			}
		}
		if len(want) != len(got) {
			t.Fatal("mixed visual glyph count", punctuation, len(want), len(got))
		}
		for i := range want {
			a, b := want[i], got[i]
			a.X, b.X = 0, 0
			if !reflect.DeepEqual(a, b) {
				t.Fatal("host mirrored visual punctuation again", punctuation, a, b)
			}
		}
	}
}

func TestBidiParagraphRepaintAndPreview(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	cols := term.v.cols
	// The first letter selects the whole paragraph's direction, even when
	// the edited first row and the row to be repainted differ.
	term.Feed([]byte("א" + strings.Repeat(".", cols-1) + "abc"))
	tt.Frame()
	if term.v.visualColumn(1, 0) != cols-3 {
		t.Fatal("continuation did not use paragraph direction")
	}
	term.Feed([]byte("\x1b[HA"))
	tt.Frame()
	if term.v.visualColumn(1, 0) != 0 {
		t.Fatal("unchanged continuation was not repainted after prefix edit")
	}
	term.Feed([]byte("\x1bc\x1b[?25lEnglish אבג (123)\r\nאבג English (123)\r\nمرحبا بالعالم لا\r\nא\u05b0ב 界 😀\r\n\x1b[8l\x1b[2 kExplicit RTL abc (123)"))
	tt.Frame()
	save(t, tt, "bidi-parity")
}

func BenchmarkBidiViewport(b *testing.B) {
	if err := Load(); err != nil {
		b.Fatal(err)
	}
	for _, text := range []string{"ascii output", "אבג English 123"} {
		b.Run(text, func(b *testing.B) {
			term, err := New(Options{Conn: newPipe(), NoBlink: true})
			if err != nil {
				b.Fatal(err)
			}
			defer term.Close()
			term.mu.Lock()
			defer term.mu.Unlock()
			if !term.term.BidiEnabled() {
				b.Fatal("normal terminal creation did not enable BiDi")
			}
			if err := term.term.Resize(120, 40, 8, 16); err != nil {
				b.Fatal(err)
			}
			term.term.Write([]byte(strings.Repeat(text+"\r\n", 40)))
			bar := term.term.Scrollbar()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				term.term.BidiRows(int(bar.Offset), int(bar.Len))
			}
		})
	}
}

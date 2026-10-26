package bidilayout

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func row(text string) []Cell {
	var cells []Cell
	for _, r := range text {
		cells = append(cells, Cell{Column: len(cells), Width: 1, Text: string(r)})
	}
	return cells
}

func visualText(source []Cell, layout Row) string {
	byCol := make(map[int]string)
	for _, c := range source {
		byCol[c.Column] = c.Text
	}
	var b strings.Builder
	for _, c := range layout.Cells {
		s := c.Display
		if s == "" {
			s = byCol[c.Column]
		}
		if s == "" {
			s = " "
		}
		b.WriteString(s)
	}
	return b.String()
}

func TestSixPresentationModes(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
		state            State
		rtl, identity    bool
	}{
		{"implicit-LTR", "אב abc", "בא abc    ", State{}, false, false},
		{"implicit-RTL", "abc", "       abc", State{RTL: true}, true, false},
		{"implicit-auto-LTR", "אב abc", "    abc בא", Default, true, false},
		{"implicit-auto-RTL", "abc", "", State{Auto: true, RTL: true}, false, true},
		{"explicit-LTR", "אב abc", "", State{Explicit: true}, false, true},
		{"explicit-RTL", "abc", "       cba", State{Explicit: true, RTL: true}, true, false},
		{"neutral-auto-RTL", "123", "       123", State{Auto: true, RTL: true}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cells := row(tc.text)
			got, ok := Paragraph([][]Cell{cells}, 10, tc.state)
			if !ok || got.RTL != tc.rtl || (len(got.Rows) == 0) != tc.identity {
				t.Fatalf("layout=%+v ok=%v", got, ok)
			}
			if !tc.identity && visualText(cells, got.Rows[0]) != tc.want {
				t.Fatalf("got %q want %q", visualText(cells, got.Rows[0]), tc.want)
			}
		})
	}
}

func TestWideCellsAndMirroring(t *testing.T) {
	cells := []Cell{{0, 1, "A"}, {1, 2, "🙂"}, {3, 1, "("}}
	got, ok := Paragraph([][]Cell{cells}, 8, State{Explicit: true, RTL: true})
	if !ok {
		t.Fatal("layout failed")
	}
	r := got.Rows[0]
	if !slices.Equal(r.LogicalToVisual, []int{7, 5, 6, 4, 3, 2, 1, 0}) || !slices.Equal(r.VisualToLogical, []int{7, 6, 5, 4, 3, 1, 1, 0}) {
		t.Fatal(r)
	}
	if visualText(cells, r) != "    )🙂A" {
		t.Fatal(visualText(cells, r))
	}
	for _, box := range []bool{false, true} {
		cells := row("┌א(")
		got, _ := Paragraph([][]Cell{cells}, 3, State{Explicit: true, RTL: true, MirrorBoxes: box})
		want := ")א┌"
		if box {
			want = ")א┐"
		}
		if visualText(cells, got.Rows[0]) != want {
			t.Fatal(visualText(cells, got.Rows[0]), want)
		}
	}
	if mirror("(\u0301", true) != "" {
		t.Fatal("mirrored a multi-scalar grapheme")
	}
}

func TestArabicShapingAcrossRowsAndMarks(t *testing.T) {
	for _, tc := range []struct {
		cells []Cell
		want  []string
	}{
		{row("بب"), []string{"\ufe91", "\ufe90"}},
		{row("ببب"), []string{"\ufe91", "\ufe92", "\ufe90"}},
		{row("لا"), []string{"\ufefb", " "}},
		{row("بلا"), []string{"\ufe91", "\ufefc", " "}},
		{row("ب\u200cب"), []string{"\ufe8f", "", "\ufe8f"}},
		{row("ب\u200dب"), []string{"\ufe91", "", "\ufe90"}},
		{[]Cell{{0, 1, "ل\u064e"}, {1, 1, "ا\u0651"}}, []string{"\ufefb\u064e\u0651", " "}},
		{row("ء"), []string{"\ufe80"}},
	} {
		if got := shapeArabic(tc.cells); !slices.Equal(got, tc.want) {
			t.Fatalf("input=%v got=%q want=%q", tc.cells, got, tc.want)
		}
	}
	rows := [][]Cell{row("ب"), row("ب")}
	got, ok := Paragraph(rows, 3, Default)
	if !ok || visualText(rows[0], got.Rows[0]) != "  \ufe91" || visualText(rows[1], got.Rows[1]) != "  \ufe90" {
		t.Fatal(got, ok)
	}
	// Explicit mode must not apply contextual shaping or lam/alef ligatures.
	got, _ = Paragraph([][]Cell{row("لا")}, 2, State{Explicit: true, RTL: true})
	if visualText(row("لا"), got.Rows[0]) != "ال" {
		t.Fatal(got)
	}
	for _, text := range []string{"ل", "\ufefb", "a\u0301", "🙂\u200d🙂"} {
		if !Isolate(text) {
			t.Fatal("must isolate", text)
		}
	}
	if Isolate("a") {
		t.Fatal("isolated ordinary ASCII")
	}
}

func TestParagraphDirectionAndBounds(t *testing.T) {
	if detectDirection(row("\u200eאב"), true) || !detectDirection(row("\u200fabc"), false) {
		t.Fatal("direction marks ignored")
	}
	if probeRune("\U0001e900") != 0x200f || probeRune("🙂") != 0xb7 || probeRune("\u0301") != 0xb7 || probeRune("ب") != 0x621 {
		t.Fatal("probe changed scalar direction or joined glyphs")
	}
	for _, cells := range [][]Cell{{{1, 1, "a"}}, {{0, 0, "a"}}, {{0, 4, "a"}}, {{0, 1, ""}}} {
		if _, ok := Paragraph([][]Cell{cells}, 3, Default); ok {
			t.Fatal("accepted malformed cell geometry", cells)
		}
	}
	if _, ok := Paragraph(make([][]Cell, 501), 3, Default); ok {
		t.Fatal("unbounded paragraph")
	}
	if _, ok := Paragraph([][]Cell{row("א\u2029")}, 3, Default); ok {
		t.Fatal("discarded part of paragraph")
	}
}

func FuzzColumnMapping(f *testing.F) {
	for _, s := range []string{"אב abc", "بلا", "a\u202b123\u202cא", "🙂א(1)", "\u2067א\u2066ab\u2069ב\u2069"} {
		f.Add(s, true, false)
	}
	f.Fuzz(func(t *testing.T, s string, rtl, explicit bool) {
		runes := []rune(s)
		if len(runes) > 64 {
			return
		}
		cells := row(string(runes))
		cols := len(cells) + 3
		got, ok := Paragraph([][]Cell{cells}, cols, State{RTL: rtl, Explicit: explicit})
		if !ok || len(got.Rows) == 0 {
			return
		}
		r := got.Rows[0]
		seen := make([]bool, cols)
		for logical, visual := range r.LogicalToVisual {
			if visual < 0 || visual >= cols || seen[visual] || r.VisualToLogical[visual] != logical {
				t.Fatal(fmt.Sprintf("invalid mapping for %q: %+v", s, r))
			}
			seen[visual] = true
		}
	})
}

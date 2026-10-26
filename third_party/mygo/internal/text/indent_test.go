package text

import (
	"math"
	"slices"
	"strings"
	"testing"
)

func TestIndentedFirstFragmentAndFullContinuation(t *testing.T) {
	s := Shared()
	style := Style{Family: "monospace", Size: 16}
	w := s.Shape(Params{Text: "中", Style: style}).Width
	p := Params{Text: strings.Repeat("中", 18), Style: style, Width: w*8 + 0.5, FirstLineIndent: w * 6, KeepSpaces: true}
	l := s.Shape(p)
	if len(l.Lines) < 3 || l.Lines[0].End != 2 || l.Lines[1].End-l.Lines[1].Start != 8 {
		t.Fatalf("first and continuation lengths: %+v", l.Lines)
	}
	for i, line := range l.Lines {
		wantX := float32(0)
		if i == 0 {
			wantX = p.FirstLineIndent
		}
		if math.Abs(float64(line.X-wantX)) > 0.01 || line.X+line.Width > p.Width+0.01 {
			t.Fatalf("line %d geometry x=%v width=%v", i, line.X, line.Width)
		}
	}
}

func TestIndentedTrackingDoesNotAddSyntheticAdvance(t *testing.T) {
	s := Shared()
	style := Style{Family: "monospace", Size: 16, LetterSpacing: 2}
	w := s.Shape(Params{Text: "中", Style: style}).Width
	p := Params{Text: strings.Repeat("中", 18), Style: style, Width: w*8 + 0.5, FirstLineIndent: w * 6, KeepSpaces: true}
	l := s.Shape(p)
	if len(l.Lines) < 3 || l.Lines[0].End != 2 || l.Lines[1].End-l.Lines[1].Start != 8 {
		t.Fatalf("tracking changed fragment capacity: %+v", l.Lines)
	}
}

func TestIndentedWordMovesWholeAndNewlineResetsInset(t *testing.T) {
	s := Shared()
	style := Style{Family: "monospace", Size: 16}
	word := "longword"
	w := s.Shape(Params{Text: word, Style: style}).Width
	p := Params{Text: word + "\nnext", Style: style, Width: w + 1, FirstLineIndent: w / 2, KeepSpaces: true}
	l := s.Shape(p)
	if len(l.Lines) != 3 || l.Lines[0].Start != 0 || l.Lines[0].End != 0 || l.Lines[1].Start != 0 || l.Lines[1].End != len(word) || l.Lines[2].X != 0 {
		t.Fatalf("whole-word/paragraph wrap: %+v", l.Lines)
	}
	p.FirstLineIndent = p.Width
	l = s.Shape(p)
	if len(l.Lines) != 3 || l.Lines[0].Width != 0 || l.Lines[1].Width > p.Width {
		t.Fatal("full first-line exclusion did not continue")
	}
}

func TestIndentedTrailingSpaceDoesNotMoveVisibleWord(t *testing.T) {
	s := Shared()
	style := Style{Family: "monospace", Size: 16}
	w := s.Shape(Params{Text: "one", Style: style}).Width
	p := Params{Text: "one two three", Style: style, Width: w * 4, FirstLineIndent: w*3 - 0.5}
	l := s.Shape(p)
	if len(l.Lines) < 2 || l.Lines[0].Start != 0 || l.Lines[0].End < 3 || l.Lines[0].Width == 0 || l.Lines[0].X+l.Lines[0].Width > p.Width+0.01 {
		t.Fatalf("trailing whitespace displaced a fitting word: %+v", l.Lines)
	}
}

func TestIndentedShapingRetainsGraphemesAndParagraphDirection(t *testing.T) {
	for _, content := range []string{"e\u0301e\u0301e\u0301 中文👩‍💻😀结束", "مرحبا بالعالم hello 123", "hello שלום עולם 123", "\n中文\n\nend"} {
		p := Params{Text: content, Style: Style{Size: 16}, Width: 100, FirstLineIndent: 70, KeepSpaces: true}
		l := Shared().Shape(p)
		var b Boundaries
		b.Reset([]rune(content))
		bounds := b.GraphemeOffsets()
		last := 0
		for _, line := range l.Lines {
			if line.Start < last || !slices.Contains(bounds, line.Start) || !slices.Contains(bounds, line.End) {
				t.Fatalf("split grapheme in %q: %d..%d", content, line.Start, line.End)
			}
			last = line.End
			for _, g := range line.Glyphs {
				if g.Cluster < line.Start || g.Cluster+g.Runes > line.End {
					t.Fatalf("rebased cluster escaped line: %+v line=%d..%d", g, line.Start, line.End)
				}
			}
			if !strings.ContainsRune(content, '\n') && line.RTL != isRTL([]rune(content)) {
				t.Fatal("continuation changed paragraph direction")
			}
		}
		if last != len([]rune(content)) {
			t.Fatal("lost trailing text")
		}
	}
}

func TestIndentedContinuationRetainsDirectionControls(t *testing.T) {
	for _, content := range []string{
		"\u202e" + strings.Repeat("abc xyz ", 8) + "\u202c",
		"\u202d" + strings.Repeat("שלום עולם ", 8) + "\u202c",
		"start \u2067אבג " + strings.Repeat("123 xyz ", 8) + "\u2069 end",
		"\u2068שלום " + strings.Repeat("123 (abc) ", 8) + "\u2069 end",
		"\u202bאבג " + strings.Repeat("123 xyz ", 8) + "\u202c end",
		"\u202eabc \u2066" + strings.Repeat("123 xyz ", 8) + "\u2069\u202c",
	} {
		p := Params{Text: content, Style: Style{Size: 16}, KeepSpaces: true}
		full := Shared().Shape(p)
		want := map[int]bool{}
		for _, line := range full.Lines {
			for _, glyph := range line.Glyphs {
				want[glyph.Cluster] = glyph.RTL
			}
		}
		p.Width, p.FirstLineIndent = 180, 80
		wrapped := Shared().Shape(p)
		if len(wrapped.Lines) < 2 {
			t.Fatal("missing continuation")
		}
		runes := []rune(content)
		for i, line := range wrapped.Lines {
			if i == 0 {
				continue
			}
			for _, glyph := range line.Glyphs {
				r := runes[glyph.Cluster]
				if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r >= 'א' && r <= 'ת' {
					if rtl, ok := want[glyph.Cluster]; ok && rtl != glyph.RTL {
						t.Fatalf("continuation lost direction at rune %d %q: rtl=%v want=%v", glyph.Cluster, r, glyph.RTL, rtl)
					}
				}
			}
		}
	}
}

func TestIndentedPrefixDoesNotChangeOriginalGlyphs(t *testing.T) {
	for _, content := range []string{"\u0301abc", "e\u0301", "😀abc", "👩‍💻", "\u200dabc", "שלום", "\u202eabc\u202c", "\u2067אבג\u2069"} {
		t.Run(content, func(t *testing.T) {
			p := Params{Text: content, Style: Style{Size: 16}, Width: 1000, KeepSpaces: true}
			plain := Shared().Shape(p)
			p.FirstLineIndent = 40
			indented := Shared().Shape(p)
			if len(plain.Lines) != 1 || len(indented.Lines) != 1 {
				t.Fatal("unexpected wrap")
			}
			a, b := plain.Lines[0].Glyphs, indented.Lines[0].Glyphs
			if len(a) != len(b) {
				t.Fatalf("prefix changed glyph count: %d to %d", len(a), len(b))
			}
			for i := range a {
				if a[i].Cluster != b[i].Cluster || a[i].Runes != b[i].Runes || a[i].ID != b[i].ID || a[i].Font != b[i].Font || a[i].RTL != b[i].RTL || math.Abs(float64(a[i].Advance-b[i].Advance)) > 0.01 {
					t.Fatalf("prefix changed glyph %d: before=%+v after=%+v", i, a[i], b[i])
				}
			}
		})
	}
}

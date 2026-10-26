package ui

import "github.com/egoist/mygo/internal/text"

// GlyphLine is a shaped line whose glyph X positions are relative to the
// container's left edge. Start and End are rune offsets, excluding a newline.
type GlyphLine struct {
	Glyphs     []Glyph
	X, Width   float32
	Start, End int
}

// ShapeLines wraps at width, reserving firstIndent DIPs on the left of the
// first line only. Tracking adds spacing in DIPs through the native shaper.
// Callers retain these lines and choose their own baselines, as a terminal
// does for an IME overlay aligned to its cell grid.
func ShapeLines(s string, f Font, width, firstIndent, tracking float32) []GlyphLine {
	style := f.style()
	style.LetterSpacing = tracking
	l := text.Shared().Shape(text.Params{Text: s, Style: style, Width: width, FirstLineIndent: firstIndent})
	out := make([]GlyphLine, len(l.Lines))
	for i, line := range l.Lines {
		out[i] = GlyphLine{X: line.X, Width: line.Width, Start: line.Start, End: line.End, Glyphs: make([]Glyph, len(line.Glyphs))}
		for j, g := range line.Glyphs {
			out[i].Glyphs[j] = Glyph{Cluster: g.Cluster, Runes: g.Runes, X: g.X, Advance: g.Advance, font: g.Font, id: g.ID, thick: f.Thicken}
		}
	}
	return out
}

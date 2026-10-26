package text

import (
	"unicode"

	"github.com/go-text/typesetting/segmenter"
)

func firstLineIndent(p Params) float32 {
	if p.NoWrap || p.Width <= 0 {
		return 0
	}
	return max(0, min(p.FirstLineIndent, p.Width))
}

// Shape the first fragment at its reduced width, then the continuation with
// the original paragraph direction and full width. The native shaper still
// owns Unicode line breaks, graphemes, ligatures and font fallback.
func (s *System) shapeIndented(runes []rune, p Params, spans []Span, rtl bool) []shapedLine {
	e := s.engine()
	available := p.Width - firstLineIndent(p)
	if available <= 0 {
		return append([]shapedLine{{}}, e.shape(runes, p.Style, spans, p.Width, rtl, p.NoBreakWords)...)
	}
	if native, ok := e.(interface {
		shapeIndent([]rune, Style, []Span, float32, float32, bool, bool) []shapedLine
	}); ok {
		return native.shapeIndent(runes, p.Style, spans, p.Width, firstLineIndent(p), rtl, p.NoBreakWords)
	}
	narrow := e.shape(runes, p.Style, spans, available, rtl, p.NoBreakWords)
	if len(narrow) == 0 {
		return narrow
	}
	first := narrow[0]
	cut := first.end
	if len(narrow) > 1 {
		cut = narrow[1].start
	}
	trim := first.end
	for trim > first.start && unicode.IsSpace(runes[trim-1]) {
		trim--
	}
	visibleWidth := float32(0)
	for _, run := range first.runs {
		for _, glyph := range run.glyphs {
			if glyph.Cluster < trim {
				visibleWidth += glyph.Advance
			}
		}
	}
	skip := visibleWidth > available
	// A whole word that fits a continuation line should move there rather
	// than be split only because the prompt occupies most of the first line.
	if !skip && cut > 0 && cut < len(runes) {
		var seg segmenter.Segmenter
		breaks := lineBreaks(&seg, nil, runes)
		if !breaks[cut] {
			end := cut
			for end < len(runes) && !breaks[end] {
				end++
			}
			for end > 0 && unicode.IsSpace(runes[end-1]) {
				end--
			}
			word := e.shape(runes[:end], p.Style, spansIn(spans, 0, end), 0, rtl, p.NoBreakWords)
			if len(word) == 1 && shapedWidth(word[0]) <= p.Width {
				skip = true
			}
		}
	}
	if skip {
		return append([]shapedLine{{}}, e.shape(runes, p.Style, spans, p.Width, rtl, p.NoBreakWords)...)
	}
	if cut >= len(runes) {
		return []shapedLine{first}
	}
	if cut <= 0 {
		return append([]shapedLine{{}}, e.shape(runes, p.Style, spans, p.Width, rtl, p.NoBreakWords)...)
	}
	tail := e.shape(runes[cut:], p.Style, spansIn(spans, cut, len(runes)), p.Width, rtl, p.NoBreakWords)
	for i := range tail {
		tail[i].start += cut
		tail[i].end += cut
		for j := range tail[i].runs {
			r := &tail[i].runs[j]
			r.start += cut
			r.end += cut
			for k := range r.glyphs {
				r.glyphs[k].Cluster += cut
			}
		}
	}
	return append([]shapedLine{first}, tail...)
}

func shapedWidth(line shapedLine) (width float32) {
	for _, run := range line.runs {
		for _, glyph := range run.glyphs {
			width += glyph.Advance
		}
	}
	return width
}

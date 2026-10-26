package bidi

import (
	"slices"

	ucd "oshell-desktop/internal/terminal/internal/bidi/ucd"
)

// Resolve computes paragraph context without applying line-end rules. It
// returns the consumed runes; paragraph separators terminate the input.
// The caller supplies its resolved base direction or Neutral for UAX P2/P3.
func (p *Paragraph) Resolve(text []rune, direction Direction) int {
	p.text = append(p.text[:0], text...)
	p.prepareInput()
	p.embeddingLevel = implicitLevel
	if direction == LeftToRight {
		p.embeddingLevel = 0
	} else if direction == RightToLeft {
		p.embeddingLevel = 1
	}
	if len(p.text) == 0 {
		if p.embeddingLevel == implicitLevel {
			p.embeddingLevel = 0
		}
		return 0
	}
	p.run()
	return len(p.text)
}

func (p *Paragraph) BaseLevel() Level { return p.embeddingLevel }

// LineOrder owns an immutable line result. Order contains paragraph-relative
// logical indices in visual order; Levels is indexed relative to the line.
// Format controls keep a cell in the permutation, even when they draw nothing.
type LineOrder struct {
	Order  []int
	Levels []Level
}

// Line applies UAX #9 L1 and L2 to a real terminal row using the enclosing
// paragraph's resolved levels. It never resolves the row as a new paragraph.
func (p *Paragraph) Line(start, end int) (LineOrder, bool) {
	if start < 0 || end < start || end > len(p.resultLevels) {
		return LineOrder{}, false
	}
	levels := slices.Clone(p.resultLevels[start:end])
	types := p.initialTypes[start:end]
	for i, kind := range types {
		if kind&(ucd.BD_B|ucd.BD_S) == 0 {
			continue
		}
		levels[i] = p.embeddingLevel
		for j := i - 1; j >= 0 && isWhitespace(types[j]); j-- {
			levels[j] = p.embeddingLevel
		}
	}
	for i := len(levels) - 1; i >= 0 && isWhitespace(types[i]); i-- {
		levels[i] = p.embeddingLevel
	}
	order := make([]int, len(levels))
	var highest Level
	lowestOdd := Level(127)
	for i, level := range levels {
		order[i] = start + i
		if level > highest {
			highest = level
		}
		if level&1 != 0 && level < lowestOdd {
			lowestOdd = level
		}
	}
	for level := highest; level >= lowestOdd; level-- {
		for i := 0; i < len(order); {
			if levels[order[i]-start] < level {
				i++
				continue
			}
			end := i + 1
			for end < len(order) && levels[order[end]-start] >= level {
				end++
			}
			slices.Reverse(order[i:end])
			i = end
		}
	}
	return LineOrder{Order: order, Levels: levels}, true
}

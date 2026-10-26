package terminal

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"oshell-desktop/internal/terminal/internal/vt"
)

// Link discovery uses buffer coordinates, including offscreen wrapped rows.
// Keep the row copies local to a lookup so output cannot leave stale targets.
type linkRow struct {
	cells   []vt.LogicalCell
	wrapped bool
}

func linkWhitespace(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsSpace(r)
}

func linkEdges(r linkRow) (first, last int) {
	first, last = -1, -1
	for i, c := range r.cells {
		if !linkWhitespace(c.Text) {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	return
}

func linkSegment(r linkRow, start, end int) string {
	var b strings.Builder
	for _, c := range r.cells {
		if c.Column >= start && c.Column < end {
			b.WriteString(c.Text)
		}
	}
	return b.String()
}

func linkSeamCharacter(s string) bool {
	const allowed = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~:/?#[]@!$&*+,;=%()"
	return s != "" && strings.IndexFunc(s, func(r rune) bool { return !strings.ContainsRune(allowed, r) }) < 0
}

func linkRowsJoin(upper, lower linkRow, cols int) bool {
	_, last := linkEdges(upper)
	first, _ := linkEdges(lower)
	if last < 0 || first < 0 {
		return false
	}
	u, l := upper.cells[last], lower.cells[first]
	end := u.Column + u.Width
	if end-1 < max(0, cols-max(2, cols/5)) || !linkSeamCharacter(u.Text) || !linkSeamCharacter(l.Text) {
		return false
	}
	a := linkSegment(upper, max(0, end-96), end)
	b := linkSegment(lower, l.Column, l.Column+96)
	for _, m := range implicitLinkRanges(a + b) {
		if m[0] < len(a) && m[1] > len(a) {
			return true
		}
	}
	return false
}

func (v *view) implicitLinkMatchAt(col, row int) terminalLink {
	term := v.t.term
	sb := term.Scrollbar()
	cols, _ := term.Size()
	if col < 0 || col >= cols || row < 0 || row >= int(sb.Len) {
		return terminalLink{}
	}
	target := row + int(sb.Offset)
	cache := make(map[int]linkRow)
	read := func(y int) linkRow {
		if r, found := cache[y]; found {
			return r
		}
		cells, wrapped := term.LogicalRow(y)
		r := linkRow{cells, wrapped}
		cache[y] = r
		return r
	}
	start, end := target, target
	for start > 0 && read(start).wrapped {
		start--
	}
	for end+1 < int(sb.Total) && read(end+1).wrapped {
		end++
	}
	if start == target && end == target {
		for start > 0 && linkRowsJoin(read(start-1), read(start), cols) {
			start--
		}
		for end+1 < int(sb.Total) && linkRowsJoin(read(end), read(end+1), cols) {
			end++
		}
	}
	var b strings.Builder
	type mappedCell struct{ row, col, width, begin, end int }
	var cells []mappedCell
	hitStart, hitEnd := -1, -1
	for y := start; y <= end; y++ {
		r := read(y)
		first := 0
		if y > start && (r.wrapped || linkRowsJoin(read(y-1), r, cols)) {
			for first < len(r.cells) && linkWhitespace(r.cells[first].Text) {
				first++
			}
		}
		for _, c := range r.cells[first:] {
			begin := b.Len()
			b.WriteString(c.Text)
			cells = append(cells, mappedCell{y, c.Column, c.Width, begin, b.Len()})
			if y == target && col >= c.Column && col < c.Column+c.Width {
				hitStart, hitEnd = begin, b.Len()
			}
		}
	}
	if hitStart < 0 {
		return terminalLink{}
	}
	text := b.String()
	for _, m := range implicitLinkRanges(text) {
		if m[0] < hitEnd && m[1] > hitStart {
			match := terminalLink{text: text[m[0]:m[1]]}
			for _, c := range cells {
				if c.begin >= m[1] || c.end <= m[0] {
					continue
				}
				n := len(match.ranges)
				if n > 0 && match.ranges[n-1].row == c.row {
					match.ranges[n-1].end = c.col + c.width
				} else {
					match.ranges = append(match.ranges, linkRowRange{c.row, c.col, c.col + c.width})
				}
			}
			return match
		}
	}
	return terminalLink{}
}

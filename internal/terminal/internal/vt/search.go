package vt

import (
	"sort"
	"strings"
	"unicode/utf8"
	"unsafe"
)

// SearchSelectionRange reads the terminal-owned tracked selection afresh. Never
// retain its borrowed GridRefs across output or resize: their nodes may move.
func (t *Terminal) SearchSelectionRange() (row, start, endRow, end int, found bool) {
	if oshellABI >= 55 && oshellABI <= 62 {
		var logical logicalSelection
		if t.get(1005, unsafe.Pointer(&logical)) == nil {
			if logical[4] == 0 {
				return
			}
			ax, ay, bx, by := int(logical[0]), int(logical[1]), int(logical[2]), int(logical[3])
			if ay > by || ay == by && ax > bx {
				ax, ay, bx, by = bx, by, ax, ay
			}
			_, starts := t.rowText(ay, true)
			_, ends := t.rowText(by, true)
			start, end = ax, bx
			if len(starts) > 0 {
				start = sort.SearchInts(starts, ax)
			}
			if len(ends) > 0 {
				end = sort.SearchInts(ends, bx)
			}
			return ay, start, by, end, ay != by || ax != bx
		}
	}
	s := Selection{size: selectionCSize}
	if t.get(31, unsafe.Pointer(&s)) != nil || s.rectangle {
		return
	}
	type coordinate struct {
		x uint16
		_ uint16
		y uint32
	}
	var a, b coordinate
	if !ok(call(fnPointFromGridRef, t.h, uintptr(unsafe.Pointer(&s.start)), 2, uintptr(unsafe.Pointer(&a)))) ||
		!ok(call(fnPointFromGridRef, t.h, uintptr(unsafe.Pointer(&s.end)), 2, uintptr(unsafe.Pointer(&b)))) {
		return
	}
	if a.y > b.y || a.y == b.y && a.x > b.x {
		a, b = b, a
	}
	_, starts := t.rowText(int(a.y), true)
	_, ends := t.rowText(int(b.y), true)
	start = sort.SearchInts(starts, int(a.x))
	end = sort.Search(len(ends), func(i int) bool { return ends[i] > int(b.x) })
	return int(a.y), start, int(b.y), end, start < len(starts) && end > 0
}

// SearchRows snapshots the grid without moving the viewport or its selection.
// Keeping spaces on wrapped rows is essential: they may be inside a match.
func (t *Terminal) SearchRows(first int) ([]string, []bool) {
	rows := strings.Split(t.formattedTextFrom(first, false, false), "\n")
	wrapped := make([]bool, len(rows))
	width, _ := t.Size()
	for i := range rows {
		ref, found := t.screenCell(0, first+i)
		if found {
			var raw uint64
			if ok(call(fnGridRefRow, uintptr(unsafe.Pointer(&ref)), uintptr(unsafe.Pointer(&raw)))) {
				call(fnRawRowGet, uintptr(raw), 1, uintptr(unsafe.Pointer(&wrapped[i])))
			}
		}
		if wrapped[i] {
			// The formatter omits null trailing cells even with trim disabled.
			// Erasure may leave those cells inside a logical line. Read only
			// such rows from the grid, preserving wide/combining cell mapping.
			last, found := t.screenCell(width-1, first+i)
			var raw uint64
			if found && ok(call(fnGridRefCell, uintptr(unsafe.Pointer(&last)), uintptr(unsafe.Pointer(&raw)))) &&
				Wide(cell.wide.of(raw)) == Narrow && (cell.tag.of(raw) >= 2 || cell.codepoint.of(raw) == 0) {
				text, _ := t.rowText(first+i, true)
				rows[i] = string(text)
			}
		} else {
			rows[i] = strings.TrimRight(rows[i], " ")
		}
	}
	return rows, wrapped
}

// SearchRowWidths uses actual grid coordinates, not UTF-8 bytes or rune width
// guesses. Wrapped rows include their erased trailing cells.
func (t *Terminal) SearchRowWidths(first int, rows []string, wraps []bool) []int {
	widths := make([]int, len(rows))
	width, _ := t.Size()
	for i, text := range rows {
		if wraps[i] {
			widths[i] = width
			continue
		}
		count := utf8.RuneCountInString(text)
		if count == 0 {
			continue
		}
		_, columns := t.rowText(first+i, true)
		if count > len(columns) {
			widths[i] = width
			continue
		}
		x := columns[count-1]
		widths[i] = x + 1
		ref, found := t.screenCell(x, first+i)
		var raw uint64
		if found && ok(call(fnGridRefCell, uintptr(unsafe.Pointer(&ref)), uintptr(unsafe.Pointer(&raw)))) && Wide(cell.wide.of(raw)) == WideChar {
			widths[i]++
		}
	}
	return widths
}

// SelectSearchRange uses absolute grid coordinates, so matches can span more
// than one viewport and wide/combining characters retain their complete cells.
func (t *Terminal) SelectSearchRange(row, start, endRow, end int) bool {
	_, starts := t.rowText(row, true)
	_, ends := t.rowText(endRow, true)
	if start < 0 || start >= len(starts) || end < 1 || end > len(ends) {
		return false
	}
	a, ok := t.screenCell(starts[start], row)
	if !ok {
		return false
	}
	b, ok := t.screenCell(ends[end-1], endRow)
	if !ok {
		return false
	}
	s := Selection{size: selectionCSize, start: a, end: b}
	t.SetSelection(&s)
	t.ScrollToRow(row)
	return true
}

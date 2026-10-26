package vt

import "unsafe"

// LogicalCell owns one grapheme's text and its grid extent. Empty cells before
// the last printed character are spaces; trailing unprinted cells are omitted.
type LogicalCell struct {
	Column, Width int
	Text          string
}

// LogicalRow reads an absolute buffer row without changing the viewport.
// No borrowed native storage escapes; callers serialize reads with output.
func (t *Terminal) LogicalRow(y int) (cells []LogicalCell, continuation bool) {
	if y < 0 || y >= int(t.Scrollbar().Total) {
		return nil, false
	}
	cols, _ := t.Size()
	row, raw, found := t.bidiRawRow(y)
	if !found {
		ref, found := t.screenCell(0, y)
		if !found || !ok(call(fnGridRefRow, uintptr(unsafe.Pointer(&ref)), uintptr(unsafe.Pointer(&row)))) {
			return nil, false
		}
	}
	call(fnRawRowGet, uintptr(row), 2, uintptr(unsafe.Pointer(&continuation)))
	for _, c := range t.bidiCellsFromRaw(y, cols, raw) {
		cells = append(cells, LogicalCell{Column: c.Column, Width: c.Width, Text: c.Text})
	}
	return cells, continuation
}

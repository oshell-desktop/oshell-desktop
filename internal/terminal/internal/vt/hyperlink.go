package vt

import "unsafe"

// HyperlinkRange returns the half-open columns belonging to the same OSC 8
// write at a viewport cell. Equal URLs or supplied ids do not merge writes.
// The range stays in one physical row, as Swift's payload token lookup does.
func (t *Terminal) HyperlinkRange(x, y int) (start, end int, found bool) {
	if oshellABI < 43 || oshellABI > 62 || x < 0 || y < 0 {
		return
	}
	cols, rows := t.Size()
	if x >= cols || y >= rows {
		return
	}
	data := struct{ X, Y, Start, End uint32 }{X: uint32(x), Y: uint32(uint64(y) + t.Scrollbar().Offset)}
	if t.get(1003, unsafe.Pointer(&data)) != nil || data.End <= data.Start || data.End > uint32(cols) {
		return
	}
	return int(data.Start), int(data.End), true
}

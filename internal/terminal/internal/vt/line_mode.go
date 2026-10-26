package vt

import "unsafe"

// LineMode changes presentation only, leaving the underlying columns intact.
type LineMode uint8

const (
	LineSingle LineMode = iota
	LineDoubleWidth
	LineDoubleTop
	LineDoubleBottom
)

func rawLineMode(raw uint64) LineMode {
	if oshellABI < 19 || oshellABI > 62 {
		return LineSingle
	}
	var mode uint8
	if !ok(call(fnRawRowGet, uintptr(raw), 1000, uintptr(unsafe.Pointer(&mode)))) || mode > 3 {
		return LineSingle
	}
	return LineMode(mode)
}

// RowLineMode is read from the immutable render snapshot, including held frames.
func (r *RenderState) RowLineMode() LineMode {
	var raw uint64
	if !r.rowGet(2, unsafe.Pointer(&raw)) {
		return LineSingle
	}
	return rawLineMode(raw)
}

// LineModeAt returns the mode of a row in the current viewport.
func (t *Terminal) LineModeAt(y int) LineMode {
	ref, found := t.CellAt(0, y)
	return lineModeRef(ref, found)
}

func (t *Terminal) BufferLineModeAt(y int) LineMode {
	ref, found := t.screenCell(0, y)
	return lineModeRef(ref, found)
}

func lineModeRef(ref GridRef, found bool) LineMode {
	var raw uint64
	if !found || !ok(call(fnGridRefRow, uintptr(unsafe.Pointer(&ref)), uintptr(unsafe.Pointer(&raw)))) {
		return LineSingle
	}
	return rawLineMode(raw)
}

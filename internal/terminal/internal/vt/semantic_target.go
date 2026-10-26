package vt

import "unsafe"

type semanticTarget struct {
	y, reserved uint32
	id          uint64
}

// SemanticClickTarget is an opaque line incarnation, not a retained grid
// pointer or row number. Whole-row moves retain it; recycling destroys it.
type SemanticClickTarget struct {
	row   semanticTarget
	owner uintptr
}

// CaptureSemanticClickTarget performs Swift's cheap pre-gate before a host
// schedules its timer. Gesture and modifier eligibility belong to the host.
func (t *Terminal) CaptureSemanticClickTarget(y int) (SemanticClickTarget, bool) {
	if (oshellABI < 61 || oshellABI > 62) || y < 0 || uint64(y) > uint64(^uint32(0)) {
		return SemanticClickTarget{}, false
	}
	row := semanticTarget{y: uint32(y)}
	if t.get(1010, unsafe.Pointer(&row)) != nil || row.id == 0 {
		return SemanticClickTarget{}, false
	}
	return SemanticClickTarget{row: row, owner: t.h}, true
}

// ResolveSemanticClickTarget re-resolves the line after output/scrollback has
// moved it. Call SemanticClickData afterwards to recheck interaction state.
func (t *Terminal) ResolveSemanticClickTarget(target SemanticClickTarget) (int, bool) {
	if (oshellABI < 61 || oshellABI > 62) || target.owner != t.h || target.row.id == 0 {
		return 0, false
	}
	row := target.row
	if t.get(1011, unsafe.Pointer(&row)) != nil || row.id != target.row.id {
		return 0, false
	}
	return int(row.y), true
}

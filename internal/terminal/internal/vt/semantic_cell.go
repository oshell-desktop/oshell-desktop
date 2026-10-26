package vt

import "unsafe"

// SemanticContent is SwiftTerm's per-cell classification. None means the
// shell has not classified the cell; Output requires an explicit OSC 133 C/D.
type SemanticContent uint8

const (
	SemanticNone SemanticContent = iota
	SemanticPromptInitial
	SemanticPromptRight
	SemanticPromptContinuation
	SemanticPromptSecondary
	SemanticInput
	SemanticOutput
)

// SemanticContentAt reads an absolute buffer cell, including scrollback.
// It returns false for invalid coordinates or an unsupported runtime.
func (t *Terminal) SemanticContentAt(x, y int) (SemanticContent, bool) {
	if oshellABI != 62 || x < 0 || y < 0 || uint64(y) > uint64(^uint32(0)) {
		return SemanticNone, false
	}
	cols, _ := t.Size()
	if x >= cols {
		return SemanticNone, false
	}
	ref, found := t.screenCell(x, y)
	if !found {
		return SemanticNone, false
	}
	var raw uint64
	var role uint8
	if !ok(call(fnGridRefCell, uintptr(unsafe.Pointer(&ref)), uintptr(unsafe.Pointer(&raw)))) ||
		!ok(call(fnRawCellGet, uintptr(raw), 1000, uintptr(unsafe.Pointer(&role)))) || role > uint8(SemanticOutput) {
		return SemanticNone, false
	}
	return SemanticContent(role), true
}

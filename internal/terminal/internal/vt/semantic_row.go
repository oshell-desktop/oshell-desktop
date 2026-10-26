package vt

import "unsafe"

// SemanticRowKind is derived from authored marks and structural continuity.
// A legacy raw row prompt flag does not express this classification.
type SemanticRowKind uint32

const (
	SemanticRowNone SemanticRowKind = iota
	SemanticRowInitial
	SemanticRowContinuation
)

// SemanticRow returns the classification of an absolute row in the active
// buffer, including history. It must run under the same owner lock as Write.
// The boolean distinguishes an unclassified row from an unsupported/invalid
// query. No native row pointer escapes the call.
func (t *Terminal) SemanticRow(y int) (SemanticRowKind, bool) {
	if oshellABI < 59 || oshellABI > 62 || y < 0 || uint64(y) > uint64(^uint32(0)) {
		return SemanticRowNone, false
	}
	data := struct{ y, kind uint32 }{y: uint32(y)}
	if t.get(1008, unsafe.Pointer(&data)) != nil || data.kind > uint32(SemanticRowContinuation) {
		return SemanticRowNone, false
	}
	return SemanticRowKind(data.kind), true
}

package vt

import (
	"runtime"
	"unsafe"
)

// SemanticInputState is the buffer's interaction lifetime, not a cell tag.
type SemanticInputState uint8

const (
	SemanticInputIdle SemanticInputState = iota
	SemanticInputPrompt
	SemanticInputArmed
	SemanticInputSubmitted
)

// RegisterUserInput runs synchronously under the same owner lock as Write.
// Terminal protocol replies and raw transfer packets must not use this path.
func (t *Terminal) RegisterUserInput(data []byte) {
	if oshellABI < 58 || oshellABI > 62 || len(data) == 0 {
		return
	}
	var pin runtime.Pinner
	pin.Pin(&data[0])
	defer pin.Unpin()
	s := cString{ptr: uintptr(unsafe.Pointer(&data[0])), len: uintptr(len(data))}
	_ = t.set(1007, unsafe.Pointer(&s))
}

func (t *Terminal) SemanticInputState() (SemanticInputState, bool) {
	if oshellABI < 58 || oshellABI > 62 {
		return SemanticInputIdle, false
	}
	var state uint8
	if t.get(1007, unsafe.Pointer(&state)) != nil || state > uint8(SemanticInputSubmitted) {
		return SemanticInputIdle, false
	}
	return SemanticInputState(state), true
}

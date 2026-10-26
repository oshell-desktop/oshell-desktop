package vt

import "unsafe"

// ControlEncoding is local terminal protocol state, shared across screen buffers.
// It does not change normal keyboard or bracketed paste encoding.
func (t *Terminal) ControlEncoding() (eightBit bool, conformance uint8) {
	if oshellABI < 21 || oshellABI > 62 {
		return false, 65
	}
	var state uint16
	if t.get(1000, unsafe.Pointer(&state)) != nil {
		return false, 65
	}
	return state&256 != 0, uint8(state)
}

// EncodeControlReport changes only the CSI prefix of a generated focus or mouse
// report. Payload bytes may themselves be escape codes in legacy mouse formats.
func (t *Terminal) EncodeControlReport(data []byte) []byte {
	eight, _ := t.ControlEncoding()
	return controlReport(data, eight)
}

func controlReport(data []byte, eight bool) []byte {
	if !eight || len(data) < 2 || data[0] != 27 || data[1] != '[' {
		return data
	}
	data[1] = 0x9b
	return data[1:]
}

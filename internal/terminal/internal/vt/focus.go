package vt

import "unsafe"

// SetFocus records the host view state even while DECSET 1004 is disabled.
// A changed state is reported only when the program has enabled reports.
func (t *Terminal) SetFocus(focused bool) {
	if t.focused == focused || t.h == 0 {
		return
	}
	if oshellABI >= 28 && oshellABI <= 62 {
		if err := t.set(1004, unsafe.Pointer(&focused)); err != nil {
			return
		}
	} else if t.Mode(ModeFocusEvent) && t.effects.WritePTY != nil {
		t.effects.WritePTY(t.EncodeControlReport(EncodeFocus(focused)))
	}
	t.focused = focused
}

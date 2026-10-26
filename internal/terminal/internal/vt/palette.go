package vt

import "unsafe"

// SetDerivedPalette enables OShell's synchronous OSC color handling. The
// callback derives a palette from the installed ANSI colors and effective
// background/foreground. Old development libraries retain their static palette.
func (t *Terminal) SetDerivedPalette(derive func(*[256]RGB, RGB, RGB)) bool {
	if oshellABI < 2 || oshellABI > 62 {
		return false
	}
	callback := uintptr(0)
	if derive != nil {
		callback = cbDerivedPalette
	}
	if t.setValue(1001, callback) != nil {
		return false
	}
	t.derivePalette = derive
	return true
}

func (t *Terminal) updateDerivedPalette() {
	if t.derivePalette == nil {
		return
	}
	var foreground, background RGB
	if t.get(18, unsafe.Pointer(&foreground)) != nil || t.get(19, unsafe.Pointer(&background)) != nil {
		return
	}
	palette := t.installedPalette
	t.derivePalette(&palette, background, foreground)
	// With the OShell callback installed, this also clears previous OSC 4
	// overrides, matching SwiftTerm's rebuildAnsiPalette behavior.
	t.set(optColorPalette, unsafe.Pointer(&palette))
}

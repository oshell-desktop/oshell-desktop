//go:build linux && (amd64 || arm64)

package linux

import (
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

// This exercises actual surface lifecycle code with native calls substituted.
// It still needs a Linux test runtime; cross-compiling it is not execution.
func TestSurfaceInputContextOwnership(t *testing.T) {
	newContext, focusIn, focusOut, reset := gtkIMMulticontextNew, gtkIMContextFocusIn, gtkIMContextFocusOut, gtkIMContextReset
	setWindow, setCursor, filter := gtkIMContextSetClientWindow, gtkIMContextSetCursorLoc, gtkIMContextFilterKeypress
	getWindow, hasFocus := gtkWidgetGetWindow, gtkWidgetHasFocus
	connectSignal, ref, unref := gSignalConnectData, gObjectRef, gObjectUnref
	setInt := gObjectSetInt
	t.Cleanup(func() {
		gtkIMMulticontextNew, gtkIMContextFocusIn, gtkIMContextFocusOut, gtkIMContextReset = newContext, focusIn, focusOut, reset
		gtkIMContextSetClientWindow, gtkIMContextSetCursorLoc, gtkIMContextFilterKeypress = setWindow, setCursor, filter
		gtkWidgetGetWindow, gtkWidgetHasFocus = getWindow, hasFocus
		gSignalConnectData, gObjectRef, gObjectUnref = connectSignal, ref, unref
		gObjectSetInt = setInt
	})
	s := &surface{w: &window{id: 1}, area: 99}
	refs := map[ptr]int{}
	properties := map[ptr]map[string]int32{}
	gObjectSetInt = func(im ptr, name *byte, value int32, end ptr) {
		if properties[im] == nil {
			properties[im] = map[string]int32{}
		}
		properties[im][goStr(ptr(unsafe.Pointer(name)))] = value
	}
	var allocated ptr
	gtkIMMulticontextNew = func() ptr { allocated++; refs[allocated] = 1; return allocated }
	gObjectRef = func(im ptr) ptr { refs[im]++; return im }
	gObjectUnref = func(im ptr) {
		refs[im]--
		if refs[im] < 0 {
			t.Fatal("context released twice")
		}
	}
	gSignalConnectData = func(ptr, *byte, ptr, ptr, ptr, int32) uint64 { return 1 }
	gtkWidgetGetWindow = func(ptr) ptr { return 100 }
	gtkWidgetHasFocus = func(ptr) bool { return true }
	gtkIMContextSetClientWindow = func(ptr, ptr) {}
	gtkIMContextSetCursorLoc = func(ptr, *gdkRectangle) {}
	gtkIMContextFocusIn = func(im ptr) {
		if len(properties[im]) != 2 {
			t.Fatal("input purpose not configured before focus")
		}
		if !s.acceptIMInput(im) {
			t.Fatal("new context cannot retrieve its input")
		}
	}
	gtkIMContextFocusOut = func(im ptr) {
		if s.acceptIMInput(im) {
			t.Fatal("retired focus-out still accepts input")
		}
	}
	gtkIMContextReset = func(im ptr) {
		if !s.clientComposition.Canceling() || s.acceptIMInput(im) {
			t.Fatal("reset can deliver old input")
		}
	}
	first := platform.TextInputState{Active: true, FunctionalTarget: 1}
	s.SetTextInput(first)
	if allocated != 1 || !s.acceptIMInput(1) {
		t.Fatal("initial context missing")
	}
	if properties[1]["input-purpose"] != 10 || properties[1]["input-hints"] != 2 {
		t.Fatal("terminal input purpose missing")
	}
	first.Caret.X = 20
	s.SetTextInput(first)
	if allocated != 1 {
		t.Fatal("geometry change replaced context")
	}
	next := first
	next.FunctionalTarget = 2
	s.SetTextInput(next)
	if allocated != 2 || refs[1] != 0 || s.acceptIMInput(1) || !s.acceptIMInput(2) {
		t.Fatal("target switch retained old identity")
	}
	gtkIMContextFilterKeypress = func(im, event ptr) bool {
		next.FunctionalTarget = 3
		s.SetTextInput(next)
		if refs[im] != 1 {
			t.Fatal("in-flight context was freed")
		}
		return false
	}
	if !s.filterIMKeypress(123) || refs[2] != 0 {
		t.Fatal("old key escaped or native reference leaked")
	}
	s.SetTextInput(platform.TextInputState{})
	if s.im.Handle() != 0 || refs[3] != 0 || s.acceptIMInput(3) {
		t.Fatal("inactive input retained old context")
	}
	// Two editable clients also need separate identities, even without a
	// functional terminal ID.
	a := &struct{ platform.TextInputClient }{}
	b := &struct{ platform.TextInputClient }{}
	s.SetTextInput(platform.TextInputState{Active: true, Client: a})
	old := s.im.Handle()
	s.SetTextInput(platform.TextInputState{Active: true, Client: b})
	if s.im.Handle() == old || refs[old] != 0 || s.acceptIMInput(old) {
		t.Fatal("client switch shared context")
	}
	current := s.im.Handle()
	s.SetTextInput(platform.TextInputState{Active: true, Client: b, Password: true, NoSpellcheck: true})
	if s.im.Handle() != current || properties[current]["input-purpose"] != 8 || properties[current]["input-hints"] != 2 {
		t.Fatal("password purpose reset context or was not applied")
	}
	s.SetTextInput(platform.TextInputState{Active: true, Client: b, NoSpellcheck: true})
	if s.im.Handle() != current || properties[current]["input-purpose"] != 0 || properties[current]["input-hints"] != 2 {
		t.Fatal("revealed password lost no-spellcheck hint")
	}
	s.SetTextInput(platform.TextInputState{Active: true, Client: b})
	if properties[current]["input-hints"] != 0 {
		t.Fatal("plain input retained password hints")
	}
	s.w.closed = true
	s.im.Replace(0, s.retireIM)
	before := allocated
	s.SetTextInput(first)
	if allocated != before || s.im.Handle() != 0 {
		t.Fatal("closed surface recreated input context")
	}
	for im, n := range refs {
		if n != 0 {
			t.Fatalf("context %d leaked %d references", im, n)
		}
	}
}

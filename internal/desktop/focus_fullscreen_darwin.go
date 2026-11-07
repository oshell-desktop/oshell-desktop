//go:build darwin

package desktop

import (
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

type nativeFocusChrome struct {
	registered objc.ID
	saved      bool
	title      int64
	fullSize   bool
	hidden     [3]bool
}

func (s *nativeFocusChrome) apply(w *mygo.Window, enabled bool) {
	if w == nil {
		return
	}
	if enabled {
		s.prepare(w, true)
		return
	}
	window := objc.ID(w.NativeHandle())
	if window == 0 {
		return
	}
	sel := objc.RegisterName
	if s.saved && !w.IsFullScreen() {
		window.Send(sel("setTitleVisibility:"), s.title)
		mask := objc.Send[uint64](window, sel("styleMask"))
		if !s.fullSize {
			window.Send(sel("setStyleMask:"), mask&^(1<<15))
		}
		for i, hidden := range s.hidden {
			window.Send(sel("standardWindowButton:"), uint64(i)).Send(sel("setHidden:"), hidden)
		}
		s.saved = false
	}
}

func (s *nativeFocusChrome) prepare(w *mygo.Window, hideButtons bool) {
	window := objc.ID(w.NativeHandle())
	if window == 0 {
		return
	}
	sel := objc.RegisterName
	mask := objc.Send[uint64](window, sel("styleMask"))
	if !s.saved {
		s.saved = true
		s.title = objc.Send[int64](window, sel("titleVisibility"))
		s.fullSize = mask&(1<<15) != 0
		for i := range s.hidden {
			s.hidden[i] = objc.Send[bool](window.Send(sel("standardWindowButton:"), uint64(i)), sel("isHidden"))
		}
	}
	// Ordinary green-button fullscreen retains the system buttons on reveal.
	for i, hidden := range s.hidden {
		window.Send(sel("standardWindowButton:"), uint64(i)).Send(sel("setHidden:"), hideButtons || hidden)
	}
	window.Send(sel("setTitleVisibility:"), int64(1))
	window.Send(sel("setStyleMask:"), mask|1<<15)
}

// These selectors are absent from the pinned mygo delegate. Add only missing
// callbacks, without replacing its resize, focus or completed-transition methods.
var focusWindows = map[objc.ID]*App{}
var focusDelegateClasses = map[objc.Class]bool{}

func (a *App) registerFocusWindow() {
	if a.win == nil {
		return
	}
	window := objc.ID(a.win.NativeHandle())
	if window == 0 {
		return
	}
	a.focusFullscreen.native.registered = window
	focusWindows[window] = a
	class := window.Send(objc.RegisterName("delegate")).Class()
	if focusDelegateClasses[class] {
		return
	}
	focusDelegateClasses[class] = true
	class.AddMethod(objc.RegisterName("windowWillEnterFullScreen:"), objc.NewIMP(func(_ objc.ID, _ objc.SEL, n objc.ID) {
		window := n.Send(objc.RegisterName("object"))
		if owner := focusWindows[window]; owner != nil {
			owner.focusFullscreen.transitioning = true
			owner.focusFullscreen.target = true
			owner.focusFullscreen.native.prepare(owner.win, owner.focusFullscreen.chrome)
		}
	}), "v@:@")
	class.AddMethod(objc.RegisterName("windowWillExitFullScreen:"), objc.NewIMP(func(_ objc.ID, _ objc.SEL, n objc.ID) {
		window := n.Send(objc.RegisterName("object"))
		if owner := focusWindows[window]; owner != nil {
			owner.focusFullscreen.transitioning = true
			owner.focusFullscreen.target = false
			if owner.focusFullscreen.chrome {
				owner.focusFullscreen.requested = false
			}
		}
	}), "v@:@")
	for _, entering := range []bool{true, false} {
		name := "windowDidFailToExitFullScreen:"
		if entering {
			name = "windowDidFailToEnterFullScreen:"
		}
		class.AddMethod(objc.RegisterName(name), objc.NewIMP(func(_ objc.ID, _ objc.SEL, w objc.ID) {
			if owner := focusWindows[w]; owner != nil {
				owner.focusFullscreen.transitioning = false
				owner.focusFullscreen.requested = !entering
				if entering {
					owner.focusFullscreen.native.apply(owner.win, false)
				}
				owner.deferFullscreenReconcile()
			}
		}), "v@:@")
	}
}
func (a *App) unregisterFocusWindow() {
	delete(focusWindows, a.focusFullscreen.native.registered)
	a.focusFullscreen.native.registered = 0
}

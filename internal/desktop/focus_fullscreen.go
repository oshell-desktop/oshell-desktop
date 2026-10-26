package desktop

import (
	"fmt"
	"github.com/egoist/mygo"
	"os"
)

// Presentation state belongs to the window, never the saved configuration.
// requested can change during an AppKit animation; settled reconciles it later.
type focusFullscreenState struct {
	deep                                     bool
	requested, chrome, transitioning, target bool
	native                                   nativeFocusChrome
}

func (a *App) toggleDeepFocus() {
	if a.workspaceLocked() || a.modalActive() || a.settingsOpen {
		return
	}
	f := &a.focusFullscreen
	f.deep = !f.deep
	if f.deep {
		f.requested = true
	}
	a.reconcileFocusFullscreen()
	if p := a.activePane(); p != nil {
		p.focus.Focus()
	}
}

func (a *App) toggleFocusFullscreen() {
	if a.workspaceLocked() || a.modalActive() || a.settingsOpen || a.win == nil {
		return
	}
	a.traceFullscreen("toggle")
	a.registerFocusWindow()
	a.focusFullscreen.requested = !a.focusFullscreen.requested
	if !a.focusFullscreen.requested {
		a.focusFullscreen.deep = false
	}
	a.reconcileFocusFullscreen()
}
func (a *App) reconcileFocusFullscreen() {
	f := &a.focusFullscreen
	if a.win == nil || a.stopped.Load() {
		return
	}
	if f.requested && !f.chrome {
		f.native.apply(a.win, true)
		f.chrome = true
		a.win.Invalidate()
	}
	if f.transitioning {
		return
	}
	native := a.win.IsFullScreen()
	a.traceFullscreen("reconcile")
	if f.requested != native && (f.requested || f.chrome) {
		f.transitioning = true
		f.target = f.requested
		a.win.SetFullScreen(f.target)
	} else if !f.requested && f.chrome {
		f.native.apply(a.win, false)
		f.chrome = false
		a.win.Invalidate()
	}
}
func (a *App) fullscreenSettled(entered bool) {
	f := &a.focusFullscreen
	a.traceFullscreen(fmt.Sprintf("settled:%v", entered))
	// Exiting via the native window controls also exits focus presentation.
	// A queued re-entry during our own exit animation keeps its explicit request.
	if !entered && (!f.transitioning || f.target) {
		f.requested = false
		f.deep = false
	}
	f.transitioning = false
	a.deferFullscreenReconcile()
}

func (a *App) deferFullscreenReconcile() {
	if a.win == nil {
		a.post(a.reconcileFocusFullscreen)
		return
	}
	// AppKit owns the animation transaction until the delegate callback returns.
	go mygo.RunOnMain(func() {
		if !a.stopped.Load() {
			a.reconcileFocusFullscreen()
		}
	})
}

func (a *App) traceFullscreen(event string) {
	if os.Getenv("OSHELL_FULLSCREEN_TRACE") != "1" {
		return
	}
	f := a.focusFullscreen
	fmt.Fprintf(os.Stderr, "fullscreen %s requested=%v chrome=%v transition=%v target=%v\n", event, f.requested, f.chrome, f.transitioning, f.target)
}

package ui

import "github.com/egoist/mygo/internal/platform"

// WindowInteraction describes activity that can dismiss a non-modal hover card.
// It deliberately contains no typed text and cannot consume the input event.
// X/Y are window-content coordinates for pointer presses and scroll events.
type WindowInteraction struct {
	Keyboard, Blur, Pointer bool
	X, Y                    float32
}

// ObserveInteractions installs the window's observer for this frame. Register
// it every frame; omitting it removes the previous observer. It runs on the UI
// thread before normal dispatch, including input consumed by a focused editor.
// The callback may update application state, but must not rebuild the UI.
func (c *Context) ObserveInteractions(fn func(WindowInteraction)) {
	if current := c.build(); current != nil {
		current.rt.interactionObserver = fn
	}
}

// WindowFocused reports whether this view's window currently has keyboard focus.
func (c *Context) WindowFocused() bool {
	return c.runtime() != nil && c.rt.windowFocused
}

func (rt *engine) observeInteraction(ev platform.SurfaceEvent) {
	if rt.interactionObserver == nil {
		return
	}
	var activity WindowInteraction
	switch ev.Kind {
	case platform.KeyPressed, platform.TextInput, platform.TextComposition, platform.SurfaceCommand:
		activity.Keyboard = true
	case platform.SurfaceBlur:
		activity.Blur = true
	case platform.PointerDown, platform.PointerScroll:
		activity.Pointer, activity.X, activity.Y = true, float32(ev.X), float32(ev.Y)
	default:
		return
	}
	rt.interactionObserver(activity)
}

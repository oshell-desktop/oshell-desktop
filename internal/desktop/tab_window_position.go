package desktop

import "github.com/egoist/mygo"

// Coordinates use the platform's window placement space: physical pixels on
// Windows, logical screen coordinates on Linux. Negative monitor origins and
// (0, 0) are valid drop locations, not requests to center the window.
type tabWindowPosition struct{ x, y int }

var captureTabWindowPosition = currentTabWindowPosition

func (a *App) queueDetachedTab(t *tab) {
	// Capture on release, before queuing work: the pointer can move again while
	// the new window is being created.
	position := captureTabWindowPosition()
	if position != nil && a.hub != nil {
		// The native drag may finish with no registered drop target, e.g.
		// over a Windows title bar. Resolve the topmost window at release,
		// before focus or window stacking can change.
		if dest := position.windowAtRelease(a.hub.windows); dest != nil && dest != a {
			g := dest.groupByID(dest.groups.active)
			a.post(func() {
				if a.tabByID(t.id) == t && dest.acceptsTabDrag(tabDrag{t.id}, g) {
					a.transferTabAt(t, dest, g.id, "", false)
				}
			})
			return
		}
	}
	if a.tabDropOverWorkspace(position) {
		return
	}
	a.post(func() {
		if !a.tabDropOverWorkspace(position) {
			a.detachTabAt(t, position)
		}
	})
}

func (a *App) tabDropOverWorkspace(position *tabWindowPosition) bool {
	if position == nil {
		return true
	}
	windows := []*App{a}
	if a.hub != nil {
		windows = a.hub.configurationWindows()
	}
	for _, app := range windows {
		for _, window := range []*mygo.Window{app.win, app.manager, app.preferencesWindow} {
			if window != nil && window.IsVisible() && !window.IsMinimized() && position.within(window) {
				return true
			}
		}
	}
	return false
}

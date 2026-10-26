package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

// A title double click duplicates a live session, never a connecting/ended tab
// or an idle local-tools page. Keep the original pane and saved profile intact.
func canDuplicateConnectedTab(t *tab) bool {
	if t == nil || t.active == nil || t.active.closed {
		return false
	}
	p := t.active
	if p.term != nil {
		if p.term.InLocalToolsMode() {
			return false
		}
		return p.profile.Kind == "local" && p.term.LocalProcessRunning() || p.conn != nil && p.conn.Alive()
	}
	return p.files != nil && p.files.client != nil && (p.conn != nil && p.conn.Alive() || p.ftp != nil && p.status == "已连接")
}
func (a *App) duplicateConnectedTab(t *tab) {
	if !canDuplicateConnectedTab(t) || a.tabByID(t.id) != t || a.modalActive() || a.passwordLocked() {
		return
	}
	// Select explicitly: a background group's title must not duplicate into the
	// previously focused group. Split tabs duplicate their current active pane.
	a.selectTab(t)
	p := t.active
	if p.term == nil && p.files != nil {
		a.duplicateFileSession()
		return
	}
	if p.externalLaunch {
		if p.conn != nil && p.conn.Alive() {
			a.openConnection(p.profile, core.Credentials{OpenTerminal: p.terminalOnly, ExternalLaunch: true}, -2, p.conn)
		}
		return
	}
	a.request(p.profile, -2, p.terminalOnly)
}

// Commit the double-click on release. Otherwise a synchronous local-session
// creation on the second press is undone by the source title's release click.
func (a *App) tabTitleEvents(title ui.Element, t *tab) {
	state := ui.Local(title, "duplicate-on-release", func() struct{ pending, suppressClick bool } { return struct{ pending, suppressClick bool }{} })
	title.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind == ui.InputPointerDown || ev.Kind == ui.InputPointerCancel {
			state.pending = false
			state.suppressClick = false
		}
		if ev.Kind == ui.InputPointerUp && ev.Button == 0 && state.pending {
			state.pending = false
			state.suppressClick = true
			a.duplicateConnectedTab(t)
			return true
		}
		return false
	})
	if title.DoubleClicked() {
		state.pending = true
	}
	if title.Clicked() {
		if state.suppressClick {
			state.suppressClick = false
		} else {
			a.selectTab(t)
		}
	}
}

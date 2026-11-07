package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

// Keep both UI engines alive after the move: model-only tests never bind the
// handle in two windows and therefore missed the production close-tab panic.
func TestMovedTerminalFocusLifecycle(t *testing.T) {
	for _, action := range []string{"close-tab", "select-tab", "search-close", "round-trip", "command-editor", "split-pane"} {
		t.Run(action, func(t *testing.T) {
			first := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
			second := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
			moving, connection := operatorPane(t, first, "moving")
			movedTab := first.tabs[0]
			original := moving.term
			operatorPane(t, first, "source stays open")
			var sibling *pane
			if action == "split-pane" {
				sibling, _ = operatorPane(t, first, "split sibling")
				movedTab.root = &node{id: core.NewID(), first: movedTab.root, second: first.tabs[2].root, size: 250}
				first.tabs = first.tabs[:2]
			}
			other, _ := operatorPane(t, second, "destination tab")
			otherTab := second.tabs[0]
			first.ensureGroups()
			second.ensureGroups()
			var leftContext, rightContext *ui.Context
			content := func(a *App, context **ui.Context) func(*ui.Context) {
				return func(c *ui.Context) {
					*context = c
					a.refreshPaneVisibility()
					ui.Column(c).Fill().Children(func() {
						if tab := a.activeTab(); tab != nil {
							if tab.zoomed != nil {
								a.nodeView(c, tab, &node{pane: tab.zoomed})
							} else {
								a.nodeView(c, tab, tab.root)
							}
						}
						if a.config.Settings.QuickSendVisible {
							a.sendBar(c)
						}
					})
				}
			}
			left := ui.NewTester(content(first, &leftContext), 600, 350)
			right := ui.NewTester(content(second, &rightContext), 600, 350)
			first.selectTab(movedTab)
			left.Frame()
			if !moving.focus.FocusWithin(leftContext) {
				t.Fatal("fixture did not focus source terminal")
			}
			if !first.transferTabAt(movedTab, second, "", "", false) {
				t.Fatal("move rejected")
			}
			left.Frame()
			right.Frame()
			// The destination must receive the request even before its first bind.
			if !moving.focus.FocusWithin(rightContext) {
				t.Error("move did not focus destination terminal")
			}
			switch action {
			case "close-tab", "command-editor":
				second.selectTab(otherTab)
				right.Frame()
				if action == "command-editor" {
					second.config.Settings.QuickSendVisible = true
					right.Frame()
					second.lastInputFocus = second.features.operators.quickFocus
					second.lastInputFocus.Focus()
					right.Frame()
				}
				second.finishSessionClose(&sessionCloseRequest{tab: otherTab})
				right.Frame()
				if !other.closed || second.activeTab() != movedTab {
					t.Fatal("close did not select the surviving moved tab")
				}
			case "select-tab":
				second.selectTab(otherTab)
				right.Frame()
				second.selectTab(movedTab)
			case "search-close":
				second.openSearch()
				right.Frame()
				second.closeTerminalSearch(moving)
			case "split-pane":
				second.cyclePane(1)
				right.Frame()
				if !sibling.focus.FocusWithin(rightContext) || sibling.focus.FocusWithin(leftContext) {
					t.Fatal("split sibling retained its source binding")
				}
				second.togglePaneZoom()
				right.Frame()
				second.togglePaneZoom()
				right.Frame()
				second.cyclePane(1)
			case "round-trip":
				for i := 0; i < 3; i++ {
					second.transferTab(movedTab, first)
					right.Frame()
					left.Frame()
					if !moving.focus.FocusWithin(leftContext) {
						t.Fatal("return move lost focus")
					}
					first.transferTab(movedTab, second)
					left.Frame()
					right.Frame()
				}
			}
			right.Frame()
			left.Frame()
			if action == "command-editor" {
				if !second.features.operators.quickFocus.FocusWithin(rightContext) {
					t.Fatal("close stole command editor focus")
				}
				second.selectTab(movedTab)
				right.Frame()
			}
			if !moving.focus.FocusWithin(rightContext) || moving.focus.FocusWithin(leftContext) {
				t.Fatal("focus is not exclusive to the destination")
			}
			if moving.term != original || moving.closed || moving.owner.Load() != second {
				t.Fatal("move replaced or closed the terminal")
			}
			right.Type("destination-input")
			waitOperator(t, second, func() bool { return connection.text() != "" })
			if connection.text() != "destination-input" {
				t.Fatalf("destination keyboard input mismatch: %q", connection.text())
			}
		})
	}
}

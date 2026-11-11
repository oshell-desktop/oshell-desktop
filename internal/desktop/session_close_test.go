package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"testing"
)

func TestCloseTabsFollowsDisplayedOrder(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	for _, name := range []string{"A", "B", "C", "D", "E"} {
		a.tabs = append(a.tabs, &tab{id: name, name: name})
	}
	first, second, third, fourth, last := a.tabs[0], a.tabs[1], a.tabs[2], a.tabs[3], a.tabs[4]
	a.selectTab(fourth)
	a.moveTabRelative(second.id, third.id, true) // A C B D E
	for _, step := range []struct{ close, want *tab }{
		{first, fourth}, // Closing a background tab preserves focus.
		{fourth, second}, {second, third}, {third, last}, {last, nil},
	} {
		a.requestSessionClose(step.close, nil)
		if got := a.activeTab(); got != step.want {
			t.Fatalf("closing %s selected %v, want %v", step.close.name, got, step.want)
		}
	}
}

func TestCloseTabUsesNeighborsInVisibleGroup(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	for _, name := range []string{"left", "other-first", "right", "other-last"} {
		a.tabs = append(a.tabs, &tab{id: name, name: name})
	}
	left, otherFirst, right, otherLast := a.tabs[0], a.tabs[1], a.tabs[2], a.tabs[3]
	a.ensureGroups()
	other, err := a.createGroup("other")
	if err != nil {
		t.Fatal(err)
	}
	a.moveToGroup(otherFirst, other)
	a.moveToGroup(otherLast, other)
	a.selectTab(right)
	a.requestSessionClose(right, nil)
	if a.activeTab() != left {
		t.Fatal("selected a tab in the other strip instead of the left neighbor")
	}
	a.requestSessionClose(left, nil)
	if a.activeTab() != otherFirst || a.groupTab(other) != otherFirst {
		t.Fatal("empty strip did not transfer focus to the first tab of the remaining strip")
	}
}

func TestActiveSplitCloseRequiresConfirmationAndKeepsSibling(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	first, _ := operatorPane(t, a, "first")
	second, _ := operatorPane(t, a, "second")
	tab := a.tabs[0]
	tab.root = &node{id: core.NewID(), first: tab.root, second: a.tabs[1].root, size: 300}
	tab.active = first
	a.tabs = a.tabs[:1]
	a.requestSessionClose(tab, first)
	if a.features.sessionClose == nil || first.closed || second.closed {
		t.Fatal("active split closed without confirmation")
	}
	tt := ui.NewTester(a.sessionCloseDialog, 700, 400)
	if err := tt.Click("取消关闭"); err != nil {
		t.Fatal(err)
	}
	if first.closed || a.features.sessionClose != nil {
		t.Fatal("cancel closed split")
	}
	a.requestSessionClose(tab, first)
	tt.Frame()
	if err := tt.Click("确认关闭会话"); err != nil {
		t.Fatal(err)
	}
	if !first.closed || second.closed || len(a.tabs) != 1 || a.tabs[0].active != second {
		t.Fatal("wrong split closed")
	}
}
func TestCloseConfirmationCapturesTabAndRejectsMovedTarget(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	other := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, _ := operatorPane(t, a, "moving")
	tab := a.tabs[0]
	a.requestSessionClose(tab, nil)
	request := a.features.sessionClose
	a.transferTab(tab, other)
	a.finishSessionClose(request)
	if p.closed || len(other.tabs) != 1 {
		t.Fatal("confirmation closed moved target")
	}
}
func TestIdleCloseNeedsNoPromptAndEscapeCancelsActiveClose(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, _ := operatorPane(t, a, "active")
	tab := a.tabs[0]
	a.requestSessionClose(tab, nil)
	tt := ui.NewTester(a.sessionCloseDialog, 700, 400)
	tt.Key(0, ui.KeyEscape)
	if a.features.sessionClose != nil || p.closed {
		t.Fatal("Escape did not cancel")
	}
	p.term.Close()
	waitOperator(t, a, func() bool { return !p.term.HasActiveProcess() })
	a.requestSessionClose(tab, nil)
	if len(a.tabs) != 0 || a.features.sessionClose != nil {
		t.Fatal("idle terminal requested confirmation")
	}
}

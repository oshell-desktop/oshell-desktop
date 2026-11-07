package desktop

import (
	"oshell-desktop/internal/core"
	"testing"
)

func TestArrangementsSplitEveryTabAndRetainActiveGroupMetadata(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	for _, name := range []string{"one", "two", "three"} {
		operatorPane(t, a, name)
	}
	a.ensureGroups()
	named, err := a.createGroup("production")
	if err != nil {
		t.Fatal(err)
	}
	a.moveToGroup(a.tabs[1], named)
	a.moveToGroup(a.tabs[2], named)
	named.active = a.tabs[1].id
	a.hideGroup(named, true)
	empty, err := a.createGroup("empty")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"horizontal", "vertical", "tile"} {
		a.arrangeTabs(mode)
		if len(a.groups.root.leaves()) != len(a.tabs) || a.groupByID(empty.id) != nil {
			t.Fatal("arrangement kept empty/grouped layout")
		}
		if a.groupByID(a.tabs[1].group) != named || named.hidden {
			t.Fatal("active group metadata lost or hidden")
		}
		for _, tab := range a.tabs {
			g := a.groupByID(tab.group)
			if g == nil || len(a.groupTabs(g)) != 1 || tab.active.closed || !tab.active.term.Available() {
				t.Fatal("arrangement lost tab/connection")
			}
		}
	}
	a.arrangeTabs("tabs")
	if len(a.groups.root.leaves()) != 1 || a.groups.root.group.name != "" {
		t.Fatal("merge did not clear custom layout")
	}
}
func TestContextSplitMovesExistingTabWithoutReconnect(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	operatorPane(t, a, "one")
	operatorPane(t, a, "two")
	a.ensureGroups()
	a.selectTab(a.tabs[0])
	moving := a.tabs[1]
	term := moving.active.term
	target := a.tabSplitTarget(moving)
	if target == nil {
		t.Fatal("split unavailable")
	}
	a.dropTab(moving, target, "right")
	if len(a.groups.root.leaves()) != 2 || moving.active.term != term || moving.active.closed {
		t.Fatal("split replaced session")
	}
	if a.tabSplitTarget(moving) != nil {
		t.Fatal("single current tab offered empty split")
	}
}

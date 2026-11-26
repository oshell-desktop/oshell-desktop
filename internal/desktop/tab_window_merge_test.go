package desktop

import (
	"context"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func mergeWindowFixture(t *testing.T) (*App, *App, *applicationHub) {
	t.Helper()
	s := &core.Store{Dir: t.TempDir()}
	a, b := newApp(s, core.DefaultConfig()), newApp(s, core.DefaultConfig())
	h := &applicationHub{store: s, windows: []*App{a, b}}
	a.hub, b.hub = h, h
	return a, b, h
}

func TestCrossWindowTabInsertionPreservesSplitAndFileTransfer(t *testing.T) {
	a, b, h := mergeWindowFixture(t)
	p, conn := operatorPane(t, a, "moving")
	p2, _ := operatorPane(t, a, "split")
	left, _ := operatorPane(t, b, "left")
	right, _ := operatorPane(t, b, "right")
	a.ensureGroups()
	b.ensureGroups()
	moving := a.tabs[0]
	moving.root = &node{id: core.NewID(), first: moving.root, second: a.tabs[1].root, splitRatio: .35}
	a.tabs = a.tabs[:1]
	originalRoot, originalTerm := moving.root, p.term
	a.features.syncTargets = map[string]bool{p.id: true, p2.id: true}
	a.features.operators.composerTargets = map[string]bool{p.id: true}
	a.features.operators.selected = map[string]bool{p2.id: true}
	fileApp := newApp(a.store, a.config.Clone())
	fileApp.hub, h.fileWindow = h, fileApp
	filePane, _ := operatorPane(t, fileApp, "independent transfer")
	transferContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	filePane.files = &fileState{busy: true, cancel: cancel}
	g := b.groupForNewTab()
	b.receiveTabDrag(tabDrag{moving.id}, g, b.tabs[1].id, false)
	if len(a.tabs) != 0 || !a.closeAfterTabMove || len(b.tabs) != 3 || b.tabs[1] != moving || b.tabs[0].active != left || b.tabs[2].active != right || b.activeTab() != moving {
		t.Fatal("incorrect window/group insertion or source close state")
	}
	if moving.root != originalRoot || p.term != originalTerm || moving.root.splitRatio != .35 || p.owner.Load() != b || p2.owner.Load() != b || p.closed || p2.closed {
		t.Fatal("move rebuilt or closed the split")
	}
	if a.features.syncTargets[p.id] || a.features.syncTargets[p2.id] || a.features.operators.composerTargets[p.id] || a.features.operators.selected[p2.id] {
		t.Fatal("old window kept moved input targets")
	}
	if h.fileWindow != fileApp || !filePane.files.busy || filePane.closed || transferContext.Err() != nil {
		t.Fatal("last-tab move disturbed an independent transfer")
	}
	if !postPane(p, func(owner *App) {
		if owner != b {
			t.Error("callback routed to old window")
		}
		owner.status = "moved callback"
	}) {
		t.Fatal("moved callback rejected")
	}
	b.drain()
	if b.status != "moved callback" {
		t.Fatal("moved callback did not run")
	}
	p.term.Send([]byte("after move"))
	waitOperator(t, b, func() bool { return conn.text() == "after move" })
}

func TestCrossWindowTabDropTargetsAndRejection(t *testing.T) {
	for _, mode := range []string{"empty", "after", "group", "hidden", "missing-target", "wrong-group", "locked", "modal", "duplicate", "foreign-hub", "stale"} {
		t.Run(mode, func(t *testing.T) {
			a, b, _ := mergeWindowFixture(t)
			p, _ := operatorPane(t, a, "moving")
			moving := a.tabs[0]
			a.ensureGroups()
			b.ensureGroups()
			g := b.groupForNewTab()
			target := ""
			if mode != "empty" {
				operatorPane(t, b, "existing")
				b.ensureGroups()
				target = b.tabs[0].id
			}
			if mode == "group" || mode == "wrong-group" {
				leaf := groupLeaf("target")
				b.groups.root = &groupNode{id: core.NewID(), first: b.groups.root, second: leaf}
				g = leaf.group
				if mode == "group" {
					target = ""
				}
			}
			switch mode {
			case "hidden":
				g.hidden = true
			case "missing-target":
				target = "missing"
			case "locked":
				if err := b.vault.SetMaster("fixture-master"); err != nil {
					t.Fatal(err)
				}
				b.vault.Lock()
			case "modal":
				b.editing = true
			case "duplicate":
				b.tabs[0].id = moving.id
			case "foreign-hub":
				b.hub = &applicationHub{windows: []*App{b}}
			case "stale":
				a.tabs = nil
			}
			beforeA, beforeB := append([]*tab(nil), a.tabs...), append([]*tab(nil), b.tabs...)
			b.receiveTabDrag(tabDrag{moving.id}, g, target, mode == "after")
			success := mode == "empty" || mode == "after" || mode == "group"
			if success {
				if p.owner.Load() != b || moving.group != g.id || b.activeTab() != moving || !a.closeAfterTabMove {
					t.Fatal("accepted drop not applied")
				}
				if mode == "after" && b.tabs[len(b.tabs)-1] != moving {
					t.Fatal("wrong after insertion")
				}
			} else if !reflect.DeepEqual(a.tabs, beforeA) || !reflect.DeepEqual(b.tabs, beforeB) || p.owner.Load() != a {
				t.Fatal("rejected drop changed either window")
			}
		})
	}
}

func TestEmptyWindowExposesTabDropStrip(t *testing.T) {
	_, b, _ := mergeWindowFixture(t)
	view := ui.NewTester(b.view, 1000, 700)
	r, ok := view.Find("标签栏空白区域")
	if !ok || r.W < 24 || r.H < 25 {
		t.Fatal("empty workspace has no usable tab drop area")
	}
}

func TestDetachedTabDragBackToWorkspace(t *testing.T) {
	for _, target := range []string{"right", "标签栏空白区域", "会话侧栏"} {
		t.Run(target, func(t *testing.T) {
			original, detached, _ := mergeWindowFixture(t)
			operatorPane(t, original, "left")
			p, conn := operatorPane(t, original, "moving")
			operatorPane(t, original, "right")
			original.ensureGroups()
			moving := original.tabs[1]
			original.transferTab(moving, detached)
			// Put the two production views on one test surface so a pointer
			// gesture crosses their boundary without using the system pointer.
			view := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Children(func() {
					ui.Column(c).Grow(1).MinHeight(0).Children(func() { original.view(c.Key("original")) })
					detached.groupTabBar(c.Key("detached"), detached.groupForNewTab())
				})
			}, 1200, 700)
			from, ok := view.Find("moving")
			if !ok {
				t.Fatal("missing detached tab")
			}
			to, ok := view.Find(target)
			if !ok {
				t.Fatal("missing drop target", target)
			}
			view.Press(from.X+8, from.Y+12)
			view.Move(from.X+20, from.Y+12)
			view.Move(to.X+4, to.Y+12)
			view.Release(to.X+4, to.Y+12)
			view.Frame()
			wantIndex := 2
			if target == "right" {
				wantIndex = 1
			}
			if len(original.tabs) != 3 || original.tabs[wantIndex] != moving || original.activeTab() != moving ||
				len(detached.tabs) != 0 || !detached.closeAfterTabMove || p.owner.Load() != original {
				t.Fatal("drop did not return the live tab to the expected position")
			}
			p.term.Send([]byte("back"))
			waitOperator(t, original, func() bool { return conn.text() == "back" })
		})
	}
}

func TestTabDropOnBlankStripMovesToEnd(t *testing.T) {
	a, _, _ := mergeWindowFixture(t)
	operatorPane(t, a, "first")
	operatorPane(t, a, "second")
	operatorPane(t, a, "third")
	a.ensureGroups()
	moving, last := a.tabs[0], a.tabs[2]
	a.receiveTabDrag(tabDrag{moving.id}, a.groupForNewTab(), "", false)
	if a.tabs[1] != last || a.tabs[2] != moving || moving.active.owner.Load() != a || a.closeAfterTabMove {
		t.Fatal("blank strip did not reorder the existing tab to the end")
	}
}

func TestTabScrollDropStillDeliversAfterHoverEnds(t *testing.T) {
	a, _, _ := mergeWindowFixture(t)
	operatorPane(t, a, "first")
	operatorPane(t, a, "second")
	a.ensureGroups()
	moving, g := a.tabs[0], a.groupForNewTab()
	view := ui.NewTester(func(c *ui.Context) {
		a.drain()
		ui.Row(c).Children(func() {
			ui.Box(c).Size(80, 30).Drag(tabDrag{moving.id})
			scroll := ui.ScrollHorizontal(c).Size(240, 30).Children(func() { ui.Box(c).Size(20, 30) })
			a.tabDragScroll(c, scroll, g)
		})
	}, 400, 100)
	view.Press(10, 15)
	view.Move(30, 15)
	view.Move(220, 15)
	view.Release(220, 15)
	view.Frame()
	if a.tabs[1] != moving {
		t.Fatal("scroll container lost the drop after hover ended")
	}
}

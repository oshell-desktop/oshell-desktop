package desktop

import (
	"oshell-desktop/internal/core"
	"runtime"
	"testing"
	"time"
	"weak"
)

func closedTabReference(t *testing.T, source, destination *App) weak.Pointer[tab] {
	t.Helper()
	p, _ := operatorPane(t, source, "memory-fixture")
	tab := source.tabs[len(source.tabs)-1]
	reference := weak.Make(tab)
	owner := source
	if destination != nil {
		if !source.transferTabAt(tab, destination, "", "", false) {
			t.Fatal("tab move failed")
		}
		owner = destination
	}
	owner.closeTab(tab)
	p.disposal.Wait()
	owner.idleMemory.cancel()
	return reference
}

func TestClosedAndMovedTabsBecomeCollectable(t *testing.T) {
	for _, move := range []bool{false, true} {
		a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
		var b *App
		if move {
			b = newApp(a.store, core.DefaultConfig())
		}
		ref := closedTabReference(t, a, b)
		for i := 0; i < 10; i++ {
			runtime.GC()
			if ref.Value() == nil {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if ref.Value() != nil {
			t.Fatal("closed tab retained by backing storage", move)
		}
		runtime.KeepAlive(a)
		runtime.KeepAlive(b)
	}
}

func TestIdleMemoryReclaimAfterLastCloseAndCancelOnReopen(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	t.Cleanup(a.idleMemory.cancel)
	p, _ := operatorPane(t, a, "first")
	a.closeTab(a.tabs[0])
	p.disposal.Wait()
	if a.idleMemory.timer == nil {
		t.Fatal("last close did not schedule a pass")
	}
	old := a.idleMemory.generation
	a.open(core.Profile{ID: core.NewID(), Kind: "blank", Name: "reopened"}, core.Credentials{}, 0)
	a.reclaimIdleMemory(old) // Even an already queued timer must be invalidated.
	if a.idleMemory.running || a.idleMemory.completed != 0 || a.idleMemory.timer != nil {
		t.Fatal("reopen did not cancel pending reclamation")
	}
	a.closeTab(a.tabs[0])
	waitOperator(t, a, func() bool { return a.idleMemory.completed == 1 })
	if a.idleMemory.running || a.idleMemory.timer != nil {
		t.Fatal("reclaim did not finish")
	}
	a.reclaimIdleMemory(old)
	if a.idleMemory.running {
		t.Fatal("old callback ran twice")
	}
	// A disconnected pane still owns its terminal history and must block a pass.
	p, _ = operatorPane(t, a, "ended")
	p.status = "连接已结束"
	p.term.Feed([]byte("retained 中文 history"))
	a.scheduleIdleMemoryReclaim()
	if a.idleMemory.timer != nil || a.canReclaimIdleMemory() || p.term.Text() == "" {
		t.Fatal("ended tab lost history or triggered reclaim")
	}
	if p.term == nil {
		t.Fatal("reopening after reclamation failed")
	}
}

func TestIdleMemoryReclaimRespectsOtherWindows(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	b := newApp(a.store, core.DefaultConfig())
	h := &applicationHub{windows: []*App{a, b}}
	a.hub, b.hub = h, h
	t.Cleanup(a.idleMemory.cancel)
	t.Cleanup(b.idleMemory.cancel)
	p, _ := operatorPane(t, b, "active")
	a.scheduleIdleMemoryReclaim()
	if a.idleMemory.timer != nil {
		t.Fatal("reclaim scheduled with another window's terminal")
	}
	b.closeTab(b.tabs[0])
	p.disposal.Wait()
	old := b.idleMemory.generation
	a.open(core.Profile{ID: core.NewID(), Kind: "blank", Name: "new"}, core.Credentials{}, 0)
	b.reclaimIdleMemory(old)
	if b.idleMemory.running || b.idleMemory.timer != nil {
		t.Fatal("another window's reopen did not cancel the pass")
	}
	for _, tab := range a.tabs {
		a.walk(tab.root, a.dispose)
	}
}

package desktop

import (
	"runtime/debug"
	"time"
)

// State is owned by the UI thread. Reclamation happens off that thread and is
// process-wide, so another window's live/ended tabs also prevent a new pass.
type idleMemoryReclaimer struct {
	timer      *time.Timer
	generation uint64
	completed  int
	running    bool
}

func (r *idleMemoryReclaimer) cancel() {
	r.generation++
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
}

func (a *App) idleMemoryWindows() []*App {
	if a.hub != nil {
		return a.hub.windows
	}
	return []*App{a}
}

func (a *App) cancelIdleMemoryReclaim() {
	a.idleMemory.cancel()
	for _, w := range a.idleMemoryWindows() {
		if w != a {
			w.idleMemory.cancel()
		}
	}
}

func (a *App) canReclaimIdleMemory() bool {
	if a.stopped.Load() || len(a.tabs) != 0 || a.idleMemory.running {
		return false
	}
	for _, w := range a.idleMemoryWindows() {
		if len(w.tabs) != 0 || w.idleMemory.running {
			return false
		}
	}
	return true
}

func (a *App) scheduleIdleMemoryReclaim() {
	if !a.canReclaimIdleMemory() {
		return
	}
	a.cancelIdleMemoryReclaim()
	r := &a.idleMemory
	generation := r.generation
	r.timer = time.AfterFunc(2*time.Second, func() {
		a.post(func() { a.reclaimIdleMemory(generation) })
	})
}

func (a *App) reclaimIdleMemory(generation uint64) {
	r := &a.idleMemory
	if generation != r.generation || !a.canReclaimIdleMemory() {
		return
	}
	r.cancel() // Consume this pass, including a timer callback already queued.
	r.running = true
	go func() {
		// The native emulator is freed by Terminal.Close. This pass returns
		// unreachable Go search, shaping and transfer buffers to the OS.
		debug.FreeOSMemory()
		a.post(func() { r.running = false; r.completed++ })
	}()
}

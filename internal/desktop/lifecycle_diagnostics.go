package desktop

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"oshell-desktop/internal/diagnostics"
)

func traceNativeWindows() func() {
	var sequence int
	return mygo.App.OnWindowCreated(func(w *mygo.Window) {
		sequence++
		id := sequence
		emit := func(event string, stack bool) {
			fields := map[string]any{"window": id, "window_ref": fmt.Sprintf("%p", w), "native_windows": len(mygo.Windows())}
			if stack {
				fields["stack"] = diagnostics.Stack()
			}
			diagnostics.Event(event, fields)
		}
		emit("window_created", true)
		w.OnClose(func(*mygo.CloseEvent) { emit("window_close_requested", true) })
		w.OnClosed(func() { emit("window_closed", true) })
		w.OnHide(func() { emit("window_hidden", false) })
		w.OnShow(func() { emit("window_shown", false) })
		w.OnMinimize(func() { emit("window_minimized", false) })
		w.OnRestore(func() { emit("window_restored", false) })
	})
}

func (h *applicationHub) diagnosticSnapshot() map[string]any {
	fields := map[string]any{"native_windows": len(mygo.Windows()), "workspaces": len(h.windows), "file_window": h.fileWindow != nil, "connections": h.connectionCount(), "quitting": h.quitting, "quit_pending": h.quitPending, "config_busy": h.configBusy}
	tabs, panes, transfers := 0, 0, 0
	apps := append([]*App(nil), h.windows...)
	if h.fileWindow != nil {
		apps = append(apps, h.fileWindow)
	}
	for _, a := range apps {
		tabs += len(a.tabs)
		for _, t := range a.tabs {
			a.walk(t.root, func(p *pane) {
				panes++
				if p.transfer.active {
					transfers++
				}
			})
		}
	}
	fields["tabs"], fields["panes"], fields["zmodem_transfers"] = tabs, panes, transfers
	return fields
}

func (h *applicationHub) traceUI(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		var pending atomic.Bool
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !pending.CompareAndSwap(false, true) {
					diagnostics.Event("ui_heartbeat_delayed", nil)
					continue
				}
				queued := time.Now()
				mygo.RunOnMain(func() {
					defer pending.Store(false)
					if ctx.Err() != nil {
						return
					}
					fields := h.diagnosticSnapshot()
					fields["queue_delay_ms"] = time.Since(queued).Milliseconds()
					diagnostics.Event("ui_heartbeat", fields)
				})
			}
		}
	}()
}

func (a *App) traceCloseDecision(reason string) {
	diagnostics.Event("workspace_close_decision", map[string]any{"window_ref": fmt.Sprintf("%p", a.win), "reason": reason, "tabs": len(a.tabs), "active_connections": a.activeConnections(), "allow_close": a.features.allowClose})
}

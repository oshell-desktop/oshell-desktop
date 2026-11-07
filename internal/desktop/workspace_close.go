package desktop

import "github.com/egoist/mygo"

func (a *App) finishTabMoveClose() {
	if !a.closeAfterTabMove || a.win == nil || a.stopped.Load() {
		return
	}
	if len(a.tabs) != 0 {
		a.closeAfterTabMove = false
		return
	}
	if a.tabNativeDrag || a.features.sharedSync.busy || a.features.sessionPassword.busy || a.features.localConflict.active {
		return
	}
	a.closeAfterTabMove = false
	a.traceCloseDecision("empty_after_tab_move")
	a.win.Close()
}

// All workspace windows must keep processing completion events while a shared
// sync is running: committing holds the application's configuration busy flag.
func (a *App) onWorkspaceClose(event *mygo.CloseEvent) {
	if a.features.localConflict.active {
		a.traceCloseDecision("blocked_local_conflict")
		event.PreventDefault()
		a.cancelLocalConflict()
		return
	}
	if a.features.sessionPassword.busy {
		a.traceCloseDecision("blocked_password_operation")
		event.PreventDefault()
		a.cancelSessionPassword()
		return
	}
	if a.features.sharedSync.busy {
		a.traceCloseDecision("blocked_sync")
		event.PreventDefault()
		a.status = "配置同步正在进行，请等待完成或在同步设置中取消"
		return
	}
	if a.hub != nil && !a.hub.quitting && !a.features.allowClose && a.activeConnections() > 0 {
		a.traceCloseDecision("confirmation_required")
		event.PreventDefault()
		a.confirmExit(false)
		return
	}
	a.traceCloseDecision("allowed")
}

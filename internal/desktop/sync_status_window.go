package desktop

import (
	"context"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func (a *App) syncMonitorWindow() *mygo.Window {
	if a.hub != nil {
		for _, w := range a.hub.configurationWindows() {
			if w.syncMonitor != nil {
				return w.syncMonitor
			}
		}
	}
	return a.syncMonitor
}
func (a *App) showSyncStatus() {
	if monitor := a.syncMonitorWindow(); monitor != nil {
		monitor.Show()
		monitor.Focus()
		return
	}
	a.syncMonitorOpen = true
	if a.syncMonitor != nil {
		a.syncMonitor.Show()
		a.syncMonitor.Focus()
		return
	}
	if !mygo.App.IsReady() {
		return
	}
	w := mygo.NewWindow(mygo.WindowOptions{Title: "配置同步", Width: 720, Height: 520, MinWidth: 540, MinHeight: 400, Content: ui.View(a.syncStatusView)})
	a.syncMonitor = w
	w.OnClosed(func() {
		if a.syncMonitor == w {
			a.syncMonitor = nil
			a.syncMonitorOpen = false
			a.syncStatusOwner().features.sharedSync.logsOpen = false
		}
	})
}
func (a *App) syncStatusView(c *ui.Context) {
	a.drain()
	a.theme(c)
	defer a.syncStatusOwner().syncDiagnosticsDialog(c)
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() { a.syncStatusCard(c) })
		owner := a.syncStatusOwner()
		f := &owner.features.sharedSync
		if f.busy || f.phase == core.SyncCompleted || f.phase == core.SyncUnchanged {
			steps := map[core.SyncEvent]float64{core.SyncStarted: .15, core.SyncFetched: .4, core.SyncUploading: .65, core.SyncApplying: .85, core.SyncCompleted: 1, core.SyncUnchanged: 1}
			ui.Text(c, "阶段："+f.phase.Title()).Label("同步阶段")
			ui.Progress(c, steps[f.phase]).FillWidth()
		}
		ui.Row(c).Gap(8).Children(func() {
			ui.PrimaryButton(c, "立即同步").Disabled(a.syncOperationBusy() || a.configurationBusy() || a.passwordLocked()).OnClick(func() { owner.runSyncMenuAction(false) })
			ui.Button(c, "取消本次同步").Disabled(!f.busy && !f.manualQueued && !f.deferred).OnClick(owner.cancelSharedSync)
			ui.Button(c, "同步配置…").OnClick(func() { a.showPreferenceSection(3) })
		})
	})
}
func (a *App) cancelSharedSync() {
	owner := a.syncStatusOwner()
	f := &owner.features.sharedSync
	if owner.hub != nil {
		owner.hub.armSharedSync(0)
	}
	f.manualQueued, f.deferred = false, false
	f.status, f.phase = "本次同步已取消，本地修改保留", core.SyncCancelled
	if f.cancel != nil {
		f.cancel()
	}
	if !f.busy {
		if err := f.diagnostics.Record(core.SyncCancelled, true, 0, context.Canceled, ""); err != nil {
			f.logIssue = err.Error()
		}
		owner.scheduleSyncResult(context.Canceled)
	}
	owner.invalidateSyncPresentation()
}

package desktop

import (
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"time"
)

func (a *App) loadSyncDiagnostics() {
	f := &a.features.sharedSync
	doc, err := f.diagnostics.Snapshot()
	if err != nil {
		f.logIssue = err.Error()
		return
	}
	f.logIssue = f.diagnostics.WriteIssue()
	f.summary = f.diagnostics.SummaryForTarget(doc)
	f.logEntries = doc.Entries
	if !f.busy {
		f.phase = f.summary.Outcome
		if f.summary.Failure != nil && f.phase != core.SyncCancelled {
			f.phase = core.SyncFailed
		}
	}
	f.pendingLocal = f.summary.LocalFingerprint == "" || f.summary.LocalFingerprint != core.SyncLocalFingerprint(a.store, a.vault, f.savedOptions.Policy.Effective().Content)
	if !f.pendingLocal {
		f.queuedIdentity = ""
	}
}
func syncDate(value *time.Time) string {
	if value == nil {
		return "暂无"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}
func (a *App) syncStatusCard(c *ui.Context) {
	owner := a.syncStatusOwner()
	f := &owner.features.sharedSync
	p := a.syncPresentation(c)
	title := p.title
	ui.Column(c).Padding(12).Gap(7).FillWidth().Background(c.Theme().Surface).Radius(4).Children(func() {
		ui.Text(c, p.mark+" "+title).FontSize(16).Bold().TextColor(p.color)
		ui.Text(c, p.guidance).Wrap()
		if a.syncOptionsChanged() {
			ui.Text(c, "下方设置尚未应用；此卡片显示已保存配置的状态。").Wrap()
		}
		if a.passwordLocked() {
			ui.Button(c, "解锁主密码…").OnClick(func() {
				a.showVault()
				if a.win != nil {
					a.win.Focus()
					a.win.Invalidate()
				}
			})
		}
		if f.phase == core.SyncFailed && f.status != "" {
			ui.Text(c, f.status).Wrap().TextColor(c.Theme().Danger)
		}
		backend := "同步目录"
		if !f.savedEnabled {
			backend = "仅本机"
		} else if f.diagnostics.Backend == "webdav" {
			backend = "WebDAV"
		}
		ui.Text(c, backend+" · 使用已保存的同步配置").TextColor(c.Theme().TextMuted)
		ui.Text(c, syncPolicySummary(f.savedOptions.Policy)).Wrap().FontSize(11)
		if a.hub != nil && !a.hub.nextSync.IsZero() {
			ui.Text(c, "下次检查："+syncDate(&a.hub.nextSync)).FontSize(11)
		}
		ui.Text(c, "上次尝试："+syncDate(f.summary.LastAttempt)+"\n上次成功："+syncDate(f.summary.LastSuccess)).FontSize(11)
		pending := "本机没有待同步修改"
		if f.pendingLocal {
			pending = "本机修改尚待检查或同步"
		}
		ui.Text(c, pending).FontSize(11)
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, "查看同步日志…").OnClick(func() { owner.loadSyncDiagnostics(); owner.features.sharedSync.logsOpen = true })
			ui.Button(c, "导出诊断日志…").Disabled(f.logExportBusy).OnClick(a.exportSyncDiagnostics)
		})
		if f.logIssue != "" {
			ui.Text(c, f.logIssue+"；同步结果不受影响。").TextColor(c.Theme().TextMuted)
		}
	})
}
func (a *App) syncDiagnosticsDialog(c *ui.Context) {
	f := &a.features.sharedSync
	ui.Modal(c, &f.logsOpen, func() {
		width, height := c.Size()
		ui.Column(c).Width(min(float32(760), width-48)).Gap(10).Children(func() {
			ui.Text(c, "同步诊断日志").FontSize(18).Bold()
			ui.Text(c, "最近 7 天，最多 2000 条；不包含会话内容、地址、账号或凭据。").TextColor(c.Theme().TextMuted)
			ui.List(c, &f.logList, len(f.logEntries), func(i int) {
				entry := f.logEntries[len(f.logEntries)-1-i]
				text := entry.Date.Local().Format("01-02 15:04:05") + " · " + entry.Event.Title()
				if entry.Failure != nil {
					text += " · " + entry.Failure.Title()
					if entry.Failure.Code != 0 {
						text += fmt.Sprintf("（%d）", entry.Failure.Code)
					}
				}
				ui.Text(c, text).Padding(5, 7).FontSize(11).SingleLine()
			}).Height(min(float32(400), max(float32(180), height-200))).Border(1, c.Theme().Border)
			if len(f.logEntries) == 0 {
				ui.Text(c, "暂无同步记录")
			}
			ui.Button(c, "关闭").OnClick(func() { f.logsOpen = false })
		})
	})
}
func (a *App) exportSyncDiagnostics() {
	f := &a.features.sharedSync
	if f.logExportBusy {
		return
	}
	f.logExportBusy = true
	diagnostics, parent := f.diagnostics, a.preferencesWindow
	go func() {
		data, err := diagnostics.Export()
		path := ""
		if err == nil {
			path, err = mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: parent, Title: "导出同步诊断日志", DefaultPath: "OShell-sync-diagnostics.json"})
		}
		if err == nil && path != "" {
			err = core.AtomicWrite(path, data)
		}
		a.post(func() {
			f.logExportBusy = false
			if err != nil {
				f.logIssue = "诊断日志导出失败"
			} else if path != "" {
				f.status = "诊断日志已导出（不包含会话内容或凭据）"
				f.logIssue = diagnostics.WriteIssue()
			}
			if a.preferencesWindow != nil {
				a.preferencesWindow.Invalidate()
			}
		})
	}()
}

// Called after a successful local save, not while editing a settings draft.
func (a *App) markSharedSyncChanged() {
	options, err := a.loadSyncOptions()
	if err != nil || !options.Enabled {
		return
	}
	diagnostic := core.SyncDiagnostics{Directory: a.store.Dir, Target: options.Target(), Backend: options.Backend}
	fingerprint := core.SyncLocalFingerprint(a.store, a.vault, options.Policy.Effective().Content)
	if fingerprint == "" {
		return
	}
	doc, err := diagnostic.Snapshot()
	summary := core.SyncSummary{}
	if err == nil {
		summary = diagnostic.SummaryForTarget(doc)
	}
	if summary.LocalFingerprint == fingerprint {
		return
	}
	identity := options.Target() + "\n" + fingerprint
	if a.features.sharedSync.queuedIdentity == identity && (summary.Outcome == core.SyncQueued || summary.Outcome == core.SyncLocalChanged) {
		return
	}
	issue := ""
	if err = diagnostic.Record(core.SyncLocalChanged, false, 0, nil, ""); err != nil {
		issue = err.Error()
	}
	if err = diagnostic.Record(core.SyncQueued, false, 0, nil, ""); err != nil {
		issue = err.Error()
	}
	apps := []*App{a}
	if a.hub != nil {
		apps = append(apps, a.hub.windows...)
		if a.hub.fileWindow != nil {
			apps = append(apps, a.hub.fileWindow)
		}
	}
	seen := map[*App]bool{}
	for _, owner := range apps {
		if seen[owner] {
			continue
		}
		seen[owner] = true
		f := &owner.features.sharedSync
		f.pendingLocal = true
		f.phase = core.SyncQueued
		f.queuedIdentity = identity
		f.diagnostics = diagnostic
		f.savedEnabled = true
		if issue != "" {
			f.logIssue = issue
		}
		if owner.preferencesWindow != nil {
			owner.preferencesWindow.Invalidate()
		}
		if owner.win != nil {
			owner.win.Invalidate()
		}
	}
}

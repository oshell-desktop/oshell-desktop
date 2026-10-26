package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"time"
)

type syncPresentation struct {
	title, guidance, mark string
	color                 ui.Color
}

func (a *App) syncStatusOwner() *App {
	if a.hub != nil && a.hub.syncOwner != nil {
		return a.hub.syncOwner
	}
	return a
}
func (a *App) syncOperationBusy() bool {
	if a.syncStatusOwner().features.sharedSync.busy {
		return true
	}
	if a.hub != nil {
		for _, w := range a.hub.configurationWindows() {
			if w.features.sharedSync.busy {
				return true
			}
		}
	}
	return a.features.sharedSync.busy
}
func (a *App) invalidateSyncPresentation() {
	if a.hub == nil {
		return
	}
	a.hub.syncOwner = a
	for _, w := range a.hub.configurationWindows() {
		if w.win != nil {
			w.win.Invalidate()
		}
		if w.preferencesWindow != nil {
			w.preferencesWindow.Invalidate()
		}
		if w.syncMonitor != nil {
			w.syncMonitor.Invalidate()
		}
	}
}
func (a *App) syncPresentation(c *ui.Context) syncPresentation {
	owner := a.syncStatusOwner()
	f := &owner.features.sharedSync
	if !f.loaded && !f.busy {
		owner.loadSharedSyncSettings()
	}
	p := syncPresentation{"未启用同步", "选择一种同步方式；配置目前只保存在本机。", "−", c.Theme().TextMuted}
	if !f.savedEnabled {
		return p
	}
	p.color = c.Theme().Accent
	policy := f.savedOptions.Policy.Effective()
	switch {
	case a.passwordLocked():
		p.title, p.guidance, p.mark = "等待解锁主密码", "解锁后继续同步，本地修改已保留。", "◇"
	case f.busy:
		p.title, p.guidance, p.mark = "正在同步", "正在比较或应用共享配置，请稍候。", "↻"
		if f.phase == core.SyncStarted || f.phase == core.SyncFetched {
			p.title = "正在读取共享数据"
		}
		if f.phase == core.SyncUploading {
			p.title = "正在加密并上传"
		}
		if f.phase == core.SyncApplying {
			p.title = "正在应用共享数据"
			p.guidance = "正在更新本机配置，已连接的终端保持运行。"
		}
	case f.pendingDraft || f.plan != nil && (f.plan.InitialChoiceRequired || len(f.plan.Conflicts) > 0):
		p.title, p.guidance, p.mark, p.color = "同步需要确认", "关闭设置后同步，继续首次接入或冲突处理。", "!", c.Theme().Danger
	case f.phase == core.SyncFailed:
		p.title, p.guidance, p.mark, p.color = "同步未完成", "本地数据已保留；检查连接、证书和诊断日志后重试。", "!", c.Theme().Danger
		if f.summary.Failure != nil {
			switch f.summary.Failure.Kind {
			case "authentication":
				p.guidance = "请检查同步配置中的 WebDAV 账号及密码，应用后重试。"
			case "tls":
				p.guidance = "请在同步配置中测试连接，核对服务器证书后重试。"
			case "directoryUnavailable", "cloudConflict":
				p.guidance = "请检查同步目录是否可用、文件是否已下载，以及是否有冲突副本。"
			case "remoteDeleted":
				p.guidance = "共享文件已被删除；请确认目标数据后重新选择同步位置，不会自动覆盖。"
			case "network", "http":
				p.guidance = "请检查网络或服务端，修复后手动同步。"
				if policy.Automatic && policy.RetryInterval > 0 {
					p.guidance = "请检查网络或服务端；恢复后会自动重试，也可安排一次手动同步。"
				}
			default:
				p.guidance = "请查看错误详情或诊断日志，确认两端主密码与版本兼容后重试。"
			}
		}
	case f.phase == core.SyncCancelled:
		p.title, p.guidance, p.mark = "本次同步已取消", "本地修改已保留，可以重新安排同步。", "Ⅱ"
	case f.manualQueued || f.deferred || f.pendingLocal:
		p.title, p.guidance, p.mark = "等待同步", "使用已保存的配置；关闭设置后继续，未应用的草稿不会生效。", "◷"
		if !policy.Automatic && !f.manualQueued && !f.deferred {
			p.title = "等待手动同步"
			p.guidance = "本地修改已保存，请点击立即同步。"
		}
	case f.summary.LastSuccess != nil:
		p.title, p.guidance, p.mark, p.color = "同步完成", "本机与共享配置已对齐。", "✓", ui.Hex("#32B85A")
		if f.savedOptions.Backend == "directory" {
			p.title = "已与本机同步目录对齐"
			p.guidance = "目录文件已对齐；云端上传进度请在云盘客户端查看。"
		}
	default:
		p.title, p.guidance, p.mark = "等待同步", "将比较本机与共享配置，无需连接 SSH。", "◷"
		if !policy.Automatic {
			p.title = "仅手动同步"
			p.guidance = "点击立即同步，使用已保存的配置执行。"
		}
	}
	return p
}
func (a *App) syncToolbar(c *ui.Context) {
	p := a.syncPresentation(c)
	f := &a.syncStatusOwner().features.sharedSync
	status := map[string]string{"−": "未启用", "◇": "待解锁", "↻": "同步中", "!": "需处理", "Ⅱ": "已取消", "◷": "待同步", "✓": "已同步"}[p.mark]
	width, _ := c.Size()
	compact := width < 1120
	color := c.Theme().Accent
	if p.mark == "!" || p.mark == "✓" {
		color = p.color
	}
	button := ui.ButtonBase(c).Label("多设备同步").Role(ui.RoleButton).
		Tooltip(p.title+"\n最近成功："+syncDate(f.summary.LastSuccess)+"\n点击查看同步进度").
		Height(28).Padding(4, 8).Gap(5).Radius(6).Shrink(0).Cursor(ui.CursorPointer).
		Background(color.Alpha(.10)).Border(1, color.Alpha(.24)).
		OnClick(a.showSyncStatus)
	if button.Pressed() {
		button.Background(color.Alpha(.22))
	} else if button.Hovered() || button.FocusVisible() {
		button.Background(color.Alpha(.17)).BorderColor(color.Alpha(.50))
	}
	button.Children(func() {
		if f.busy && !a.localUI.ReduceMotion {
			ui.Spinner(c).Size(17, 17).TextColor(color)
		} else if p.mark == "✓" && f.summary.LastSuccess != nil && time.Since(*f.summary.LastSuccess) < 3*time.Second {
			ui.Box(c).Size(19, 19).Radius(10).Border(2, color).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Text(c, "✓").FontSize(11).TextColor(color) })
			c.After(3*time.Second - time.Since(*f.summary.LastSuccess))
		} else {
			ui.Icon(c, syncIcon).FontSize(17).TextColor(color)
		}
		label := "同步 · " + status
		if compact {
			label = "同步"
		}
		ui.Text(c, label).FontSize(12).Bold().TextColor(color).SingleLine()
		if compact {
			ui.Box(c).Size(5, 5).Radius(3).Background(p.color)
		}
	})
}

func (a *App) sharedSyncFailed(err error) {
	f := &a.features.sharedSync
	if !f.loaded {
		a.loadSharedSyncSettings()
	}
	f.status, f.phase = err.Error(), core.SyncFailed
	a.status = f.status
	a.invalidateSyncPresentation()
}

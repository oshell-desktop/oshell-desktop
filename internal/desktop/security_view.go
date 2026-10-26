package desktop

import "github.com/egoist/mygo/ui"

// All workspace surfaces share the same protection gate, including auxiliary
// windows that can otherwise retain a previously rendered connection/file list.
func (a *App) lockedWorkspace(c *ui.Context) bool {
	if !a.workspaceLocked() {
		return false
	}
	a.refreshPaneVisibility()
	ui.Column(c).Fill().Center().Gap(16).Children(func() {
		ui.Text(c, "OShell 已锁定").FontSize(24).Bold()
		ui.Text(c, "输入主密码解锁会话和终端。")
		ui.PrimaryButton(c, "解锁").OnClick(func() { a.features.startupUnlock = true; a.showVault() })
	})
	a.featureDialogs(c)
	return true
}
func (a *App) invalidateSecurityWindows() {
	apps := []*App{a}
	if a.hub != nil {
		apps = append([]*App(nil), a.hub.windows...)
		if a.hub.fileWindow != nil {
			apps = append(apps, a.hub.fileWindow)
		}
	}
	for _, window := range apps {
		window.features.sharedSync.loaded = false
		if window.win != nil {
			window.win.Invalidate()
		}
		if window.manager != nil {
			window.manager.Invalidate()
		}
		if window.preferencesWindow != nil {
			window.preferencesWindow.Invalidate()
		}
	}
}

package desktop

func (a *App) syncEditingBlocked() bool {
	windows := []*App{a}
	if a.hub != nil {
		windows = a.hub.configurationWindows()
	}
	for _, window := range windows {
		if window.manager != nil || window.managerRename != nil || window.settingsOpen || window.modalActive() || tabMoveDialogOpen(window.win) {
			return true
		}
	}
	return false
}

// Discard prepared snapshots: settings Apply may change local state or target.
// Re-prepare after close rather than committing against an obsolete baseline.
func (a *App) deferSyncForSettings(background bool) bool {
	if !a.syncEditingBlocked() {
		return false
	}
	f := &a.features.sharedSync
	a.clearSharedSyncPlan()
	f.conflictOpen = false
	f.deferred = true
	f.deferredBackground = background
	f.status = "窗口正在编辑，关闭后重新比较共享配置"
	return true
}

func (a *App) resumeDeferredSyncAfterSettings() {
	windows := []*App{a}
	if a.hub != nil {
		windows = a.hub.configurationWindows()
	}
	for _, window := range windows {
		f := &window.features.sharedSync
		if !f.deferred || window.syncEditingBlocked() {
			continue
		}
		if window.stopped.Load() || window.hub != nil && window.hub.quitting {
			f.deferred = false
			continue
		}
		if f.busy || window.configurationBusy() {
			continue
		}
		f.deferred = false
		f.background = f.deferredBackground
		window.startSharedSync()
	}
}

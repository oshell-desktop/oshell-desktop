package desktop

func (a *App) showPreferenceSection(section int) {
	owner := a
	if a.hub != nil && a.hub.settingsOwner != nil && a.hub.settingsOwner.settingsOpen {
		owner = a.hub.settingsOwner
	}
	owner.features.settingsTab = section
	if owner.settingsOpen {
		if owner.preferencesWindow != nil {
			owner.preferencesWindow.Focus()
			owner.preferencesWindow.Invalidate()
		}
		return
	}
	owner.openSettings()
}

// Direct menu entries select the existing password workflow, never bypass it.
func (a *App) passwordMenuAction(action string) {
	if a.vault == nil || a.configurationBusy() || a.features.vaultBusy {
		return
	}
	master, locked := a.vault.State()
	if action == "lock" {
		if master && !locked {
			a.lockWorkspaceSecrets()
			a.status = "已清除密码缓存；已连接终端继续运行"
		}
		return
	}
	if action == "remove" && !master {
		a.status = "尚未设置主密码"
		return
	}
	if action != "set" && action != "remove" {
		return
	}
	a.showVault()
	if a.passwordLocked() {
		a.features.vaultMenuAfterUnlock = action
		return
	}
	a.features.vaultAction = action
}

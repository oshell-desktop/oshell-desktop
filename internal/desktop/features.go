// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func (a *App) changeConfig(change func(*core.Config) error) bool {
	if a.passwordLocked() {
		a.status = "错误：请先解锁再修改配置"
		return false
	}
	if a.configurationBusy() {
		a.status = "错误：配置事务正在进行，请稍后重试"
		return false
	}
	old := a.config
	a.config = old.Clone()
	if e := change(&a.config); e != nil {
		a.config = old
		a.status = "错误：" + e.Error()
		return false
	}
	a.config.NormalizeLinkRootOrder()
	if !a.save() {
		a.config = old
		return false
	}
	a.reconcileSessionPins()
	a.refreshSessionTitles()
	if a.win != nil {
		a.win.Invalidate()
	}
	return true
}
func (a *App) nextAuthentication() {
	if len(a.authQueue) == 0 {
		a.batchTerminalOwner = nil
		a.batchTerminalOpened = false
		return
	}
	p := a.authQueue[0]
	a.authQueue = a.authQueue[1:]
	split := 0
	if p.Kind != "ftp" && p.Kind != "sftp" {
		if a.batchTerminalOpened {
			split = -2
		}
		a.batchTerminalOpened = true
	}
	owner := a
	if p.Kind != "ftp" && p.Kind != "sftp" && a.batchTerminalOwner != nil {
		owner = a.batchTerminalOwner
	}
	if owner != a {
		owner.authQueue = append(owner.authQueue, a.authQueue...)
		a.authQueue = nil
		a.batchTerminalOwner = nil
		owner.batchTerminalOpened = a.batchTerminalOpened
		owner.batchTerminalOwner = owner
	}
	owner.request(p, split)
	if p.Kind == "local" {
		owner.nextAuthentication()
	}
}
func (a *App) showVault() {
	a.features.vaultMenuAfterUnlock = ""
	a.features.reloadAfterUnlock = false
	if a.vault == nil {
		a.status = "密码库不可用，请检查启动时的错误"
		return
	}
	a.features.vaultOpen = true
	a.features.vaultError = ""
	a.features.master = ""
	a.features.masterAgain = ""
	a.features.masterCurrent = ""
	locked := a.passwordLocked()
	a.features.vaultAction = "manage"
	if locked {
		a.features.vaultAction = "unlock"
	}
}
func (a *App) runVaultAction() {
	f := &a.features
	if f.vaultBusy || a.vault == nil {
		return
	}
	action, password, current := f.vaultAction, f.master, f.masterCurrent
	if a.configurationBusy() {
		f.vaultError = "配置事务正在进行，请稍后重试"
		return
	}
	if action == "remove" || action == "set" {
		options, e := a.loadSyncOptions()
		if e != nil {
			f.vaultError = e.Error()
			return
		}
		pending, pendingError := core.LoadPendingSyncDirectory(a.store.Dir)
		if pendingError != nil {
			f.vaultError = pendingError.Error()
			return
		}
		if options.Enabled || pending != nil {
			f.vaultError = "请先停用配置同步，再修改主密码保护"
			return
		}
	}
	if action == "set" && password != f.masterAgain {
		f.vaultError = "两次主密码不一致"
		return
	}
	configSnapshot := a.config.Clone()
	configRevision, readErr := a.store.Revision()
	if readErr != nil {
		f.vaultError = readErr.Error()
		return
	}
	vaultRevision := a.vault.Revision()
	f.vaultBusy = true
	a.setConfigurationBusy(true)
	f.vaultError = ""
	f.master = ""
	f.masterAgain = ""
	f.masterCurrent = ""
	go func() {
		var e error
		var loaded *core.Config
		switch action {
		case "unlock":
			e = a.vault.Unlock(password)
			if e == nil {
				if protected, _ := a.store.ConfigurationProtection(); protected {
					var cfg core.Config
					cfg, e = a.store.UnlockConfiguration([]byte(password))
					if e == nil {
						loaded = &cfg
					} else {
						a.store.LockConfiguration()
						a.vault.Lock()
					}
				} else if masterEnabled, _ := a.vault.State(); masterEnabled {
					e = a.store.ProtectConfiguration(configSnapshot, []byte(password), configRevision)
					if e != nil {
						a.store.LockConfiguration()
						a.vault.Lock()
					}
				}
			}
		case "set":
			if password == "" {
				e = fmt.Errorf("请输入新主密码")
			} else {
				e = a.store.ChangeProtection(context.Background(), configSnapshot, a.vault, current, password, configRevision, vaultRevision)
			}
		case "remove":
			e = a.store.ChangeProtection(context.Background(), configSnapshot, a.vault, current, "", configRevision, vaultRevision)

		}

		a.post(func() {
			f.vaultBusy = false
			if loaded != nil && e == nil {
				a.applyCommittedConfig(*loaded)
			}
			if !errors.Is(e, core.ErrPendingTransaction) {
				a.setConfigurationBusy(false)
			}
			if e != nil {
				f.vaultError = e.Error()
				return
			}
			a.afterVaultUnlock()

		})
	}()
}
func (a *App) featureDialogs(c *ui.Context) {
	if a.passwordLocked() {
		a.vaultDialog(c)
		return
	}
	if !a.settingsOpen {
		a.sharedSyncConflictDialog(c)
	}
	a.sessionCloseDialog(c)
	a.linkEditorDialog(c)
	a.authenticationChallengeDialog(c)
	f := &a.features
	a.vaultDialog(c)
	a.standaloneSCPDialog(c)
	a.uploadConfirmationDialog(c)
	a.fileActionDialog(c)
	a.inputDialogs(c)
	a.defaultsDialog(c)
	a.operatorDialogs(c)
	a.groupsDialog(c)
	a.livePropertiesDialog(c)
	a.archiveDialog(c)
	a.nativeMigrationDialog(c)
	a.proxyManagerDialog(c)
	a.quickSwitchDialog(c)
	a.quickCommandDialog(c)
	a.sidebarRenameDialog(c)
	ui.Modal(c, &f.directoryOpen, func() {
		ui.Column(c).Width(470).Gap(12).Children(func() {
			ui.Text(c, f.directoryAction).FontSize(18).Bold()
			if f.directoryAction == "移动所选会话" || f.directoryAction == "移动实际会话" {
				ui.Text(c, "目标目录：/"+strings.Trim(f.directoryName, "/"))
				a.directoryTree(c, &f.directoryName, &f.directoryPicker)
			} else if f.directoryAction == "新建目录" {
				ui.Text(c, "所在目录：/"+f.directorySource)
				ui.TextInput(c, &f.directoryName).Label("新目录名称").AutoFocus()
			} else {
				ui.TextInput(c, &f.directoryName).Label("目标目录").AutoFocus()
			}
			if f.directoryAction != "新建目录" {
				ui.Text(c, "使用 / 分隔目录，例如 /生产/数据库。").TextColor(c.Theme().TextMuted)
			}
			if f.directoryError != "" {
				ui.Text(c, f.directoryError).TextColor(c.Theme().Danger)
			}
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(func() { f.directoryOpen = false })
				ui.PrimaryButton(c, "应用").OnClick(a.applyDirectory)
			})
		})
	})
	if !a.settingsOpen {
		a.localConflictDialog(c)
	}
}
func (a *App) openCommands() {
	a.features.commandEdit = nil
	a.features.commandOriginal = nil
	a.features.commandDelete = nil
	a.features.commandRow = -1
	a.features.commandList = ui.ListState{}
	a.features.commandError = ""
	a.features.commandsOpen = true
}
func (a *App) openDirectory(action, source string) {
	a.features.directorySidebar = a.sidebar.context != nil
	a.features.directoryRows = nil
	if action == "移动所选会话" || action == "移动实际会话" {
		a.features.directoryRows = append([]sessionRow(nil), a.selectedManagerRows()...)
	}
	a.closeManager()
	a.features.directoryPicker = directoryPickerState{}
	a.features.directoryAction = action
	a.features.directorySource = source
	a.features.directoryName = "/" + source
	if action == "新建目录" {
		a.features.directoryName = "新目录"
	}
	a.features.directoryError = ""
	a.features.directoryOpen = true
}
func (a *App) applyDirectory() {
	f := &a.features
	keepPin := f.directoryAction == "移动 / 重命名目录" && a.localUI.Pins["d:"+f.directorySource]
	var operationError error
	ok := a.changeConfig(func(cfg *core.Config) error {
		destination := f.directoryName
		if f.directoryAction == "新建目录" {
			var err error
			destination, err = core.LinkFolderChild(f.directorySource, f.directoryName)
			if err != nil {
				operationError = err
				return err
			}
			if f.directorySource != "" {
				found := false
				for _, dir := range cfg.AllDirectories() {
					if dir == f.directorySource {
						found = true
						break
					}
				}
				if !found {
					operationError = fmt.Errorf("所在目录已不存在，请重新选择")
					return operationError
				}
			}
		}
		d, e := core.NormalizeDirectory(destination)
		if e != nil {
			operationError = e
			return e
		}
		switch f.directoryAction {
		case "新建目录":
			e = cfg.AddDirectory(d)
		case "移动 / 重命名目录":
			e = cfg.MoveDirectory(f.directorySource, d)
		case "移动所选会话", "移动实际会话":
			var profiles, dirs, links []string
			if len(f.directoryRows) == 0 {
				e = fmt.Errorf("请选择要移动的项目")
				break
			}
			for _, row := range f.directoryRows {
				if row.parent {
					e = fmt.Errorf("上级目录行不能移动")
					break
				}
				if row.folder {
					dirs = append(dirs, row.group)
				} else if row.linkID != "" {
					links = append(links, row.linkID)
				} else {
					found := false
					for _, profile := range cfg.Profiles {
						if profile.ID == row.profile.ID && profile.Group == row.group {
							found = true
							break
						}
					}
					if !found {
						e = fmt.Errorf("所选会话已变化，请重新选择")
						break
					}
					profiles = append(profiles, row.profile.ID)
				}
			}
			if e == nil {
				e = cfg.MoveSessionItems(profiles, dirs, links, d, f.directoryAction != "移动实际会话")
			}

		}
		operationError = e
		return e
	})
	if ok {
		if keepPin {
			if d, err := core.NormalizeDirectory(f.directoryName); err == nil && managerParent(d) == "" {
				if row, exists := a.sidebarRow("folder:" + d); exists {
					a.setSessionPins([]sessionRow{row}, true)
				}
			}
		}
		f.directoryOpen = false
		if f.directorySidebar {
			a.sidebar.query = ""
			destination := f.directoryName
			if f.directoryAction == "新建目录" {
				destination, _ = core.LinkFolderChild(f.directorySource, f.directoryName)
			}
			if len(f.directoryRows) == 1 && !f.directoryRows[0].folder {
				a.revealSidebar(f.directoryRows[0].id)
			} else if directory, err := core.NormalizeDirectory(destination); err == nil {
				a.revealSidebar("folder:" + directory)
			}
			return
		}
		a.managerRow = -1
		a.managerSelection.Clear()
		a.openManager()
	} else if operationError != nil {
		f.directoryError = operationError.Error()
	} else {
		f.directoryError = a.status
	}
}
func (a *App) addCurrentLink() {
	p := a.activePane()
	if p == nil || p.closed || p.profile.Kind == "blank" {
		return
	}
	a.addProfileLinkToRoot(p.profile)
}
func (a *App) addProfileLinkToRoot(profile core.Profile) {
	if e := profile.Validate(); e != nil {
		a.status = "错误：" + e.Error()
		return
	}
	for _, link := range a.config.Links {
		if link.ProfileID == profile.ID && link.Folder == "" && a.config.Settings.LinksVisible {
			return
		}
	}
	a.changeConfig(func(cfg *core.Config) error {
		found := false
		selected := profile
		for _, v := range cfg.Profiles {
			if v.ID == profile.ID {
				found = true
				selected = v
				break
			}
		}
		if !found {
			cfg.Profiles = append(cfg.Profiles, profile)
		}
		cfg.Settings.LinksVisible = true
		for _, link := range cfg.Links {
			if link.ProfileID == profile.ID && link.Folder == "" {
				return nil
			}
		}
		return cfg.AddLink(selected, "")
	})
}
func (a *App) cloneActive() {
	p := a.activePane()
	if p == nil || p.conn == nil {
		return
	}
	a.openConnection(p.profile, core.Credentials{OpenTerminal: p.terminalOnly, ExternalLaunch: p.externalLaunch}, 0, p.conn)
}
func (a *App) toggleLog() {
	p := a.activePane()
	if p == nil || p.term == nil {
		return
	}
	_, active, _ := p.term.LogState()
	if active {
		p.term.StopLog()
		a.status = "日志记录已停止"
		return
	}
	go func() {
		file, e := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.win, Title: "记录终端输出（ANSI 原始日志，追加写入）", DefaultPath: "oshell-session.log"})
		if e != nil {
			a.report(e)
			return
		}
		if file == "" {
			return
		}
		e = p.term.StartLog(file)
		a.post(func() {
			if e != nil {
				a.status = "日志记录失败：" + e.Error()
			} else {
				a.status = "正在记录终端输出"
			}
		})
	}()
}
func (a *App) exportScreen() {
	p := a.activePane()
	if p == nil || p.term == nil {
		return
	}
	term := p.term
	go func() {
		file, e := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.win, Title: "保存终端文本及回滚历史", DefaultPath: "oshell-terminal.txt"})
		if e != nil {
			a.report(e)
			return
		}
		if file != "" {
			if e = core.AtomicWrite(file, []byte(term.Text())); e != nil {
				a.report(e)
			}
		}
	}()
}
func (a *App) vaultDialog(c *ui.Context) {
	f := &a.features
	ui.Modal(c, &f.vaultOpen, func() {
		ui.Column(c).Width(460).Gap(12).Children(func() {
			ui.Text(c, "密码保护").FontSize(18).Bold()
			if a.vault == nil {
				ui.Text(c, "密码库不可用")
				return
			}

			if f.vaultBusy {
				ui.Text(c, "正在更新密码保护…")
				return
			}
			master, locked := a.vault.State()
			if f.vaultAction == "unlock" {
				ui.Text(c, "输入主密码以读取保存的会话密码。")
				passwordInput(c, &f.master).Label("主密码").AutoFocus()
				ui.PrimaryButton(c, "解锁").Disabled(f.vaultBusy).OnClick(a.runVaultAction)

			} else if f.vaultAction == "set" {
				if master {
					ui.Field(c, "当前主密码", func() { passwordInput(c, &f.masterCurrent).Label("当前主密码") })
				}
				ui.Field(c, "新主密码", func() { passwordInput(c, &f.master).AutoFocus() })
				ui.Field(c, "再次输入", func() { passwordInput(c, &f.masterAgain) })
				ui.PrimaryButton(c, "设置主密码").Disabled(f.vaultBusy).OnClick(a.runVaultAction)
			} else if f.vaultAction == "remove" {
				ui.Field(c, "当前主密码", func() { passwordInput(c, &f.masterCurrent).Label("当前主密码").AutoFocus() })
				ui.Text(c, "移除主密码后，保存的密码将改由本机密钥加密。\n能读取应用数据目录的用户也可能取得该密钥。")
				ui.PrimaryButton(c, "确认改用本机加密").Disabled(f.vaultBusy).OnClick(a.runVaultAction)
			} else {
				mode := "当前使用本机密钥加密；密码不会以明文保存。\n本机密钥依赖操作系统的用户文件权限保护。"
				if master {
					mode = "主密码保护已启用。"
				}
				if locked {
					mode += " 密码库已锁定。"
				}
				ui.Text(c, mode).TextColor(c.Theme().TextMuted)
				ui.Button(c, "设置 / 修改主密码").Disabled(locked || f.vaultBusy || a.configurationBusy()).OnClick(func() { f.vaultAction = "set" })
				if master {
					ui.Button(c, "锁定密码库").Disabled(locked || f.vaultBusy || a.configurationBusy()).OnClick(func() { a.lockWorkspaceSecrets(); f.vaultAction = "unlock" })
					ui.Button(c, "移除主密码").Disabled(locked || f.vaultBusy || a.configurationBusy()).OnClick(func() { f.vaultAction = "remove" })
				}
			}
			if f.vaultError != "" {
				ui.Text(c, f.vaultError).TextColor(c.Theme().Danger)
			}
			if f.vaultBusy {
				ui.Text(c, "正在处理密码保护…").TextColor(c.Theme().TextMuted)
			}
			ui.Button(c, "关闭").Disabled(f.vaultBusy).OnClick(func() { f.vaultOpen = false; f.vaultMenuAfterUnlock = ""; f.resumeAuth = false; a.authQueue = nil })
		})
	})
	if !f.vaultOpen {
		f.reloadAfterUnlock = false
		f.vaultMenuAfterUnlock = ""
		f.masterCurrent = ""
		f.master = ""
		f.masterAgain = ""
	}
}

func (a *App) afterVaultUnlock() {
	a.authorizeWorkspace()
	f := &a.features
	f.vaultAction = "manage"
	a.invalidateSecurityWindows()
	if a.hub != nil {
		for _, window := range a.hub.windows {
			if window != a && window.features.startupUnlock {
				window.features.startupUnlock = false
				window.features.vaultOpen = false
				window.features.vaultAction = "manage"
			}
		}
		a.hub.drainExternalLaunch()
		a.hub.scheduleUpdates()
		a.hub.scheduleStartupSync()
	}
	if f.startupUnlock {
		f.startupUnlock = false
		f.vaultOpen = false
	}
	if f.resumeAuth {
		f.resumeAuth = false
		f.vaultOpen = false
		a.request(a.pendingProfile, a.pendingSplit, f.pendingTerminal)
	}
	if action := f.vaultMenuAfterUnlock; action != "" {
		f.vaultMenuAfterUnlock = ""
		f.vaultAction = action
		f.vaultOpen = true
		return
	}
	if f.reloadAfterUnlock {
		f.reloadAfterUnlock = false
		f.vaultOpen = false
		a.refreshLocalSharedData()
		return
	}
	a.maybeResumeLocalConflict()
}

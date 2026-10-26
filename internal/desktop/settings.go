// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func (a *App) prepareSettings() {
	a.localUIDraft = a.localUI.clone()
	a.localUIOriginal = a.localUI.clone()
	a.titleOverwrite = true
	a.sidebar.capture = false
	a.features.sharedSync.logList = ui.ListState{}
	a.features.sharedSync.conflictList = ui.ListState{}
	a.features.sharedSync.conflictDetailKey = ""
	a.features.sharedSync.logsOpen = false
	a.features.sharedSync.conflictOpen = false
	if !a.features.sharedSync.busy {
		a.features.sharedSync.loaded = false
	}
	a.settingsPreviewApplied = false
	a.features.settingsDraft = a.config.Clone().Settings
	a.clockFollowsText = a.features.settingsDraft.TerminalClockColor == ""
	a.clockSizeDraft = strconv.FormatFloat(a.features.settingsDraft.TerminalClockFontSize, 'f', -1, 64)
	a.prepareUpdateSettings()
	if a.features.settingsDraft.KeyboardShortcuts.Overrides == nil {
		a.features.settingsDraft.KeyboardShortcuts.Overrides = map[string]core.ShortcutOverride{}
	}
	a.features.settingsLines = strconv.Itoa(a.config.Settings.ScrollbackLines)
	a.features.historyCustomText = a.features.settingsLines
	a.features.historyCustom = historyPresetIndex(a.features.settingsLines) == len(historyPresets)
	a.features.settingsError = ""
	a.prepareStorageSettings()
	a.features.colorDraft = a.features.settingsDraft.Scheme()
	a.features.settingsCapture = ""
	a.features.shortcutError = ""
	a.settingsOpen = true
	if mygo.App.IsReady() {
		a.loadFontCatalog()
	}
}
func (a *App) openSettings() {
	if a.configurationBusy() {
		a.status = "配置正在保存，请稍后打开设置"
		return
	}
	if !a.settingsOpen && a.features.sharedSync.conflictOpen {
		return
	}
	if a.hub != nil && a.hub.settingsOwner != nil && a.hub.settingsOwner.preferencesWindow != nil {
		a.hub.settingsOwner.preferencesWindow.Focus()
		return
	}
	a.prepareSettings()
	if a.win == nil {
		return
	}
	if a.hub != nil {
		a.hub.settingsOwner = a
	}
	w := mygo.NewWindow(mygo.WindowOptions{Parent: a.win, Title: "设置", Width: 800, Height: 650, MinWidth: 700, MinHeight: 560, Content: ui.View(a.settingsView)})
	a.preferencesWindow = w
	w.OnClose(func(e *mygo.CloseEvent) {
		if a.features.localConflict.active {
			e.PreventDefault()
			a.cancelLocalConflict()
		}
	})
	w.OnClosed(func() {
		if a.preferencesWindow == w {
			a.preferencesWindow = nil
			a.closeSettings()
		}
		if a.hub != nil && a.hub.settingsOwner == a {
			a.hub.settingsOwner = nil
		}
		a.restoreAfterSettings()
	})
}
func (a *App) closeSettings() {
	a.storagePickerGeneration++
	queued := a.features.sharedSync.manualQueued
	a.features.sharedSync.manualQueued = false
	a.cancelWebDAVProbe()
	a.features.sharedSync.directoryGeneration++
	a.features.sharedSync.directoryChanged = false
	a.features.sharedSync.directoryDisabled = false
	a.features.sharedSync.pendingDirectory = nil
	a.features.sharedSync.loaded = false
	a.features.sharedSync.password = ""

	a.features.settingsCapture = ""
	a.refreshMenu()
	if a.features.fontCancel != nil {
		a.features.fontCancel()
	}
	a.settingsOpen = false
	if a.preferencesWindow != nil {
		w := a.preferencesWindow
		a.preferencesWindow = nil
		w.Close()
	}
	if a.hub != nil && a.hub.settingsOwner == a {
		a.hub.settingsOwner = nil
	}
	if queued && !a.stopped.Load() && (a.hub == nil || !a.hub.quitting) {
		a.features.sharedSync.background = false
		a.startSharedSync()
		a.status = a.features.sharedSync.status
	}
	a.resumeDeferredSyncAfterSettings()
}
func (a *App) commitColorDraft() error {
	f := &a.features
	if e := f.colorDraft.Validate(); e != nil {
		return e
	}
	original := f.settingsDraft.Scheme()
	if reflect.DeepEqual(original, f.colorDraft) {
		return nil
	}
	value := f.colorDraft
	custom := false
	for i, c := range f.settingsDraft.CustomSchemes {
		if c.ID == value.ID {
			f.settingsDraft.CustomSchemes[i] = value
			custom = true
			break
		}
	}
	if !custom {
		value = value.Copy()
		f.settingsDraft.CustomSchemes = append(f.settingsDraft.CustomSchemes, value)
	}
	f.settingsDraft.ColorSchemeID = value.ID
	f.colorDraft = value
	return nil
}
func (a *App) applySettings() {
	if a.configurationBusy() || a.features.sharedSync.busy {
		return
	}
	f := &a.features
	if !reflect.DeepEqual(a.localUI, a.localUIOriginal) {
		f.settingsError = "本机设置已在其他窗口修改，请重新打开设置后再应用"
		return
	}
	if err := a.validateClockDraft(); err != nil {
		f.settingsError = err.Error()
		return
	}
	if a.storageDirectoryDraft != a.storageDirectoryOriginal {
		if err := a.store.ValidateStorageDirectory(a.storageDirectoryDraft); err != nil {
			f.settingsError = err.Error()
			return
		}
	}
	if err := a.validateSyncDraft(); err != nil {
		f.settingsError = err.Error()
		return
	}
	if e := a.validateUpdateSettings(); e != nil {
		f.settingsError = e.Error()
		return
	}
	if e := f.settingsDraft.KeyboardShortcuts.Validate(runtime.GOOS); e != nil {
		f.settingsError = e.Error()
		return
	}
	if conflict := f.settingsDraft.KeyboardShortcuts.Conflict(a.localUIDraft.SidebarShortcut, "", runtime.GOOS); conflict != "" {
		f.settingsError = "侧栏快捷键与 " + conflict + " 冲突"
		return
	}
	if conflict := f.settingsDraft.KeyboardShortcuts.Conflict(a.localUIDraft.DeepFocusShortcut, "", runtime.GOOS); conflict != "" {
		f.settingsError = "全心专注快捷键与 " + conflict + " 冲突"
		return
	}
	if a.localUIDraft.DeepFocusShortcut.KeyCode == a.localUIDraft.SidebarShortcut.KeyCode && shortcutMods(a.localUIDraft.DeepFocusShortcut) == shortcutMods(a.localUIDraft.SidebarShortcut) {
		f.settingsError = "侧栏与全心专注快捷键冲突"
		return
	}
	if err := a.checkChangedSystemShortcuts(); err != nil {
		f.settingsError = err.Error()
		return
	}
	lines, e := strconv.Atoi(f.settingsLines)
	if e != nil || lines < 500 || lines > 100000 {
		f.settingsError = "历史行数范围为 500–100000"
		return
	}
	if e = a.commitColorDraft(); e != nil {
		f.settingsError = e.Error()
		return
	}
	f.settingsDraft.ScrollbackLines = lines
	f.settingsDraft.Dark = f.settingsDraft.InterfaceTheme == "dark"
	apply := func() {
		if a.titleOverwrite {
			a.applyTitleDraft()
		}
		if a.changeConfig(func(cfg *core.Config) error { cfg.Settings = f.settingsDraft; return nil }) {
			if !a.saveLocalPreferences(a.localUIDraft) {
				return
			}
			a.applyAppearance()
			a.applySyncDraft(func() {
				if a.applyStorageSettings() {
					a.closeSettings()
				}
			})
		}
	}
	if lines < a.config.Settings.ScrollbackLines && a.activeConnections() > 0 && a.win != nil {
		go func() {
			r, e := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.preferencesWindow, Type: mygo.MessageWarning, Message: "降低历史上限会丢弃较早的终端输出，继续？", Buttons: []string{"取消", "应用"}, DefaultButton: 0, CancelButton: 0})
			if e == nil && r.Button == 1 {
				a.post(apply)
			}
		}()
	} else {
		apply()
	}
}
func (a *App) settingsView(c *ui.Context) {
	defer a.resumeDeferredUpdate() // Modal teardown may happen after drain during this frame.
	a.drain()
	if auxiliaryEscape(c, func() {
		if a.sidebar.capture {
			a.sidebar.capture = false
		} else {
			a.closeSettings()
		}
	}) {
		return
	}
	a.themeSettings(c, a.features.settingsDraft)
	dark := c.Theme().Dark
	if a.preferencesWindow != nil && (!a.settingsPreviewApplied || a.settingsPreviewDark != dark) {
		previewWindowAppearance(a.preferencesWindow, dark)
		a.settingsPreviewApplied = true
		a.settingsPreviewDark = dark
	}
	if a.passwordLocked() && a.features.settingsTab == 3 {
		a.loadSharedSyncSettings()
		ui.Column(c).Fill().Padding(16).Children(func() { a.syncStatusCard(c) })
		a.featureDialogs(c)
		return
	}
	if a.lockedWorkspace(c) {
		return
	}
	f := &a.features
	s := &f.settingsDraft
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Tabs(c, &f.settingsTab, "常规", "主题与配色", "快捷键", "同步", "更新", "关于", "存储", "时钟", "选择与侧栏")
		ui.Scroll(c).Grow(1).MinHeight(0).Gap(12).Children(func() {
			switch f.settingsTab {
			case 0:
				a.titleSettings(c)
				ui.Text(c, "终端显示").Bold()
				ui.Form(c, func() {
					ui.Field(c, "终端字体", func() {
						a.fontPicker(c)
					})
					ui.Row(c).Gap(8).Children(func() {
						ui.Button(c, "系统等宽").OnClick(func() { s.FontFamily = "" })
						ui.Button(c, "内置 DejaVu Mono").Width(225).OnClick(func() { s.FontFamily = "DejaVu Sans Mono" })
					})
					ui.Row(c).Gap(10).Children(func() {
						ui.Text(c, fmt.Sprintf("字号 %.0f", s.FontSize)).Grow(1)
						ui.Button(c, "－").OnClick(func() { s.FontSize = max(10, s.FontSize-1) })
						ui.Button(c, "＋").OnClick(func() { s.FontSize = min(26, s.FontSize+1) })
					})
					a.historySettings(c)
				}).Gap(10)
				ui.Box(c).Padding(12).Background(c.Theme().Surface).Radius(4).Children(func() {
					ui.Text(c, "Aa 0123456789  中文预览\n❯ ~/workspace   main  ✓").Font(terminalFamily(s.FontFamily)).FontSize(float32(s.FontSize))
				})
				ui.Checkbox(c, &s.Metal, "使用 GPU 加速绘制终端（Metal）")
				ui.Checkbox(c, &s.PreferColorEmoji, "优先显示彩色 Emoji")
				ui.Checkbox(c, &s.AmbiguousCharactersAreWide, "将歧义宽度字符视为双宽（新建连接生效）")
				ui.Divider(c)

				ui.Text(c, "输入与粘贴").Bold()
				ui.Checkbox(c, &s.Input.RightClickPaste, "右键粘贴；Shift+右键保留终端菜单")
				ui.Checkbox(c, &s.Input.ConfirmMultiline, "多行粘贴前预览")
				ui.Checkbox(c, &s.AutoZmodem, "自动识别 ZMODEM 上传和下载")
				ui.Button(c, "密码保护…").OnClick(func() { a.closeSettings(); a.showVault() })
				ui.Button(c, "突出显示规则…").OnClick(func() {
					a.closeSettings()
					a.showHighlights()
					if a.win != nil {
						a.win.Invalidate()
					}
				})
			case 1:
				a.colorSettings(c)
			case 2:
				a.shortcutsSettings(c)
			case 3:
				a.sharedSyncSettings(c)
			case 4:
				a.updateSettingsView(c)
			case 5:
				a.aboutSettings(c)
			case 6:
				a.storageSettings(c)
			case 7:
				a.clockSettings(c)
			case 8:
				a.localSelectionSettings(c)
			}
		})
		if f.settingsError != "" {
			ui.Text(c, f.settingsError).TextColor(c.Theme().Danger)
		}
		ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
			ui.Button(c, "取消").OnClick(a.closeSettings)
			ui.PrimaryButton(c, "应用").OnClick(a.applySettings)
		})
	})
	a.shortcutCapture(c)
	a.sharedSyncConflictDialog(c)
	if a.syncMonitorWindow() == nil {
		a.syncDiagnosticsDialog(c)
	}
	a.localConflictDialog(c)
	a.vaultDialog(c)
}
func (a *App) colorSettings(c *ui.Context) {
	f := &a.features
	s := &f.settingsDraft
	ui.Form(c, func() {
		ui.Field(c, "界面主题", func() {
			selectValue(c, "界面主题", &s.InterfaceTheme, [][2]string{{"system", "跟随系统"}, {"light", "浅色"}, {"dark", "深色"}})
		})
		ui.Field(c, "终端配色", func() {
			ui.MenuButton(c, f.colorDraft.Name, func(m *ui.Menu) {
				for _, scheme := range s.Schemes() {
					if m.Item(scheme.Name).Checked(scheme.ID == s.ColorSchemeID).Chosen() {
						if e := a.commitColorDraft(); e != nil {
							f.settingsError = e.Error()
							return
						}
						s.ColorSchemeID = scheme.ID
						f.colorDraft = scheme
					}
				}
			})
		})
	})
	ui.Row(c).Gap(6).Children(func() {
		ui.TextInput(c, &f.colorDraft.Name).Grow(1)
		ui.Button(c, "复制为自定义").OnClick(func() {
			f.colorDraft = f.colorDraft.Copy()
			s.CustomSchemes = append(s.CustomSchemes, f.colorDraft)
			s.ColorSchemeID = f.colorDraft.ID
		})
		ui.Button(c, "导入").OnClick(a.importColorSettings)
		ui.Button(c, "导出").OnClick(a.exportColorSettings)
	})
	ui.Checkbox(c, &f.colorDraft.AutoCursor, "光标颜色自动对比背景")
	ui.Text(c, "自动选择清晰的黑色或白色光标；关闭后可自定义光标和光标文字颜色。").FontSize(12).TextColor(c.Theme().TextMuted)
	a.colorSchemePreview(c)
	fields := []struct {
		label string
		value *string
	}{{"背景", &f.colorDraft.Background}, {"文字", &f.colorDraft.Foreground}, {"光标", &f.colorDraft.Cursor}, {"光标文字", &f.colorDraft.CursorText}, {"选区背景", &f.colorDraft.Selection}, {"选区文字", &f.colorDraft.SelectionText}}
	ui.Grid(c).Columns(3).Gap(10).Children(func() {
		for _, field := range fields {
			if f.colorDraft.AutoCursor && (field.label == "光标" || field.label == "光标文字") {
				continue
			}
			colorSettingField(c.Key(field.label), field.label, field.value)
		}
	})
	ui.Text(c, "ANSI 颜色").Bold()
	ui.Grid(c).Columns(4).Gap(8).Children(func() {
		for i := range f.colorDraft.ANSI {
			colorSettingField(c.Key(fmt.Sprintf("ansi-%d", i)), fmt.Sprintf("%d · %s", i, ansiColorNames[i]), &f.colorDraft.ANSI[i])
		}
	})
}
func (a *App) importColorSettings() {
	parent := a.preferencesWindow
	go func() {
		files, e := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: parent, Title: "导入 OShell JSON / Xshell 配色"})
		if e != nil || len(files) == 0 {
			return
		}
		data, e := core.ReadBounded(files[0])
		if e != nil {
			a.report(e)
			return
		}
		schemes, e := core.ImportColors(data)
		a.post(func() {
			if e != nil {
				a.features.settingsError = e.Error()
				return
			}
			f := &a.features
			f.settingsDraft.CustomSchemes = append(f.settingsDraft.CustomSchemes, schemes...)
			f.settingsDraft.ColorSchemeID = schemes[0].ID
			f.colorDraft = schemes[0]
			if parent != nil {
				parent.Invalidate()
			}
		})
	}()
}
func (a *App) exportColorSettings() {
	scheme := a.features.colorDraft
	parent := a.preferencesWindow
	if e := scheme.Validate(); e != nil {
		a.features.settingsError = e.Error()
		return
	}
	go func() {
		file, e := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: parent, Title: "导出配色", DefaultPath: "OShell-colors.json"})
		if e != nil || file == "" {
			return
		}
		data, e := json.MarshalIndent(scheme, "", "  ")
		if e == nil {
			e = core.AtomicWrite(file, data)
		}
		a.report(e)
	}()
}

// Schedule after the native close event finishes. Restoring inside window-close
// callbacks can be overwritten by AppKit's key-window transition.
func (a *App) restoreAfterSettings() {
	if a.win == nil || a.stopped.Load() || !mygo.App.IsReady() {
		return
	}
	go mygo.RunOnMain(func() {
		if a.stopped.Load() || a.win == nil || a.hub != nil && a.hub.quitting {
			return
		}
		a.win.Show()
		a.win.Focus()
		a.lastInputFocus.Focus()
		a.win.Invalidate()
	})
}

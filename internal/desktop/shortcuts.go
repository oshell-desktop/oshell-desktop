// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"runtime"
	"strconv"
	"strings"
)

func shortcutKey(code uint16) ui.Key {
	name := core.ShortcutKeyNames[code]
	if len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z' {
		return ui.KeyA + ui.Key(name[0]-'A')
	}
	if len(name) == 1 && name[0] >= '0' && name[0] <= '9' {
		return ui.Key0 + ui.Key(name[0]-'0')
	}
	if strings.HasPrefix(name, "F") {
		n, e := strconv.Atoi(name[1:])
		if e == nil && n >= 13 && n <= 35 {
			return ui.KeyF13 + ui.Key(n-13)
		}
		if e == nil && n >= 1 && n <= 12 {
			return ui.KeyF1 + ui.Key(n-1)
		}
	}
	return map[string]ui.Key{"Tab": ui.KeyTab, "Enter": ui.KeyEnter, "Escape": ui.KeyEscape, "Space": ui.KeySpace, "Backspace": ui.KeyBackspace, "Left": ui.KeyLeft, "Right": ui.KeyRight, "Up": ui.KeyUp, "Down": ui.KeyDown, "Home": ui.KeyHome, "End": ui.KeyEnd, "PageUp": ui.KeyPageUp, "PageDown": ui.KeyPageDown, ",": ui.KeyComma, ".": ui.KeyPeriod, "/": ui.KeySlash, ";": ui.KeySemicolon, "'": ui.KeyQuote, "[": ui.KeyBracketLeft, "]": ui.KeyBracketRight, "\\": ui.KeyBackslash, "-": ui.KeyMinus, "=": ui.KeyEqual, "`": ui.KeyBackquote}[name]
}
func shortcutMods(s core.KeyboardShortcut) ui.Modifiers {
	var m ui.Modifiers
	if s.Modifiers&1 != 0 {
		if runtime.GOOS == "darwin" {
			m |= ui.Cmd
		} else {
			m |= ui.Ctrl
		}
	}
	if s.Modifiers&2 != 0 {
		m |= ui.Alt
	}
	if s.Modifiers&4 != 0 {
		m |= ui.Ctrl
	}
	if s.Modifiers&8 != 0 {
		m |= ui.Shift
	}
	return m
}
func shortcutEvent(ev ui.InputEvent) (core.KeyboardShortcut, bool) {
	s := core.KeyboardShortcut{}
	found := false
	for code := range core.ShortcutKeyNames {
		if key := shortcutKey(code); key != ui.KeyUnknown && key == ev.Key {
			s.KeyCode = code
			found = true
			break
		}
	}
	if !found {
		return s, false
	}
	if ev.Mods&ui.Cmd != 0 {
		s.Modifiers |= 1
	}
	if ev.Mods&ui.Alt != 0 {
		s.Modifiers |= 2
	}
	if ev.Mods&ui.Ctrl != 0 {
		s.Modifiers |= 4
	}
	if ev.Mods&ui.Shift != 0 {
		s.Modifiers |= 8
	}
	return s, s.Valid()
}
func (a *App) modalActive() bool {
	return len(a.challenges) > 0 || a.nonAuthenticationModalActive()
}
func (a *App) nonAuthenticationModalActive() bool {
	if a.sidebar.rename != nil || a.imagePaste != nil {
		return true
	}
	f := &a.features
	return f.sessionClose != nil || f.localConflict.active || f.nativeMigration.open || f.scpDraft != nil || f.uploadDraft != nil || f.sharedSync.conflictOpen || f.sharedSync.logsOpen || f.linkEditor.open || f.switcher.open || f.proxyManager.open || f.archive.open || a.editing || a.authOpen || a.confirmSend || f.vaultOpen || f.commandsOpen || f.directoryOpen || f.fileOpen || f.pasteOpen || f.syncOpen || f.highlightOpen || f.defaultsOpen || f.liveOpen || f.operators.targetsOpen || f.operators.groupsOpen || a.groups.dialog || a.groups.numberOpen
}
func (a *App) shortcuts(c *ui.Context) {
	if a.settingsOpen || a.modalActive() {
		return
	}
	deepKey := a.localUI.DeepFocusShortcut
	if deepKey.Valid() && a.config.Settings.KeyboardShortcuts.Conflict(deepKey, "", runtime.GOOS) == "" && c.Shortcut(shortcutMods(deepKey), shortcutKey(deepKey.KeyCode)) {
		a.toggleDeepFocus()
		return
	}
	key := a.localUI.SidebarShortcut
	if key.Valid() && a.config.Settings.KeyboardShortcuts.Conflict(key, "", runtime.GOOS) == "" && c.Shortcut(shortcutMods(key), shortcutKey(key.KeyCode)) {
		a.toggleSidebar()
		return
	}
	p := a.activePane()
	terminalFocused := p != nil && p.term != nil && (p.focus.FocusWithin(c) || c.Resolve(p.focus).FocusWithin())
	for _, action := range core.ShortcutActions {
		if core.TerminalEditingAction(action.ID) {
			continue // Encoded by the focused terminal, never by application shortcuts.
		}
		if action.ID == "copy" || action.ID == "paste" || action.ID == "selectAll" {
			if p := a.activePane(); p == nil || !p.focus.FocusWithin(c) {
				continue
			}
		}
		for _, key := range a.config.Settings.KeyboardShortcuts.Bindings(action.ID, runtime.GOOS) {
			if terminalFocused && terminalControlShortcut(shortcutMods(key), shortcutKey(key.KeyCode)) {
				continue
			}
			if uikey := shortcutKey(key.KeyCode); uikey != ui.KeyUnknown && c.Shortcut(shortcutMods(key), uikey) {
				a.performAction(action.ID)
				return
			}
		}
	}
}

// Plain Control letters are terminal editing/signalling keys. Application
// shortcuts may still use them when focus is outside the terminal.
func terminalControlShortcut(mods ui.Modifiers, key ui.Key) bool {
	return mods == ui.Ctrl && key >= ui.KeyA && key <= ui.KeyZ
}
func lockedWindowAction(action string) bool {
	return action == "quit" || action == "hide" || action == "minimize"
}
func (a *App) performAction(action string) {
	if !lockedWindowAction(action) && a.workspaceMenuBlocked() {
		return
	}
	if a.workspaceLocked() && !lockedWindowAction(action) {
		a.features.startupUnlock = true
		a.showVault()
		return
	}
	p := a.activePane()
	t := a.activeTab()
	if strings.HasPrefix(action, "tab") && len(action) == 4 {
		n, _ := strconv.Atoi(action[3:])
		a.selectTabNumber(n)
		return
	}
	switch action {
	case "focusFullscreen":
		a.toggleFocusFullscreen()
	case "newWindow":
		if a.hub != nil {
			a.hub.newWindow()
		}
	case "settings":
		a.openSettings()
	case "sessionManager":
		a.openManager()
	case "connectSelected":
		a.connectSelectedSessionFromMenu()
	case "newSession":
		a.edit(a.newProfile("ssh"))
	case "newBlank":
		a.newBlank()
	case "local":
		a.local()
	case "currentProperties":
		if p != nil && p.profile.Kind != "blank" {
			a.edit(p.profile)
		}
	case "liveProperties":
		a.showLiveProperties()
	case "defaults":
		a.showDefaults()
	case "importSessions":
		a.importProfiles()
	case "exportSessions":
		a.exportProfiles()
	case "reconnect":
		a.reconnectActive()
	case "closePane":
		if t != nil {
			a.requestSessionClose(t, t.active)
		} else if a.win != nil {
			a.win.Close()
		}
	case "closeTab":
		if t != nil {
			a.requestSessionClose(t, nil)
		}
	case "copy", "paste", "selectAll":
		if p != nil && p.term != nil {
			p.term.Edit(action)
		}
	case "find":
		a.openSearch()
	case "findNext":
		a.navigateSearch(1)
	case "findPrevious":
		a.navigateSearch(-1)
	case "splitVertical":
		a.splitActivePane(1)
	case "splitHorizontal":
		a.splitActivePane(2)
	case "zoomPane":
		a.togglePaneZoom()
	case "nextPane":
		a.cyclePane(1)
	case "previousPane":
		a.cyclePane(-1)
	case "quickSwitch":
		a.showQuickSwitch()
	case "logging":
		a.toggleLog()
	case "nextTab":
		a.navigateTab(1)
	case "previousTab":
		a.navigateTab(-1)
	case "recentTab":
		if recent := a.tabByID(a.groups.recent); recent != nil {
			a.selectTab(recent)
		}
	case "tabNumber":
		a.groups.numberOpen = true
		a.groups.number = ""
		a.groups.error = ""
	case "newGroup":
		a.promptGroup(nil)
	case "showGroups":
		a.showAllGroups()
	case "arrangeTabs":
		a.arrangeTabs("tabs")
	case "arrangeHorizontal":
		a.arrangeTabs("horizontal")
	case "arrangeVertical":
		a.arrangeTabs("vertical")
	case "arrangeTiled":
		a.arrangeTabs("tile")
	case "links":
		a.changeConfig(func(c *core.Config) error { c.Settings.LinksVisible = !c.Settings.LinksVisible; return nil })
	case "quickSend":
		a.toggleQuickSend()
	case "focusQuickSend":
		a.focusQuickSend()
	case "composer":
		if a.features.operators.composerOpen {
			a.features.operators.composerOpen = false
		} else {
			a.openComposer(a.quick, !a.features.operators.noReturn)
		}
	case "syncInput":
		a.showSyncInput()
	case "stopSync":
		a.features.syncEnabled = false
		a.features.syncTargets = map[string]bool{}
	case "files":
		if p != nil && p.conn != nil {
			a.toggleFiles()
		}
	case "fileManager":
		a.showFileManager()
	case "quickCommands":
		a.openCommands()
	case "highlights":
		a.showHighlights()
	case "appearance":
		a.openSettings()
		a.features.settingsTab = 1
	case "hide":
		if a.win != nil {
			a.win.Hide()
		}
	case "minimize":
		if a.win != nil {
			a.win.Minimize()
		}
	case "quit":
		mygo.App.Quit()
	}
}
func (a *App) refreshMenu() {
	if a.hub != nil && mygo.App.IsReady() {
		a.hub.installMenu()
	}
}
func (a *App) shortcutsSettings(c *ui.Context) {
	f := &a.features
	k := &f.settingsDraft.KeyboardShortcuts
	if k.Overrides == nil {
		k.Overrides = map[string]core.ShortcutOverride{}
	}
	ui.Text(c, "键盘快捷键").Bold()
	if f.shortcutSystemNotice != "" {
		ui.Text(c, f.shortcutSystemNotice).Wrap().TextColor(c.Theme().TextMuted)
	}
	ui.Text(c, "终端编辑替代键仅在终端内生效；Ctrl+W、Ctrl+A 等原有控制键保持不变。行首替代键也可使用 Home。").TextColor(c.Theme().TextMuted)
	ui.TextInput(c, &f.shortcutFilter).Placeholder("筛选操作名称")
	if f.shortcutError != "" {
		ui.Text(c, f.shortcutError).TextColor(c.Theme().Danger)
	}
	for _, action := range core.ShortcutActions {
		if f.shortcutFilter != "" && !strings.Contains(strings.ToLower(action.Title+action.ID), strings.ToLower(f.shortcutFilter)) {
			continue
		}
		var labels []string
		for _, s := range k.Bindings(action.ID, runtime.GOOS) {
			labels = append(labels, s.Display(runtime.GOOS))
		}
		label := strings.Join(labels, " / ")
		if label == "" {
			label = "未设置"
		}
		ui.Row(c.Key(action.ID)).Gap(6).Children(func() {
			ui.Text(c, action.Title).Grow(1)
			ui.Button(c, label).Width(200).OnClick(func() { f.settingsCapture = action.ID; f.shortcutError = ""; a.refreshMenu() })
			ui.Button(c, "清除").OnClick(func() { k.Overrides[action.ID] = core.ShortcutOverride{} })
			ui.Button(c, "默认").OnClick(func() { delete(k.Overrides, action.ID) })
		})
	}
	ui.Button(c, "恢复全部默认快捷键").OnClick(func() { k.Overrides = map[string]core.ShortcutOverride{}; f.shortcutError = "" })
}
func (a *App) shortcutCapture(c *ui.Context) {
	f := &a.features
	if f.settingsCapture == "" {
		return
	}
	open := true
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(440).Gap(14).Children(func() {
			ui.Text(c, "按下新的快捷键").FontSize(18).Bold()
			ui.Text(c, "字母或数字需搭配 Ctrl、Alt 或 Cmd；Esc 取消。")
			if f.shortcutError != "" {
				ui.Text(c, f.shortcutError).TextColor(c.Theme().Danger)
			}
			ui.Box(c).Height(60).FillWidth().Focusable().AutoFocus().Border(1, c.Theme().Accent).Label("快捷键录入").HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind != ui.InputKeyDown || ev.Repeat {
					return false
				}
				s, ok := shortcutEvent(ev)
				if !ok && !s.ValidFor(f.settingsCapture, runtime.GOOS) {
					f.shortcutError = "请使用带修饰键的组合，或 F1–F20"
					return true
				}
				a.acceptCapturedShortcut(s)
				return true
			})
			ui.Button(c, "取消录入").OnClick(func() { f.settingsCapture = ""; a.refreshMenu() })
		})
	})
	if !open {
		f.settingsCapture = ""
		a.refreshMenu()
	}
}
func (h *applicationHub) installMenu() {
	if runtime.GOOS == "windows" {
		// The workspace renders the complete menu in its single header row.
		mygo.App.SetMenu(nil)
		return
	}
	mygo.App.SetMenu(mygo.NewMenu(h.menuItems()))
}

func (h *applicationHub) menuItems() []*mygo.MenuItem {
	item := func(id string) *mygo.MenuItem {
		label := id
		for _, action := range core.ShortcutActions {
			if action.ID == id {
				label = action.Title
				break
			}
		}
		acc := " "
		bindings := h.config.Settings.KeyboardShortcuts.Bindings(id, runtime.GOOS)
		if len(bindings) > 0 && !terminalControlShortcut(shortcutMods(bindings[0]), shortcutKey(bindings[0].KeyCode)) {
			acc = bindings[0].Display(runtime.GOOS)
		}
		disabled := false
		for _, a := range h.windows {
			if a.features.settingsCapture != "" {
				disabled = true
			}
		}
		entry := &mygo.MenuItem{ID: id, Label: label, Accelerator: acc, Disabled: disabled, Click: func(_ *mygo.MenuItem, w *mygo.Window) {
			if h.quitting {
				return
			}
			if id == "newWindow" {
				h.newWindow()
				return
			}
			a := h.focused(w)
			a.performAction(id)
			if a.win != nil {
				a.win.Invalidate()
			}
		}}
		switch id {
		case "copy":
			entry.Role = mygo.RoleCopy
			entry.Click = nil
		case "paste":
			entry.Role = mygo.RolePaste
			entry.Click = nil
		case "selectAll":
			entry.Role = mygo.RoleSelectAll
			entry.Click = nil
		}
		return entry
	}
	menu := func(label string, ids ...string) *mygo.MenuItem {
		out := &mygo.MenuItem{Label: label}
		for _, id := range ids {
			if id == "-" {
				out.Submenu = append(out.Submenu, mygo.Separator())
			} else {
				out.Submenu = append(out.Submenu, item(id))
			}
		}
		return out
	}
	appMenu := menu("OShell", "newWindow", "newBlank", "local", "newSession", "-", "settings")
	appMenu.Submenu = append(appMenu.Submenu, mygo.Separator(),
		h.workspaceMenuItem("关于 OShell", func(a *App) { a.showPreferenceSection(5) }),
		h.workspaceMenuItem("开源许可…", func(a *App) { a.openAboutDocument("LICENSE") }),
		h.workspaceMenuItem("第三方声明…", func(a *App) { a.openAboutDocument("THIRD_PARTY_NOTICES.md") }),
		h.workspaceMenuItem("更新设置…", func(a *App) { a.showPreferenceSection(4) }),
		h.workspaceMenuItem("主题与配色…", func(a *App) { a.showPreferenceSection(1) }),
		mygo.Separator(),
		h.workspaceMenuItem("锁定主密码", func(a *App) { a.passwordMenuAction("lock") }),
		h.workspaceMenuItem("修改主密码…", func(a *App) { a.passwordMenuAction("set") }),
		h.workspaceMenuItem("清除主密码…", func(a *App) { a.passwordMenuAction("remove") }))
	appMenu.Submenu = append(appMenu.Submenu, mygo.Separator(), h.workspaceMenuItem("密码保护…", func(a *App) { a.showVault() }))
	sessionMenu := menu("会话", "newWindow", "newSession", "newBlank", "local", "sessionManager", "quickSwitch", "connectSelected", "currentProperties", "liveProperties", "defaults", "reconnect", "-", "importSessions", "exportSessions")
	sessionMenu.Submenu = append(sessionMenu.Submenu,
		h.workspaceMenuItem("编辑会话…", func(a *App) { a.editSelectedSessionFromMenu() }),
		h.workspaceMenuItem("删除会话…", func(a *App) { a.deleteSelectedSessionFromMenu() }),
		mygo.Separator(), item("closePane"), item("closeTab"))
	sessionMenu.Submenu = append(sessionMenu.Submenu, mygo.Separator(),
		h.workspaceMenuItem("刷新本地 / 共享数据", func(a *App) { a.refreshLocalSharedData() }),
		h.workspaceMenuItem("立即同步共享数据", func(a *App) { a.runSyncMenuAction(false) }),
		h.workspaceMenuItem("处理同步冲突…", func(a *App) { a.runSyncMenuAction(false) }),
		h.workspaceMenuItem("放弃本机同步草稿…", func(a *App) { a.runSyncMenuAction(true) }))
	sessionMenu.Submenu = append(sessionMenu.Submenu, h.workspaceMenuItem("处理本地配置变更…", func(a *App) { a.openLocalConflict() }))
	sessionMenu.Submenu = append(sessionMenu.Submenu, h.workspaceMenuItem("迁移原版配置…", func(a *App) { a.showNativeMigration() }))
	sessionMenu.Submenu = append(sessionMenu.Submenu, mygo.Separator(), h.workspaceMenuItem("代理管理…", func(a *App) {
		if !a.passwordLocked() {
			a.showProxyManager()
		} else {
			a.showVault()
		}
	}))

	appMenu.Submenu = append(appMenu.Submenu, h.workspaceMenuItem(h.updateMenuLabel(), func(a *App) { a.invokeUpdater() }), mygo.Separator(), item("quit"))
	viewMenu := menu("视图", "focusFullscreen", "-", "splitVertical", "splitHorizontal", "zoomPane", "nextPane", "previousPane", "-", "nextTab", "previousTab", "recentTab", "tabNumber", "tab1", "tab2", "tab3", "tab4", "tab5", "tab6", "tab7", "tab8", "tab9", "-", "newGroup", "showGroups", "arrangeTabs", "arrangeHorizontal", "arrangeVertical", "arrangeTiled")
	viewMenu.Submenu = append(viewMenu.Submenu, h.workspaceMenuItem("显示/隐藏会话侧栏", func(a *App) { a.toggleSidebar() }))
	viewMenu.Submenu = append(viewMenu.Submenu, h.workspaceMenuItem("全心专注 / 退出", func(a *App) { a.toggleDeepFocus() }))
	viewMenu.Submenu = append(viewMenu.Submenu, h.workspaceMenuItem("刷新标题识别", func(a *App) { a.refreshActiveHostFromMenu() }))
	return []*mygo.MenuItem{
		appMenu,
		sessionMenu,
		h.nativeQuickConnectionMenu(),
		menu("编辑", "copy", "paste", "selectAll", "-", "find", "findNext", "findPrevious"),
		viewMenu,
		menu("窗口", "newWindow", "closePane", "closeTab", "-", "hide", "minimize"),
		h.nativeCommandMenu(),
		menu("工具", "logging", "links", "quickSend", "focusQuickSend", "composer", "syncInput", "stopSync", "files", "quickCommands", "highlights", "appearance"),
	}
}

func (a *App) acceptCapturedShortcut(shortcut core.KeyboardShortcut) bool {
	f := &a.features
	if f.settingsCapture == "" {
		return false
	}
	if !shortcut.ValidFor(f.settingsCapture, runtime.GOOS) {
		f.shortcutError = "请使用有效组合键或功能键；终端替代键也支持 Home，不能占用原有 Ctrl+字母控制键。"
		return false
	}
	if other := f.settingsDraft.KeyboardShortcuts.Conflict(shortcut, f.settingsCapture, runtime.GOOS); other != "" {
		f.shortcutError = fmt.Sprintf("与“%s”冲突，请换一个组合", other)
		return false
	}
	if err := a.checkSystemShortcut(shortcut); err != nil {
		f.shortcutError = err.Error()
		return false
	}
	if f.settingsDraft.KeyboardShortcuts.Overrides == nil {
		f.settingsDraft.KeyboardShortcuts.Overrides = map[string]core.ShortcutOverride{}
	}
	f.settingsDraft.KeyboardShortcuts.Overrides[f.settingsCapture] = core.ShortcutOverride{Shortcut: &shortcut}
	f.settingsCapture = ""
	f.shortcutError = ""
	a.refreshMenu()
	return true
}

package desktop

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
)

type filePresentationState struct {
	owner            atomic.Pointer[App]
	visible, focused bool
	placement        string
	focus            ui.Handle
	split            groupNode
	draft, original  core.FileManagementSettings
	loadError        error
	commandPane      *pane
	commandTerminal  *terminal.Terminal
	command          string
	confirmOpen      bool
}

func (a *App) fileParent() *mygo.Window {
	if owner := a.filePresentation.owner.Load(); owner != nil {
		return owner.win
	}
	return a.win
}
func (a *App) hideFileDock() {
	f := &a.filePresentation
	f.visible = false
	f.focused = false
	if owner := f.owner.Load(); owner != nil && owner.win != nil {
		owner.win.Invalidate()
		if p := owner.activePane(); p != nil {
			p.focus.Focus()
		}
	}
}

// Only the host changes. The file App, tabs, connection leases and workers are
// retained when docking, hiding or returning to a separate window.
func (h *applicationHub) presentFileWorkspace(owner *App, placement string) {
	files := h.fileWindow
	if files == nil {
		return
	}
	if placement == "" {
		settings, err := core.LoadFileManagementSettings(h.store.Dir)
		if err != nil {
			files.status = err.Error()
			placement = "window"
		} else {
			placement = settings.Placement
		}
	}
	if owner == nil || owner == files || owner.stopped.Load() {
		owner = files.filePresentation.owner.Load()
		if owner == nil || owner.stopped.Load() {
			for _, candidate := range h.windows {
				if !candidate.stopped.Load() {
					owner = candidate
				}
			}
		}
	}
	previous := files.filePresentation.owner.Load()
	if placement == "window" || owner == nil {
		files.filePresentation.owner.Store(nil)
		files.filePresentation.placement = "window"
		if files.win != nil {
			files.win.Show()
			files.win.Focus()
		}
	} else {
		files.filePresentation.owner.Store(owner)
		if files.filePresentation.placement != placement {
			files.filePresentation.split = groupNode{splitRatio: .55}
			if placement == "left" {
				files.filePresentation.split.splitRatio = .4
			}
		}
		files.filePresentation.placement = placement
		if files.win != nil {
			files.win.Hide()
		}
		if owner.win != nil {
			owner.win.Focus()
			owner.win.Invalidate()
		}
	}
	files.filePresentation.visible = true
	if previous != nil && previous.win != nil {
		previous.win.Invalidate()
	}
}
func (a *App) fileDockWorkspace(c *ui.Context) {
	terminalView := func() {
		region := ui.Box(c.Key("terminal-workspace")).Fill().MinWidth(0).MinHeight(0).Label("终端工作区").Children(func() { a.groupWorkspace(c, a.groups.root) })
		a.clockOverlay(c, region)
	}
	if a.hub == nil || a.hub.fileWindow == nil {
		terminalView()
		return
	}
	files := a.hub.fileWindow
	f := &files.filePresentation
	if f.owner.Load() != a || !f.visible || a.focusFullscreen.deep {
		terminalView()
		return
	}
	fileView := func() {
		ui.Box(c.Key("docked-file-workspace")).Fill().MinWidth(0).MinHeight(0).Bind(&f.focus).Label("停靠文件管理").Children(func() { files.fileWindowView(c.Key("file-content")) })
		f.focused = f.focus.FocusWithin(c)
	}
	f.split.vertical = f.placement == "bottom"
	if f.placement == "left" {
		a.groupSplit(c.Key("file-dock-split"), &f.split, fileView, terminalView)
	} else {
		a.groupSplit(c.Key("file-dock-split"), &f.split, terminalView, fileView)
	}
}

func (a *App) fileManagementSettings(c *ui.Context) {
	f := &a.filePresentation
	sectionTitle(c, "打开方式与位置")
	ui.Text(c, "切换内置面板位置时，保留文件标签、连接与传输。").Wrap()
	selectValue(c, "文件管理打开方式", &f.draft.Mode, [][2]string{{"builtIn", "内置文件管理"}, {"terminal", "当前终端运行命令"}})
	if f.draft.Mode == "builtIn" {
		selectValue(c, "文件管理位置", &f.draft.Placement, [][2]string{{"window", "独立窗口"}, {"left", "窗口左侧"}, {"right", "窗口右侧"}, {"bottom", "窗口底部"}})
	} else {
		ui.MenuButton(c, "选择常用命令", func(m *ui.Menu) {
			for _, command := range []string{"yazi", "ranger", "lf", "nnn", "mc"} {
				if m.Item(command).Chosen() {
					f.draft.Command = command
				}
			}
		})
		ui.TextInput(c, &f.draft.Command).Label("文件管理命令").FillWidth()
	}
	ui.Text(c, "命令在当前会话、当前目录执行，启动前需确认 Shell 空闲。对应工具需自行安装；不会另开连接或同步输入到其他终端。").Wrap()
	ui.Text(c, "SFTP / FTP、外部文件链接和上传仍使用内置管理器。位置和命令仅保存在本机，不参与共享同步。").Wrap()
	if f.loadError != nil {
		ui.Text(c, f.loadError.Error()).TextColor(c.Theme().Danger).Wrap()
	}
}
func (a *App) validateFileSettingsDraft() error {
	f := &a.filePresentation
	if f.loadError != nil {
		return f.loadError
	}
	current, err := core.LoadFileManagementSettings(a.store.Dir)
	if err != nil {
		return err
	}
	if current != f.original {
		return fmt.Errorf("文件管理设置已变化，请重新打开设置")
	}
	f.draft.Command = strings.TrimSpace(f.draft.Command)
	return f.draft.Validate()
}
func (a *App) applyFileSettingsDraft() error {
	f := &a.filePresentation
	if f.draft == f.original {
		return nil
	}
	if err := core.SaveFileManagementSettings(a.store.Dir, f.draft); err != nil {
		return err
	}
	if a.hub != nil && a.hub.fileWindow != nil && a.hub.fileWindow.filePresentation.visible && f.draft.Placement != f.original.Placement {
		owner := a
		if a == a.hub.fileWindow {
			owner = a.filePresentation.owner.Load()
		}
		a.hub.presentFileWorkspace(owner, f.draft.Placement)
	}
	f.original = f.draft
	return nil
}
func (a *App) terminalFileCommandBlockReason(p *pane) string {
	if a.workspaceLocked() || p == nil || p.closed || p.term == nil || (p.profile.Kind != "local" && p.profile.Kind != "ssh" && !p.terminalOnly) {
		return "请先选择已连接的 SSH 终端或本地 Shell"
	}
	if p.agent != nil && p.agent.Busy() {
		return "当前终端由 MCP 占用，请先接管"
	}
	if !p.term.FileCommandReady() || p.transfer.active {
		return "当前终端正在传输、认证或运行交互程序，请先回到空闲 Shell"
	}
	return ""
}
func (a *App) prepareTerminalFileManager(command string) {
	p := a.activePane()
	if reason := a.terminalFileCommandBlockReason(p); reason != "" {
		a.status = reason
		return
	}
	f := &a.filePresentation
	f.commandPane = p
	f.commandTerminal = p.term
	f.command = command
	f.confirmOpen = true
}
func (a *App) runTerminalFileCommand() {
	f := &a.filePresentation
	p := f.commandPane
	if reason := a.terminalFileCommandBlockReason(p); reason != "" {
		a.status = reason
		f.confirmOpen = false
		return
	}
	if p.term != f.commandTerminal || p.owner.Load() != nil && p.owner.Load() != a {
		a.status = "目标连接已变化，未发送命令"
		f.confirmOpen = false
		return
	}
	if p.agent != nil {
		if err := p.agent.Takeover(context.Background(), false); err != nil {
			a.status = err.Error()
			return
		}
	}
	// Send targets exactly this terminal and bypasses the keyboard broadcast path.
	p.term.Send([]byte(f.command + "\r"))
	p.focus.Focus()
	f.confirmOpen = false
}
func (a *App) terminalFileCommandDialog(c *ui.Context) {
	f := &a.filePresentation
	if !f.confirmOpen {
		return
	}
	desktopModal(c.Key("terminal-file-command"), &f.confirmOpen, func() {
		dialogBody(c, 540).Gap(10).Children(func() {
			ui.Text(c, "在当前终端启动文件管理？").Bold()
			ui.Text(c, "目标："+f.commandPane.profile.Name).Wrap()
			ui.Text(c, "命令："+f.command).Wrap()
			ui.Text(c, "请确认当前 Shell 空闲，没有正在运行的命令或密码提示。仅发送到此会话。").Wrap()
			dialogActions(c).Children(func() {
				ui.Button(c, "取消").OnClick(func() { f.confirmOpen = false })
				ui.PrimaryButton(c, "确认空闲并运行").OnClick(a.runTerminalFileCommand)
			})
		})
	})
	if !f.confirmOpen {
		f.commandPane = nil
		f.commandTerminal = nil
		f.command = ""
	}
}

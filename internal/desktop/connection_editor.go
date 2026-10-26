// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"context"
	"fmt"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

type tunnelEditor struct {
	Rule             core.TunnelRule
	Port, TargetPort string
}
type featureState struct {
	shortcutSystemNotice                                                                                    string
	sessionClose                                                                                            *sessionCloseRequest
	localConflict                                                                                           localConflictState
	managerReturn                                                                                           *managerReturnState
	nativeMigration                                                                                         nativeMigrationState
	editorKind                                                                                              string
	pendingTerminal                                                                                         bool
	about                                                                                                   *aboutDetails
	commandEdit, commandOriginal, commandDelete                                                             *core.QuickCommand
	commandRow                                                                                              int
	commandList                                                                                             ui.ListState
	directoryRows                                                                                           []sessionRow
	directorySidebar                                                                                        bool
	directoryPicker                                                                                         directoryPickerState
	sessionPassword                                                                                         sessionPasswordEditor
	linkEditor                                                                                              linkEditorState
	updates                                                                                                 updateController
	sharedSync                                                                                              sharedSyncState
	switcher                                                                                                switchState
	proxyManager                                                                                            proxyManagerState
	fontCatalog                                                                                             []core.FontEntry
	fontLoading                                                                                             bool
	fontError                                                                                               string
	fontCancel                                                                                              context.CancelFunc
	archive                                                                                                 archiveState
	settingsCapture, shortcutError, shortcutFilter                                                          string
	liveOpen                                                                                                bool
	livePane                                                                                                *pane
	liveKeep                                                                                                core.KeepAliveSettings
	liveInterval, liveError                                                                                 string
	reconnectTarget                                                                                         *pane
	commandError                                                                                            string
	operators                                                                                               operatorState
	highlightSets                                                                                           []core.HighlightSet
	highlightEditing, highlightActive, highlightError                                                       string
	defaults                                                                                                core.SessionDefaults
	defaultsOpen                                                                                            bool
	defaultsTab                                                                                             int
	defaultsSSHPort, defaultsFTPPort, defaultsInterval, defaultsMissed, defaultsIdleInterval, defaultsError string
	authPreparing                                                                                           bool
	authGeneration                                                                                          int
	authCancel                                                                                              context.CancelFunc
	savedPassword                                                                                           string
	savedIdentity                                                                                           *core.SSHIdentity
	startupUnlock                                                                                           bool
	settingsDraft                                                                                           core.Settings
	settingsTab                                                                                             int
	historyCustom                                                                                           bool
	historyCustomText                                                                                       string
	settingsLines, settingsError                                                                            string
	colorDraft                                                                                              core.ColorScheme

	syncOpen, syncEnabled     bool
	syncTargets               map[string]bool
	syncSelectionDraft        map[string]bool
	syncDraftEnabled          bool
	syncDraftError            string
	pasteOpen, pasteNoPreview bool
	pasteText                 string
	pasteTargets              []pasteDestination
	highlightOpen             bool

	closePending, allowClose                                        bool
	filePane                                                        *pane
	fileActionState                                                 *fileState
	fileActionDirectory, fileActionOriginal                         string
	scpDraft                                                        *standaloneSCPDraft
	uploadDraft                                                     *uploadConfirmation
	fileAction, fileName, fileError                                 string
	fileOpen                                                        bool
	editTab                                                         int
	proxyPort, interval, missed, idleInterval                       string
	tunnels                                                         []tunnelEditor
	proxyPassword, proxyKeyPassword                                 string
	savePassword                                                    bool
	passwordProtection                                              string
	vaultOpen, vaultBusy, resumeAuth                                bool
	reloadAfterUnlock                                               bool
	vaultMenuAfterUnlock                                            string
	vaultAction, master, masterAgain, masterCurrent, vaultError     string
	commandsOpen                                                    bool
	directoryOpen                                                   bool
	directoryAction, directorySource, directoryName, directoryError string
}

func (a *App) initConnectionEditor() {
	a.draftTitleMode = a.sessionTitleMode(a.draft)
	if _, exists := a.profileByID(a.draft.ID); !exists {
		a.draftTitleMode = a.localUI.DefaultTitleMode
	}
	a.initSessionPassword()
	a.features.editorKind = a.draft.Kind
	a.features.directoryPicker = directoryPickerState{}
	f := &a.features
	f.editTab = 0
	f.proxyPort = strconv.Itoa(a.draft.Proxy.Port)
	f.interval = strconv.Itoa(a.draft.KeepAlive.Interval)
	f.missed = strconv.Itoa(a.draft.KeepAlive.MaxMissed)
	f.idleInterval = strconv.Itoa(a.draft.KeepAlive.IdleInterval)
	f.tunnels = nil
	for _, r := range a.draft.Tunnels {
		f.tunnels = append(f.tunnels, tunnelEditor{r, strconv.Itoa(r.ListenPort), strconv.Itoa(r.DestinationPort)})
	}
}
func selectValue(c *ui.Context, label string, value *string, choices [][2]string) {
	s := ui.SelectBase(c, value)
	name := *value
	for _, v := range choices {
		if v[0] == *value {
			name = v[1]
		}
	}
	s.Trigger.Label(label).Padding(5, 8).Radius(4).Background(c.Theme().Surface).Border(1, c.Theme().Border).Children(func() { ui.Text(c, name).Grow(1); ui.Icon(c, chevronIcon).FontSize(12) })
	s.Popup(func(panel ui.Element) {
		panel.Padding(4).Background(c.Theme().Background).Border(1, c.Theme().Border).Radius(5)
		for _, v := range choices {
			item := s.Item(v[0]).Padding(6, 10)
			if item.Highlighted() {
				item.Background(c.Theme().Selection)
			}
			item.Children(func() { ui.Text(c, v[1]) })
		}
	})
}
func (a *App) connectionEditor(c *ui.Context) {
	a.syncEditorProtocol()
	ui.Modal(c, &a.editing, func() {
		ui.Column(c).Width(640).Gap(12).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Icon(c, settingsIcon).FontSize(20).TextColor(c.Theme().Accent)
				ui.Text(c, "会话属性").FontSize(17).Bold()
			})
			pages, sections := editorProtocolPages(a.draft.Kind)
			if a.features.editTab >= len(pages) {
				a.features.editTab = 0
			}
			ui.Tabs(c, &a.features.editTab, pages...)
			field := func(label string, v *string) { ui.Field(c, label, func() { ui.TextInput(c, v) }) }
			ui.Scroll(c).Height(335).Disabled(a.features.sessionPassword.busy).Children(func() {
				switch sections[a.features.editTab] {
				case 0:
					ui.Form(c, func() {
						ui.Field(c, "协议", func() {
							selectValue(c, "协议", &a.draft.Kind, [][2]string{{"ssh", "SSH 终端"}, {"sftp", "SFTP 文件会话"}, {"ftp", "FTP 文件会话（明文）"}})
							a.syncEditorProtocol()
							if a.draft.Kind == "ssh" {
								selectValue(c, "标签标题（仅本机）", &a.draftTitleMode, sessionTitleModes)
							}
						})
						field("名称", &a.draft.Name)
						a.sessionDirectoryField(c)
						field("主机地址", &a.draft.Host)
						field("端口", &a.port)
						field("用户名", &a.draft.Username)
						a.sessionPasswordFields(c)
						ui.Checkbox(c, &a.draft.QuickConnect, "显示在快捷连接菜单中")
						if a.draft.Kind != "ftp" {
							ui.Field(c, "私钥文件", func() {
								ui.Row(c).Gap(6).Children(func() {
									ui.TextInput(c, &a.draft.IdentityFile).Grow(1)
									ui.Button(c, "选择").OnClick(func() {
										go func() {
											paths, e := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "选择 SSH 私钥"})
											if e == nil && len(paths) > 0 {
												a.post(func() { a.draft.IdentityFile = paths[0] })
											}
										}()
									})
								})
							})
						}
						field("文件初始目录", &a.draft.InitialDirectory)
						if a.draft.Kind != "ftp" {
							ui.Checkbox(c, &a.draft.LegacySSH, "启用旧版 SSH 算法兼容（仅用于确有需要的服务器）")
						}
					}).Gap(10)
				case 1:
					a.proxyReferenceEditor(c)
				case 2:
					ui.Column(c).Gap(12).Children(func() {
						for i := range a.features.tunnels {
							v := &a.features.tunnels[i]
							ui.Column(c.Key(v.Rule.ID)).Gap(7).Padding(10).Border(1, c.Theme().Border).Radius(4).Children(func() {
								ui.Row(c).Gap(8).Children(func() {
									ui.Checkbox(c, &v.Rule.Enabled, "启用")
									ui.Box(c).Width(170).Children(func() {
										selectValue(c, "转发类型", &v.Rule.Kind, [][2]string{{"local", "本地转发"}, {"remote", "远程转发"}, {"dynamic", "动态 SOCKS4/5"}})
									})
									ui.Spacer(c)
									ui.Button(c, "删除").OnClick(func() { a.features.tunnels = append(a.features.tunnels[:i], a.features.tunnels[i+1:]...) })
								})
								ui.Row(c).Gap(8).Children(func() {
									ui.Column(c).Grow(1).Children(func() { field("监听地址", &v.Rule.BindHost) })
									ui.Column(c).Width(100).Children(func() { field("监听端口", &v.Port) })
								})
								if v.Rule.Kind != "dynamic" {
									ui.Row(c).Gap(8).Children(func() {
										ui.Column(c).Grow(1).Children(func() { field("目标主机", &v.Rule.DestinationHost) })
										ui.Column(c).Width(100).Children(func() { field("目标端口", &v.TargetPort) })
									})
								}
								field("备注", &v.Rule.Note)
							})
						}
						ui.Button(c, "添加端口转发").OnClick(func() {
							r := core.NewTunnel()
							a.features.tunnels = append(a.features.tunnels, tunnelEditor{r, "8080", "80"})
						})
						ui.Text(c, "默认只监听本机 127.0.0.1；远程转发还需服务端允许。").FontSize(11).TextColor(c.Theme().TextMuted)
					})
				case 3:
					ui.Button(c, "使用全局默认").OnClick(a.applyDefaultKeepAlive)
					ui.Form(c, func() {
						ui.Checkbox(c, &a.draft.KeepAlive.Enabled, "启用 SSH 协议保活")
						field("间隔（秒）", &a.features.interval)
						field("最大未响应次数", &a.features.missed)
						ui.Checkbox(c, &a.draft.KeepAlive.TCP, "启用 TCP KeepAlive")
						ui.Divider(c)
						ui.Checkbox(c, &a.draft.KeepAlive.IdleEnabled, "空闲时发送字符串").Disabled(a.draft.Kind != "ssh")
						idleDisabled := a.draft.Kind != "ssh" || !a.draft.KeepAlive.IdleEnabled
						ui.Field(c, "空闲间隔（秒）", func() { ui.TextInput(c, &a.features.idleInterval) }).Disabled(idleDisabled)
						ui.Field(c, "空闲字符串", func() { ui.TextInput(c, &a.draft.KeepAlive.IdleText) }).Disabled(idleDisabled)
						ui.Text(c, `支持 \n、\r、\t、\e 和 \\；修改在新连接中生效。`).FontSize(11).TextColor(c.Theme().TextMuted)
					}).Gap(10)
				}
			})
			if a.features.sessionPassword.busy {
				ui.Text(c, "正在保存会话和密码…").TextColor(c.Theme().TextMuted)
			}
			if a.formError != "" {
				ui.Text(c, a.formError).TextColor(c.Theme().Danger)
			}
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(func() { a.editing = false })
				ui.Button(c, "保存").Disabled(a.features.sessionPassword.busy).OnClick(func() { a.saveDraft(false) })
				ui.PrimaryButton(c, "保存并连接").Disabled(a.features.sessionPassword.busy).OnClick(func() { a.saveDraft(true) })
			})
		})
	})
	if !a.editing {
		a.cancelSessionPassword()
		if !a.features.linkEditor.open {
			a.restoreManagerReturn(nil)
		}
	}
}
func (a *App) applyConnectionEditor() error {
	a.syncEditorProtocol()
	if a.draft.Kind == "ftp" {
		a.draft.IdentityFile = ""
		a.draft.JumpHost = ""
		a.draft.ProxyID = ""
		a.draft.Proxy = core.ProxySettings{Kind: "none", Port: 1080}
		a.draft.Tunnels = nil
		a.draft.LegacySSH = false
		a.draft.KeepAlive = core.DefaultKeepAlive()
		a.draft.KeepAlive.IdleEnabled = false
		return nil
	}
	if a.draft.Kind != "ssh" {
		a.draft.KeepAlive.IdleEnabled = false
	}
	parse := func(s, label string) (int, error) {
		n, e := strconv.Atoi(s)
		if e != nil {
			return 0, fmt.Errorf("%s必须是整数", label)
		}
		return n, nil
	}
	var e error
	if a.features.proxyPort != "" {
		a.draft.Proxy.Port, e = parse(a.features.proxyPort, "代理端口")
		if e != nil {
			return e
		}
	}
	a.draft.KeepAlive.Interval, e = parse(a.features.interval, "保活间隔")
	if e != nil {
		return e
	}
	a.draft.KeepAlive.MaxMissed, e = parse(a.features.missed, "最大未响应次数")
	if e != nil {
		return e
	}
	a.draft.KeepAlive.IdleInterval, e = parse(a.features.idleInterval, "空闲间隔")
	if e != nil {
		return e
	}
	a.draft.Tunnels = nil
	for _, t := range a.features.tunnels {
		r := t.Rule
		r.ListenPort, e = parse(t.Port, "隧道监听端口")
		if e != nil {
			return e
		}
		if r.Kind != "dynamic" {
			r.DestinationPort, e = parse(t.TargetPort, "隧道目标端口")
			if e != nil {
				return e
			}
		}
		a.draft.Tunnels = append(a.draft.Tunnels, r)
	}
	return nil
}

func (a *App) applyDefaultKeepAlive() {
	a.draft.KeepAlive = a.config.SessionDefaults.KeepAlive
	if a.draft.Kind != "ssh" {
		a.draft.KeepAlive.IdleEnabled = false
	}
	a.features.interval = strconv.Itoa(a.draft.KeepAlive.Interval)
	a.features.missed = strconv.Itoa(a.draft.KeepAlive.MaxMissed)
	a.features.idleInterval = strconv.Itoa(a.draft.KeepAlive.IdleInterval)
}

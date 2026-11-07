// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strconv"
	"strings"
)

func (a *App) newBlank() {
	p := core.NewProfile()
	p.Kind = "blank"
	p.Name = "本机网络工具"
	a.open(p, core.Credentials{}, 0)
}
func (a *App) showLiveProperties() {
	p := a.activePane()
	if !canEditLiveKeepAlive(p) {
		return
	}
	f := &a.features
	f.livePane = p
	f.liveKeep = p.profile.KeepAlive
	f.liveInterval = strconv.Itoa(f.liveKeep.IdleInterval)
	f.liveSSHInterval = strconv.Itoa(f.liveKeep.Interval)
	f.liveMissed = strconv.Itoa(f.liveKeep.MaxMissed)
	f.liveError = ""
	f.liveOpen = true
}
func (a *App) livePropertiesDialog(c *ui.Context) {
	f := &a.features
	desktopModal(c, &f.liveOpen, func() {
		dialogBody(c, 430).Gap(12).Children(func() {
			web := f.livePane != nil && f.livePane.profile.RuntimeBastionEndpoint != ""
			if web {
				windowHeading(c, "堡垒机会话 · 保持活动")
				ui.Checkbox(c, &f.liveKeep.Enabled, "启用 SSH 协议保活")
				ui.TextInput(c, &f.liveSSHInterval).Label("SSH 保活间隔（秒）")
				ui.TextInput(c, &f.liveMissed).Label("最大未响应次数")
				ui.Checkbox(c, &f.liveKeep.TCP, "启用 TCP KeepAlive")
			} else {
				windowHeading(c, "当前会话空闲保活")
			}
			ui.Checkbox(c, &f.liveKeep.IdleEnabled, "空闲时发送文本")
			ui.TextInput(c, &f.liveInterval).Label("间隔（秒）")
			ui.TextInput(c, &f.liveKeep.IdleText).Label(`发送文本（支持 \n、\r、\t、\e 和 \\）`)
			hint := "仅影响当前连接，不改写保存的会话配置。"
			button := "应用到当前连接"
			if web {
				hint = "空闲发送立即生效。SSH 和 TCP 保活在下次连接 " + f.livePane.profile.RuntimeBastionEndpoint + " 时生效。"
				button = "应用并保存"
			}
			ui.Text(c, hint).TextColor(c.Theme().TextMuted)
			if f.liveError != "" {
				ui.Text(c, f.liveError).TextColor(c.Theme().Danger)
			}
			dialogActions(c).Children(func() {
				ui.Button(c, "取消").OnClick(func() { f.liveOpen = false })
				ui.PrimaryButton(c, button).OnClick(func() {
					v, e := strconv.Atoi(f.liveInterval)
					if e != nil {
						f.liveError = "间隔需为整数"
						return
					}
					p := f.livePane
					if p == nil || p.closed || p.conn == nil {
						f.liveError = "连接已关闭"
						return
					}
					f.liveKeep.IdleInterval = v
					if web {
						f.liveKeep.Interval, e = strconv.Atoi(f.liveSSHInterval)
						if e != nil {
							f.liveError = "SSH 保活间隔需为整数"
							return
						}
						f.liveKeep.MaxMissed, e = strconv.Atoi(f.liveMissed)
						if e != nil {
							f.liveError = "最大未响应次数需为整数"
							return
						}
					}
					if e = p.conn.UpdateIdleKeepAlive(f.liveKeep); e != nil {
						f.liveError = e.Error()
						return
					}
					p.profile.KeepAlive = f.liveKeep
					if endpoint := p.profile.RuntimeBastionEndpoint; endpoint != "" {
						if e = core.SaveBastionKeepAlive(a.store.Dir, endpoint, f.liveKeep); e != nil {
							f.liveError = "当前连接已应用，本机偏好保存失败：" + e.Error()
							return
						}
					}
					f.liveOpen = false
				})
			})
		})
	})
}
func (a *App) reconnectActive() {
	p := a.activePane()
	if p == nil {
		return
	}
	if p.profile.Kind == "ftp" && p.ftp != nil {
		a.reconnectFTPFiles(p)
		return
	}
	if a.opensAsFileSession(p.profile) && !p.terminalOnly {
		a.reopenSSHFileSession(p, true)
		return
	}

	begin := func() {
		profile := p.profile
		// Saved keepalive options apply on a new connection. Reconnecting an
		// existing tab must not silently retain its old disabled/default values.
		// Keep the current host identity and one-off connections unchanged.
		if !p.externalLaunch {
			for _, saved := range a.config.Profiles {
				if strings.EqualFold(saved.ID, profile.ID) && saved.Kind == profile.Kind && saved.Identity() == profile.Identity() {
					profile.KeepAlive = saved.KeepAlive
					break
				}
			}
		}
		a.features.reconnectTarget = p
		a.request(profile, -1, p.terminalOnly)
	}
	if a.win == nil || p.term == nil || !p.term.Available() || (p.term.InLocalToolsMode() && !p.term.ToolsProcessActive()) {
		begin()
		return
	}
	go func() {
		r, e := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.win, Title: "重新连接", Message: "结束当前连接或本机工具，并重新建立原会话？", Buttons: []string{"取消", "重新连接"}, DefaultButton: 0, CancelButton: 0})
		a.post(func() {
			if e == nil && r.Button == 1 && !p.closed {
				begin()
			}
		})
	}()
}

func canEditLiveKeepAlive(p *pane) bool {
	return p != nil && !p.closed && p.conn != nil && p.term != nil && p.term.Available() && !p.term.InLocalToolsMode()
}

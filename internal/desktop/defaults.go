// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"strconv"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func (a *App) newProfile(kind string) core.Profile {
	return a.config.SessionDefaults.MakeProfile(kind, a.sessionActionGroup())
}
func (a *App) showDefaults() {
	a.closeManager()
	f := &a.features
	f.defaults = a.config.SessionDefaults
	f.defaultsSSHPort = strconv.Itoa(f.defaults.SSHPort)
	f.defaultsFTPPort = strconv.Itoa(f.defaults.FTPPort)
	f.defaultsInterval = strconv.Itoa(f.defaults.KeepAlive.Interval)
	f.defaultsMissed = strconv.Itoa(f.defaults.KeepAlive.MaxMissed)
	f.defaultsIdleInterval = strconv.Itoa(f.defaults.KeepAlive.IdleInterval)
	f.defaultsError = ""
	f.defaultsOpen = true
}
func (a *App) defaultsDialog(c *ui.Context) {
	f := &a.features
	d := &f.defaults
	desktopModal(c, &f.defaultsOpen, func() {
		dialogBody(c, 550).Gap(12).Children(func() {
			windowHeading(c, "会话默认属性")
			ui.Text(c, "用于以后新建的会话，不覆盖已有配置。").TextColor(c.Theme().TextMuted)
			ui.Tabs(c, &f.defaultsTab, "SSH / SFTP", "FTP", "保持活动")
			field := func(label string, s *string) { ui.Field(c, label, func() { ui.TextInput(c, s) }) }
			ui.Scroll(c).Height(300).Children(func() {
				ui.Form(c, func() {
					switch f.defaultsTab {
					case 0:
						field("SSH 端口", &f.defaultsSSHPort)
						field("用户名", &d.SSHUsername)
						field("私钥路径", &d.IdentityFile)
						field("文件初始目录", &d.SSHDirectory)
						ui.Checkbox(c, &d.LegacySSH, "旧 SSH 算法兼容")
						ui.Checkbox(c, &d.QuickConnect, "显示在快捷连接菜单中")
					case 1:
						field("FTP 端口", &f.defaultsFTPPort)
						field("FTP 用户名", &d.FTPUsername)
						field("FTP 初始目录", &d.FTPDirectory)
					case 2:
						ui.Checkbox(c, &d.KeepAlive.Enabled, "SSH 协议保活")
						field("间隔（秒）", &f.defaultsInterval)
						field("最大未响应次数", &f.defaultsMissed)
						ui.Checkbox(c, &d.KeepAlive.TCP, "TCP KeepAlive")
						ui.Checkbox(c, &d.KeepAlive.IdleEnabled, "空闲时发送字符串")
						field("空闲间隔（秒）", &f.defaultsIdleInterval)
						field("空闲字符串", &d.KeepAlive.IdleText)
					}
				})
			})
			if f.defaultsError != "" {
				ui.Text(c, f.defaultsError).TextColor(c.Theme().Danger)
			}
			dialogActions(c).Children(func() {
				ui.Button(c, "取消").OnClick(func() { f.defaultsOpen = false })
				ui.PrimaryButton(c, "保存默认属性").OnClick(func() {
					var e error
					fields := []struct {
						value *int
						text  string
					}{{&d.SSHPort, f.defaultsSSHPort}, {&d.FTPPort, f.defaultsFTPPort}, {&d.KeepAlive.Interval, f.defaultsInterval}, {&d.KeepAlive.MaxMissed, f.defaultsMissed}, {&d.KeepAlive.IdleInterval, f.defaultsIdleInterval}}
					for _, field := range fields {
						*field.value, e = strconv.Atoi(field.text)
						if e != nil {
							f.defaultsError = "端口和间隔必须为整数"
							return
						}
					}
					if e = d.Validate(); e != nil {
						f.defaultsError = e.Error()
						return
					}
					if a.changeConfig(func(cfg *core.Config) error { cfg.SessionDefaults = *d; return nil }) {
						f.defaultsOpen = false
					}
				})
			})
		})
	})
}

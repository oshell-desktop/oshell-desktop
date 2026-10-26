// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"net"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
)

func (a *App) dialogs(c *ui.Context) {
	a.imagePasteDialog(c)
	a.connectionEditor(c)
	ui.Modal(c, &a.authOpen, func() {
		ui.Column(c).Width(440).Gap(14).Children(func() {
			ui.Text(c, "连接 "+a.pendingProfile.Name).FontSize(17).Bold()
			endpoint := a.pendingProfile.Username + "@" + a.pendingProfile.Address()
			if identity := a.features.savedIdentity; identity != nil {
				endpoint = identity.User + "@" + net.JoinHostPort(identity.Host, strconv.Itoa(identity.Port))
			}
			ui.Text(c, endpoint).TextColor(c.Theme().TextMuted)
			if a.features.authPreparing {
				ui.Text(c, "正在解析目标并读取已保存凭据…").TextColor(c.Theme().TextMuted)
				ui.Button(c, "取消").OnClick(func() { a.authOpen = false; a.authQueue = nil })
				return
			}
			ui.Field(c, "登录密码（私钥 / Agent 认证可留空）", func() {
				revealPasswordInput(c, &a.password, &a.authPasswordRevealed).AutoFocus().Disabled(a.features.authPreparing)
			})
			if a.pendingProfile.IdentityFile != "" {
				ui.Field(c, "私钥口令（未加密可留空）", func() { passwordInput(c, &a.keyPassword) })
			}
			if a.pendingProfile.Proxy.Kind != "" && a.pendingProfile.Proxy.Kind != "none" || a.pendingProfile.JumpHost != "" {
				ui.Field(c, "代理 / 跳板机密码", func() { passwordInput(c, &a.features.proxyPassword) })
				if a.pendingProfile.Proxy.Kind == "jump" || a.pendingProfile.JumpHost != "" {
					ui.Field(c, "跳板机私钥口令", func() { passwordInput(c, &a.features.proxyKeyPassword) })
				}
			}
			ui.Checkbox(c, &a.features.savePassword, "密码认证成功后保存登录密码").Disabled(a.vault == nil)
			if a.features.savePassword {
				passwordProtectionField(c, &a.features.passwordProtection)
			}
			ui.Text(c, "保存此会话的登录密码。代理密码可在代理管理中保存。").FontSize(11).TextColor(c.Theme().TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "取消").OnClick(func() { a.password = ""; a.keyPassword = ""; a.authOpen = false; a.authQueue = nil })
				ui.PrimaryButton(c, "连接").Disabled(a.features.authPreparing).OnClick(func() {
					cred := core.Credentials{OpenTerminal: a.features.pendingTerminal, Password: a.password, KeyPassphrase: a.keyPassword, ProxyPassword: a.features.proxyPassword, ProxyKeyPassphrase: a.features.proxyKeyPassword, SavePassword: a.features.savePassword, PasswordProtection: a.features.passwordProtection}
					if a.features.savedIdentity != nil {
						identity := *a.features.savedIdentity
						cred.ExpectedIdentity = &identity
					}
					a.password = ""
					a.keyPassword = ""
					a.authOpen = false
					a.open(a.pendingProfile, cred, a.pendingSplit)
					a.nextAuthentication()
				})
			})
		})
	})
	// Modal can also be dismissed by Escape or by clicking its backdrop.
	if !a.authOpen {
		a.authPasswordRevealed = false
		if a.features.authPreparing {
			a.features.authGeneration++
			if a.features.authCancel != nil {
				a.features.authCancel()
			}
			a.features.authPreparing = false
		}
		a.features.savedPassword = ""
		a.features.savedIdentity = nil
		a.features.proxyPassword = ""
		a.features.proxyKeyPassword = ""
		a.password = ""
		a.keyPassword = ""
	}
	a.featureDialogs(c)
	a.refreshAuthentication()
}
func (a *App) saveDraft(connect bool) {
	if a.passwordLocked() || a.configurationBusy() || a.features.sessionPassword.busy {
		return
	}
	if err := a.applyConnectionEditor(); err != nil {
		a.formError = err.Error()
		return
	}
	port, err := strconv.Atoi(a.port)
	if err != nil {
		a.formError = "端口必须是数字"
		return
	}
	p := a.draft
	p.Port = port
	if p.Kind == "" {
		p.Kind = "ssh"
	}
	p.Name = strings.TrimSpace(p.Name)
	p.Host = strings.TrimSpace(p.Host)
	p.Username = strings.TrimSpace(p.Username)
	p.Group, err = core.NormalizeDirectory(p.Group)
	if err != nil {
		a.formError = err.Error()
		return
	}

	if err = p.Validate(); err != nil {
		a.formError = err.Error()
		return
	}
	if a.saveSessionCredential(p, connect) {
		return
	}
	old := a.config.Profiles
	a.config.Profiles = append([]core.Profile(nil), old...)
	found := false
	for i, v := range a.config.Profiles {
		if v.ID == p.ID {
			a.config.Profiles[i] = p
			found = true
			break
		}
	}
	if !found {
		a.config.Profiles = append(a.config.Profiles, p)
	}
	if !a.save() {
		if a.features.localConflict.open {
			id := p.ID
			a.features.localConflict.afterSave = func() {
				if saved, ok := a.profileByID(id); ok {
					a.finishSavedDraft(saved, connect)
				}
			}
		}
		a.config.Profiles = old
		a.formError = a.status
		return
	}
	a.finishSavedDraft(p, connect)
}
func (a *App) applyAppearance() {
	for _, t := range a.tabs {
		a.walk(t.root, func(p *pane) {
			if p.term != nil {
				theme := terminalTheme(a.config.Settings.Scheme())
				p.term.SetTheme(theme, nil)
				p.term.SetFont(terminal.Font{Family: terminalFamily(a.config.Settings.FontFamily), Size: float32(a.config.Settings.FontSize)})
				a.applyTerminalPreferences(p)
			}
		})
	}
}
func (a *App) report(err error) {
	if err != nil {
		a.post(func() { a.status = "错误：" + err.Error() })
	}
}

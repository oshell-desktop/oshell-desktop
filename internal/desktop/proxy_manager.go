// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"context"
	"errors"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strconv"
	"strings"
)

type proxyManagerState struct {
	protection                      string
	open, editing, busy, remember   bool
	selected, password, port, error string
	draft                           core.ProxyProfile
	original                        *core.ProxyProfile
}

func (a *App) showProxyManager() { a.features.proxyManager = proxyManagerState{open: true} }
func (a *App) editProxy(p *core.ProxyProfile) {
	f := &a.features.proxyManager
	f.editing = true
	f.password = ""
	f.error = ""
	f.original = nil
	f.remember = false
	f.protection = "local"
	f.draft = core.NewProxyProfile()
	if p != nil {
		v := *p
		f.original = &v
		f.draft = v
		if a.vault != nil {
			f.remember = a.vault.HasCredential(core.ProxyCredentialProfile(p.Settings))
			f.protection = a.vault.CredentialProtection(core.ProxyCredentialProfile(p.Settings).ID)
		}
	}
	f.port = strconv.Itoa(f.draft.Settings.Port)
}
func (a *App) saveProxy() {
	f := &a.features.proxyManager
	if f.busy || a.configurationBusy() {
		return
	}
	port, e := strconv.Atoi(f.port)
	if e != nil {
		f.error = "端口需为整数"
		return
	}
	f.draft.Settings.Port = port
	candidate := a.config.Clone()
	found := false
	for i, p := range candidate.Proxies {
		if p.ID() == f.draft.ID() {
			candidate.Proxies[i] = f.draft
			found = true
		}
	}
	if !found {
		candidate.Proxies = append(candidate.Proxies, f.draft)
	}
	if e = candidate.Validate(); e != nil {
		f.error = e.Error()
		return
	}
	if f.remember && f.password == "" && (f.original == nil || !a.vault.HasCredential(core.ProxyCredentialProfile(f.original.Settings))) {
		f.error = "请输入代理密码，或取消保存"
		return
	}
	password := f.password
	f.password = ""
	definition := f.draft
	protection, original := f.protection, f.original
	vaultRevision := a.vault.Revision()
	remember := f.remember && (definition.Settings.Kind == "jump" || definition.Settings.Kind == "http" || definition.Settings.Kind == "socks5")
	if remember && password == "" && f.original != nil {
		old := f.original.Settings
		next := definition.Settings
		if old.Host != next.Host || old.Port != next.Port || old.Username != next.Username || old.Kind != next.Kind {
			f.error = "代理目标已变化，请重新输入密码或取消保存"
			return
		}
	}
	if remember && password != "" && definition.Settings.Username == "" {
		f.error = "保存代理密码需要用户名"
		return
	}
	revision, e := a.store.Revision()
	if e != nil {
		f.error = e.Error()
		return
	}
	f.busy = true
	a.setConfigurationBusy(true)
	go func() {
		var updates []core.CredentialUpdate
		var e error
		profile := core.ProxyCredentialProfile(definition.Settings)
		identity := profile.Identity()
		if remember {
			if definition.Settings.Kind == "jump" {
				var resolved core.ResolvedSSH
				resolved, e = core.ResolveSSH(context.Background(), profile)
				identity = resolved.Profile.Identity()
			}
			if e == nil {
				var old *core.Profile
				if original != nil {
					value := core.ProxyCredentialProfile(original.Settings)
					old = &value
				}
				updates, e = a.vault.PrepareCredentialEdit(old, profile, password, true, identity, protection)
			}
		}
		if !remember {
			updates = append(updates, core.CredentialUpdate{Profile: profile, Delete: true})
		}
		if e == nil {
			e = a.store.CommitCredentialEdit(context.Background(), candidate, a.vault, updates, revision, vaultRevision)
		}
		retry := localConflictRetry(e, candidate, updates)
		for i := range updates {
			updates[i].Password = ""
		}
		a.post(func() {
			f.busy = false
			if !errors.Is(e, core.ErrPendingTransaction) {
				a.setConfigurationBusy(false)
			}
			if e != nil {
				f.error = e.Error()
				if retry != nil {
					retry(a)
				}
				return
			}
			a.applyCommittedConfig(candidate)
			f.selected = definition.ID()
			f.editing = false
		})
	}()
}
func (a *App) removeProxy(id string) {
	for _, p := range a.config.Profiles {
		if strings.EqualFold(p.ProxyID, id) {
			a.features.proxyManager.error = "仍有会话引用此代理，请先更换会话的代理"
			return
		}
	}
	for _, p := range a.config.Proxies {
		if strings.EqualFold(p.UpstreamID, id) {
			a.features.proxyManager.error = "仍有下级代理引用此代理，请先更换上级"
			return
		}
	}
	a.changeConfig(func(cfg *core.Config) error {
		for i, p := range cfg.Proxies {
			if p.ID() == id {
				cfg.Proxies = append(cfg.Proxies[:i], cfg.Proxies[i+1:]...)
				break
			}
		}
		return nil
	})
}
func (a *App) proxyManagerDialog(c *ui.Context) {
	f := &a.features.proxyManager
	desktopModal(c, &f.open, func() {
		dialogBody(c, 700).Gap(10).Children(func() {
			windowHeading(c, "共享代理管理")
			ui.Text(c, "供 SSH/SFTP 会话共用；上级代理组成最多 8 级链路。修改只影响新连接。").TextColor(c.Theme().TextMuted)
			if f.editing {
				ui.Form(c, func() {
					ui.Field(c, "名称", func() { ui.TextInput(c, &f.draft.Name) })
					ui.Field(c, "类型", func() {
						selectValue(c, "代理类型", &f.draft.Settings.Kind, [][2]string{{"socks5", "SOCKS5"}, {"http", "HTTP CONNECT"}, {"socks4", "SOCKS4"}, {"socks4a", "SOCKS4A"}, {"jump", "SSH 跳板机"}})
					})
					ui.Field(c, "主机", func() { ui.TextInput(c, &f.draft.Settings.Host) })
					ui.Field(c, "端口", func() { ui.TextInput(c, &f.port) })
					ui.Field(c, "用户名", func() { ui.TextInput(c, &f.draft.Settings.Username) })
					choices := [][2]string{{"", "无（直接连接）"}}
					for _, p := range a.config.Proxies {
						if p.ID() != f.draft.ID() {
							choices = append(choices, [2]string{p.ID(), p.Name})
						}
					}
					ui.Field(c, "上级代理", func() { selectValue(c, "上级代理", &f.draft.UpstreamID, choices) })
					if f.draft.Settings.Kind == "jump" {
						if f.draft.Settings.SSHAuthentication == "" {
							f.draft.Settings.SSHAuthentication = "automatic"
						}
						ui.Field(c, "SSH 认证", func() {
							selectValue(c, "SSH 认证", &f.draft.Settings.SSHAuthentication, [][2]string{{"automatic", "自动 / SSH Agent"}, {"password", "密码"}, {"privateKey", "私钥"}})
						})
						ui.Field(c, "私钥路径", func() {
							ui.Row(c).Gap(6).Children(func() {
								ui.TextInput(c, &f.draft.Settings.IdentityFile).Grow(1)
								ui.Button(c, "选择…").OnClick(func() {
									go func() {
										files, e := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "选择跳板机私钥"})
										a.post(func() {
											if e == nil && len(files) > 0 {
												f.draft.Settings.IdentityFile = files[0]
											}
										})
									}()
								})
							})
						})
					}
					if f.draft.Settings.Kind == "jump" || f.draft.Settings.Kind == "socks5" || f.draft.Settings.Kind == "http" {
						ui.Checkbox(c, &f.remember, "保存代理密码")
						if f.remember {
							passwordProtectionField(c, &f.protection)
						}
						ui.Field(c, "密码", func() {
							passwordInput(c, &f.password).Placeholder("已保存时留空保留；新密码会替换旧值").Disabled(!f.remember)
						})
					}
				}).Gap(8)
				if f.error != "" {
					ui.Text(c, f.error).TextColor(c.Theme().Danger)
				}
				dialogActions(c).Children(func() {
					ui.Button(c, "取消编辑").Disabled(f.busy).OnClick(func() { f.editing = false; f.password = "" })
					ui.PrimaryButton(c, "保存代理").Disabled(f.busy).OnClick(a.saveProxy)
				})
			} else {
				ui.Scroll(c).Height(280).Gap(5).Children(func() {
					for _, p := range a.config.Proxies {
						ui.Row(c.Key(p.ID())).Gap(8).Padding(6).Children(func() {
							ui.Text(c, p.Name).Width(160)
							ui.Text(c, p.Settings.Kind+" · "+p.Settings.Address()).Grow(1)
							ui.Button(c, "编辑").OnClick(func() { a.editProxy(&p) })
							ui.Button(c, "删除").OnClick(func() { a.removeProxy(p.ID()) })
						})
					}
				})
				if f.error != "" {
					ui.Text(c, f.error).TextColor(c.Theme().Danger)
				}
				ui.Row(c).Gap(8).Children(func() {
					ui.Button(c, "新增代理").OnClick(func() { a.editProxy(nil) })
					ui.Spacer(c)
					ui.Button(c, "关闭").OnClick(func() { f.open = false })
				})
			}
		})
	})
	if !f.open {
		f.password = ""
	}
}
func (a *App) proxyReferenceEditor(c *ui.Context) {
	choices := [][2]string{{"", "不使用共享代理"}}
	for _, p := range a.config.Proxies {
		choices = append(choices, [2]string{p.ID(), p.Name + " · " + p.Settings.Kind + " · " + p.Settings.Address()})
	}
	ui.Field(c, "共享代理", func() { selectValue(c, "共享代理", &a.draft.ProxyID, choices) })
	ui.Button(c, "管理共享代理…").OnClick(a.showProxyManager)
	if a.draft.ProxyID != "" {
		a.draft.Proxy.Kind = "none"
		a.draft.JumpHost = ""
		if route, e := core.ProxyRoute(a.draft.ProxyID, a.config.Proxies); e == nil {
			description := "本机"
			for _, p := range route {
				description += " → " + p.Name
			}
			ui.Text(c, description+" → 目标会话").TextColor(c.Theme().TextMuted)
		} else {
			ui.Text(c, e.Error()).TextColor(c.Theme().Danger)
		}
	} else {
		ui.TextInput(c, &a.draft.JumpHost).Label("旧跳板链（可选）").Placeholder("user@host:port，多级用逗号分隔")
	}
	ui.Text(c, fmt.Sprintf("已保存 %d 个共享代理；密码按每级独立读取。", len(a.config.Proxies))).FontSize(11).TextColor(c.Theme().TextMuted)
}

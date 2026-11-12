package desktop

import (
	"fmt"
	"oshell-desktop/internal/core"

	"github.com/egoist/mygo/ui"
)

type globalProxyState struct {
	saved     core.GlobalProxySettings
	loadError error
	open      bool
	draft     core.GlobalProxySettings
	error     string
}

func (a *App) connectionProfile(p core.Profile) (core.Profile, error) {
	if p.Kind != "ssh" && p.Kind != "sftp" {
		return a.config.ConnectionProfile(p)
	}
	s, err := core.LoadGlobalProxy(a.store.Dir)
	if err != nil {
		return p, fmt.Errorf("全局代理设置读取失败，连接已停止：%w", err)
	}
	return s.ConnectionProfile(a.config, p, a.vault)
}

func (a *App) showGlobalProxy() {
	if a.passwordLocked() {
		a.showVault()
		return
	}
	s, err := core.LoadGlobalProxy(a.store.Dir)
	a.features.globalProxy = globalProxyState{open: true, draft: s, saved: s, loadError: err}
	if err != nil {
		a.features.globalProxy.error = err.Error() + "；重新选择并应用可修复本机设置。"
	}
}

func (a *App) saveGlobalProxy() {
	f := &a.features.globalProxy
	if _, err := f.draft.ConnectionProfile(a.config, core.Profile{Kind: "ssh"}, a.vault); err != nil {
		f.error = err.Error()
		return
	}
	if err := f.draft.Save(a.store.Dir); err != nil {
		f.error = err.Error()
		return
	}
	f.open = false
	f.saved, f.loadError = f.draft, nil
	if a.hub != nil {
		for _, w := range a.hub.configurationWindows() {
			w.features.globalProxy.saved = f.saved
			w.features.globalProxy.loadError = nil
			if w.win != nil {
				w.win.Invalidate()
			}
		}
	}
	a.status = "全局代理设置已保存，仅对新连接生效"
}

func (a *App) globalProxyDialog(c *ui.Context) {
	f := &a.features.globalProxy
	desktopModal(c, &f.open, func() {
		dialogBody(c, 560).Gap(10).Children(func() {
			windowHeading(c, "全局代理（仅本机）")
			ui.Checkbox(c, &f.draft.Enabled, "启用全局代理")
			choices := [][2]string{{"", "请选择已有代理"}}
			found := f.draft.ProxyID == ""
			for _, p := range a.config.Proxies {
				choices = append(choices, [2]string{p.ID(), p.Name + " · " + p.Settings.Kind})
				if p.ID() == f.draft.ProxyID {
					found = true
				}
			}
			if !found {
				choices = append(choices, [2]string{f.draft.ProxyID, "所选代理已不存在，请重新选择"})
			}
			selectValue(c, "全局代理路线", &f.draft.ProxyID, choices)
			ui.Text(c, "新建 SSH、SFTP 及其 SCP 传输先经过全局代理，再经过会话代理或跳板。衔接处重叠的代理只经过一次，顺序冲突或超过 8 级会停止连接。").TextColor(c.Theme().TextMuted)
			ui.Text(c, "仅保存本机开关和选择。代理不可用时停止连接；不影响 FTP、同步、更新或本地命令。").TextColor(c.Theme().TextMuted)
			if f.error != "" {
				ui.Text(c, f.error).TextColor(c.Theme().Danger)
			}
			dialogActions(c).Children(func() {
				ui.Button(c, "管理已有代理…").OnClick(func() { f.open = false; a.showProxyManager() })
				ui.Spacer(c)
				ui.Button(c, "取消").OnClick(func() { f.open = false })
				ui.PrimaryButton(c, "应用全局代理").OnClick(a.saveGlobalProxy)
			})
		})
	})
}

func (a *App) proxyToolbar(c *ui.Context) {
	f := &a.features.globalProxy
	hint := "全局代理：未配置"
	if f.loadError != nil {
		hint = "全局代理：设置读取失败"
	} else if f.saved.ProxyID != "" {
		state := "已关闭"
		if f.saved.Enabled {
			state = "已开启"
		}
		name := "配置失效"
		for _, p := range a.config.Proxies {
			if p.ID() == f.saved.ProxyID {
				name = p.Name
				break
			}
		}
		hint = "全局代理：" + state + " · " + name
	}
	headerButton(c, "全局代理", networkIcon, "", false, f.saved.Enabled).Role(ui.RoleToggleButton).Checked(f.saved.Enabled).Tooltip(hint + "。点击开关，右键或 Alt 点击设置").OnClick(func() {
		if c.Modifiers()&ui.Alt != 0 {
			a.showGlobalProxy()
			return
		}
		a.showGlobalProxy()
		if f.error != "" || (!f.draft.Enabled && f.draft.ProxyID == "") {
			return
		}
		f.draft.Enabled = !f.draft.Enabled
		a.saveGlobalProxy()
	}).ContextMenu(func(m *ui.Menu) {
		if m.Item("全局代理设置…").Chosen() {
			a.showGlobalProxy()
		}
	})
}

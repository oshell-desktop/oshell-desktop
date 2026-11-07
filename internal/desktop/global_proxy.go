package desktop

import (
	"fmt"
	"oshell-desktop/internal/core"

	"github.com/egoist/mygo/ui"
)

type globalProxyState struct {
	open  bool
	draft core.GlobalProxySettings
	error string
}

func (a *App) connectionProfile(p core.Profile) (core.Profile, error) {
	if p.Kind != "ssh" && p.Kind != "sftp" {
		return a.config.ConnectionProfile(p)
	}
	s, err := core.LoadGlobalProxy(a.store.Dir)
	if err != nil {
		return p, fmt.Errorf("全局代理设置读取失败，连接已停止：%w", err)
	}
	return s.ConnectionProfile(a.config, p)
}

func (a *App) showGlobalProxy() {
	if a.passwordLocked() {
		a.showVault()
		return
	}
	s, err := core.LoadGlobalProxy(a.store.Dir)
	a.features.globalProxy = globalProxyState{open: true, draft: s}
	if err != nil {
		a.features.globalProxy.error = err.Error() + "；重新选择并应用可修复本机设置。"
	}
}

func (a *App) saveGlobalProxy() {
	f := &a.features.globalProxy
	if _, err := f.draft.ConnectionProfile(a.config, core.Profile{Kind: "ssh"}); err != nil {
		f.error = err.Error()
		return
	}
	if err := f.draft.Save(a.store.Dir); err != nil {
		f.error = err.Error()
		return
	}
	f.open = false
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
			ui.Text(c, "启用后，新建 SSH、SFTP 及其 SCP 传输使用所选代理链。关闭后恢复会话路线；既有连接与复用通道不受影响。").TextColor(c.Theme().TextMuted)
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

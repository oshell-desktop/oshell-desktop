package desktop

import (
	"github.com/egoist/mygo/ui"
	"strings"
)

func (a *App) hostKeyCleanupButton(c *ui.Context, p *pane) {
	if p.hostKeyMismatch == nil || p.closed {
		return
	}
	ui.Button(c, "清理旧主机密钥…").OnClick(func() {
		a.hostKeyCleanup = p
		a.hostKeyCleanupOpen = true
		a.hostKeyCleanupError = ""
	})
}

func (a *App) hostKeyCleanupDialog(c *ui.Context) {
	p := a.hostKeyCleanup
	if p == nil {
		return
	}
	if p.closed || p.hostKeyMismatch == nil {
		a.hostKeyCleanupOpen = false
	}
	if !a.hostKeyCleanupOpen {
		a.hostKeyCleanup = nil
		return
	}
	e := p.hostKeyMismatch
	desktopModal(c.Key("host-key-cleanup"), &a.hostKeyCleanupOpen, func() {
		dialogBody(c, 560).Gap(12).Children(func() {
			windowHeading(c, "清理旧主机密钥")
			ui.Text(c, "实际冲突地址："+e.Address).Wrap()
			ui.Text(c, "主机密钥变化可能来自服务器重装或被冒充。请先通过可信渠道核实服务器身份。清理后重新连接仍需确认新指纹。").Wrap()
			ui.Scroll(c).MaxHeight(160).Children(func() {
				ui.Text(c, "旧指纹：\n"+strings.Join(e.PreviousFingerprints(), "\n")).Wrap()
				ui.Text(c, "本次收到："+e.Algorithm+"\n"+e.Fingerprint).Wrap()
			})
			ui.Text(c, "仅清理此地址的冲突记录，其他主机保留。备份保存为 known_hosts.before-cleanup（保留最近一次）。").Wrap().TextColor(c.Theme().TextMuted)
			if a.hostKeyCleanupError != "" {
				ui.Text(c, a.hostKeyCleanupError).Wrap()
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "取消").OnClick(func() { a.hostKeyCleanupOpen = false })
				clean := func(reconnect bool) {
					if err := a.trust.RemoveHostKey(e); err != nil {
						a.hostKeyCleanupError = err.Error()
						return
					}
					p.hostKeyMismatch = nil
					p.status = "连接失败：旧主机密钥已清理，请重新连接并核对新指纹"
					a.status = "旧主机密钥已清理，未信任新密钥"
					a.hostKeyCleanupOpen = false
					if reconnect {
						a.reconnectPane(p)
					}
				}
				ui.Button(c, "确认清理").OnClick(func() { clean(false) })
				ui.PrimaryButton(c, "清理并重新连接").OnClick(func() { clean(true) })
			})
		})
	})
}

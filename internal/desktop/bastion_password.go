package desktop

import (
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

type bastionPasswordDialog struct {
	pane                             *pane
	conn                             *core.Connection
	vault                            *core.Vault
	target                           core.BastionTarget
	prompt, revision, typed, message string
	open                             bool
}

func (a *App) bastionPasswordTarget(p *pane) (core.BastionTarget, bool) {
	if p == nil || p.closed || p.externalLaunch || p.term == nil || !p.conn.Alive() || p.term.InLocalToolsMode() || p.owner.Load() != a || a.vault == nil {
		return core.BastionTarget{}, false
	}
	stored, ok := a.profileByID(p.profile.ID)
	if !ok || a.sessionType(stored) != bastionSSH || stored.Kind != p.profile.Kind || stored.Identity() != p.profile.Identity() || p.conn.Identity.User != stored.Username {
		return core.BastionTarget{}, false
	}
	return stored.BastionTarget()
}

func (a *App) openBastionPassword(p *pane) {
	if a.passwordLocked() {
		a.showVault()
		return
	}
	target, ok := a.bastionPasswordTarget(p)
	if !ok || a.configurationBusy() || a.modalActive() || a.activePane() != p {
		return
	}
	prompt, _ := p.term.HostPrompt()
	if !target.PasswordPrompt(prompt) {
		prompt = ""
	}
	a.bastionPassword = &bastionPasswordDialog{
		pane: p, conn: p.conn, vault: a.vault, target: target, open: true,
		prompt: prompt, revision: a.vault.Revision(),
	}
}

func (a *App) closeBastionPassword() {
	if d := a.bastionPassword; d != nil {
		d.typed = ""
		a.bastionPassword = nil
	}
}

func (a *App) validBastionPassword(d *bastionPasswordDialog) bool {
	if a.bastionPassword != d || a.activePane() != d.pane || a.vault != d.vault ||
		a.stopped.Load() || a.passwordLocked() || a.configurationBusy() || d.pane.conn != d.conn {
		return false
	}
	target, ok := a.bastionPasswordTarget(d.pane)
	return ok && target.Credential.Identity() == d.target.Credential.Identity() && target.Credential.ID == d.target.Credential.ID
}

func validTargetPassword(password string) bool {
	return password != "" && len(password) <= 4096 && !strings.ContainsFunc(password, unicode.IsControl)
}

func (a *App) saveBastionPassword(d *bastionPasswordDialog, forget bool) {
	if !a.validBastionPassword(d) {
		a.closeBastionPassword()
		return
	}
	profile := d.target.Credential
	var err error
	if forget {
		err = d.vault.DeleteForRevision(profile.ID, d.revision)
	} else {
		if !validTargetPassword(d.typed) {
			d.message = "请输入单行密码，最多 4096 字节。"
			return
		}
		err = d.vault.PutForRevision(profile, d.typed, d.conn.Identity, d.revision)
	}
	if err != nil {
		d.message = err.Error()
		return
	}
	d.revision = d.vault.Revision()
	d.typed = ""
	d.message = "目标服务器密码已保存"
	if forget {
		d.message = "已忘记目标服务器密码"
	}
}

func (a *App) fillBastionPassword(d *bastionPasswordDialog, submitAccount string) {
	if !a.validBastionPassword(d) {
		a.closeBastionPassword()
		return
	}
	if d.prompt == "" || d.typed != "" || d.vault.Revision() != d.revision {
		d.message = "请核对当前密码提示和已保存的密码后重新打开。"
		return
	}
	secret, err := d.vault.GetFor(d.target.Credential, d.conn.Identity)
	if err != nil {
		d.message = err.Error()
		return
	}
	if secret == "" {
		d.message = "当前连接身份没有已保存的目标密码。"
		return
	}
	result, ok := d.pane.term.FillServerPassword(d.conn, d.prompt, secret, submitAccount)
	if !ok {
		d.message = "终端或密码提示已变化，请重新打开。"
		return
	}
	a.closeBastionPassword()
	d.pane.awaitTargetRetry = submitAccount != ""
	d.pane.focus.Focus()
	go func() {
		err := <-result
		postPane(d.pane, func(owner *App) {
			if err != nil {
				owner.status = "填入目标密码失败：" + err.Error()
			} else if submitAccount != "" {
				owner.status = "已提交目标服务器密码"
			} else {
				owner.status = "目标密码已填入，请在该终端按回车"
			}
		})
	}()
}

func (a *App) drawBastionPassword(c *ui.Context) {
	if p := a.activePane(); p != nil && (p.offerTargetLogin || p.awaitTargetRetry) && !a.modalActive() && !a.passwordLocked() && !a.configurationBusy() {
		target, ok := a.bastionPasswordTarget(p)
		if !ok {
			p.offerTargetLogin = false
			p.awaitTargetRetry = false
		} else if p.awaitTargetRetry && p.term.TargetLoginRetry(target.Account) {
			p.awaitTargetRetry = false
			a.openBastionPassword(p)
			if dialog := a.bastionPassword; dialog != nil {
				dialog.message = "目标服务器再次请求密码，请核对密码后重试。"
			}
		} else if p.offerTargetLogin && p.term.TargetLoginPrompt(target.Account) {
			p.offerTargetLogin = false
			a.openBastionPassword(p)
			if dialog := a.bastionPassword; dialog != nil && a.vault.HasCredential(target.Credential) {
				a.fillBastionPassword(dialog, target.Account)
			}
		}
	}
	d := a.bastionPassword
	if d == nil {
		return
	}
	if !a.validBastionPassword(d) {
		a.closeBastionPassword()
		return
	}
	terminalModal(c.Key("bastion-target-password"), &d.open, func() {
		dialogBody(c, 470).Gap(10).Children(func() {
			windowHeading(c, "目标服务器密码")
			ui.Text(c, "堡垒机 "+d.conn.Identity.Host+"  ·  资产 "+d.target.Asset+"  ·  账号 "+d.target.Account).Wrap()
			saved := d.vault.HasCredential(d.target.Credential)
			label := "目标服务器密码"
			if saved {
				label = "新密码（修改时填写）"
			}
			ui.Field(c, label, func() { passwordInput(c, &d.typed).Label(label) })
			if d.prompt != "" {
				ui.Text(c, "终端提示："+d.prompt).Wrap()
			} else {
				ui.Text(c, "看到目标服务器密码提示后，可重新打开此窗口填入。").Wrap()
			}
			if d.message != "" {
				ui.Text(c, d.message).Wrap()
			}
			ui.Row(c).Gap(6).Justify(ui.End).Children(func() {
				ui.Button(c, "关闭").OnClick(a.closeBastionPassword)
				if saved {
					ui.Button(c, "忘记密码").OnClick(func() { a.saveBastionPassword(d, true) })
				}
				ui.Button(c, "保存密码").Disabled(d.typed == "").OnClick(func() { a.saveBastionPassword(d, false) })
				ui.PrimaryButton(c, "填入已保存密码").Disabled(!saved || d.prompt == "" || d.typed != "").OnClick(func() { a.fillBastionPassword(d, "") })
			})
		})
	})
	if !d.open {
		a.closeBastionPassword()
	}
}

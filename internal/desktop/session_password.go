package desktop

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

type sessionPasswordEditor struct {
	lastQuickLogin, serverAccount string
	targetIdentity                core.SSHIdentity
	targetVersion                 string
	targetPassword                string
	forgetTarget                  bool
	original                      *core.Profile
	originalRemember              bool
	originalProtection            string
	originalVersion               string
	queuedSave, queuedConnect     bool
	password                      string
	protection                    string
	remember, busy                bool
	revealed                      bool
	cancelRequested               bool
	cancel                        context.CancelFunc
}

func (a *App) initSessionPassword() {
	f := &a.features.sessionPassword
	*f = sessionPasswordEditor{protection: "local"}
	if p, ok := a.profileByID(a.draft.ID); ok {
		a.draft.ID = p.ID
		f.original = &p
	}
	if a.vault != nil {
		f.remember = a.vault.HasStoredCredential(a.draft.ID)
		f.protection = a.vault.CredentialProtection(a.draft.ID)
		f.originalRemember, f.originalProtection = f.remember, f.protection
		if f.original != nil {
			f.originalVersion = a.vault.CredentialVersion(*f.original)
		}
	}
	a.editorLogin()
	if a.features.editorChoice == bastionSSH {
		f.bindTarget(a.draft, a.vault)
	}
}

func (f *sessionPasswordEditor) bindTarget(p core.Profile, vault *core.Vault) (core.BastionTarget, bool) {
	target, ok := p.BastionTarget()
	identity := target.Credential.Identity()
	if identity != f.targetIdentity {
		if f.targetIdentity.User != "" {
			f.targetPassword = ""
		}
		f.targetIdentity = identity
		f.targetVersion, f.forgetTarget = "", false
		if ok && vault != nil {
			f.targetVersion = vault.CredentialVersion(target.Credential)
		}
	}
	return target, ok
}

func (a *App) sessionPasswordFields(c *ui.Context) {
	f := &a.features.sessionPassword
	bastion := a.features.editorChoice == bastionSSH
	label, remember := "密码", "保存密码"
	if bastion {
		label, remember = "堡垒机登录密码", "保存堡垒机登录密码"
	} else {
		f.targetPassword, f.forgetTarget = "", false
	}
	ui.Field(c, label, func() {
		revealPasswordInput(c, &f.password, &f.revealed).Label("会话密码").Placeholder("已保存（不回显）；留空沿用，输入新值替换")
	})
	ui.Checkbox(c, &f.remember, remember).Disabled(a.vault == nil)
	if f.remember {
		selectValue(c, "密码保护", &f.protection, [][2]string{{"local", "本机自动加密（无需主密码）"}, {"master", "使用主密码加密"}})
	}
	if bastion {
		p := a.draft
		p.Host = strings.TrimSpace(p.Host)
		p.Username, _ = a.editorLogin()
		p.Port, _ = strconv.Atoi(a.port)
		target, routed := f.bindTarget(p, a.vault)
		placeholder := "随会话保存到本机密码库"
		if f.targetVersion != "" {
			placeholder = "已保存，留空沿用，输入新值替换"
		}
		ui.Field(c, "服务器登录密码", func() {
			passwordInput(c.Key(f.targetIdentity), &f.targetPassword).Label("服务器登录密码").Placeholder(placeholder)
		}).Disabled(a.vault == nil || f.forgetTarget)
		if routed {
			ui.Text(c, "资产 "+target.Asset+"  ·  账号 "+target.Account).TextColor(c.Theme().TextMuted)
		}
		if f.targetVersion != "" {
			ui.Checkbox(c, &f.forgetTarget, "清除已保存的目标服务器密码")
		}
	}
}
func (a *App) saveSessionCredential(p core.Profile, connect bool) bool {
	f := &a.features.sessionPassword
	var target core.BastionTarget
	var editTarget bool
	if a.features.editorChoice == bastionSSH {
		var routed bool
		target, routed = f.bindTarget(p, a.vault)
		editTarget = routed && (f.targetPassword != "" || f.forgetTarget)
	}
	if a.vault == nil {
		if f.remember || editTarget {
			a.formError = "密码库不可用"
			return true
		}
		return false
	}
	// Keeping the same credential envelope needs neither decryption nor SSH
	// resolution. In particular, a renamed offline session remains editable.
	keepGateway := f.original != nil && f.original.Kind == p.Kind && f.original.Identity() == p.Identity() &&
		f.password == "" && f.remember == f.originalRemember && f.protection == f.originalProtection
	if keepGateway && !editTarget {
		return false
	}
	vaultRevision := a.vault.Revision()
	if !keepGateway && f.original != nil && a.vault.CredentialVersion(*f.original) != f.originalVersion {
		a.formError = "登录密码已变化，请重新打开会话属性后再修改。"
		return true
	}
	if !editTarget && !f.remember && !a.vault.HasStoredCredential(p.ID) {
		// Changing a routed login also invalidates its separate terminal password.
		if f.original == nil {
			return false
		}
		old, ok := f.original.BastionTarget()
		if !ok || (f.original.Kind == p.Kind && f.original.Identity() == p.Identity()) || !a.vault.HasStoredCredential(old.Credential.ID) {
			return false
		}
	}

	candidate := a.config.Clone()
	found := false
	for i, profile := range candidate.Profiles {
		if profile.ID == p.ID {
			candidate.Profiles[i] = p
			found = true
			break
		}
	}
	if !found {
		candidate.Profiles = append(candidate.Profiles, p)
	}
	candidate.Version = 3
	if err := candidate.MigrateProxyCatalog(); err != nil {
		a.formError = err.Error()
		return true
	}
	candidate.NormalizeLinkRootOrder()
	if err := candidate.Validate(); err != nil {
		a.formError = err.Error()
		return true
	}
	revision, err := a.store.Revision()
	if err != nil {
		a.formError = err.Error()
		return true
	}
	if editTarget && a.vault.CredentialVersion(target.Credential) != f.targetVersion {
		a.formError = "目标密码已在其他窗口修改，请重新打开会话属性。"
		return true
	}
	targetPassword, forgetTarget := f.targetPassword, f.forgetTarget
	if editTarget && !forgetTarget && !validTargetPassword(targetPassword) {
		a.formError = "目标服务器密码须为单行，最多 4096 字节。"
		return true
	}
	password, remember, original := f.password, f.remember, f.original
	protection := f.protection
	f.cancelRequested = false
	f.busy = true
	a.setConfigurationBusy(true)
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	go func() {
		identity := p.Identity()
		var err error
		if ((!keepGateway && remember) || (editTarget && !forgetTarget)) && (p.Kind == "ssh" || p.Kind == "sftp") {
			var resolved core.ResolvedSSH
			resolved, err = core.ResolveSSH(ctx, p)
			identity = resolved.Profile.Identity()
		}
		var changes []core.CredentialUpdate
		if err == nil && !keepGateway {
			changes, err = a.vault.PrepareCredentialEdit(original, p, password, remember, identity, protection)
		}
		password = ""
		if err == nil && editTarget {
			change := core.CredentialUpdate{Profile: target.Credential, Delete: forgetTarget}
			if !forgetTarget {
				change.Password, change.Identity = targetPassword, identity
			}
			changes = append(changes, change)
		}
		targetPassword = ""
		if err == nil {
			err = a.store.CommitCredentialEdit(ctx, candidate, a.vault, changes, revision, vaultRevision)
		}
		var conflictChanges []core.CredentialUpdate
		if localConflictError(err) {
			conflictChanges = append(conflictChanges, changes...)
		}
		for i := range changes {
			changes[i].Password = ""
		}
		cancel()
		a.post(func() {
			defer a.resumeSessionSaves(nil)
			f.busy = false
			f.cancel = nil
			if !errors.Is(err, core.ErrPendingTransaction) {
				a.setConfigurationBusy(false)
			}
			if errors.Is(err, context.Canceled) {
				a.editing = false
				f.clearInputs()
				a.formError = ""
				a.status = "已取消保存"
				return
			}
			if err != nil {
				a.formError = err.Error()
				a.status = "保存失败：" + err.Error()
				if localConflictError(err) && !f.cancelRequested {
					a.beginLocalConflict(candidate, conflictChanges)
					a.features.localConflict.afterSave = func() {
						if saved, ok := a.profileByID(p.ID); ok {
							a.finishSavedDraft(saved, connect && !f.cancelRequested)
						}
					}
				}
				for i := range conflictChanges {
					conflictChanges[i].Password = ""
				}
				return
			}
			a.applyCommittedConfig(candidate)
			a.markSharedSyncChanged()
			if a.hub != nil {
				a.hub.scheduleSharedSync(2 * time.Second)
			}
			a.finishSavedDraft(p, connect && !f.cancelRequested)
			if connect && f.cancelRequested {
				a.status = "配置已保存，已取消连接"
			}
		})
	}()
	return true
}
func (a *App) finishSavedDraft(p core.Profile, connect bool) {
	localSaved := true
	local := a.localUI.clone()
	id := strings.ToLower(p.ID)
	_, _, routed := core.SplitBastionLogin(p.Username)
	plain := p.Kind == "ssh" && routed && a.features.editorChoice == "ssh"
	typeChanged := local.PlainSSH[id] != plain
	local.setPlainSSH(id, plain)
	if validTitleMode(a.draftTitleMode) {
		if local.TitleModes == nil {
			local.TitleModes = map[string]string{}
		}
		local.TitleModes[id] = a.draftTitleMode
	}
	if validTitleMode(a.draftTitleMode) || typeChanged {
		localSaved = a.saveLocalPreferences(local)
	}
	if typeChanged && !localSaved {
		a.formError = "会话已保存，本机协议类型保存失败：" + a.features.settingsError
		return
	}
	a.features.sessionPassword.clearInputs()
	a.selected = p.ID
	a.editing = false
	if localSaved {
		a.status = "已保存 " + p.Name
	}
	if !connect {
		a.restoreManagerReturn(&p)
	} else {
		a.features.managerReturn = nil
	}
	if connect {
		if saved, ok := a.profileByID(p.ID); ok {
			p = saved
		}
		a.request(p, 0)
	}
}

func (f *sessionPasswordEditor) clearInputs() {
	f.password, f.targetPassword = "", ""
	f.revealed, f.forgetTarget = false, false
}

func (a *App) cancelSessionPassword() {
	a.features.sessionPassword.queuedSave = false
	a.features.sessionPassword.clearInputs()
	if a.features.sessionPassword.busy {
		a.features.sessionPassword.cancelRequested = true
	}
	if a.features.sessionPassword.cancel != nil {
		a.features.sessionPassword.cancel()
	}
}

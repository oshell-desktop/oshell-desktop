package desktop

import (
	"context"
	"errors"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strings"
	"time"
)

type sessionPasswordEditor struct {
	original        *core.Profile
	password        string
	protection      string
	remember, busy  bool
	revealed        bool
	cancelRequested bool
	cancel          context.CancelFunc
}

func (a *App) initSessionPassword() {
	f := &a.features.sessionPassword
	*f = sessionPasswordEditor{protection: "local"}
	if p, ok := a.profileByID(a.draft.ID); ok {
		f.original = &p
	}
	if a.vault != nil {
		f.remember = a.vault.HasStoredCredential(a.draft.ID)
		f.protection = a.vault.CredentialProtection(a.draft.ID)
	}
}
func (a *App) sessionPasswordFields(c *ui.Context) {
	f := &a.features.sessionPassword
	ui.Field(c, "密码", func() {
		revealPasswordInput(c, &f.password, &f.revealed).Label("会话密码").Placeholder("已保存（不回显）；留空沿用，输入新值替换")
	})
	ui.Checkbox(c, &f.remember, "保存密码").Disabled(a.vault == nil)
	if f.remember {
		selectValue(c, "密码保护", &f.protection, [][2]string{{"local", "本机自动加密（无需主密码）"}, {"master", "使用主密码加密"}})
	}

}
func (a *App) saveSessionCredential(p core.Profile, connect bool) bool {
	f := &a.features.sessionPassword
	if a.vault == nil {
		if f.remember {
			a.formError = "密码库不可用"
			return true
		}
		return false
	}
	if !f.remember && !a.vault.HasStoredCredential(p.ID) {
		return false
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
	vaultRevision := a.vault.Revision()
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
		if remember && (p.Kind == "ssh" || p.Kind == "sftp") {
			var resolved core.ResolvedSSH
			resolved, err = core.ResolveSSH(ctx, p)
			identity = resolved.Profile.Identity()
		}
		var changes []core.CredentialUpdate
		if err == nil {
			changes, err = a.vault.PrepareCredentialEdit(original, p, password, remember, identity, protection)
		}
		password = ""
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
			f.busy = false
			f.cancel = nil
			if !errors.Is(err, core.ErrPendingTransaction) {
				a.setConfigurationBusy(false)
			}
			if errors.Is(err, context.Canceled) {
				a.editing = false
				f.password = ""
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
	if validTitleMode(a.draftTitleMode) {
		local := a.localUI.clone()
		if local.TitleModes == nil {
			local.TitleModes = map[string]string{}
		}
		local.TitleModes[strings.ToLower(p.ID)] = a.draftTitleMode
		localSaved = a.saveLocalPreferences(local)
	}
	a.features.sessionPassword.password = ""
	a.features.sessionPassword.revealed = false
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

func (a *App) cancelSessionPassword() {
	a.features.sessionPassword.password = ""
	a.features.sessionPassword.revealed = false
	if a.features.sessionPassword.busy {
		a.features.sessionPassword.cancelRequested = true
	}
	if a.features.sessionPassword.cancel != nil {
		a.features.sessionPassword.cancel()
	}
}

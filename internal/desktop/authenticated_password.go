package desktop

import (
	"context"
	"errors"
	"oshell-desktop/internal/core"
	"time"
)

func canSaveSessionPassword(profile core.Profile, cred core.Credentials, vault *core.Vault) bool {
	if vault == nil || cred.ExternalLaunch {
		return false
	}
	if _, locked := vault.State(); locked {
		return false
	}
	return profile.Kind == "ssh" || profile.Kind == "sftp" || profile.Kind == "ftp"
}

// Existing-session callers must never recreate a deleted configuration.
func (a *App) saveAuthenticatedPassword(profile core.Profile, password string, identity core.SSHIdentity, vault *core.Vault, revision string, protection ...string) {
	a.saveAuthenticatedSessionPassword(profile, password, identity, vault, revision, true, protection...)
}

// Called on the UI thread only after the complete protocol login succeeds.
func (a *App) saveAuthenticatedSessionPassword(profile core.Profile, password string, identity core.SSHIdentity, vault *core.Vault, revision string, requireExisting bool, protection ...string) *core.Profile {
	if password == "" || vault == nil {
		return nil
	}
	if _, locked := vault.State(); locked {
		a.status = "主密码缓存已清除，本次登录密码未保存"
		return nil
	}
	if a.vault != vault {
		a.status = "密码库已变化，本次密码未保存"
		return nil
	}
	if a.configurationBusy() {
		a.status = "配置正在保存，本次登录密码未保存"
		return nil
	}
	target := profile
	index := -1
	for i, stored := range a.config.Profiles {
		if stored.ID != profile.ID {
			continue
		}
		if stored.Kind != profile.Kind || stored.Host != profile.Host || stored.Port != profile.Port || stored.Username != profile.Username || stored.IdentityFile != profile.IdentityFile || stored.ProxyID != profile.ProxyID || stored.JumpHost != profile.JumpHost || stored.Proxy != profile.Proxy {
			a.status = "会话已变化，本次密码未保存"
			return nil
		}
		index = i
		target = stored
		break
	}
	if index < 0 && requireExisting {
		a.status = "会话已变化，本次密码未保存"
		return nil
	}
	if target.Username == "" {
		target.Username = identity.User
	}
	var err error
	if index >= 0 && target.Username == profile.Username {
		err = vault.PutForRevision(target, password, identity, revision, protection...)
	} else {
		if a.store == nil {
			a.status = "配置存储不可用，本次密码未保存"
			return nil
		}
		candidate := a.config.Clone()
		if index < 0 {
			candidate.Profiles = append(candidate.Profiles, target)
		} else {
			candidate.Profiles[index] = target
		}
		candidate.NormalizeLinkRootOrder()
		var configRevision string
		configRevision, err = a.store.Revision()
		if err == nil {
			mode := ""
			if len(protection) > 0 {
				mode = protection[0]
			}
			err = a.store.CommitCredentialEdit(context.Background(), candidate, vault, []core.CredentialUpdate{{Profile: target, Password: password, Identity: identity, Protection: mode}}, configRevision, revision)
		}
		if err == nil {
			a.applyCommittedConfig(candidate)
		}
	}
	if err != nil {
		if errors.Is(err, core.ErrPendingTransaction) {
			a.setConfigurationBusy(true)
		}
		a.status = "保存密码失败：" + err.Error()
		return nil
	}
	a.status = "登录密码已保存"
	if a.store != nil {
		a.markSharedSyncChanged()
		if a.hub != nil {
			a.hub.scheduleSharedSync(2 * time.Second)
		}
	}
	return &target
}

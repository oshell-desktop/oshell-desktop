package desktop

import (
	"context"
	"errors"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"time"
)

type localConflictState struct {
	hasSavedDraft               bool
	resume                      bool
	draftID                     string
	active, blocked             bool
	open, busy, cancelRequested bool
	draft                       core.Config
	changes                     []core.CredentialUpdate
	plan                        *core.LocalResolutionPlan
	choices                     map[string]*string
	index                       int
	list                        ui.ListState
	error                       string
	cancel                      context.CancelFunc
	afterSave                   func()
}

func localConflictError(err error) bool {
	return errors.Is(err, core.ErrConfigurationChanged) || errors.Is(err, core.ErrVaultChanged)
}
func (a *App) beginLocalConflict(draft core.Config, changes []core.CredentialUpdate) {
	if a.passwordLocked() || a.configurationBusy() {
		return
	}
	f := &a.features.localConflict
	if a.manager != nil {
		a.features.managerReturn = a.captureManagerReturn()
		a.closeManager()
	}
	if f.plan != nil {
		f.plan.Close()
	}
	*f = localConflictState{active: true, open: true, hasSavedDraft: a.store.LocalConflictDraftExists(), draft: draft.Clone(), changes: append([]core.CredentialUpdate(nil), changes...)}
	a.setConfigurationBusy(true)
	a.prepareLocalConflict()
}
func (a *App) prepareLocalConflict() {
	f := &a.features.localConflict
	if f.busy || !f.open {
		return
	}
	if f.plan != nil {
		f.plan.Close()
		f.plan = nil
	}
	f.busy = true
	f.error = ""
	f.cancelRequested = false
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	draft := f.draft.Clone()
	changes := append([]core.CredentialUpdate(nil), f.changes...)
	vault := a.vault
	resume := f.resume
	go func() {
		defer cancel()
		defer func() {
			for i := range changes {
				changes[i].Password = ""
			}
		}()
		var plan *core.LocalResolutionPlan
		var id string
		var err error
		if resume {
			plan, id, err = a.store.ResumeLocalResolution(ctx, vault)
		} else {
			plan, err = a.store.PrepareLocalResolution(ctx, draft, vault, changes)
			if err == nil {
				id, err = a.store.SaveLocalConflictDraft(ctx, plan)
				if err != nil {
					plan.Close()
					plan = nil
				}
			}
		}
		if !a.post(func() {
			f.busy = false
			f.cancel = nil
			if f.cancelRequested {
				if plan != nil {
					plan.Close()
				}
				a.cancelLocalConflict()
				return
			}
			if err != nil {
				f.error = err.Error()
				return
			}
			f.plan = plan
			f.draftID = id
			f.hasSavedDraft = true
			f.resume = true
			for i := range f.changes {
				f.changes[i].Password = ""
			}
			f.changes = nil
			f.index = 0
			f.list = ui.ListState{}
			f.choices = map[string]*string{}
			for _, conflict := range plan.Merge.Conflicts {
				value := ""
				f.choices[conflict.Key] = &value
			}
		}) && plan != nil {
			plan.Close()
		}
	}()
}
func (a *App) cancelLocalConflict() {
	f := &a.features.localConflict
	if !f.active {
		return
	}
	if f.busy {
		f.cancelRequested = true
		if f.cancel != nil {
			f.cancel()
		}
		return
	}
	if f.plan != nil {
		f.plan.Close()
		f.plan = nil
	}
	for i := range f.changes {
		f.changes[i].Password = ""
	}
	f.changes = nil
	f.draft = core.Config{}
	f.choices = nil
	f.open = false
	f.active = false
	f.afterSave = nil
	if !f.blocked {
		a.setConfigurationBusy(false)
	}
	if a.store.LocalConflictDraftExists() {
		a.status = "磁盘仍有本地配置冲突草稿，可从会话菜单继续处理"
	}
	if !a.editing && !a.features.linkEditor.open {
		a.restoreManagerReturn(nil)
	}
}
func (a *App) resolveLocalConflict(reload bool) {
	f := &a.features.localConflict
	if f.busy || f.plan == nil {
		return
	}
	choices := map[string]core.SharedMergeChoice{}
	for key, value := range f.choices {
		choices[key] = core.SharedMergeChoice(*value)
	}
	if !reload {
		for _, conflict := range f.plan.Merge.Conflicts {
			if choices[conflict.Key] != core.SharedKeepLocal && choices[conflict.Key] != core.SharedKeepRemote {
				f.error = "请为每一项冲突选择保留版本"
				return
			}
		}
	}
	f.busy = true
	f.error = ""
	f.cancelRequested = false
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	plan := f.plan
	draftID := f.draftID
	go func() {
		defer cancel()
		var cfg core.Config
		var vault *core.Vault
		var err error
		if reload {
			cfg, vault, err = plan.ReloadDisk(a.store)
		} else {
			cfg, vault, err = plan.Commit(ctx, a.store, choices)
		}
		var cleanupErr error
		if err == nil && draftID != "" {
			cleanupErr = a.store.ClearLocalConflictDraft(draftID)
		}
		if !a.post(func() {
			f.busy = false
			f.cancel = nil
			if err != nil {
				if errors.Is(err, core.ErrPendingTransaction) {
					f.blocked = true
					f.error = err.Error()
					return
				}
				if f.cancelRequested {
					a.cancelLocalConflict()
					return
				}
				f.error = err.Error() + "；可重新比较磁盘版本。"
				return
			}
			after := f.afterSave
			follow := !reload && !f.cancelRequested
			old := a.vault
			a.vault = vault
			if a.hub != nil {
				a.hub.vault = vault
				for _, window := range a.hub.windows {
					window.vault = vault
				}
				if a.hub.fileWindow != nil {
					a.hub.fileWindow.vault = vault
				}
			}
			if old != nil && old != vault {
				old.Lock()
			}
			a.applyCommittedConfig(cfg)
			f.plan = nil
			plan.Close()
			f.open = false
			f.active = false
			f.afterSave = nil
			for i := range f.changes {
				f.changes[i].Password = ""
			}
			f.changes = nil
			f.draft = core.Config{}
			f.choices = nil
			a.setConfigurationBusy(false)
			a.editing = false
			a.features.sessionPassword.password = ""
			a.features.commandsOpen = false
			a.features.commandEdit = nil
			a.features.linkEditor.open = false
			a.features.directoryOpen = false
			a.features.defaultsOpen = false
			a.features.highlightOpen = false
			a.features.proxyManager.editing = false
			a.features.proxyManager.error = ""
			a.features.archive.open = false
			if a.settingsOpen {
				a.features.settingsDraft = cfg.Settings
				a.closeSettings()
			}
			a.status = "本地配置冲突已处理"
			a.markSharedSyncChanged()
			if a.hub != nil {
				a.hub.scheduleSharedSync(2 * time.Second)
			}
			if follow && after != nil {
				after()
			} else {
				a.restoreManagerReturn(nil)
			}
			if cleanupErr != nil {
				a.status = "配置已处理；草稿清理失败，文件已保留：" + cleanupErr.Error()
			}
			a.invalidateSecurityWindows()
		}) {
			if vault != nil {
				vault.Lock()
			}
			plan.Close()
		}
	}()
}
func (a *App) localConflictDialog(c *ui.Context) {
	f := &a.features.localConflict
	desktopModal(c, &f.open, func() {
		dialogBody(c, 760).Gap(10).Children(func() {
			windowHeading(c, "本地配置发生变化")
			ui.Text(c, "其他进程修改了磁盘配置。会话和对应凭据作为整体选择；独立改动会自动合并，当前连接保持运行。").Wrap()
			if f.error != "" {
				ui.Text(c, f.error).Wrap().TextColor(c.Theme().Danger)
			}
			if f.busy {
				ui.Text(c, "正在读取或保存配置…")
			} else if f.plan != nil {
				conflicts := f.plan.Merge.Conflicts
				ui.Text(c, fmt.Sprintf("%d 项需要选择；密码内容不显示。", len(conflicts)))
				f.list.Selected = &f.index
				ui.List(c, &f.list, len(conflicts), func(i int) { ui.Text(c, conflicts[i].Title).SingleLine() }).Height(120)
				if f.index >= 0 && f.index < len(conflicts) {
					item := conflicts[f.index]
					ui.Scroll(c).Height(160).Children(func() {
						ui.Text(c, "本机草稿\n"+item.LocalDescription+"\n\n磁盘版本\n"+item.RemoteDescription).Wrap()
					})
					selectValue(c, "保留版本", f.choices[item.Key], [][2]string{{"", "请选择"}, {"local", "本机草稿"}, {"remote", "磁盘版本"}})
				}
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "取消处理").OnClick(a.cancelLocalConflict)
				if f.hasSavedDraft {
					ui.Button(c, "放弃已保存草稿…").Disabled(f.busy || f.blocked).OnClick(a.discardLocalConflictDraft)
				}
				ui.Button(c, "重新比较").Disabled(f.busy || f.blocked).OnClick(a.prepareLocalConflict)
				ui.Button(c, "放弃草稿并读取磁盘").Disabled(f.busy || f.blocked || f.plan == nil).OnClick(func() { a.resolveLocalConflict(true) })
				ui.PrimaryButton(c, "按选择合并并保存").Disabled(f.busy || f.blocked || f.plan == nil).OnClick(func() { a.resolveLocalConflict(false) })
			})
		})
	})
	if !f.open && f.active {
		a.cancelLocalConflict()
	}
}

// Capture before a worker clears its temporary credential updates; run on the UI thread.
func localConflictRetry(err error, draft core.Config, changes []core.CredentialUpdate) func(*App) {
	if !localConflictError(err) {
		return nil
	}
	candidate := draft.Clone()
	retained := append([]core.CredentialUpdate(nil), changes...)
	return func(a *App) {
		a.beginLocalConflict(candidate, retained)
		for i := range retained {
			retained[i].Password = ""
		}
		retained = nil
	}
}

func (a *App) openLocalConflict() {
	if a.passwordLocked() {
		a.showVault()
		return
	}
	if a.configurationBusy() {
		return
	}
	if !a.store.LocalConflictDraftExists() {
		a.beginLocalConflict(a.config, nil)
		return
	}
	f := &a.features.localConflict
	if f.plan != nil {
		f.plan.Close()
	}
	*f = localConflictState{active: true, open: true, resume: true, hasSavedDraft: true}
	if a.manager != nil {
		a.features.managerReturn = a.captureManagerReturn()
		a.closeManager()
	}
	a.setConfigurationBusy(true)
	a.prepareLocalConflict()
}
func (a *App) maybeResumeLocalConflict() {
	if a.passwordLocked() || a.configurationBusy() || !a.store.LocalConflictDraftExists() {
		return
	}
	if a.modalActive() || a.settingsOpen {
		a.status = "有待处理的本地配置冲突草稿，可从会话菜单继续处理"
		return
	}
	a.openLocalConflict()
}
func (a *App) discardLocalConflictDraft() {
	f := &a.features.localConflict
	if f.busy || f.blocked {
		return
	}
	parent := a.win
	if a.settingsOpen && a.preferencesWindow != nil {
		parent = a.preferencesWindow
	}
	go func() {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: parent, Type: mygo.MessageWarning, Title: "放弃本地冲突草稿", Message: "删除已保存的本地冲突草稿？", Detail: "不会修改当前磁盘配置或活动连接，草稿中的未保存改动将被放弃。", Buttons: []string{"取消", "放弃草稿"}, DefaultButton: 0, CancelButton: 0})
		if err == nil && result.Button == 1 {
			a.post(func() {
				if f.busy {
					return
				}
				if err := a.store.ClearLocalConflictDraft(""); err != nil {
					f.error = err.Error()
					return
				}
				a.cancelLocalConflict()
				a.status = "本地冲突草稿已放弃，磁盘配置未修改"
			})
		}
	}()
}

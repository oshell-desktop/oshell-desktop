package desktop

import (
	"context"
	"errors"
	"fmt"
	"github.com/egoist/mygo"
	"oshell-desktop/internal/core"
)

func (a *App) refreshLocalSharedData() { a.reloadLocalSharedData(true) }

func (a *App) reloadLocalSharedData(interactive bool) {
	if a.configurationBusy() {
		if interactive {
			a.status = "配置操作正在进行，请稍后刷新"
		}
		return
	}
	if a.passwordLocked() {
		if interactive {
			a.showVault()
			a.features.reloadAfterUnlock = true
		}
		return
	}
	windows := []*App{a}
	if a.hub != nil {
		windows = append([]*App(nil), a.hub.windows...)
		if a.hub.fileWindow != nil {
			windows = append(windows, a.hub.fileWindow)
		}
	}
	for _, window := range windows {
		if window.manager != nil || window.managerRename != nil || window.settingsOpen || window.modalActive() || tabMoveDialogOpen(window.win) {
			if interactive {
				a.status = "请先完成或取消正在编辑的内容，再刷新数据"
			}
			return
		}
	}
	options, err := a.loadSyncOptions()
	if err != nil {
		a.status = err.Error()
		return
	}
	if options.Enabled {
		if interactive {
			a.startSavedSharedSync()
		} else {
			a.features.sharedSync.background = true
			a.startSharedSync()
		}
		return
	}
	if a.store.LocalConflictDraftExists() {
		if interactive {
			a.openLocalConflict()
		}
		return
	}
	revision, err := a.store.Revision()
	if err != nil {
		a.status = err.Error()
		return
	}
	if revision == "" {
		if interactive {
			a.status = "尚无已保存的本地配置"
		}
		return
	}
	changed, err := a.store.LocalStateChanged(a.vault)
	if err != nil {
		a.status = err.Error()
		return
	}
	if !changed {
		if interactive {
			a.status = "本地配置与凭据没有变化"
		}
		return
	}
	a.setConfigurationBusy(true)
	cfg := a.config.Clone()
	vault := a.vault
	updating := a.updatesBlockRepositoryReload()
	go func() {
		next, replacement, err := a.store.ReloadLocalConfiguration(context.Background(), cfg, vault, func(remote core.Config) error {
			if updating && remote.Settings.UpdateRepository != cfg.Settings.UpdateRepository {
				return fmt.Errorf("更新正在进行，暂不能载入不同的更新仓库设置")
			}
			return nil
		})
		done := func() {
			a.setConfigurationBusy(false)
			if err != nil {
				a.status = err.Error()
				if interactive && errors.Is(err, core.ErrLocalDraftChanged) {
					a.beginLocalConflict(cfg, nil)
				}
				return
			}
			old := a.vault
			a.vault = replacement
			if a.hub != nil {
				a.hub.vault = replacement
				for _, window := range a.hub.windows {
					window.vault = replacement
				}
				if a.hub.fileWindow != nil {
					a.hub.fileWindow.vault = replacement
				}
			}
			if old != nil && old != replacement {
				old.Lock()
			}
			a.applyCommittedConfig(next)
			a.status = "已刷新本地配置与凭据，现有连接保持不变"
		}
		if mygo.App.IsReady() {
			mygo.RunOnMain(done)
		} else if !a.post(done) && replacement != nil {
			replacement.Lock()
		}
	}()
}

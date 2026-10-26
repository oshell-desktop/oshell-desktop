package desktop

import (
	"fmt"
	"oshell-desktop/internal/core"
)

func (a *App) syncOptionsChanged() bool {
	f := &a.features.sharedSync
	return f.loaded && (!equivalentSyncOptions(f.options, f.savedOptions) || f.password != "")
}

func (a *App) validateSyncDraft() error {
	if a.features.sharedSync.loaded {
		if err := a.features.sharedSync.options.Policy.Validate(); err != nil {
			return err
		}
	}
	if err := a.validateDirectoryDraft(); err != nil {
		return err
	}
	f := &a.features.sharedSync
	if !a.syncOptionsChanged() && !f.directoryChanged {
		return nil
	}
	saved, err := a.loadSyncOptions()
	if err != nil {
		return err
	}
	if f.loaded && saved != f.savedOptions {
		return fmt.Errorf("同步设置已被其他窗口修改，请取消并重新打开设置")
	}
	options := f.options
	if !f.loaded {
		options = saved
	}
	if f.directoryDisabled && options.Backend == "directory" {
		options.Enabled = false
	}
	if options.Enabled {
		protected, locked := a.vault.State()
		if !protected || locked {
			return fmt.Errorf("请先启用并解锁主密码保护")
		}
		if _, err := options.Transport(f.password, a.store.Dir); err != nil {
			return err
		}
	}
	if f.pendingDirectory != nil && !f.directoryChanged {
		return fmt.Errorf("存在旧版待生效目录，请先重启完成迁移或取消待生效设置")
	}
	if options.Enabled && options.Backend == "directory" && (saved.Backend != "directory" || saved.Address != options.Address || !saved.Enabled) {
		master, err := a.vault.MasterSecret()
		if err != nil {
			return err
		}
		defer clear(master)
		pending, err := core.ValidateSyncDirectoryTarget(a.store.Dir, options.Address, master)
		if err != nil {
			return err
		}
		f.options.Address = pending.Path
		f.options.RequireExisting = pending.Mode == "existing"
	}
	return nil
}

func (a *App) applySyncDraft(done func()) {
	f := &a.features.sharedSync
	if err := a.validateSyncDraft(); err != nil {
		a.features.settingsError = err.Error()
		return
	}
	changed := a.syncOptionsChanged()
	if f.directoryDisabled && f.options.Backend == "directory" {
		f.options.Enabled = false
		changed = f.loaded && !equivalentSyncOptions(f.options, f.savedOptions)
	}
	if err := a.applyDirectoryDraft(); err != nil {
		a.features.settingsError = err.Error()
		return
	}
	if changed {
		a.saveSharedSyncSettingsThen(done)
		return
	}
	done()
}

func equivalentSyncOptions(a, b core.SyncOptions) bool {
	a.Policy = a.Policy.Effective()
	b.Policy = b.Policy.Effective()
	return a == b
}

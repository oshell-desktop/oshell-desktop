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

type nativeMigrationState struct {
	completed, syncScheduled           bool
	open, busy                         bool
	directory, password, error, backup string
	preview                            *core.NativeStorageSnapshot
	master                             []byte
	previous                           core.Config
	configRevision, vaultRevision      string
	cancel                             context.CancelFunc
}

func (a *App) showNativeMigration() {
	if a.passwordLocked() {
		a.showVault()
		return
	}
	if a.configurationBusy() {
		return
	}
	f := &a.features.nativeMigration
	clear(f.master)
	*f = nativeMigrationState{open: true}
}
func (a *App) runNativeMigration(apply bool) {
	f := &a.features.nativeMigration
	if f.busy || f.completed || a.configurationBusy() || a.vault == nil {
		return
	}
	protected, locked := a.vault.State()
	if !protected || locked {
		f.error = "请先通过密码保护设置并解锁本地主密码"
		return
	}
	if apply && f.preview == nil {
		return
	}
	revision, err := a.store.Revision()
	if err != nil {
		f.error = err.Error()
		return
	}
	directory := f.directory
	master := []byte(f.password)
	f.password = ""
	if apply {
		clear(master)
		master = append([]byte(nil), f.master...)
	}
	previous := a.config.Clone()
	vault := a.vault
	vaultRevision := vault.Revision()
	preview := f.preview
	if apply {
		previous = f.previous.Clone()
		revision = f.configRevision
		vaultRevision = f.vaultRevision
	}
	f.busy = true
	f.error = ""
	a.setConfigurationBusy(true)
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	go func() {
		defer cancel()
		defer clear(master)
		var next core.Config
		var backup string
		var result core.NativeStorageSnapshot
		var err error
		if apply {
			next, backup, err = a.store.AdoptNativeStorage(ctx, *preview, master, previous, vault, revision, vaultRevision)
		} else {
			result, err = core.ReadNativeStorage(directory, master)
			if err == nil {
				err = ctx.Err()
			}
		}
		retained := []byte(nil)
		if err == nil && !apply {
			retained = append(retained, master...)
		}
		if !a.post(func() {
			f.busy = false
			f.cancel = nil
			if !errors.Is(err, core.ErrPendingTransaction) {
				a.setConfigurationBusy(false)
			}
			if err != nil {
				f.error = err.Error()
				f.backup = backup
				return
			}
			if apply {
				a.applyCommittedConfig(next)
				if f.preview.WebDAV == nil {
					clear(f.master)
					f.master = nil
				}
				f.completed = true
				f.backup = backup
				f.open = f.preview.SyncDirectory != "" || f.preview.WebDAV != nil
				a.status = "原版配置已迁移；迁移前备份：" + backup
			} else {
				clear(f.master)
				f.master = retained
				f.preview = &result
				f.previous = previous
				f.configRevision = revision
				f.vaultRevision = vaultRevision
			}
		}) {
			clear(retained)
		}
	}()
}
func (a *App) nativeMigrationDialog(c *ui.Context) {
	f := &a.features.nativeMigration
	desktopModal(c, &f.open, func() {
		dialogBody(c, 600).Gap(10).Children(func() {
			windowHeading(c, "迁移原版配置")
			ui.Text(c, "读取 OShell 原版数据目录。确认后替换本地会话、代理、快捷命令和偏好；现有配置先备份，原目录不变。").Wrap()
			if f.completed {
				ui.Text(c, "配置迁移已完成。").Bold()
				if f.preview.SyncDirectory != "" {
					ui.Text(c, "原同步目录："+f.preview.SyncDirectory).Wrap()
					ui.Button(c, "恢复原目录同步（重启生效）").Disabled(f.busy || f.syncScheduled).OnClick(a.scheduleMigratedDirectory)
				}
				if f.preview.WebDAV != nil {
					ui.Text(c, "原 WebDAV："+f.preview.WebDAV.Address).Wrap()
					ui.Button(c, "恢复原 WebDAV 同步").Disabled(f.busy || f.syncScheduled).OnClick(a.restoreMigratedWebDAV)
				}
				ui.Text(c, "恢复同步前需使用与原版相同的主密码。也可关闭此窗口，继续仅使用本地配置。").Wrap()
				if f.syncScheduled {
					ui.Text(c, "同步目标已保存；目录同步重启生效，WebDAV 按正常同步流程处理差异。").Wrap()
				}
			} else if f.preview == nil {
				ui.TextInput(c, &f.directory).Label("原版数据目录").Disabled(f.busy)
				ui.Button(c, "选择原版目录…").Disabled(f.busy).OnClick(func() {
					paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "选择原版 OShell 数据目录", Directory: true})
					if err != nil {
						f.error = err.Error()
					} else if len(paths) > 0 {
						f.directory = paths[0]
					}
				})
				passwordInput(c, &f.password).Label("原版主密码（未设置可留空）").Disabled(f.busy)
				ui.Button(c, "读取并预览").Disabled(f.busy).OnClick(func() { a.runNativeMigration(false) })
			} else {
				if f.preview.WebDAV != nil {
					ui.Text(c, "原 WebDAV："+f.preview.WebDAV.Address).Wrap()
				}
				d := f.preview.Document
				ui.Text(c, "读取来源："+f.preview.Directory).Wrap()
				if f.preview.SyncDirectory != "" {
					ui.Text(c, "原同步目录："+f.preview.SyncDirectory).Wrap()
				}
				ui.Text(c, fmt.Sprintf("%d 个会话 · %d 个代理 · %d 个引用 · %d 条快速命令", len(d.Profiles), len(d.Proxies), len(d.Links.Entries), len(d.QuickCommands)))
				ui.PrimaryButton(c, "备份当前配置并迁移").Disabled(f.busy).OnClick(func() { a.runNativeMigration(true) })
				ui.Button(c, "重新选择来源").Disabled(f.busy).OnClick(func() { f.preview = nil; clear(f.master); f.master = nil })
			}
			if f.error != "" {
				ui.Text(c, f.error).Wrap().TextColor(c.Theme().Danger)
			}
			if f.backup != "" {
				ui.Text(c, "已保留备份："+f.backup).Wrap()
			}
			closeLabel := "取消"
			if f.completed {
				closeLabel = "关闭"
			}
			ui.Button(c, closeLabel).OnClick(func() {
				if f.cancel != nil {
					f.cancel()
				} else {
					f.open = false
				}
			})
		})
	})
	if !f.open {
		if f.cancel != nil {
			f.cancel()
		}
		clear(f.master)
		f.master = nil
		f.password = ""
	}
}

func (a *App) scheduleMigratedDirectory() {
	f := &a.features.nativeMigration
	if !f.completed || f.preview == nil || f.preview.SyncDirectory == "" || f.busy || f.syncScheduled || a.configurationBusy() || a.vault == nil {
		return
	}
	master, err := a.vault.MasterSecret()
	if err != nil {
		f.error = err.Error()
		return
	}
	target := f.preview.SyncDirectory
	f.busy = true
	f.error = ""
	a.setConfigurationBusy(true)
	go func() {
		defer clear(master)
		err := core.ScheduleSyncDirectory(a.store.Dir, target, "existing", master)
		a.post(func() {
			f.busy = false
			a.setConfigurationBusy(false)
			if err != nil {
				f.error = "配置已经迁移，但同步目录尚未启用：" + err.Error()
				return
			}
			f.syncScheduled = true
			a.features.sharedSync.loaded = false
			a.status = "原同步目录已保存，重启后启用"
		})
	}()
}

func (a *App) restoreMigratedWebDAV() {
	f := &a.features.nativeMigration
	if !f.completed || f.preview == nil || f.preview.WebDAV == nil || f.busy || f.syncScheduled || a.configurationBusy() || a.vault == nil {
		return
	}
	master := append([]byte(nil), f.master...)
	snapshot := *f.preview
	f.busy = true
	f.error = ""
	a.setConfigurationBusy(true)
	go func() {
		defer clear(master)
		err := a.store.RestoreNativeWebDAV(snapshot, master, a.vault)
		a.post(func() {
			f.busy = false
			a.setConfigurationBusy(false)
			if err != nil {
				f.error = "配置已迁移，但 WebDAV 尚未恢复：" + err.Error()
				return
			}
			f.syncScheduled = true
			clear(f.master)
			f.master = nil
			a.features.sharedSync.loaded = false
			a.status = "原 WebDAV 同步设置已恢复"
			if a.hub != nil {
				a.hub.scheduleSharedSync(2 * time.Second)
			}
		})
	}()
}

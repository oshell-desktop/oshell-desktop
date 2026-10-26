// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"context"
	"errors"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"reflect"
)

type archiveState struct {
	exportConfig                         *core.Config
	vendor                               *core.ThirdPartyReport
	fillMissing                          bool
	open, busy, export, includePasswords bool
	archive                              core.PortableArchive
	password, again, directory, error    string
	cancel                               context.CancelFunc
}

func (a *App) configurationBusy() bool { return a.configBusy || a.hub != nil && a.hub.configBusy }
func (a *App) setConfigurationBusy(busy bool) {
	a.configBusy = busy
	if a.hub != nil {
		a.hub.configBusy = busy
	}
}
func (a *App) applyCommittedConfig(cfg core.Config) {
	a.config = cfg.Clone()
	a.reconcileSessionPins()
	if a.hub != nil {
		a.hub.config = cfg.Clone()
		for _, w := range a.hub.configurationWindows() {
			w.config = cfg.Clone()
			w.applyAppearance()
			if w.win != nil {
				w.win.Invalidate()
			}
			if w.manager != nil {
				w.manager.Invalidate()
			}
		}
		a.refreshMenu()
	} else {
		a.applyAppearance()
	}
}
func (a *App) importProfiles() {
	if a.configurationBusy() || a.features.archive.busy {
		a.status = "错误：配置正在写入，请稍后重试"
		return
	}
	destination := a.sessionActionGroup()
	go func() {
		paths, e := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "导入 OShell 会话", Filters: []mygo.FileFilter{{Name: "OShell 会话", Extensions: []string{"json"}}}})
		if e != nil {
			a.report(e)
			return
		}
		if len(paths) == 0 {
			return
		}
		data, e := core.ReadBounded(paths[0])
		if e != nil {
			a.report(e)
			return
		}
		archive, e := core.DecodePortableArchive(data)
		if e != nil {
			a.report(e)
			return
		}
		a.post(func() { a.features.archive = archiveState{open: true, archive: archive, directory: destination} })
	}()
}
func (a *App) exportProfiles() {
	if a.features.archive.busy {
		return
	}
	a.closeManager()
	a.features.archive = archiveState{open: true, export: true}
}
func (a *App) runArchive() {
	f := &a.features.archive
	if f.busy {
		return
	}
	if f.includePasswords && f.export && f.password != f.again {
		f.error = "两次导出密码不一致"
		return
	}
	if f.includePasswords && a.vault == nil {
		f.error = "密码库不可用"
		return
	}
	if a.vault != nil {
		_, locked := a.vault.State()
		if locked && f.includePasswords {
			f.error = "请先在密码保护中解锁本机密码库"
			return
		}
	}
	snapshot := a.config.Clone()
	if f.export && f.exportConfig != nil {
		snapshot = f.exportConfig.Clone()
	}
	revision, e := a.store.Revision()
	if e != nil {
		f.error = e.Error()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	f.busy = true
	f.error = ""
	vendor, fillMissing := f.vendor, f.fillMissing && f.includePasswords
	master := []byte(f.password)
	f.password = ""
	f.again = ""
	include, export, directory, archive := f.includePasswords, f.export, f.directory, f.archive
	if !export {
		a.setConfigurationBusy(true)
	}
	go func() {
		defer clear(master)
		defer cancel()
		importedSessions, importedPasswords := len(archive.Profiles), 0
		var result core.Config
		var retry func(*App)
		var e error
		if export {
			var portable core.PortableArchive
			portable, e = a.vault.PortableArchive(ctx, snapshot, include, master)
			if e == nil {
				var data []byte
				data, e = portable.Encode()
				if e == nil {
					var path string
					path, e = mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.win, Title: "导出 OShell 会话", DefaultPath: "OShell.sessions.json"})
					if e == nil && path == "" {
						e = context.Canceled
					}
					if e == nil {
						e = ctx.Err()
					}
					if e == nil {
						e = core.AtomicWrite(path, data)
					}
				}
			}
		} else {
			var credentials []core.CredentialUpdate
			if vendor != nil {
				var stats core.VendorImportStats
				result, credentials, stats, e = vendor.Merge(ctx, snapshot, a.vault, directory, include, fillMissing, string(master))
				importedSessions = stats.Sessions
			} else {
				result, credentials, e = archive.Merge(ctx, snapshot, directory, include, master)
			}
			importedPasswords = len(credentials)
			if e == nil {
				e = a.store.CommitCredentials(ctx, result, a.vault, credentials, revision)
			}
			retry = localConflictRetry(e, result, credentials)
			for i := range credentials {
				credentials[i].Password = ""
			}
		}
		a.post(func() {
			f.busy = false
			f.cancel = nil
			if !export && !errors.Is(e, core.ErrPendingTransaction) {
				a.setConfigurationBusy(false)
			}
			if e != nil {
				if errors.Is(e, context.Canceled) {
					f.error = "已取消"
				} else {
					f.error = e.Error()
				}
				if retry != nil {
					retry(a)
				}
				return
			}
			if !export {
				if !reflect.DeepEqual(a.config, snapshot) {
					f.error = "配置在导入过程中变化；磁盘已保存，请重新打开应用刷新"
					return
				}
				a.applyCommittedConfig(result)
				a.status = fmt.Sprintf("已导入 %d 个会话及 %d 个密码", importedSessions, importedPasswords)
			} else {
				a.status = "会话已导出"
			}
			f.open = false
		})
	}()
}
func (a *App) archiveDialog(c *ui.Context) {
	f := &a.features.archive
	ui.Modal(c, &f.open, func() {
		ui.Column(c).Width(520).Gap(12).Children(func() {
			title := "导入会话"
			if f.export {
				title = "导出会话"
			}
			ui.Text(c, title).FontSize(18).Bold()
			if f.export {
				if selected := f.exportConfig; selected != nil {
					ui.Text(c, fmt.Sprintf("导出所选范围：%d 个会话 · %d 个引用 · %d 个目录", len(selected.Profiles), len(selected.Links), len(selected.Directories)))
				} else {
					ui.Text(c, "导出全部会话")
				}
			}
			if !f.export {
				ui.Text(c, fmt.Sprintf("%d 个会话 · %d 个快捷引用 · %d 个目录 · %d 项加密密码", len(f.archive.Profiles), len(f.archive.Links), len(f.archive.Directories), f.passwordCount()))
				ui.TextInput(c, &f.directory).Label("导入目标目录").Disabled(f.busy)
			}
			ui.Checkbox(c, &f.includePasswords, "包含保存的密码（登录与代理）").Disabled(f.busy || !f.export && f.passwordCount() == 0)
			if f.vendor != nil && f.vendor.Format == "Xshell" && f.includePasswords {
				ui.Checkbox(c, &f.fillMissing, "仅补全已存在会话中缺失的密码").Disabled(f.busy)
			}
			if f.includePasswords {
				label := "文件的导出密码"
				if f.export {
					label = "设置导出密码（至少 8 个字符）"
				}
				ui.Field(c, label, func() { passwordInput(c, &f.password).Disabled(f.busy) })
				if f.export {
					ui.Field(c, "再次输入", func() { passwordInput(c, &f.again).Disabled(f.busy) })
				}
			}
			if f.vendor != nil {
				ui.Text(c, "来源："+f.vendor.Format+"。不迁移私钥内容、指纹、登录脚本或宏；代理与隧道需重新配置。").TextColor(c.Theme().TextMuted)
				if len(f.vendor.Issues) > 0 {
					ui.Scroll(c).Height(120).Children(func() {
						for _, issue := range f.vendor.Issues {
							prefix := "提示"
							if issue.Skipped {
								prefix = "跳过"
							}
							ui.Text(c, prefix+" · "+issue.Source+"："+issue.Message).FontSize(11)
						}
					})
				}
			}
			ui.Text(c, "会话导入为独立副本；私钥内容和服务器指纹不导入。密码会重新绑定到新会话。").TextColor(c.Theme().TextMuted)
			if f.error != "" {
				ui.Text(c, f.error).TextColor(c.Theme().Danger)
			}
			if f.busy {
				ui.Text(c, "正在处理…")
				ui.Button(c, "取消操作").OnClick(func() {
					if f.cancel != nil {
						f.cancel()
					}
				})
			} else {
				ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
					ui.Button(c, "取消").OnClick(func() { f.open = false; f.password = ""; f.again = "" })
					ui.PrimaryButton(c, title).OnClick(a.runArchive)
				})
			}
		})
	})
	if !f.open {
		f.password = ""
		f.again = ""
		if f.busy && f.cancel != nil {
			f.cancel()
		}
	}
}

func (f *archiveState) passwordCount() int {
	if f.vendor != nil {
		return len(f.vendor.Passwords)
	}
	return f.archive.PasswordCount()
}
func (a *App) importThirdParty(format string, directory bool) {
	if a.configurationBusy() || a.features.archive.busy {
		a.status = "错误：配置正在写入"
		return
	}
	go func() {
		paths, e := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "导入 " + format + " 会话", Directory: directory, Multiple: !directory})
		if e != nil {
			a.report(e)
			return
		}
		if len(paths) == 0 {
			return
		}
		report, e := core.ReadThirdParty(context.Background(), paths, format)
		if e != nil {
			a.report(e)
			return
		}
		a.post(func() {
			a.features.archive = archiveState{open: true, archive: report.Archive(), vendor: &report, directory: a.managerGroup}
		})
	}()
}

func (a *App) exportManagerSelection() {
	if a.features.archive.busy {
		return
	}
	rows := a.selectedManagerRows()
	if len(rows) == 0 {
		a.exportProfiles()
		return
	}
	var profiles, dirs, links []string
	for _, row := range rows {
		if row.parent {
			a.status = "导出失败：请只选择会话、目录或引用，不包含上级目录行"
			return
		}
		if row.folder {
			dirs = append(dirs, row.group)
		} else if row.linkID != "" {
			links = append(links, row.linkID)
		} else {
			profiles = append(profiles, row.profile.ID)
		}
	}
	selected, err := a.config.ExportSelection(profiles, dirs, links)
	if err != nil {
		a.status = "导出失败：" + err.Error()
		return
	}
	a.exportProfiles()
	a.features.archive.exportConfig = &selected
}

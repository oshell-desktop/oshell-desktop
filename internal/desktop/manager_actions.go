// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"oshell-desktop/internal/core"
)

func (a *App) selectedManagerRows() []sessionRow {
	if context := a.sidebar.context; context != nil {
		if context.valid {
			if len(context.rows) > 0 {
				return context.rows
			}
			return []sessionRow{context.row}
		}
		return nil
	}
	rows := a.managerRows()
	var chosen []sessionRow
	for _, r := range rows {
		if a.managerSelection.Has(r.id) {
			chosen = append(chosen, r)
		}
	}
	if len(chosen) == 0 && a.managerRow >= 0 && a.managerRow < len(rows) {
		chosen = append(chosen, rows[a.managerRow])
	}
	return chosen
}
func (a *App) selectedProfileIDs() map[string]bool {
	ids := map[string]bool{}
	for _, r := range a.selectedManagerRows() {
		if !r.folder && r.linkID == "" {
			ids[r.profile.ID] = true
		}
	}
	return ids
}
func (a *App) connectManagerSelection() {
	rows := a.selectedManagerRows()
	a.authQueue = nil
	a.batchTerminalOpened = false
	a.batchTerminalOwner = a
	seen := map[string]bool{}
	for _, r := range rows {
		if !r.folder && !seen[r.profile.ID] {
			seen[r.profile.ID] = true
			a.authQueue = append(a.authQueue, r.profile)
		}
	}
	if len(a.authQueue) > 0 {
		a.nextAuthentication()
	} else if len(rows) == 1 {
		a.enterRow(rows[0])
	}
}
func (a *App) duplicateManagerSelection() {
	if a.passwordLocked() || a.configurationBusy() || a.vault == nil {
		return
	}
	ids := a.selectedProfileIDs()
	var sources []core.Profile
	for _, p := range a.config.Clone().Profiles {
		if ids[p.ID] {
			sources = append(sources, p)
		}
	}
	a.copyManagerProfiles(sources, nil, nil)
}

// A nil destination retains the existing duplicate-in-place behavior.
func (a *App) copyManagerProfiles(originals []core.Profile, destination *string, completed func()) {
	if len(originals) == 0 || a.passwordLocked() || a.configurationBusy() || a.vault == nil {
		return
	}
	revision, err := a.store.Revision()
	if err != nil {
		a.status = "复制失败：" + err.Error()
		return
	}
	vault := a.vault
	vaultRevision := vault.Revision()
	candidate := a.config.Clone()
	fromSidebar := a.sidebar.context != nil
	titleModes := map[string]string{}
	for _, p := range originals {
		titleModes[p.ID] = a.sessionTitleMode(p)
	}
	a.setConfigurationBusy(true)
	go func() {
		var updates []core.CredentialUpdate
		defer func() {
			for i := range updates {
				updates[i].Password = ""
			}
		}()
		copied := map[string]bool{}
		copiedModes := map[string]string{}
		var err error
		copyCredential := func(source, target core.Profile) error {
			update, exists, err := vault.PrepareCredentialCopy(source, target)
			if exists {
				updates = append(updates, update)
			}
			return err
		}
		for _, source := range originals {
			target := source
			if destination != nil {
				target.Group = *destination
			}
			duplicate := core.DuplicateProfile(target, candidate.Profiles)
			if destination != nil {
				collision := false
				for _, existing := range candidate.Profiles {
					collision = collision || existing.Group == target.Group && existing.Name == target.Name
				}
				if !collision {
					duplicate.Name = target.Name
				}
			}
			if err = copyCredential(source, duplicate); err != nil {
				break
			}
			if source.ProxyID == "" && source.Proxy.Kind != "" && source.Proxy.Kind != "none" {
				if err = copyCredential(core.ProxyCredentialProfile(source.Proxy), core.ProxyCredentialProfile(duplicate.Proxy)); err != nil {
					break
				}
			}
			candidate.Profiles = append(candidate.Profiles, duplicate)
			copied[duplicate.ID] = true
			copiedModes[strings.ToLower(duplicate.ID)] = titleModes[source.ID]
		}
		if err == nil {
			err = a.store.CommitCredentialEdit(context.Background(), candidate, vault, updates, revision, vaultRevision)
		}
		retry := localConflictRetry(err, candidate, updates)
		a.post(func() {
			if !errors.Is(err, core.ErrPendingTransaction) {
				a.setConfigurationBusy(false)
			}
			if err != nil {
				a.status = "复制失败：" + err.Error()
				if retry != nil {
					retry(a)
				}
				return
			}
			a.applyCommittedConfig(candidate)
			local := a.localUI.clone()
			if local.TitleModes == nil {
				local.TitleModes = map[string]string{}
			}
			for id, mode := range copiedModes {
				local.TitleModes[id] = mode
			}
			savedLocal := a.saveLocalPreferences(local)
			a.markSharedSyncChanged()
			if a.hub != nil {
				a.hub.scheduleSharedSync(2 * time.Second)
			}
			if fromSidebar {
				for _, profile := range candidate.Profiles {
					if copied[profile.ID] {
						a.revealSidebar(profile.ID)
						break
					}
				}
			} else {
				a.query = ""
				if destination != nil {
					a.managerGroup, a.managerAll = *destination, false
					a.managerProtocol, a.managerLastProtocol = "", ""
				} else if len(copied) == 1 {
					for _, profile := range candidate.Profiles {
						if copied[profile.ID] {
							a.managerGroup = profile.Group
							a.managerAll = false
							break
						}
					}
				} else {
					a.managerAll = true
				}
				a.managerSelection.Clear()
				a.managerRow = -1
				for i, row := range a.managerRows() {
					if copied[row.profile.ID] && row.linkID == "" {
						a.managerSelection.Add(row.id)
						if a.managerRow < 0 {
							a.managerRow = i
							a.selected = row.profile.ID
						}
					}
				}
			}
			a.status = fmt.Sprintf("已复制 %d 个会话", len(copied))
			if destination != nil {
				a.status = fmt.Sprintf("已粘贴 %d 个会话配置", len(copied))
			}
			if !savedLocal {
				a.status += "；本机标题设置保存失败：" + a.features.settingsError
			}
			if completed != nil {
				completed()
			}
			if a.manager != nil {
				a.manager.Invalidate()
			}
		})
	}()
}
func (a *App) deleteManagerSelection() {
	if a.passwordLocked() || a.configurationBusy() || a.vault == nil {
		return
	}
	rows := a.selectedManagerRows()
	if !a.managerSelectionMutable() {
		a.status = "错误：上级目录行或 Links 根目录不能删除"
		return
	}
	if len(rows) == 0 {
		return
	}
	a.confirmSessionDeletion(rows)
}
func (a *App) confirmSessionDeletion(rows []sessionRow) {
	if len(rows) == 0 || a.passwordLocked() || a.configurationBusy() || a.vault == nil {
		return
	}
	snapshot := a.config.Clone()
	var profiles, dirs, links, names []string
	for _, row := range rows {
		names = append(names, row.name)
		if row.folder {
			dirs = append(dirs, row.group)
		} else if row.linkID != "" {
			links = append(links, row.linkID)
		} else {
			profiles = append(profiles, row.profile.ID)
		}
	}
	candidate, summary, err := snapshot.PlanSessionDeletion(profiles, dirs, links)
	if err != nil {
		a.status = "删除失败：" + err.Error()
		return
	}
	revision, err := a.store.Revision()
	if err != nil {
		a.status = "删除失败：" + err.Error()
		return
	}
	vaultRevision := a.vault.Revision()
	apply := func() { a.commitManagerDeletion(snapshot, candidate, summary, revision, vaultRevision) }
	if len(rows) == 1 && rows[0].folder && summary.Profiles == 0 && summary.Links == 0 && summary.Directories == 1 {
		apply()
		return
	}
	parent := a.manager
	if parent == nil {
		parent = a.win
	}
	go func() {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: parent, Type: mygo.MessageWarning, Title: "删除所选项目", Message: fmt.Sprintf("删除 %d 个会话、%d 个目录和 %d 个快捷引用？", summary.Profiles, summary.Directories, summary.Links), Detail: strings.Join(names, "\n") + "\n\n只删除本地配置；活动连接和服务器文件不受影响。", Buttons: []string{"取消", "删除"}, DefaultButton: 0, CancelButton: 0})
		if err == nil && result.Button == 1 {
			a.post(apply)
		}
	}()
}
func (a *App) commitManagerDeletion(snapshot, candidate core.Config, summary core.SessionDeletion, revision, vaultRevision string) {
	if a.configurationBusy() || a.passwordLocked() || !reflect.DeepEqual(snapshot, a.config) {
		a.status = "删除失败：配置已变化，请重新确认"
		return
	}
	vault := a.vault
	if vault == nil {
		return
	}
	removed := map[string]bool{}
	for _, id := range summary.RemovedIDs {
		removed[id] = true
	}
	var changes []core.CredentialUpdate
	for _, profile := range snapshot.Profiles {
		if removed[profile.ID] {
			changes = append(changes, core.CredentialUpdate{Profile: profile, Delete: true})
			if profile.ProxyID == "" && profile.Proxy.Kind != "" && profile.Proxy.Kind != "none" {
				changes = append(changes, core.CredentialUpdate{Profile: core.ProxyCredentialProfile(profile.Proxy), Delete: true})
			}
		}
	}
	a.setConfigurationBusy(true)
	go func() {
		err := a.store.CommitCredentialEdit(context.Background(), candidate, vault, changes, revision, vaultRevision)
		retry := localConflictRetry(err, candidate, changes)
		a.post(func() {
			if !errors.Is(err, core.ErrPendingTransaction) {
				a.setConfigurationBusy(false)
			}
			if err != nil {
				a.status = "删除失败：" + err.Error()
				if retry != nil {
					retry(a)
				}
				return
			}
			a.applyCommittedConfig(candidate)
			a.markSharedSyncChanged()
			if a.hub != nil {
				a.hub.scheduleSharedSync(2 * time.Second)
			}
			a.managerRow = -1
			a.managerSelection.Clear()
			a.selected = ""
			a.status = fmt.Sprintf("已删除 %d 个会话、%d 个目录和 %d 个引用", summary.Profiles, summary.Directories, summary.Links)
			if a.manager != nil {
				a.manager.Invalidate()
			}
		})
	}()
}

func (a *App) managerConnectCount() int {
	ids := map[string]bool{}
	for _, row := range a.selectedManagerRows() {
		if !row.folder {
			ids[row.profile.ID] = true
		}
	}
	return len(ids)
}

func (a *App) managerSelectionMutable() bool {
	for _, row := range a.selectedManagerRows() {
		if row.parent || row.folder && row.group == "Links" {
			return false
		}
	}
	return true
}

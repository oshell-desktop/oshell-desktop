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

type sharedSyncState struct {
	method                               string
	deferred                             bool
	deferredBackground                   bool
	savedOptions                         core.SyncOptions
	savedVaultRevision                   string
	directoryChanged                     bool
	directoryDisabled                    bool
	directoryGeneration                  uint64
	manualQueued                         bool
	probeCancel                          context.CancelFunc
	probeGeneration                      uint64
	probeStatus                          string
	probing                              bool
	pendingDirectory                     *core.PendingSyncDirectory
	queuedIdentity                       string
	logExportBusy                        bool
	diagnostics                          core.SyncDiagnostics
	summary                              core.SyncSummary
	phase                                core.SyncEvent
	logEntries                           []core.SyncDiagnosticEntry
	logIssue                             string
	logsOpen, savedEnabled, pendingLocal bool
	logList                              ui.ListState
	conflictIndex                        int
	conflictDetailKey                    string
	conflictList                         ui.ListState
	options                              core.SyncOptions
	password, status                     string
	loaded, busy, conflictOpen           bool
	pendingDraft                         bool
	background                           bool
	cancel                               context.CancelFunc
	plan                                 *core.SharedSyncPlan
	choices                              map[string]core.SharedMergeChoice
	master                               []byte
}

func (a *App) loadSharedSyncSettings() {
	f := &a.features.sharedSync
	if f.loaded {
		return
	}
	f.loaded = true
	options, e := a.loadSyncOptions()
	f.options = options
	f.method = "none"
	if options.Enabled {
		f.method = options.Backend
	}
	f.savedOptions = options
	f.savedVaultRevision = a.vault.Revision()
	f.pendingDirectory, _ = core.LoadPendingSyncDirectory(a.store.Dir)
	f.directoryChanged = false
	f.directoryDisabled = false
	f.savedEnabled = options.Enabled
	if a.passwordLocked() {
		status, _ := core.SyncOptionsStatus(a.store.Dir)
		f.savedEnabled = status.Enabled
	}
	f.diagnostics = core.SyncDiagnostics{Directory: a.store.Dir, Target: options.Target(), Backend: options.Backend}
	a.loadSyncDiagnostics()
	f.pendingDraft = (core.SharedConflictStore{Directory: a.store.Dir, Target: options.Target()}).Exists()
	if e != nil {
		f.status = e.Error()
	}
}
func (a *App) sharedSyncSettings(c *ui.Context) {
	a.loadSharedSyncSettings()
	f := &a.features.sharedSync
	a.syncStatusCard(c)
	ui.Text(c, "多设备同步").Bold()
	ui.Text(c, "默认仅同步会话资料；可选择其他类别，未选择的设置保留在本机。").Wrap()
	current := "none"
	if f.options.Enabled {
		current = f.options.Backend
	}
	if f.method == "" {
		f.method = current
	}
	selectValue(c, "同步方式", &f.method, [][2]string{{"none", "不启用同步"}, {"directory", "同步目录"}, {"webdav", "WebDAV"}})
	if f.method != current {
		a.selectSyncMethod(f.method)
	}
	if f.options.Enabled {
		if f.options.Backend == "directory" {
			ui.Field(c, "同步目录", func() { ui.TextInput(c, &f.options.Address).Label("同步目录地址") })
			ui.Button(c, "选择同步目录…").Disabled(f.busy).OnClick(a.chooseLiveSyncDirectory)
			ui.Text(c, "自动识别空目录或已有加密配置，点击应用立即切换；首次接入仍需确认。").Wrap()
		} else {
			ui.Field(c, "WebDAV 地址", func() {
				ui.TextInput(c, &f.options.Address).Label("同步目录地址").Placeholder("HTTP / HTTPS WebDAV 父目录地址")
			})
			ui.Text(c, "自动使用或创建 oshell 子目录；已有直接同步文件的旧地址继续沿用。测试连接可能创建该目录。").Wrap()
			ui.Field(c, "用户名", func() { ui.TextInput(c, &f.options.Username) })
			ui.Field(c, "密码", func() { passwordInput(c, &f.password).Placeholder("留空沿用此地址已保存的密码") })
			a.webDAVCertificateSettings(c)
		}
	}
	ui.Row(c).Gap(8).Children(func() {
		if f.options.Enabled && f.options.Backend == "webdav" {
			ui.Button(c, "测试 WebDAV 连接").Disabled(f.probing || f.busy || a.configurationBusy()).OnClick(a.testWebDAVConnection)
			if f.probing {
				ui.Button(c, "取消连接测试").OnClick(a.cancelWebDAVProbe)
			}
		}
		ui.PrimaryButton(c, "关闭设置后同步").Disabled(a.syncOperationBusy() || a.configurationBusy()).OnClick(a.queueSavedSharedSync)
		owner := a.syncStatusOwner()
		active := &owner.features.sharedSync
		if active.busy || active.manualQueued || active.deferred {
			ui.Button(c, "取消本次").OnClick(owner.cancelSharedSync)
		}
		if (f.plan != nil || f.pendingDraft) && !f.busy {
			ui.Button(c, "处理待确认差异").OnClick(a.startSavedSharedSync)
			ui.Button(c, "放弃本机冲突草稿…").OnClick(a.discardSharedConflictDraft)
		}
	})
	a.syncPolicySettings(c)
	if f.probeStatus != "" {
		ui.Text(c, f.probeStatus).Wrap()
	}
	if f.status != "" {
		ui.Text(c, f.status).Wrap()
	}
	a.syncLocationSettings(c)
	ui.Text(c, "WebDAV 共用 oshell-vault.json；同步目录共用 configuration.json。本机数据目录保持独立，不要同步本机密钥。云盘进度请在对应客户端查看。").TextColor(c.Theme().TextMuted)
}
func (a *App) saveSharedSyncSettings() { a.saveSharedSyncSettingsThen(nil) }

func (a *App) saveSharedSyncSettingsThen(done func()) {
	f := &a.features.sharedSync
	if a.syncOperationBusy() || a.configurationBusy() {
		return
	}
	options, password := f.options, f.password
	expected := f.savedOptions
	expectedVault := f.savedVaultRevision
	options.Version = 1
	if err := options.Policy.Validate(); err != nil {
		f.status = err.Error()
		return
	}
	if options.Backend != "directory" {
		options.RequireExisting = false
	}
	protected, locked := a.vault.State()
	if options.Enabled && (!protected || locked) {
		f.status = "请先启用并解锁主密码保护"
		return
	}
	if options.Enabled {
		if _, e := options.Transport(password, a.store.Dir); e != nil {
			f.status = e.Error()
			return
		}
	}
	f.busy = true
	a.setConfigurationBusy(true)
	go func() {
		err := core.SaveSyncConnection(a.vault, expected, options, password, expectedVault)
		if err == nil {
			event := core.SyncConfigured
			if !options.Enabled {
				event = core.SyncDisabled
			}
			_ = (core.SyncDiagnostics{Directory: a.store.Dir, Target: options.Target(), Backend: options.Backend}).Record(event, true, 0, nil, "")
		}
		complete := func() {
			f.busy = false
			a.setConfigurationBusy(false)
			if err != nil {
				f.status = err.Error()
				a.features.settingsError = "同步设置保存失败：" + err.Error()
				return
			}
			f.status = "同步设置已保存"
			a.invalidateSyncPresentation()
			if f.password == password {
				f.password = ""
			}
			if a.hub != nil {
				a.hub.syncAutomaticFailed = false
				a.hub.scheduleSharedSync(2 * time.Second)
			}
			a.clearSharedSyncPlan()
			f.loaded = false
			a.loadSharedSyncSettings()
			if done != nil && !a.stopped.Load() {
				done()
			}
		}
		if !a.post(complete) {
			mygo.RunOnMain(complete)
		}
	}()
}
func (a *App) clearSharedSyncPlan() {
	f := &a.features.sharedSync
	clear(f.master)
	f.master = nil
	f.plan = nil
	f.choices = nil
	f.conflictIndex = 0
	f.conflictDetailKey = ""
	f.conflictList = ui.ListState{}
	f.conflictOpen = false
}
func (a *App) startSharedSync() {
	f := &a.features.sharedSync
	if a.syncOperationBusy() || a.configurationBusy() {
		return
	}
	background := f.background
	f.background = false
	if a.deferSyncForSettings(background) {
		return
	}
	f.deferred = false
	options, e := a.loadSyncOptions()
	if e != nil {
		a.sharedSyncFailed(e)
		return
	}
	if !options.Enabled {
		f.status = "请先保存并启用同步设置"
		return
	}
	if background && !options.Policy.Effective().Automatic {
		return
	}
	master, e := a.vault.MasterSecret()
	if e != nil {
		a.sharedSyncFailed(e)
		return
	}
	password := ""
	if options.Backend == "webdav" {
		p := options.CredentialProfile()
		password, e = a.vault.GetFor(p, p.Identity())
		if e != nil {
			clear(master)
			a.sharedSyncFailed(e)
			return
		}
	}
	transport, e := options.Transport(password, a.store.Dir)
	if e != nil {
		clear(master)
		a.sharedSyncFailed(e)
		return
	}
	a.clearSharedSyncPlan()
	// The worker owns master until Prepare returns. Closing a window may wipe
	// the UI copy while the worker is still unwinding cancellation.
	f.master = append([]byte(nil), master...)
	f.savedEnabled, f.savedOptions = true, options
	f.loaded = true
	f.options = options
	f.busy = true
	a.invalidateSyncPresentation()
	f.status = "正在读取共享配置…"
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	cfg := a.config.Clone()
	engine := &core.SharedSyncEngine{Manual: !background, Store: a.store, Vault: a.vault, Transport: transport, Replica: core.SharedReplicaStore{Directory: a.store.Dir, Target: options.Target()}}
	policy := options.Policy.Effective()
	engine.Selection = &policy.Content
	f.diagnostics = engine.Diagnostics()
	engine.Progress = func(event core.SyncEvent, issue error) {
		a.post(func() {
			f.phase = event
			a.invalidateSyncPresentation()
			if issue != nil {
				f.logIssue = issue.Error()
			} else {
				f.logIssue = ""
			}
		})
	}
	go func() {
		defer clear(master)
		plan, err := engine.Prepare(ctx, cfg, master, !options.RequireExisting)
		if !a.post(func() {
			f.busy = false
			if err == nil && ctx.Err() != nil {
				err = ctx.Err()
			}
			cancel()
			f.cancel = nil
			f.pendingDraft = engine.ConflictStore().Exists()
			a.loadSyncDiagnostics()
			a.invalidateSyncPresentation()
			if err != nil {
				f.status = err.Error()
				a.scheduleSyncResult(err)
				a.invalidateSyncPresentation()
				if !background {
					a.status = f.status
				}
				a.clearSharedSyncPlan()
				return
			}
			if a.deferSyncForSettings(background) {
				return
			}
			f.plan = plan
			f.choices = map[string]core.SharedMergeChoice{}
			if plan.InitialChoiceRequired || len(plan.Conflicts) > 0 {
				f.conflictOpen = !background
				if f.conflictOpen && a.win != nil {
					a.win.Show()
					a.win.Focus()
				}
				if a.hub != nil {
					a.hub.armSharedSync(0)
				}
				f.status = "共享配置有待确认差异"
				a.status = f.status
				return
			}
			a.commitSharedSync()
		}) {
			clear(master)
			mygo.RunOnMain(func() {
				cancel()
				f.cancel, f.busy = nil, false
				a.clearSharedSyncPlan()
				f.phase = core.SyncCancelled
				a.invalidateSyncPresentation()
			})
		}
	}()
}
func (a *App) commitSharedSync() {
	f := &a.features.sharedSync
	if f.busy || f.plan == nil || a.configurationBusy() {
		return
	}
	f.conflictOpen = false // The user has submitted this conflict dialog.
	if a.deferSyncForSettings(false) {
		return
	}
	f.busy = true
	f.conflictOpen = false
	f.status = "正在提交共享配置…"
	a.setConfigurationBusy(true)
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	plan, master := f.plan, append([]byte(nil), f.master...)
	choices := map[string]core.SharedMergeChoice{}
	for k, v := range f.choices {
		choices[k] = v
	}
	go func() {
		_, err := plan.Commit(ctx, choices, master)
		clear(master)
		cancel()
		// Reload disk even on a late replica-write failure: configuration may have
		// committed already, and the UI must not keep an obsolete copy.
		saved, loadErr := a.store.Load()
		complete := func() {
			f.busy = false
			f.cancel = nil
			a.setConfigurationBusy(false)
			a.clearSharedSyncPlan()
			f.pendingDraft = plan.ConflictDraftExists()
			a.loadSyncDiagnostics()
			if loadErr == nil {
				a.applyCommittedConfig(saved)
			}
			if err != nil {
				f.status = err.Error()
			} else if loadErr != nil {
				f.status = loadErr.Error()
			} else {
				f.status = "同步完成"
			}
			a.status = f.status
			a.scheduleSyncResult(err)
			if errors.Is(err, core.ErrSyncDeferred) && !errors.Is(err, context.Canceled) {
				f.phase = core.SyncQueued
				f.pendingLocal = true
				if a.hub != nil {
					a.hub.scheduleSharedSync(2 * time.Second)
				}
			}
			a.invalidateSyncPresentation()
		}
		if !a.post(complete) {
			// A native/forced window close must not strand the shared busy
			// flag. The application UI loop can outlive this window.
			mygo.RunOnMain(complete)
		}
	}()
}
func (a *App) sharedSyncConflictDialog(c *ui.Context) {
	f := &a.features.sharedSync
	ui.Modal(c, &f.conflictOpen, func() {
		if f.plan == nil {
			return
		}
		width, height := c.Size()
		ui.Column(c).Width(min(float32(1000), max(float32(520), width-48))).Gap(12).Children(func() {
			ui.Text(c, "共享配置差异").FontSize(20).Bold()
			if f.plan.InitialChoiceRequired {
				ui.Text(c, fmt.Sprintf("共享端有 %d 个会话，本机有 %d 个。仅应用已选择的同步类别；未选类别保留原值。", len(f.plan.Remote.Profiles), len(f.plan.Local.Profiles))).Wrap()
				for _, option := range []struct {
					title string
					merge bool
				}{{"使用共享版本", false}, {"合并本机会话", true}} {
					ui.Button(c, option.title).OnClick(func() {
						if e := f.plan.ChooseInitial(option.merge); e != nil {
							f.status = e.Error()
							return
						}
						if len(f.plan.Conflicts) == 0 {
							a.commitSharedSync()
						}
					})
				}
			} else {
				a.sharedConflictComparison(c, min(float32(390), max(float32(180), height-200)))
				ui.PrimaryButton(c, "应用选择并同步").Disabled(a.sharedConflictChoiceCount() != len(f.plan.Conflicts)).OnClick(a.commitSharedSync)
			}
			ui.Button(c, "稍后处理").OnClick(func() {
				f.conflictOpen = false
				f.status = "有待确认差异，尚未同步"
				if err := f.diagnostics.Record(core.SyncPostponed, true, 0, nil, ""); err != nil {
					f.logIssue = err.Error()
				}
			})
		})
	})
}

// One timer per application, event-debounced after edits and otherwise once a
// minute, matching the source app. Disabled sync leaves no timer running.
func (h *applicationHub) scheduleSharedSync(delay time.Duration) {
	windows := h.configurationWindows()
	if len(windows) > 0 {
		if options, err := windows[0].loadSyncOptions(); err == nil {
			p := options.Policy.Effective()
			if options.Enabled && p.Automatic && h.syncAutomaticFailed {
				if h.syncTimer != nil {
					return
				}
				delay = time.Duration(p.RetryInterval) * time.Second
			} else if delay == 2*time.Second {
				if p.AfterChanges {
					delay = time.Duration(p.ChangeDelay) * time.Second
				} else {
					// An unrelated save must not postpone the periodic check forever.
					if options.Enabled && p.Automatic && h.syncTimer != nil {
						return
					}
					delay = time.Duration(p.CheckInterval) * time.Second
				}
			} else if delay == time.Minute {
				delay = time.Duration(p.CheckInterval) * time.Second
			}
		}
	}
	h.armSharedSync(delay)
}

func (h *applicationHub) armSharedSync(delay time.Duration) {
	h.syncTimerGeneration++
	generation := h.syncTimerGeneration
	if h.syncTimer != nil {
		h.syncTimer.Stop()
		h.syncTimer = nil
	}
	h.nextSync = time.Time{}
	windows := h.configurationWindows()
	if h.quitting || len(windows) == 0 {
		return
	}
	options, e := windows[0].loadSyncOptions()
	if e != nil || !options.Enabled || !options.Policy.Effective().Automatic || delay <= 0 {
		return
	}
	owner := windows[0]
	h.nextSync = time.Now().Add(delay)
	h.syncTimer = time.AfterFunc(delay, func() {
		owner.post(func() {
			if h.syncTimerGeneration != generation {
				return
			}
			h.syncTimer = nil
			h.nextSync = time.Time{}
			_, locked := owner.vault.State()
			if owner.syncStatusOwner().features.sharedSync.plan != nil {
				return
			}
			if !locked && !owner.syncEditingBlocked() && !owner.configurationBusy() && !owner.syncOperationBusy() {
				owner.features.sharedSync.background = true
				owner.startSharedSync()
			} else {
				// Keep a queued change/startup check when periodic checks are disabled.
				h.armSharedSync(time.Second)
				return
			}
			if !owner.syncOperationBusy() {
				h.scheduleSharedSync(time.Minute)
			}
		})
	})
}

func (a *App) discardSharedConflictDraft() {
	f := &a.features.sharedSync
	if a.syncOperationBusy() || a.configurationBusy() {
		return
	}
	options, err := a.loadSyncOptions()
	if err != nil {
		f.status = err.Error()
		return
	}
	parent := a.preferencesWindow
	if parent == nil {
		parent = a.win
	}
	result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: parent, Type: mygo.MessageWarning, Title: "放弃本机同步草稿？", Message: "仅删除本机尚未确认的冲突草稿，本机已保存配置和共享数据保持不变。", Buttons: []string{"取消", "放弃草稿"}, DefaultButton: 0, CancelButton: 0})
	if err != nil {
		f.status = err.Error()
		return
	}
	if result.Button != 1 {
		return
	}
	if err = (core.SharedConflictStore{Directory: a.store.Dir, Target: options.Target()}).Clear(); err != nil {
		f.status = err.Error()
		return
	}
	a.clearSharedSyncPlan()
	f.pendingDraft = false
	f.status = "已放弃本机冲突草稿，共享数据未修改"
}

func (a *App) startSavedSharedSync() {
	if a.features.sharedSync.busy || a.configurationBusy() {
		return
	}
	a.features.sharedSync.manualQueued = false
	a.features.sharedSync.deferred = false
	a.closeSettings()
	a.features.sharedSync.background = false
	a.startSharedSync()
}

// Menu commands use the saved sync target, never the settings draft.
func (a *App) runSyncMenuAction(discard bool) {
	if a.passwordLocked() {
		a.features.startupUnlock = true
		a.showVault()
		return
	}
	if a.configurationBusy() || a.features.sharedSync.busy {
		return
	}
	if discard {
		a.discardSharedConflictDraft()
	} else {
		a.startSavedSharedSync()
	}
	// Surface immediate setup/read errors even when preferences are closed.
	a.status = a.features.sharedSync.status
}

func (a *App) queueSavedSharedSync() {
	f := &a.features.sharedSync
	if a.syncOperationBusy() || a.configurationBusy() {
		return
	}
	f.manualQueued = true
	a.invalidateSyncPresentation()
	f.status = "已安排同步，关闭设置后使用已保存的配置执行"
}

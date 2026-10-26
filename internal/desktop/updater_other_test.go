//go:build !darwin

package desktop

import (
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strings"
	"testing"
	"time"
)

func TestRetainedU01SourceBindingAndBusyGuard(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.prepareSettings()
	state := a.updateState()
	state.tokenDraft = "synthetic-token-retained"
	a.saveUpdateToken(false)
	a.features.settingsDraft.UpdateRepository = "fixture/other"
	state.busy = true
	if err := a.validateUpdateSettings(); err == nil {
		t.Fatal("source changed while busy")
	}
	state.busy = false
	state.tokenDraft = "synthetic-new-token"
	a.saveUpdateToken(false)
	if token, err := core.ReadUpdateToken(a.store.Dir, "fixture/other"); err != nil || token != "" {
		t.Fatal("token crossed unapplied source")
	}
	a.applySettings()
	a.prepareSettings()
	if state.hasToken || state.tokenDraft != "" {
		t.Fatal("old source token shown for new source")
	}
	state.tokenDraft = "synthetic-secret\r\ninvalid"
	a.saveUpdateToken(false)
	if strings.Contains(state.status, "synthetic-secret") {
		t.Fatal("error leaked secret")
	}
}

func TestP2U04DeferredInstallAndIdleSkipsConfirmation(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.Settings.UpdateRepository = "owner/repo"
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	state := a.updateState()
	state.prepared = &core.PortableUpdate{}
	state.source = "owner/repo"
	oldStart, oldConfirm := startPortableUpdate, showUpdateConfirmation
	defer func() { startPortableUpdate = oldStart; showUpdateConfirmation = oldConfirm }()
	started := make(chan struct{}, 2)
	startPortableUpdate = func(*core.PortableUpdate) error { started <- struct{}{}; return fmt.Errorf("synthetic start failure") }
	showUpdateConfirmation = func(mygo.MessageOptions) (mygo.MessageResult, error) {
		t.Error("idle install asked session confirmation")
		return mygo.MessageResult{}, nil
	}
	a.features.uploadDraft = &uploadConfirmation{open: true}
	a.installUpdate()
	if !state.pendingInstall || state.busy {
		t.Fatal("modal did not queue install")
	}
	a.features.uploadDraft = nil
	a.drain()
	waitOperator(t, a, func() bool { return !state.busy && state.status == "synthetic start failure" })
	select {
	case <-started:
	default:
		t.Fatal("deferred installation never resumed")
	}
	a.drain()
	select {
	case <-started:
		t.Fatal("failed install retried without user action")
	default:
	}
	if state.prepared == nil {
		t.Fatal("failure discarded prepared update")
	}
}

func TestUpdateSettingsDraftValidationAndView(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.prepareSettings()
	a.features.settingsTab = 4
	tt := ui.NewTester(a.settingsView, 800, 650)
	if !tt.HasText("签名更新") || !tt.HasText("检查更新") {
		t.Fatal(tt.Texts())
	}
	a.features.settingsDraft.AutomaticUpdateChecks = true
	if e := a.validateUpdateSettings(); e != nil || a.features.settingsDraft.UpdateRepository != core.DefaultUpdateRepository {
		t.Fatal("default repository unavailable", e)
	}
	a.features.settingsDraft.UpdateRepository = "https://github.com/owner/repo.git"
	if e := a.validateUpdateSettings(); e != nil || a.features.settingsDraft.UpdateRepository != "owner/repo" {
		t.Fatal("source normalization", e)
	}
	a.closeSettings()
	if a.config.Settings.UpdateRepository != "" || a.config.Settings.AutomaticUpdateChecks {
		t.Fatal("cancel saved update preferences")
	}
	a.prepareSettings()
	a.features.settingsDraft.UpdateRepository = "owner/repo"
	a.features.updates.busy = true
	if e := a.validateUpdateSettings(); e == nil {
		t.Fatal("changed repository during update")
	}
	a.features.updates.busy = false
	a.applySettings()
	if a.config.Settings.UpdateRepository != "owner/repo" {
		t.Fatal("apply lost update preference")
	}
}

func TestUpdateTokenSettingsSaveAndClear(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.prepareSettings()
	state := a.updateState()
	state.tokenDraft = "github_pat_synthetic_fixture"
	a.saveUpdateToken(false)
	if !state.hasToken || state.tokenDraft != "" {
		t.Fatal("token not saved", state.status)
	}
	if token, e := core.ReadUpdateToken(a.store.Dir, core.DefaultUpdateRepository); e != nil || token != "github_pat_synthetic_fixture" {
		t.Fatal("token read failed", e)
	}
	state.busy = true
	a.saveUpdateToken(true)
	if !state.hasToken {
		t.Fatal("token changed during update")
	}
	state.busy = false
	a.saveUpdateToken(true)
	if state.hasToken {
		t.Fatal("token not cleared")
	}
}
func TestUpdateSchedulerKeepsDailyDeadlineAndStopsWhenDisabled(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.Settings.UpdateRepository = "owner/repo"
	cfg.Settings.AutomaticUpdateChecks = true
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	h := &applicationHub{store: a.store, config: cfg, vault: a.vault, windows: []*App{a}}
	a.hub = h
	h.updates.scheduledSource = "owner/repo"
	h.updates.nextCheck = time.Now().Add(time.Hour)
	h.scheduleUpdates()
	timer := h.updates.timer
	deadline := h.updates.nextCheck
	if timer == nil {
		t.Fatal("enabled checks not scheduled")
	}
	defer h.stopUpdates()
	h.scheduleUpdates()
	if h.updates.timer != timer || !h.updates.nextCheck.Equal(deadline) {
		t.Fatal("ordinary config save postponed daily check")
	}
	h.config.Settings.AutomaticUpdateChecks = false
	h.scheduleUpdates()
	if h.updates.timer != nil {
		t.Fatal("disabled checks retained idle timer")
	}
	h.updates.source = "owner/old"
	h.updates.candidate = &core.ReleaseUpdate{}
	h.scheduleUpdates()
	if h.updates.candidate != nil {
		t.Fatal("disabled automatic checks retained candidate from old repository")
	}
}

func TestUpdateInstallationWaitsForAllWindowModals(t *testing.T) {
	cases := []struct {
		name string
		open func(*App)
	}{
		{"upload confirmation", func(a *App) { a.features.uploadDraft = &uploadConfirmation{open: true} }},
		{"standalone SCP", func(a *App) { a.features.scpDraft = &standaloneSCPDraft{open: true} }},
		{"sync diagnostics", func(a *App) { a.features.sharedSync.logsOpen = true }},
		{"vault operation", func(a *App) { a.features.vaultBusy = true }},
		{"terminal search", func(a *App) {
			p := &pane{search: terminalSearchState{open: true}}
			a.tabs = []*tab{{root: &node{pane: p}, active: p}}
		}},
		{"shortcut capture", func(a *App) { a.features.settingsCapture = "copy" }},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			store := &core.Store{Dir: t.TempDir()}
			owner := newApp(store, core.DefaultConfig())
			other := newApp(store, core.DefaultConfig())
			files := newApp(store, core.DefaultConfig())
			hub := &applicationHub{windows: []*App{owner, other}, fileWindow: files}
			owner.hub = hub
			owner.settingsOpen = true // The updater's own preferences window is allowed.
			if owner.updateDialogsOpen() {
				t.Fatal("preferences alone blocked updater")
			}
			for _, target := range []*App{owner, other, files} {
				item.open(target)
				if !owner.updateDialogsOpen() {
					t.Fatal("pending modal did not block install")
				}
				target.features = featureState{}
				for _, p := range target.allPanes() {
					p.search.open = false
				}
				if owner.updateDialogsOpen() {
					t.Fatal("dismissed modal kept install blocked")
				}
			}
		})
	}
}

func TestInstallConfirmationSingleFlightAndRetainsPreparedUpdate(t *testing.T) {
	for _, mode := range []string{"cancel", "error", "changed", "quitting"} {
		t.Run(mode, func(t *testing.T) {
			cfg := core.DefaultConfig()
			cfg.Settings.UpdateRepository = "owner/repo"
			a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
			a.hub = &applicationHub{windows: []*App{a}, config: cfg}
			operatorPane(t, a, "active update fixture")
			prepared := &core.PortableUpdate{}
			state := a.updateState()
			state.prepared = prepared
			state.source = "owner/repo"
			entered := make(chan struct{}, 2)
			release := make(chan struct{})
			old := showUpdateConfirmation
			showUpdateConfirmation = func(mygo.MessageOptions) (mygo.MessageResult, error) {
				entered <- struct{}{}
				<-release
				if mode == "error" {
					return mygo.MessageResult{}, fmt.Errorf("fixture error")
				}
				button := 0
				if mode == "changed" || mode == "quitting" {
					button = 1
				}
				return mygo.MessageResult{Button: button}, nil
			}
			t.Cleanup(func() { showUpdateConfirmation = old })
			a.installUpdate()
			a.installUpdate()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("confirmation not opened")
			}
			select {
			case <-entered:
				t.Fatal("duplicate confirmation")
			default:
			}
			if !state.confirming || state.busy {
				t.Fatal("incorrect confirmation state")
			}
			if mode == "changed" {
				state.prepared = &core.PortableUpdate{}
			}
			if mode == "quitting" {
				a.hub.quitting = true
			}
			kept := state.prepared
			close(release)
			waitOperator(t, a, func() bool { return !state.confirming })
			if state.prepared != kept || state.installing || state.busy || a.configurationBusy() {
				t.Fatal("confirmation lost staged update or started install")
			}
			if (mode == "error" || mode == "changed") && state.status == "" {
				t.Fatal("confirmation failure not reported")
			}
		})
	}
}

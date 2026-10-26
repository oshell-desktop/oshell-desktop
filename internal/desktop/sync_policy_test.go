package desktop

import (
	"errors"
	"oshell-desktop/internal/core"
	"strings"
	"testing"
	"time"
)

func policyFixture(t *testing.T, policy core.SyncPolicy) (*App, *applicationHub) {
	t.Helper()
	store := &core.Store{Dir: t.TempDir()}
	a := newApp(store, core.DefaultConfig())
	h := &applicationHub{store: store, vault: a.vault, windows: []*App{a}}
	a.hub = h
	options := core.SyncOptions{Version: 1, Enabled: true, Backend: "directory", Address: t.TempDir(), Policy: policy}
	if err := core.SaveSyncOptions(store.Dir, options); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		h.quitting = true
		h.syncTimerGeneration++
		if h.syncTimer != nil {
			h.syncTimer.Stop()
		}
	})
	return a, h
}
func TestSyncPolicyFailureDoesNotRestartFromOrdinarySave(t *testing.T) {
	p := core.DefaultSyncPolicy()
	p.RetryInterval = 0
	a, h := policyFixture(t, p)
	a.scheduleSyncResult(errors.New("synthetic network failure"))
	h.scheduleSharedSync(2 * time.Second)
	if h.syncTimer != nil || !h.syncAutomaticFailed {
		t.Fatal("save restarted disabled failure retry")
	}
	a.scheduleSyncResult(nil)
	if h.syncTimer == nil || h.syncAutomaticFailed {
		t.Fatal("success did not restore periodic checks")
	}
}
func TestSyncPolicyDisabledChangeTriggerPreservesPeriodicDeadline(t *testing.T) {
	p := core.DefaultSyncPolicy()
	p.AfterChanges = false
	_, h := policyFixture(t, p)
	h.scheduleSharedSync(time.Minute)
	timer, deadline := h.syncTimer, h.nextSync
	h.scheduleSharedSync(2 * time.Second)
	if h.syncTimer != timer || !h.nextSync.Equal(deadline) {
		t.Fatal("save postponed periodic check")
	}
}
func TestSyncPolicyManualModeRejectsAutomaticEntry(t *testing.T) {
	p := core.DefaultSyncPolicy()
	p.Automatic = false
	a, h := policyFixture(t, p)
	a.features.sharedSync.background = true
	a.startSharedSync()
	if a.features.sharedSync.busy || a.features.sharedSync.status != "" {
		t.Fatal("manual policy reached automatic sync", a.features.sharedSync.status)
	}
	h.scheduleStartupSync()
	if h.syncTimer != nil {
		t.Fatal("manual policy armed startup timer")
	}
}
func TestSyncPolicySummaryOmitsInactiveTriggers(t *testing.T) {
	p := core.DefaultSyncPolicy()
	p.OnStartup = false
	p.AfterChanges = false
	p.CheckInterval = 0
	p.RetryInterval = 0
	s := syncPolicySummary(p)
	if strings.Contains(s, "启动时") || strings.Contains(s, "修改后") || strings.Contains(s, "每 0") || !strings.Contains(s, "等待手动重试") {
		t.Fatal(s)
	}
}
func TestSyncPolicyBlockedOneShotRemainsQueued(t *testing.T) {
	p := core.DefaultSyncPolicy()
	p.CheckInterval = 0
	a, h := policyFixture(t, p)
	a.editing = true
	h.armSharedSync(time.Millisecond)
	first := h.syncTimer
	waitOperator(t, a, func() bool { return h.syncTimer != first })
	if h.syncTimer == nil || h.nextSync.IsZero() || a.features.sharedSync.busy {
		t.Fatal("blocked one-shot was discarded")
	}
}

func TestSyncPolicyStartupRestoresFailurePauseAndCancellation(t *testing.T) {
	p := core.DefaultSyncPolicy()
	p.RetryInterval, p.CheckInterval = 0, 0
	a, h := policyFixture(t, p)
	o, err := a.loadSyncOptions()
	if err != nil {
		t.Fatal(err)
	}
	d := core.SyncDiagnostics{Directory: a.store.Dir, Target: o.Target(), Backend: o.Backend}
	if err := d.Record(core.SyncFailed, false, 0, errors.New("synthetic failure"), ""); err != nil {
		t.Fatal(err)
	}
	if err := d.Record(core.SyncQueued, false, 0, nil, ""); err != nil {
		t.Fatal(err)
	}
	h.scheduleStartupSync()
	if !h.syncAutomaticFailed || h.syncTimer != nil {
		t.Fatal("startup bypassed failure pause")
	}
	a.loadSharedSyncSettings()
	if a.features.sharedSync.phase != core.SyncFailed {
		t.Fatal("failure hidden after local save")
	}
	h.armSharedSync(time.Hour)
	generation := h.syncTimerGeneration
	a.features.sharedSync.manualQueued = true
	a.cancelSharedSync()
	if h.syncTimer != nil || !h.nextSync.IsZero() || h.syncTimerGeneration == generation || a.features.sharedSync.manualQueued {
		t.Fatal("cancel retained queued timer")
	}
	if !h.syncAutomaticFailed {
		t.Fatal("cancel cleared failure pause")
	}
}

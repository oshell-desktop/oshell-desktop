package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSyncDiagnosticsFailureSurvivesQueueAndReopen(t *testing.T) {
	d := SyncDiagnostics{Directory: t.TempDir(), Target: "fixture", Backend: "directory"}
	for _, event := range []SyncEvent{SyncFailed, SyncLocalChanged, SyncQueued, SyncCancelled} {
		var err error
		if event == SyncFailed {
			err = context.DeadlineExceeded
		}
		if err := d.Record(event, false, 0, err, ""); err != nil {
			t.Fatal(err)
		}
	}
	doc, err := d.read()
	if err != nil || doc.Summary.Failure == nil || doc.Summary.Failure.Kind != "network" {
		t.Fatal("queued/cancelled event discarded persistent failure", err, doc.Summary)
	}
	copy := cloneSyncDiagnostic(doc)
	copy.Summary.Failure.Kind = "tls"
	if doc.Summary.Failure.Kind != "network" {
		t.Fatal("summary alias")
	}
	other := d
	other.Target = "other"
	if other.SummaryForTarget(doc).Failure != nil {
		t.Fatal("failure crossed targets")
	}
	if err := d.Record(SyncUnchanged, true, 0, nil, ""); err != nil {
		t.Fatal(err)
	}
	doc, err = d.read()
	if err != nil || doc.Summary.Failure != nil {
		t.Fatal("success retained failure", err)
	}
}

func TestSyncDiagnosticsRedactionPersistenceAndExport(t *testing.T) {
	d := SyncDiagnostics{Directory: t.TempDir(), Target: "https://user:private-password@example.invalid/private/path", Backend: "webdav"}
	if e := d.Record(SyncStarted, true, 0, nil, ""); e != nil {
		t.Fatal(e)
	}
	fingerprint := d.scope()
	if e := d.Record(SyncCompleted, true, time.Second, nil, fingerprint); e != nil {
		t.Fatal(e)
	}
	if e := d.Record(SyncFailed, false, time.Second, &os.PathError{Op: "open", Path: "private-path", Err: errors.New("private-password")}, ""); e != nil {
		t.Fatal(e)
	}
	if e := d.Record(SyncEvent("private-password"), false, 0, nil, ""); e == nil {
		t.Fatal("arbitrary event accepted")
	}
	doc, e := d.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	if doc.Summary.LastAttempt == nil || doc.Summary.LastSuccess == nil || doc.Summary.Outcome != SyncFailed {
		t.Fatal("failure discarded successful history")
	}
	export, e := d.Export()
	if e != nil {
		t.Fatal(e)
	}
	persisted, e := os.ReadFile(d.path())
	if e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{"private-password", "example.invalid", "private-path", "user:"} {
		if bytes.Contains(export, []byte(secret)) || bytes.Contains(persisted, []byte(secret)) {
			t.Fatal("diagnostics disclosed private data")
		}
	}
	if bytes.Contains(export, []byte(fingerprint)) || bytes.Contains(export, []byte("localFingerprint")) {
		t.Fatal("export disclosed local scope metadata")
	}
	reopened := SyncDiagnostics{Directory: d.Directory, Target: d.Target, Backend: d.Backend}
	restored, e := reopened.Snapshot()
	if e != nil || restored.Summary.LastSuccess == nil {
		t.Fatal("last success lost on reopen", e)
	}
	info, e := os.Stat(d.path())
	if e != nil || !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("diagnostics permissions", e)
	}
}
func TestSyncDiagnosticsBoundedRetentionAndInvalidDocument(t *testing.T) {
	d := SyncDiagnostics{Directory: t.TempDir(), Target: "fixture", Backend: "directory"}
	doc := SyncDiagnosticDocument{Version: 1}
	for i := 0; i < 2100; i++ {
		date := time.Now()
		if i < 50 {
			date = date.Add(-8 * 24 * time.Hour)
		}
		doc.Entries = append(doc.Entries, SyncDiagnosticEntry{Date: date, Backend: "directory", Event: SyncFetched})
	}
	raw, _ := json.Marshal(doc)
	if e := os.MkdirAll(filepath.Dir(d.path()), 0700); e != nil {
		t.Fatal(e)
	}
	if e := AtomicWrite(d.path(), raw); e != nil {
		t.Fatal(e)
	}
	if e := d.Record(SyncCancelled, true, 0, context.Canceled, ""); e != nil {
		t.Fatal(e)
	}
	doc, e := d.Snapshot()
	if e != nil || len(doc.Entries) != 2000 {
		t.Fatal("retention limit failed", len(doc.Entries), e)
	}
	if doc.Entries[len(doc.Entries)-1].Failure.Kind != "cancelled" {
		t.Fatal("cancellation classification")
	}
	doc.Entries[0].Event = SyncEvent("private-injected-content")
	raw, _ = json.Marshal(doc)
	if e = AtomicWrite(d.path(), raw); e != nil {
		t.Fatal(e)
	}
	if exported, e := d.Export(); e == nil || bytes.Contains(exported, []byte("private-injected-content")) {
		t.Fatal("unvalidated log exported")
	}
}
func TestSharedSyncRecordsRealDiagnosticStages(t *testing.T) {
	engine, cfg, master := syncEngineFixture(t, t.TempDir())
	engine.Manual = true
	plan, e := engine.Prepare(context.Background(), cfg, master, true)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = plan.Commit(context.Background(), nil, master); e != nil {
		t.Fatal(e)
	}
	doc, e := engine.Diagnostics().Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	for _, wanted := range []SyncEvent{SyncStarted, SyncFetched, SyncUploading, SyncApplying, SyncCompleted} {
		found := false
		for _, entry := range doc.Entries {
			if entry.Event == wanted && entry.Manual {
				found = true
			}
		}
		if !found {
			t.Fatal("missing real stage", wanted)
		}
	}
	if doc.Summary.LastSuccess == nil || doc.Summary.LocalFingerprint == "" {
		t.Fatal("missing success summary")
	}
}

func TestSharedSyncDiagnosticRunsAreIndependentAndCorrelated(t *testing.T) {
	engine, cfg, master := syncEngineFixture(t, t.TempDir())
	for i := 0; i < 2; i++ {
		plan, err := engine.Prepare(context.Background(), cfg, master, true)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err = plan.Commit(context.Background(), nil, master)
		if err != nil {
			t.Fatal(err)
		}
	}
	doc, err := engine.Diagnostics().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	runs := map[string][]SyncEvent{}
	for _, entry := range doc.Entries {
		if entry.RunID == "" || !validDiagnosticRun(entry.RunID) || entry.RunID == cfg.Profiles[0].ID {
			t.Fatal("invalid or reused session identity in run ID")
		}
		runs[entry.RunID] = append(runs[entry.RunID], entry.Event)
	}
	if len(runs) != 2 {
		t.Fatal("sync attempts not separately correlated", len(runs))
	}
	for _, events := range runs {
		if events[0] != SyncStarted || events[len(events)-1] != SyncCompleted && events[len(events)-1] != SyncUnchanged {
			t.Fatal("prepare and commit split across run IDs", events)
		}
	}
	if err = engine.Diagnostics().record(SyncStarted, true, 0, nil, "", "private-host.invalid"); err == nil {
		t.Fatal("arbitrary run data accepted")
	}
}

func TestSyncDiagnosticsWriteFailureRetainsExportableHistoryAndRecovers(t *testing.T) {
	d := SyncDiagnostics{Directory: t.TempDir(), Target: "private-target.invalid", Backend: "directory"}
	t.Cleanup(func() {
		syncDiagnosticsMu.Lock()
		defer syncDiagnosticsMu.Unlock()
		delete(syncDiagnosticUnsaved, d.path())
		delete(syncDiagnosticClean, d.path())
	})
	if err := d.Record(SyncCompleted, true, 0, nil, d.scope()); err != nil {
		t.Fatal(err)
	}
	before, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(d.path())
	backup := directory + "-saved"
	if err = os.Rename(directory, backup); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(directory, []byte("temporary write obstruction"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = d.Record(SyncStarted, true, 0, nil, ""); err == nil {
		t.Fatal("write failure not reported")
	}
	if err = d.Record(SyncFailed, true, time.Second, &os.PathError{Op: "open", Path: "private-path", Err: errors.New("private-password")}, ""); err == nil {
		t.Fatal("write failure not reported")
	}
	memory, err := d.Snapshot()
	if err != nil || len(memory.Entries) != 3 || d.WriteIssue() == "" {
		t.Fatal("unsaved history inaccessible", err)
	}
	if !memory.Summary.LastSuccess.Equal(*before.Summary.LastSuccess) {
		t.Fatal("last success lost during disk failure")
	}
	memory.Entries[0].Event = SyncEvent("private-injected")
	memory.Entries[2].Failure.Kind = "private-injected"
	*memory.Summary.LastSuccess = time.Time{}
	exported, err := d.Export()
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-injected", "private-path", "private-password", "private-target.invalid"} {
		if bytes.Contains(exported, []byte(secret)) {
			t.Fatal("memory snapshot leaked/mutated diagnostics")
		}
	}
	if err = os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(backup, directory); err != nil {
		t.Fatal(err)
	}
	if err = d.Record(SyncCompleted, true, time.Second, nil, d.scope()); err != nil {
		t.Fatal(err)
	}
	if d.WriteIssue() != "" {
		t.Fatal("write issue not cleared after recovery")
	}
	persisted, err := d.read()
	if err != nil || len(persisted.Entries) != 4 {
		t.Fatal("recovery failed to flush retained events", err)
	}
}
func TestSharedSyncCompletesWhenDiagnosticWritesFail(t *testing.T) {
	engine, cfg, master := syncEngineFixture(t, t.TempDir())
	d := engine.Diagnostics()
	t.Cleanup(func() {
		syncDiagnosticsMu.Lock()
		defer syncDiagnosticsMu.Unlock()
		delete(syncDiagnosticUnsaved, d.path())
		delete(syncDiagnosticClean, d.path())
	})
	if err := os.WriteFile(filepath.Join(engine.Store.Dir, "logs"), []byte("blocked log directory"), 0600); err != nil {
		t.Fatal(err)
	}
	plan, err := engine.Prepare(context.Background(), cfg, master, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = plan.Commit(context.Background(), nil, master); err != nil {
		t.Fatal("logging failure blocked sync", err)
	}
	doc, err := d.Snapshot()
	if err != nil || doc.Summary.LastSuccess == nil || doc.Entries[len(doc.Entries)-1].Event != SyncCompleted {
		t.Fatal("completed sync missing from memory history", err)
	}
	if _, err = d.Export(); err != nil {
		t.Fatal("unsaved history not exportable", err)
	}
}

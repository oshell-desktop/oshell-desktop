package desktop

import (
	"bytes"
	"context"
	"github.com/egoist/mygo/ui"
	"os"
	"oshell-desktop/internal/core"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSFTPTransferContentNoOverwriteAndCancelRecovery(t *testing.T) {
	a, p, _ := savedConnectionFixture(t)
	p.Kind = "sftp"
	p.InitialDirectory = "/"
	a.config.Profiles[0] = p
	if err := a.vault.PutFor(p, "saved-fixture-password", p.Identity()); err != nil {
		t.Fatal(err)
	}
	a.request(p, 0)
	waitOperator(t, a, func() bool {
		return len(a.tabs) == 1 && a.tabs[0].active.files != nil && !a.tabs[0].active.files.busy && a.tabs[0].active.files.client != nil
	})
	pane := a.tabs[0].active
	dir := t.TempDir()
	source := filepath.Join(dir, "payload.bin")
	payload := bytes.Repeat([]byte("fixture-payload-\x00\xff"), 65536)
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	a.uploadDropped(pane, []string{source})
	if a.features.uploadDraft == nil {
		t.Fatal("upload confirmation missing")
	}
	if err := ui.NewTester(a.fileWindowView, 1080, 720).Click("确认上传"); err != nil {
		t.Fatal(err)
	}
	waitOperator(t, a, func() bool {
		if pane.files.busy {
			return false
		}
		for _, entry := range pane.files.entries {
			if entry.Name() == "payload.bin" {
				return true
			}
		}
		return false
	})
	// Answer the native conflict dialog with "keep both" for this upload.
	if err := os.WriteFile(source, []byte("do-not-overwrite"), 0600); err != nil {
		t.Fatal(err)
	}
	a.withTransfer(pane, func(ctx context.Context, client core.RemoteFileSystem) error {
		rec := client.(*recordedFileSystem)
		rec.chooseConflict = func(target string) (bool, error) {
			if target != "/payload.bin" {
				t.Errorf("unexpected conflict %q", target)
			}
			return false, nil
		}
		return uploadUnique(ctx, client, []string{source}, "/", a.transferProgress(pane, ctx))
	})
	waitOperator(t, a, func() bool {
		if pane.files.busy {
			return false
		}
		for _, entry := range pane.files.entries {
			if entry.Name() == "payload.bin.1" {
				return true
			}
		}
		return false
	})
	numbered := t.TempDir()
	for _, name := range []string{"payload.bin", "payload.bin.1"} {
		if err := os.WriteFile(filepath.Join(numbered, name), []byte("keep-existing"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a.withTransfer(pane, func(ctx context.Context, client core.RemoteFileSystem) error {
		return downloadUnique(ctx, client, "/payload.bin", numbered, a.transferProgress(pane, ctx))
	})
	waitOperator(t, a, func() bool { return !pane.files.busy })
	numberedData, err := os.ReadFile(filepath.Join(numbered, "payload.bin.2"))
	if err != nil || !bytes.Equal(numberedData, payload) {
		t.Fatal("numbered download did not preserve payload", err)
	}
	for _, name := range []string{"payload.bin", "payload.bin.1"} {
		data, err := os.ReadFile(filepath.Join(numbered, name))
		if err != nil || string(data) != "keep-existing" {
			t.Fatal("numbered download overwrote existing file", err)
		}
	}
	// Simulate another process creating the chosen destination after preflight.
	raceTarget := filepath.Join(dir, "created-during-download.bin")
	var createOnce sync.Once
	a.withTransfer(pane, func(ctx context.Context, client core.RemoteFileSystem) error {
		return client.Download(ctx, "/payload.bin", raceTarget, false, func(core.TransferProgress) {
			createOnce.Do(func() {
				if err := os.WriteFile(raceTarget, []byte("concurrent-file"), 0600); err != nil {
					t.Error(err)
				}
			})
		})
	})
	waitOperator(t, a, func() bool { return !pane.files.busy })
	raceData, err := os.ReadFile(raceTarget)
	if err != nil || string(raceData) != "concurrent-file" || !strings.HasPrefix(pane.files.status, "操作失败") {
		t.Fatal("late-created destination overwritten", err)
	}
	target := filepath.Join(dir, "download.bin")
	original := []byte("original-local-file")
	if err := os.WriteFile(target, original, 0600); err != nil {
		t.Fatal(err)
	}
	progressSeen := make(chan struct{})
	var once sync.Once
	a.withTransfer(pane, func(ctx context.Context, client core.RemoteFileSystem) error {
		report := a.transferProgress(pane, ctx)
		return client.Download(ctx, "/payload.bin", target, true, func(progress core.TransferProgress) {
			report(progress)
			once.Do(func() { close(progressSeen) })
			<-ctx.Done()
		})
	})
	waitOperator(t, a, func() bool {
		select {
		case <-progressSeen:
			return true
		default:
			return false
		}
	})
	tt := ui.NewTester(a.fileWindowView, 1080, 720)
	if err := tt.Click("取消传输"); err != nil {
		t.Fatal(err)
	}
	waitOperator(t, a, func() bool { return !pane.files.busy && pane.files.status == "传输已取消" })
	got, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatal("cancel replaced existing local file", err)
	}
	temporary, err := filepath.Glob(filepath.Join(dir, ".oshell-download-*"))
	if err != nil || len(temporary) != 0 {
		t.Fatal("cancel left staged download", err)
	}
	if !pane.conn.Alive() {
		t.Fatal("cancel closed the shared SSH transport")
	}
	a.withTransfer(pane, func(ctx context.Context, client core.RemoteFileSystem) error {
		return client.Download(ctx, "/payload.bin", target, true, a.transferProgress(pane, ctx))
	})
	waitOperator(t, a, func() bool { return !pane.files.busy })
	got, err = os.ReadFile(target)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatal("recovered download lost original remote data", err, "status", pane.files.status, "bytes", len(got), "want", len(payload))
	}
}

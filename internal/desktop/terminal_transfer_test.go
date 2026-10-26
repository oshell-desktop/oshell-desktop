package desktop

import (
	"errors"
	"os"
	"oshell-desktop/internal/terminal"
	"oshell-desktop/internal/zmodem"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerminalSkippedBatchNotice(t *testing.T) {
	a := &App{}
	p := &pane{transfer: transferNoticeState{active: true, visible: true}}
	p.owner.Store(a)
	var opts terminal.Options
	a.configureTerminalTransfer(p, &opts, true)
	opts.TransferFinished(&zmodem.BatchResultError{Skipped: []string{"kept.bin"}})
	a.drain()
	if p.transfer.active || !strings.Contains(p.transfer.status, "kept.bin") || strings.Contains(p.transfer.status, "传输完成") || !p.transfer.dismissAt.IsZero() {
		t.Fatalf("skipped file presented as success: %+v", p.transfer)
	}
}

func TestTerminalDownloadCommitAbortAndCollision(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"../escape", "..", "a/b", "a\\b", "disk:stream", ""} {
		if f, e := newTransferFile(dir, name); e == nil {
			f.Abort(errors.New("test"))
			f.Close()
			t.Fatalf("accepted %q", name)
		}
	}
	f, e := newTransferFile(dir, "done.bin")
	if e != nil {
		t.Fatal(e)
	}
	f.Write([]byte("complete"))
	if _, e = os.Stat(filepath.Join(dir, "done.bin")); !os.IsNotExist(e) {
		t.Fatal("partial file exposed")
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	f, e = newTransferFile(dir, "cancel.bin")
	if e != nil {
		t.Fatal(e)
	}
	f.Write([]byte("partial"))
	f.Abort(errors.New("cancelled"))
	if e = f.Close(); e == nil {
		t.Fatal("abort succeeded")
	}
	f, e = newTransferFile(dir, "race.bin")
	if e != nil {
		t.Fatal(e)
	}
	f.Write([]byte("new"))
	os.WriteFile(filepath.Join(dir, "race.bin"), []byte("existing"), 0600)
	if e = f.Close(); e == nil {
		t.Fatal("collision overwritten")
	}
	got, _ := os.ReadFile(filepath.Join(dir, "race.bin"))
	if string(got) != "existing" {
		t.Fatal("lost existing file")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatalf("temporary files remain: %v", entries)
	}
}

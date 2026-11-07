package desktop

import (
	"errors"
	"fmt"
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
	for _, name := range []string{"..", "a/..", "a\\", "disk:stream", ""} {
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
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "race.bin"))
	if string(got) != "existing" {
		t.Fatal("lost existing file")
	}
	got, _ = os.ReadFile(filepath.Join(dir, "race.bin.0"))
	if string(got) != "new" {
		t.Fatal("concurrent collision was not renamed")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Fatalf("temporary files remain: %v", entries)
	}
}

func TestTerminalDownloadJunkPathAndRename(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "item.bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "item.bin.0"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"../../item.bin", "/remote/item.bin", `C:\remote\item.bin`} {
		f, err := newTransferFile(dir, name)
		if err != nil {
			t.Fatal(err)
		}
		actual := ""
		f.onPublished = func(name string) { actual = name }
		if _, err = f.Write([]byte("content")); err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf("item.bin.%d", i+1)
		if actual != want || filepath.Dir(f.target) != dir {
			t.Fatalf("bad local target %q (%q)", f.target, actual)
		}
		data, err := os.ReadFile(filepath.Join(dir, want))
		if err != nil || string(data) != "content" {
			t.Fatalf("renamed file: %v", err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dir, "item.bin.0"))
	if string(data) != "keep" {
		t.Fatal("existing file overwritten")
	}
}

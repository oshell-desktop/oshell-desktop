package core

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func TestFontArchiveActiveOnlyMergeAndUnchangedJSON(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	library := NewFontLibrary(filepath.Join(root, "source"))
	mono := writeFontFixture(t, root, "mono.ttf", gomono.TTF)
	regular := writeFontFixture(t, root, "regular.ttf", goregular.TTF)
	draft, err := library.PrepareImport(ctx, []string{mono, regular})
	if err != nil {
		t.Fatal(err)
	}
	if err = library.Commit(draft); err != nil {
		t.Fatal(err)
	}
	draft, _ = library.Load()
	retired := draft.Fonts[1].Asset.Name()
	draft.Fonts = draft.Fonts[:1]
	if err = library.Commit(draft); err != nil {
		t.Fatal(err)
	}
	writeFontFixture(t, filepath.Dir(library.Directory), "credential-key", []byte("must-not-export"))
	path := filepath.Join(root, "fixture.json")
	data := []byte(`{"fixture":"shared JSON unchanged"}`)
	if err = ExportSessionArchive(ctx, path, data, library, true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != string(data) {
		t.Fatal("archive JSON changed")
	}
	if _, err = os.Stat(filepath.Join(path+".assets", "fonts", retired)); !os.IsNotExist(err) {
		t.Fatal("retired font exported")
	}
	entries, _ := os.ReadDir(path + ".assets")
	if len(entries) != 1 || entries[0].Name() != "fonts" {
		t.Fatal("non-font data exported", entries)
	}
	destination := NewFontLibrary(filepath.Join(root, "dest"))
	existing, err := destination.PrepareImport(ctx, []string{regular})
	if err != nil {
		t.Fatal(err)
	}
	if err = destination.Commit(existing); err != nil {
		t.Fatal(err)
	}
	restored, err := destination.PrepareArchiveRestore(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = destination.Commit(restored); err != nil {
		t.Fatal(err)
	}
	again, err := destination.PrepareArchiveRestore(ctx, path)
	if err != nil || !reflect.DeepEqual(restored.Fonts, again.Fonts) {
		t.Fatal("restore not idempotent", err)
	}
	if err = ExportSessionArchive(ctx, path, data, library, false); err == nil {
		t.Fatal("stale same-name attachments accepted")
	}
	manifest, _ := NewFontLibrary(path + ".assets").Manifest()
	bad := filepath.Join(path+".assets", "fonts", manifest.Fonts[0].Name())
	os.WriteFile(bad, []byte("bad"), 0600)
	before, _ := destination.Manifest()
	if _, err = destination.PrepareArchiveRestore(ctx, path); err == nil {
		t.Fatal("corrupt font accepted")
	}
	after, _ := destination.Manifest()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("corrupt restore mutated library")
	}
}
func TestFontArchiveCancellationAndDefaultNoAttachments(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "fixture.json")
	library := NewFontLibrary(root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ExportSessionArchive(ctx, path, []byte("{}"), library, true); err == nil {
		t.Fatal("cancel ignored")
	}
	files, _ := os.ReadDir(root)
	if len(files) != 0 {
		t.Fatal("cancelled export retained temporary outputs", files)
	}
	if err := ExportSessionArchive(context.Background(), path, []byte("{}"), library, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".assets"); !os.IsNotExist(err) {
		t.Fatal("default export included fonts")
	}
}

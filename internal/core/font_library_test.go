package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func writeFontFixture(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFontLibraryImportRemoveReload(t *testing.T) {
	l := NewFontLibrary(t.TempDir())
	source := t.TempDir()
	path := writeFontFixture(t, source, "mono.TTF", gomono.TTF)
	before, err := l.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	d, err := l.PrepareImport(context.Background(), []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Fonts) != 1 || d.Fonts[0].Faces[0].Name != "GoMono" || !d.Fonts[0].Faces[0].Monospaced {
		t.Fatalf("faces %+v", d.Fonts)
	}
	if got, _ := l.Manifest(); !reflect.DeepEqual(before, got) {
		t.Fatal("prepare wrote manifest")
	}
	if err = l.Commit(d); err != nil {
		t.Fatal(err)
	}
	loaded, err := l.Load()
	if err != nil || len(loaded.Fonts) != 1 {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(l.Directory, "library.json")
	info, _ := os.Stat(manifestPath)
	duplicate, err := l.PrepareImport(context.Background(), []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err = l.Commit(duplicate); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(manifestPath)
	if !after.ModTime().Equal(info.ModTime()) {
		t.Fatal("duplicate rewrote manifest")
	}
	assetPath := filepath.Join(l.Directory, loaded.Fonts[0].Asset.Name())
	loaded.Fonts = nil
	if err = l.Commit(loaded); err != nil {
		t.Fatal(err)
	}
	removed, err := l.Load()
	if err != nil || len(removed.Fonts) != 0 {
		t.Fatal("removal did not persist", err)
	}
	if data, err := os.ReadFile(assetPath); err != nil || !bytes.Equal(data, gomono.TTF) {
		t.Fatal("removed history was deleted", err)
	}
	backups, _ := os.ReadDir(filepath.Join(l.Directory, "backups"))
	if len(backups) < 2 {
		t.Fatal("manifest backups missing")
	}
}

func TestFontLibraryDirectoryAndInvalidBatch(t *testing.T) {
	l := NewFontLibrary(t.TempDir())
	source := t.TempDir()
	writeFontFixture(t, source, "sub/a.ttf", gomono.TTF)
	writeFontFixture(t, source, "sub/b.ttf", goregular.TTF)
	for _, name := range []string{".hidden.ttf", ".hidden/a.ttf", "ignored.app/a.ttf", "ignored.bundle/a.ttf", "notes.txt"} {
		writeFontFixture(t, source, name, []byte("not a font"))
	}
	d, err := l.PrepareImport(context.Background(), []string{source, filepath.Join(source, "sub")})
	if err != nil || len(d.Fonts) != 2 {
		t.Fatal(err, len(d.Fonts))
	}
	if err = l.Commit(d); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(l.Directory, "library.json"))
	bad := writeFontFixture(t, source, "invalid.otf", []byte("not a font at all"))
	if _, err = l.PrepareImport(context.Background(), []string{bad}); err == nil {
		t.Fatal("accepted malformed font")
	}
	after, _ := os.ReadFile(filepath.Join(l.Directory, "library.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("invalid import changed active library")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = l.PrepareImport(ctx, []string{source}); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestFontLibraryLimitsAndIntegrity(t *testing.T) {
	t.Run("file size", func(t *testing.T) {
		l := NewFontLibrary(t.TempDir())
		p := writeFontFixture(t, t.TempDir(), "big.ttf", nil)
		f, _ := os.OpenFile(p, os.O_WRONLY, 0600)
		if err := f.Truncate(MaxFontFile + 1); err != nil {
			t.Fatal(err)
		}
		f.Close()
		if _, err := l.PrepareImport(context.Background(), []string{p}); err == nil {
			t.Fatal("accepted >20 MiB")
		}
	})
	t.Run("file count", func(t *testing.T) {
		l := NewFontLibrary(t.TempDir())
		dir := t.TempDir()
		for i := 0; i < MaxFontFiles; i++ {
			writeFontFixture(t, dir, fmt.Sprintf("%d.ttf", i), append(bytes.Clone(gomono.TTF), byte(i)))
		}
		d, err := l.PrepareImport(context.Background(), []string{dir})
		if err != nil {
			t.Fatal(err)
		}
		if err = l.Commit(d); err != nil {
			t.Fatal(err)
		}
		p := writeFontFixture(t, t.TempDir(), "extra.ttf", goregular.TTF)
		if _, err = l.PrepareImport(context.Background(), []string{p}); err == nil {
			t.Fatal("accepted 65th active font")
		}
	})
	t.Run("total size", func(t *testing.T) {
		l := NewFontLibrary(t.TempDir())
		dir := t.TempDir()
		for i := 0; i < 4; i++ {
			data := make([]byte, 17<<20)
			copy(data, gomono.TTF)
			data[len(data)-1] = byte(i)
			writeFontFixture(t, dir, fmt.Sprintf("%d.ttf", i), data)
		}
		if _, err := l.PrepareImport(context.Background(), []string{dir}); err == nil {
			t.Fatal("accepted >64 MiB")
		}
		if _, err := os.Stat(l.Directory); !os.IsNotExist(err) {
			t.Fatal("failed batch wrote library")
		}
	})
	t.Run("scan", func(t *testing.T) {
		dir := t.TempDir()
		for i := 0; i < MaxFontScan; i++ {
			writeFontFixture(t, dir, fmt.Sprintf("%d.txt", i), nil)
		}
		if _, err := SelectedFontFiles(context.Background(), []string{dir}); err == nil {
			t.Fatal("accepted >10000 scanned entries")
		}
	})
	t.Run("corrupt blob", func(t *testing.T) {
		l := NewFontLibrary(t.TempDir())
		p := writeFontFixture(t, t.TempDir(), "a.ttf", gomono.TTF)
		d, err := l.PrepareImport(context.Background(), []string{p})
		if err != nil {
			t.Fatal(err)
		}
		if err = l.Commit(d); err != nil {
			t.Fatal(err)
		}
		writeFontFixture(t, l.Directory, d.Fonts[0].Asset.Name(), goregular.TTF)
		if _, err = l.Load(); err == nil {
			t.Fatal("accepted wrong digest")
		}
	})
	t.Run("unknown manifest", func(t *testing.T) {
		l := NewFontLibrary(t.TempDir())
		for _, data := range []string{`{}`, `{"format":"OShell.font-library","version":2,"fonts":[]}`, `{"format":"OShell.font-library","version":1,"fonts":[],"extra":true}`} {
			writeFontFixture(t, l.Directory, "library.json", []byte(data))
			if _, err := l.Load(); err == nil {
				t.Fatal("accepted", data)
			}
		}
	})
}

// A standard TTC wraps real SFNT fixtures, with absolute table offsets.
func collectionFixture(fonts ...[]byte) []byte {
	data := make([]byte, 12+4*len(fonts))
	copy(data, "ttcf")
	binary.BigEndian.PutUint32(data[4:], 0x00010000)
	binary.BigEndian.PutUint32(data[8:], uint32(len(fonts)))
	for i, font := range fonts {
		for len(data)%4 != 0 {
			data = append(data, 0)
		}
		offset := len(data)
		binary.BigEndian.PutUint32(data[12+i*4:], uint32(offset))
		data = append(data, font...)
		n := int(binary.BigEndian.Uint16(font[4:6]))
		for j := 0; j < n; j++ {
			p := offset + 12 + 16*j + 8
			binary.BigEndian.PutUint32(data[p:], binary.BigEndian.Uint32(data[p:])+uint32(offset))
		}
	}
	return data
}

func TestFontLibraryCollectionsAndManifestConflict(t *testing.T) {
	data := collectionFixture(gomono.TTF, goregular.TTF)
	faces, err := FontFaces(data)
	if err != nil || len(faces) != 2 {
		t.Fatal(err, faces)
	}
	for _, ext := range []string{"ttc", "otc"} {
		l := NewFontLibrary(t.TempDir())
		p := writeFontFixture(t, t.TempDir(), "collection."+ext, data)
		d, err := l.PrepareImport(context.Background(), []string{p})
		if err != nil {
			t.Fatal(err)
		}
		if err = l.Commit(d); err != nil {
			t.Fatal(err)
		}
		if err = l.Commit(d); err == nil {
			t.Fatal("stale manifest overwritten")
		}
	}
	binary.BigEndian.PutUint32(data[8:], 129)
	if _, err := FontFaces(data); err == nil {
		t.Fatal("accepted oversized collection")
	}
	l := NewFontLibrary(t.TempDir())
	m := FontManifest{Format: "OShell.font-library", Version: 1, Fonts: []FontAsset{}}
	b, _ := json.Marshal(m)
	writeFontFixture(t, l.Directory, "library.json", b)
	if _, err = l.Load(); err != nil {
		t.Fatal(err)
	}
}

func TestFontLibraryOpenTypeAndStorageMigration(t *testing.T) {
	data, err := os.ReadFile("testdata/fonts/CFFTest.otf")
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		ext  string
		data []byte
	}{{"otf", data}, {"otc", collectionFixture(data)}} {
		l := NewFontLibrary(t.TempDir())
		p := writeFontFixture(t, t.TempDir(), "cff."+fixture.ext, fixture.data)
		d, err := l.PrepareImport(context.Background(), []string{p})
		if err != nil {
			t.Fatal(fixture.ext, err)
		}
		if err = l.Commit(d); err != nil {
			t.Fatal(err)
		}
		if _, err = l.Load(); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("OSHELL_DATA_DIR", filepath.Join(t.TempDir(), "source"))
	store, err := OpenStore()
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Save(DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	l := NewFontLibrary(store.Dir)
	path := writeFontFixture(t, t.TempDir(), "font.ttf", gomono.TTF)
	draft, err := l.PrepareImport(context.Background(), []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err = l.Commit(draft); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "moved")
	if err = store.ScheduleStorageDirectory(target); err != nil {
		t.Fatal(err)
	}
	moved, err := OpenStore()
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewFontLibrary(moved.Dir).Load()
	if err != nil || len(reloaded.Fonts) != 1 || !bytes.Equal(reloaded.Fonts[0].Data, gomono.TTF) {
		t.Fatal("migration lost fonts", err)
	}
}

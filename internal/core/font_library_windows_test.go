package core

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
)

func TestFontLibraryWindowsHiddenAndLinks(t *testing.T) {
	dir := t.TempDir()
	regular := writeFontFixture(t, dir, "regular.ttf", gomono.TTF)
	hidden := writeFontFixture(t, dir, "hidden.ttf", gomono.TTF)
	path, err := syscall.UTF16PtrFromString(hidden)
	if err != nil {
		t.Fatal(err)
	}
	if err = syscall.SetFileAttributes(path, syscall.FILE_ATTRIBUTE_HIDDEN); err != nil {
		t.Fatal(err)
	}
	files, err := SelectedFontFiles(context.Background(), []string{dir})
	if err != nil || len(files) != 1 || files[0] != regular {
		t.Fatal("hidden file scanned", files, err)
	}
	t.Run("symbolic link", func(t *testing.T) {
		link := filepath.Join(dir, "linked.ttf")
		if err := os.Symlink(regular, link); err != nil {
			t.Skipf("Windows symbolic-link creation unavailable: %v", err)
		}
		files, err := SelectedFontFiles(context.Background(), []string{dir})
		if err != nil || len(files) != 1 {
			t.Fatal("followed symbolic link", files, err)
		}
		if _, err = SelectedFontFiles(context.Background(), []string{link}); err == nil {
			t.Fatal("accepted explicit symbolic link")
		}
	})
}

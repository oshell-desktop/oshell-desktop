package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Font attachments stay beside the unmodified sessions JSON. Publish them only
// after validating every active object; a failed JSON write removes our new set.
func ExportSessionArchive(ctx context.Context, path string, data []byte, library FontLibrary, includeFonts bool) error {
	attachments := path + ".assets"
	if _, err := os.Lstat(attachments); err == nil {
		return fmt.Errorf("同名字体附件已存在，请选择新的备份文件名")
	} else if !os.IsNotExist(err) {
		return err
	}
	created := false
	if includeFonts {
		draft, err := library.Load()
		if err != nil {
			return err
		}
		stage, err := os.MkdirTemp(filepath.Dir(path), ".oshell-font-backup-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stage) // Only this call's freshly created directory.
		fontDir := filepath.Join(stage, "fonts")
		if err = os.Mkdir(fontDir, 0700); err != nil {
			return err
		}
		for _, font := range draft.Fonts {
			if err = ctx.Err(); err != nil {
				return err
			}
			if err = AtomicWrite(filepath.Join(fontDir, font.Asset.Name()), font.Data); err != nil {
				return err
			}
		}
		manifest, err := json.Marshal(draft.Before)
		if err != nil {
			return err
		}
		if err = AtomicWrite(filepath.Join(fontDir, "library.json"), manifest); err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = os.Rename(stage, attachments); err != nil {
			return err
		}
		created = true
	}
	err := ctx.Err()
	if err == nil {
		err = AtomicWrite(path, data)
	}
	if err != nil && created {
		_ = os.RemoveAll(attachments)
	}
	return err
}
func (library FontLibrary) PrepareArchiveRestore(ctx context.Context, path string) (FontLibraryDraft, error) {
	draft, err := library.Load()
	if err != nil {
		return draft, err
	}
	attachments := path + ".assets"
	info, err := os.Lstat(attachments)
	if err != nil {
		return draft, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return draft, fmt.Errorf("附件备份须为普通本机目录")
	}
	incoming := NewFontLibrary(attachments)
	if _, err = readFontFile(filepath.Join(incoming.Directory, "library.json"), 65536); err != nil {
		return draft, err
	}
	other, err := incoming.Load()
	if err != nil {
		return draft, err
	}
	seen := map[string]bool{}
	for _, f := range draft.Fonts {
		seen[f.Asset.Name()] = true
	}
	for _, f := range other.Fonts {
		if err = ctx.Err(); err != nil {
			return draft, err
		}
		if !seen[f.Asset.Name()] {
			draft.Fonts = append(draft.Fonts, f)
			seen[f.Asset.Name()] = true
		}
	}
	manifest := FontManifest{Format: "OShell.font-library", Version: 1, Fonts: []FontAsset{}}
	for _, f := range draft.Fonts {
		manifest.Fonts = append(manifest.Fonts, f.Asset)
	}
	if err = manifest.validate(); err != nil {
		return draft, err
	}
	sort.Slice(draft.Fonts, func(i, j int) bool { return draft.Fonts[i].Asset.Name() < draft.Fonts[j].Asset.Name() })
	return draft, nil
}

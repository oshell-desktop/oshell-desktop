package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
)

type FontSyncTransport interface {
	FetchFont(context.Context, string) (*SharedObject, error)
	PutFont(context.Context, string, []byte, string) error
}

func MergeFontManifests(base, local, remote FontManifest) (FontManifest, error) {
	merged := FontManifest{Format: "OShell.font-library", Version: 1, Fonts: []FontAsset{}}
	for _, m := range []FontManifest{base, local, remote} {
		if err := m.validate(); err != nil {
			return merged, err
		}
	}
	old, own, other := map[FontAsset]bool{}, map[FontAsset]bool{}, map[FontAsset]bool{}
	for _, a := range base.Fonts {
		old[a] = true
	}
	for _, a := range local.Fonts {
		own[a] = true
	}
	for _, a := range remote.Fonts {
		other[a] = true
	}
	all := map[FontAsset]bool{}
	for a := range own {
		all[a] = true
	}
	for a := range other {
		all[a] = true
	}
	for a := range all {
		if !old[a] || own[a] && other[a] {
			merged.Fonts = append(merged.Fonts, a)
		}
	}
	sort.Slice(merged.Fonts, func(i, j int) bool { return merged.Fonts[i].Name() < merged.Fonts[j].Name() })
	return merged, merged.validate()
}

func FontBaselinePath(directory, target string) string {
	sum := sha256.Sum256([]byte(target))
	return filepath.Join(directory, "font-baselines", hex.EncodeToString(sum[:])+".json")
}

func verifyFontAsset(asset FontAsset, data []byte) ([]FontEntry, error) {
	sum := sha256.Sum256(data)
	if len(data) != asset.Size || hex.EncodeToString(sum[:]) != asset.SHA256 {
		return nil, fmt.Errorf("字体附件摘要或大小不匹配，未应用清单")
	}
	return FontFaces(data)
}

// SynchronizeFonts follows OShell's immutable-object / conditional-manifest
// protocol. Configuration encryption and its replica are deliberately separate.
func SynchronizeFonts(ctx context.Context, local FontLibrary, transport FontSyncTransport, baseline string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	draft, err := local.Load()
	if err != nil {
		return err
	}
	own := draft.Before
	object, err := transport.FetchFont(ctx, "library.json")
	if err != nil {
		return err
	}
	remote := FontManifest{Format: "OShell.font-library", Version: 1, Fonts: []FontAsset{}}
	etag := ""
	if object != nil {
		remote, err = DecodeFontManifest(object.Data)
		if err != nil {
			return err
		}
		etag = object.ETag
	}
	base := FontManifest{Format: "OShell.font-library", Version: 1, Fonts: []FontAsset{}}
	data, err := readFontFile(baseline, 65536)
	if err == nil {
		base, err = DecodeFontManifest(data)
	} else if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	if object == nil && len(base.Fonts) > 0 {
		return fmt.Errorf("远端字体清单已消失，保留本机字体；请检查同步位置")
	}
	merged, err := MergeFontManifests(base, own, remote)
	if err != nil {
		return err
	}
	available := map[FontAsset]LibraryFont{}
	for _, f := range draft.Fonts {
		available[f.Asset] = f
	}
	remoteAssets := map[FontAsset]bool{}
	for _, a := range remote.Fonts {
		remoteAssets[a] = true
	}
	draft.Fonts = nil
	for _, a := range merged.Fonts {
		if err := ctx.Err(); err != nil {
			return err
		}
		f, exists := available[a]
		if !exists {
			blob, err := transport.FetchFont(ctx, a.Name())
			if err != nil {
				return err
			}
			if blob == nil {
				return fmt.Errorf("远端字体文件缺失，未应用清单")
			}
			faces, err := verifyFontAsset(a, blob.Data)
			if err != nil {
				return err
			}
			f = LibraryFont{a, faces, blob.Data}
		}
		draft.Fonts = append(draft.Fonts, f)
	}
	for _, f := range draft.Fonts {
		if !remoteAssets[f.Asset] {
			blob, err := transport.FetchFont(ctx, f.Asset.Name())
			if err != nil {
				return err
			}
			if blob != nil {
				if _, err = verifyFontAsset(f.Asset, blob.Data); err != nil {
					return err
				}
			} else if err = transport.PutFont(ctx, f.Asset.Name(), f.Data, ""); err != nil {
				return err
			}
		}
	}
	current, err := local.Manifest()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, own) {
		return fmt.Errorf("本机字体库已变化，未提交远端清单；请重新同步")
	}
	saveBaseline := func(m FontManifest) error {
		if reflect.DeepEqual(base, m) {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(baseline), 0700); err != nil {
			return err
		}
		data, _ := json.Marshal(m)
		if err := AtomicWrite(baseline, data); err != nil {
			return err
		}
		base = m
		return nil
	}
	if !reflect.DeepEqual(merged, remote) || object == nil {
		data, _ := json.Marshal(merged)
		if err = transport.PutFont(ctx, "library.json", data, etag); err != nil {
			return err
		}
		// The uploaded snapshot remains the merge base if a later local edit
		// prevents apply; remote-only additions must survive the next attempt.
		if err = saveBaseline(own); err != nil {
			return err
		}
	} else {
		latest, err := transport.FetchFont(ctx, "library.json")
		if err != nil {
			return err
		}
		if latest == nil || latest.ETag != etag {
			return safeSyncFailure("conflict")
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = local.Commit(draft); err != nil {
		return err
	}
	return saveBaseline(merged)
}

func (e *SharedSyncEngine) synchronizeFonts(ctx context.Context, selection SyncContentSelection) error {
	if !selection.FontFiles {
		return nil
	}
	t, ok := e.Transport.(FontSyncTransport)
	if !ok {
		return fmt.Errorf("当前同步通道不支持字体附件")
	}
	target := e.FontTarget
	if target == "" {
		target = e.Replica.Target
	}
	if err := SynchronizeFonts(ctx, NewFontLibrary(e.Store.Dir), t, FontBaselinePath(e.Store.Dir, target)); err != nil {
		return fmt.Errorf("配置同步已完成，字体附件同步未完成：%w", err)
	}
	return nil
}

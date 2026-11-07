package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/goregular"
)

func importSyncFont(t *testing.T, library FontLibrary, data []byte) FontAsset {
	t.Helper()
	p := writeFontFixture(t, t.TempDir(), "fixture.ttf", data)
	d, err := library.PrepareImport(context.Background(), []string{p})
	if err != nil {
		t.Fatal(err)
	}
	if err = library.Commit(d); err != nil {
		t.Fatal(err)
	}
	for _, f := range d.Fonts {
		if bytes.Equal(f.Data, data) {
			return f.Asset
		}
	}
	t.Fatal("import missing")
	return FontAsset{}
}

func fontNames(t *testing.T, l FontLibrary) []string {
	t.Helper()
	d, err := l.Load()
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, f := range d.Fonts {
		names = append(names, f.Asset.Name())
	}
	return names
}

func removeSyncFont(t *testing.T, l FontLibrary, asset FontAsset) {
	t.Helper()
	d, err := l.Load()
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range d.Fonts {
		if f.Asset == asset {
			d.Fonts = append(d.Fonts[:i], d.Fonts[i+1:]...)
			break
		}
	}
	if err = l.Commit(d); err != nil {
		t.Fatal(err)
	}
}

// Fault injection wraps the real filesystem transport; merge and persistence
// always run through the production implementation.
type interruptedFontTransport struct {
	FontSyncTransport
	beforePut   func(string) error
	beforeFetch func(string) error
}

func (f interruptedFontTransport) PutFont(ctx context.Context, name string, data []byte, tag string) error {
	if f.beforePut != nil {
		if err := f.beforePut(name); err != nil {
			return err
		}
	}
	return f.FontSyncTransport.PutFont(ctx, name, data, tag)
}
func (f interruptedFontTransport) FetchFont(ctx context.Context, name string) (*SharedObject, error) {
	if f.beforeFetch != nil {
		if err := f.beforeFetch(name); err != nil {
			return nil, err
		}
	}
	return f.FontSyncTransport.FetchFont(ctx, name)
}

func TestFontSyncDirectoryMergeDeletionNoopAndTargets(t *testing.T) {
	ctx := context.Background()
	remote := DirectoryTransport{t.TempDir()}
	a, b := NewFontLibrary(t.TempDir()), NewFontLibrary(t.TempDir())
	baseA, baseB := FontBaselinePath(t.TempDir(), "A"), FontBaselinePath(t.TempDir(), "A")
	sync := func(l FontLibrary, base string) {
		t.Helper()
		if err := SynchronizeFonts(ctx, l, remote, base); err != nil {
			t.Fatal(err)
		}
	}
	mono := importSyncFont(t, a, gomono.TTF)
	sync(a, baseA)
	regular := importSyncFont(t, b, goregular.TTF)
	sync(b, baseB)
	sync(a, baseA)
	if len(fontNames(t, a)) != 2 || !reflect.DeepEqual(fontNames(t, a), fontNames(t, b)) {
		t.Fatal("independent additions lost")
	}
	before, _ := os.Stat(filepath.Join(remote.Directory, "fonts", "library.json"))
	baseBefore, _ := os.Stat(baseA)
	sync(a, baseA)
	after, _ := os.Stat(filepath.Join(remote.Directory, "fonts", "library.json"))
	baseAfter, _ := os.Stat(baseA)
	if !before.ModTime().Equal(after.ModTime()) || !baseBefore.ModTime().Equal(baseAfter.ModTime()) {
		t.Fatal("no-op rewrote state")
	}
	removeSyncFont(t, a, mono)
	sync(a, baseA)
	importSyncFont(t, b, gomonobold.TTF)
	sync(b, baseB)
	sync(a, baseA)
	if len(fontNames(t, a)) != 2 || !reflect.DeepEqual(fontNames(t, a), fontNames(t, b)) {
		t.Fatal("deletion/addition merge failed")
	}
	for _, n := range fontNames(t, a) {
		if n == mono.Name() {
			t.Fatal("deleted font resurrected")
		}
	}
	if _, err := os.Stat(filepath.Join(remote.Directory, "fonts", mono.Name())); err != nil {
		t.Fatal("historical blob removed", err)
	}
	other := DirectoryTransport{t.TempDir()}
	baseOther := FontBaselinePath(filepath.Dir(filepath.Dir(baseA)), "B")
	if err := SynchronizeFonts(ctx, a, other, baseOther); err != nil {
		t.Fatal(err)
	}
	removeSyncFont(t, b, regular)
	sync(b, baseB)
	sync(a, baseA)
	if len(fontNames(t, a)) != 1 {
		t.Fatal("switching target lost original baseline")
	}
	if err := os.Remove(filepath.Join(remote.Directory, "fonts", "library.json")); err != nil {
		t.Fatal(err)
	}
	if err := SynchronizeFonts(ctx, a, remote, baseA); err == nil || len(fontNames(t, a)) != 1 {
		t.Fatal("missing remote caused deletion", err)
	}
}

func TestFontSyncRejectsInvalidAttachmentsAndManifest(t *testing.T) {
	ctx := context.Background()
	for _, fault := range []string{"missing", "digest", "unknown", "type", "format", "limit"} {
		t.Run(fault, func(t *testing.T) {
			remote := DirectoryTransport{t.TempDir()}
			source, target := NewFontLibrary(t.TempDir()), NewFontLibrary(t.TempDir())
			asset := importSyncFont(t, source, gomono.TTF)
			if err := SynchronizeFonts(ctx, source, remote, FontBaselinePath(t.TempDir(), "r")); err != nil {
				t.Fatal(err)
			}
			importSyncFont(t, target, goregular.TTF)
			before := fontNames(t, target)
			manifest := filepath.Join(remote.Directory, "fonts", "library.json")
			blob := filepath.Join(remote.Directory, "fonts", asset.Name())
			var err error
			switch fault {
			case "missing":
				err = os.Remove(blob)
			case "digest":
				err = os.WriteFile(blob, goregular.TTF, 0600)
			default:
				data, e := os.ReadFile(manifest)
				if e != nil {
					t.Fatal(e)
				}
				s := string(data)
				switch fault {
				case "unknown":
					s = strings.Replace(s, `"version":1`, `"version":1,"extra":true`, 1)
				case "type":
					s = strings.Replace(s, `"version":1`, `"version":"1"`, 1)
				case "format":
					s = strings.Replace(s, "OShell.font-library", "unknown", 1)
				case "limit":
					s = strings.Repeat(" ", 65537)
				}
				err = os.WriteFile(manifest, []byte(s), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			base := FontBaselinePath(t.TempDir(), "r")
			if err = SynchronizeFonts(ctx, target, remote, base); err == nil {
				t.Fatal("invalid remote accepted")
			}
			if !reflect.DeepEqual(before, fontNames(t, target)) {
				t.Fatal("failed sync replaced active fonts")
			}
			if _, err = os.Stat(base); !os.IsNotExist(err) {
				t.Fatal("failed sync changed baseline")
			}
		})
	}
}

func TestFontSyncInterruptedPublicationAndLocalEdit(t *testing.T) {
	for _, fault := range []string{"object", "remote-race", "local-before", "local-after", "cancel"} {
		t.Run(fault, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			remote := DirectoryTransport{t.TempDir()}
			a, b := NewFontLibrary(t.TempDir()), NewFontLibrary(t.TempDir())
			base := FontBaselinePath(t.TempDir(), "r")
			importSyncFont(t, b, goregular.TTF)
			if err := SynchronizeFonts(ctx, b, remote, FontBaselinePath(t.TempDir(), "r")); err != nil {
				t.Fatal(err)
			}
			importSyncFont(t, a, gomono.TTF)
			before, _ := remote.FetchFont(ctx, "library.json")
			transport := interruptedFontTransport{FontSyncTransport: remote, beforePut: func(name string) error {
				if name != "library.json" {
					if fault == "object" {
						return errors.New("synthetic object failure")
					}
					if fault == "local-before" {
						importSyncFont(t, a, gomonobold.TTF)
					}
					return nil
				}
				switch fault {
				case "remote-race":
					changed := append(append([]byte{}, before.Data...), '\n')
					if err := remote.PutFont(ctx, name, changed, before.ETag); err != nil {
						t.Fatal(err)
					}
				case "local-after":
					importSyncFont(t, a, gomonobold.TTF)
				case "cancel":
					cancel()
				}
				return nil
			}}
			if err := SynchronizeFonts(ctx, a, transport, base); err == nil {
				t.Fatal("interruption accepted")
			}
			current, err := remote.FetchFont(context.Background(), "library.json")
			if err != nil {
				t.Fatal(err)
			}
			if fault != "local-after" && fault != "remote-race" && !bytes.Equal(current.Data, before.Data) {
				t.Fatal("partial objects published")
			}
			if fault == "local-after" {
				if len(fontNames(t, a)) != 2 {
					t.Fatal("local edit overwritten")
				}
				if err := SynchronizeFonts(ctx, a, remote, base); err != nil {
					t.Fatal(err)
				}
				if len(fontNames(t, a)) != 3 {
					t.Fatal("continuation baseline lost remote-only addition")
				}
			}
		})
	}
}

func TestFontSyncPolicyFingerprintAndSharedGate(t *testing.T) {
	ctx := context.Background()
	remote := t.TempDir()
	a, cfg, master := syncEngineFixture(t, remote)
	options := SyncOptions{Version: 1, Enabled: true, Backend: "directory", Address: remote}
	legacyTarget, fontTarget := options.Target(), options.FontTarget()
	if options.Policy.Effective().Content.FontFiles {
		t.Fatal("fonts enabled by default")
	}
	scope := DefaultSyncPolicy().Content
	beforeOff := SyncLocalFingerprint(a.Store, a.Vault, scope)
	scope.FontFiles = true
	beforeOn := SyncLocalFingerprint(a.Store, a.Vault, scope)
	l := NewFontLibrary(a.Store.Dir)
	importSyncFont(t, l, gomono.TTF)
	if beforeOff != SyncLocalFingerprint(a.Store, a.Vault, DefaultSyncPolicy().Content) || beforeOn == SyncLocalFingerprint(a.Store, a.Vault, scope) {
		t.Fatal("font fingerprint ignores scope")
	}
	plan, err := a.Prepare(ctx, cfg, master, true)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = plan.Commit(ctx, nil, master)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(remote, "fonts")); !os.IsNotExist(err) {
		t.Fatal("disabled sync touched fonts")
	}
	options.Policy = DefaultSyncPolicy()
	options.Policy.Content.FontFiles = true
	if options.FontTarget() != fontTarget || options.Target() == legacyTarget {
		t.Fatal("target identity broken")
	}
	if err = SaveSyncOptions(a.Store.Dir, options); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSyncOptions(a.Store.Dir)
	if err != nil || !loaded.Policy.Content.FontFiles {
		t.Fatal("font policy not persisted", err)
	}
	a.Selection = &scope
	a.FontTarget = fontTarget
	plan, err = a.Prepare(ctx, cfg, master, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = plan.Commit(ctx, nil, master); err != nil {
		t.Fatal(err)
	}
	if len(fontNames(t, NewFontLibrary(remote))) != 1 {
		t.Fatal("unchanged configuration skipped fonts")
	}
	sealed, err := os.ReadFile(filepath.Join(remote, "configuration.json"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := OpenShared(sealed, master)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(plain, []byte("fontFiles")) || bytes.Contains(plain, []byte("sha256")) {
		t.Fatal("font state leaked into shared contract")
	}
	b, other, _ := syncEngineFixture(t, remote)
	b.Selection = &scope
	if _, err := b.Prepare(ctx, other, []byte("incorrect"), true); err == nil {
		t.Fatal("incorrect password accepted")
	}
	plan, err = b.Prepare(ctx, other, master, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = plan.Commit(ctx, nil, master); err == nil {
		t.Fatal("first contact bypassed")
	}
	if len(fontNames(t, NewFontLibrary(b.Store.Dir))) != 0 {
		t.Fatal("fonts applied before confirmation")
	}
	if err = plan.ChooseInitial(false); err != nil {
		t.Fatal(err)
	}
	if _, err = plan.Commit(ctx, nil, master); err != nil {
		t.Fatal(err)
	}
	if len(fontNames(t, NewFontLibrary(b.Store.Dir))) != 1 {
		t.Fatal("confirmed sync omitted fonts")
	}
	options.Policy.Content = SyncContentSelection{FontFiles: true}
	if options.Policy.Validate() != nil || options.FontTarget() != fontTarget {
		t.Fatal("font-only scope rejected")
	}
	options.Policy.Content.FontFiles = false
	options.Policy.Content.Sessions = true
	if options.Target() != legacyTarget {
		t.Fatal("disabled target no longer backward compatible")
	}
}

func TestFontSyncManifestWireAndMergedLimits(t *testing.T) {
	l := NewFontLibrary(t.TempDir())
	asset := importSyncFont(t, l, gomono.TTF)
	manifest, _ := l.Manifest()
	data, _ := json.Marshal(manifest)
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(data, &wire); err != nil || len(wire) != 3 {
		t.Fatal("manifest wire contract", err)
	}
	decoded, err := DecodeFontManifest(data)
	if err != nil || !reflect.DeepEqual(decoded, manifest) {
		t.Fatal("manifest round trip", err)
	}
	for _, data := range []string{`{"format":"OShell.font-library","version":1,"fonts":null}`, `{"format":"OShell.font-library","version":1,"fonts":[] ,"extra":1}`} {
		if _, err := DecodeFontManifest([]byte(data)); err == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
	empty := FontManifest{Format: manifest.Format, Version: 1, Fonts: []FontAsset{}}
	own, remote := empty, empty
	for i := 0; i < 65; i++ {
		a := asset
		a.SHA256 = strings.Repeat("0", 62) + string("0123456789abcdef"[i/16]) + string("0123456789abcdef"[i%16])
		if i < 33 {
			own.Fonts = append(own.Fonts, a)
		} else {
			remote.Fonts = append(remote.Fonts, a)
		}
	}
	if _, err := MergeFontManifests(empty, own, remote); err == nil {
		t.Fatal("merged count limit ignored")
	}
}

func TestFontOnlySyncPreservesConfigurationAndDisabledSkipsInvalidLibrary(t *testing.T) {
	ctx := context.Background()
	remote := t.TempDir()
	a, ac, master := syncEngineFixture(t, remote)
	plan, err := a.Prepare(ctx, ac, master, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = plan.Commit(ctx, nil, master); err != nil {
		t.Fatal(err)
	}
	configBefore, err := os.ReadFile(filepath.Join(remote, "configuration.json"))
	if err != nil {
		t.Fatal(err)
	}
	b, bc, _ := syncEngineFixture(t, remote)
	bc.Settings.FontSize = 23
	if err = b.Store.Save(bc); err != nil {
		t.Fatal(err)
	}
	if err = b.Vault.Put(bc.Profiles[0], "synthetic-local-secret"); err != nil {
		t.Fatal(err)
	}
	scope := SyncContentSelection{FontFiles: true}
	b.Selection = &scope
	importSyncFont(t, NewFontLibrary(b.Store.Dir), gomono.TTF)
	plan, err = b.Prepare(ctx, bc, master, true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.InitialChoiceRequired {
		if err = plan.ChooseInitial(false); err != nil {
			t.Fatal(err)
		}
	}
	result, err := plan.Commit(ctx, nil, master)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, bc) {
		t.Fatal("font-only sync replaced configuration")
	}
	secret, err := b.Vault.GetFor(bc.Profiles[0], bc.Profiles[0].Identity())
	if err != nil || secret != "synthetic-local-secret" {
		t.Fatal("font-only sync changed credential", err)
	}
	configAfter, err := os.ReadFile(filepath.Join(remote, "configuration.json"))
	if err != nil || !bytes.Equal(configBefore, configAfter) {
		t.Fatal("font-only sync rewrote shared configuration", err)
	}
	if len(fontNames(t, NewFontLibrary(remote))) != 1 {
		t.Fatal("font-only did not upload")
	}
	writeFontFixture(t, a.Store.Dir, "fonts/library.json", []byte("invalid local font manifest"))
	writeFontFixture(t, remote, "fonts/library.json", []byte("invalid remote font manifest"))
	plan, err = a.Prepare(ctx, ac, master, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = plan.Commit(ctx, nil, master); err != nil {
		t.Fatal("disabled font sync read invalid library", err)
	}
}

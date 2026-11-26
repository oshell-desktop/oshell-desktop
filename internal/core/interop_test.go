package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestSwiftSharedProtocolRoundTrip(t *testing.T) {
	helper := os.Getenv("OSHELL_SWIFT_INTEROP_HELPER")
	if helper == "" {
		t.Skip("run scripts/test-sync-interop.py with the native OShell checkout")
	}
	dir := t.TempDir()
	swift := filepath.Join(dir, "swift.json")
	goFile := filepath.Join(dir, "go.json")
	back := filepath.Join(dir, "back.json")
	run := func(args ...string) {
		t.Helper()
		if output, err := exec.Command(helper, args...).CombinedOutput(); err != nil {
			t.Fatalf("Swift helper: %v %s", err, output)
		}
	}
	run("create", swift)
	data, err := os.ReadFile(swift)
	if err != nil {
		t.Fatal(err)
	}
	master := []byte("interop-fixture-master-🔐")
	doc, err := DecodeSharedDocument(data, master)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := doc.Configuration()
	if err != nil {
		t.Fatal(err)
	}
	v, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = v.SetMaster(string(master)); err != nil {
		t.Fatal(err)
	}
	for _, p := range doc.Profiles {
		password, err := DecryptMacPassword(*p.Password, master, p.Definition)
		if err != nil || password != "fixture-"+p.Definition.Kind {
			t.Fatal("credential", err)
		}
		if err = v.PutFor(p.Definition, password, p.Password.Identity); err != nil {
			t.Fatal(err)
		}
	}
	for _, proxy := range doc.Proxies {
		p := ProxyCredentialProfile(proxy.Definition.Settings)
		password, err := DecryptMacPassword(*proxy.Password, master, p)
		if err != nil {
			t.Fatal(err)
		}
		if err = v.PutFor(p, password, proxy.Password.Identity); err != nil {
			t.Fatal(err)
		}
	}
	exported, _, err := v.ExportSharedDocument(context.Background(), cfg, master, &doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err = exported.Encode(master)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(goFile, data, 0600); err != nil {
		t.Fatal(err)
	}
	run("roundtrip", goFile, back)
	stableData, err := os.ReadFile(back)
	if err != nil {
		t.Fatal(err)
	}
	stable, err := DecodeSharedDocument(stableData, master)
	if err != nil {
		t.Fatal(err)
	}
	if !sameShared(exported, stable) {
		a, _ := sharedFields(exported)
		b, _ := sharedFields(stable)
		for key, value := range a {
			if !equalSharedJSON(value, b[key]) {
				t.Errorf("round trip changes section %s", key)
			}
		}
	}
	run("edit", goFile, back)
	data, err = os.ReadFile(back)
	if err != nil {
		t.Fatal(err)
	}
	round, err := DecodeSharedDocument(data, master)
	if err != nil {
		t.Fatal(err)
	}
	if round.Profiles[0].Definition.Name != "Swift 修改" {
		t.Fatal("Swift edit lost")
	}
}

func TestSwiftMultiNodeSyncTransports(t *testing.T) {
	helper := os.Getenv("OSHELL_SWIFT_INTEROP_HELPER")
	if helper == "" {
		t.Skip("Swift interoperability helper required")
	}
	for _, backend := range []string{"directory", "webdav"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			master := []byte("interop-fixture-master-🔐")
			run := func(args ...string) {
				t.Helper()
				if output, err := exec.Command(helper, args...).CombinedOutput(); err != nil {
					t.Fatalf("Swift: %v %s", err, output)
				}
			}
			var transport SyncTransport
			var address string
			if backend == "directory" {
				address = t.TempDir()
				transport = DirectoryTransport{address}
			} else {
				var mu sync.Mutex
				var data []byte
				tag := ""
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					defer mu.Unlock()
					user, password, ok := r.BasicAuth()
					if !ok || user != "test-user" || password != "test-password" {
						w.WriteHeader(401)
						return
					}
					if r.Method == "GET" && r.URL.Path == "/vault/oshell-vault.json" {
						w.WriteHeader(404)
						return
					}
					if r.Method == "PROPFIND" && r.URL.Path == "/vault/oshell/" {
						writeDAVCollection(w, r.URL.Path)
						return
					}
					if r.URL.Path != "/vault/oshell/oshell-vault.json" {
						http.NotFound(w, r)
						return
					}
					switch r.Method {
					case "GET":
						if data == nil {
							w.WriteHeader(404)
							return
						}
						w.Header().Set("ETag", tag)
						w.Write(data)
					case "PUT":
						if data == nil && r.Header.Get("If-None-Match") != "*" || data != nil && r.Header.Get("If-Match") != tag {
							w.WriteHeader(412)
							return
						}
						payload, err := io.ReadAll(io.LimitReader(r.Body, MaxSharedVault+1))
						if err != nil || len(payload) > MaxSharedVault {
							w.WriteHeader(400)
							return
						}
						data = payload
						sum := sha256.Sum256(data)
						tag = fmt.Sprintf("\"%x\"", sum)
						w.WriteHeader(204)
					default:
						w.WriteHeader(405)
					}
				}))
				defer server.Close()
				address = server.URL + "/vault"
				var err error
				transport, err = NewWebDAVTransport(address, "test-user", "test-password", true)
				if err != nil {
					t.Fatal(err)
				}
			}
			fixture := filepath.Join(t.TempDir(), "swift.json")
			run("create", fixture)
			data, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatal(err)
			}
			if err = transport.Put(ctx, data, ""); err != nil {
				t.Fatal(err)
			}
			node := func() (*SharedSyncEngine, Config) {
				dir := t.TempDir()
				store := &Store{Dir: dir}
				cfg := DefaultConfig()
				cfg.Settings.FontSize = 20
				cfg.SessionDefaults.SSHPort = 2220
				if err := store.Save(cfg); err != nil {
					t.Fatal(err)
				}
				vault, err := OpenVault(dir)
				if err != nil {
					t.Fatal(err)
				}
				if err = vault.SetMaster(string(master)); err != nil {
					t.Fatal(err)
				}
				engine := &SharedSyncEngine{Store: store, Vault: vault, Transport: transport, Replica: SharedReplicaStore{Directory: dir, Target: address}}
				plan, err := engine.Prepare(ctx, cfg, master, false)
				if err != nil {
					t.Fatal(err)
				}
				if plan.InitialChoiceRequired {
					if err = plan.ChooseInitial(false); err != nil {
						t.Fatal(err)
					}
				}
				cfg, err = plan.Commit(ctx, nil, master)
				if err != nil {
					t.Fatal(err)
				}
				if cfg.Settings.FontSize != 20 || cfg.SessionDefaults.SSHPort != 2220 || len(cfg.QuickCommands) != 0 {
					t.Fatal("Swift sync overwrote local client settings")
				}
				return engine, cfg
			}
			a, local := node()
			b, other := node()
			// Swift and Go edit different sessions from a common baseline.
			local.Profiles[1].Name = "Go 修改"
			if err = a.Store.Save(local); err != nil {
				t.Fatal(err)
			}
			run(backend+"-edit", address)
			plan, err := a.Prepare(ctx, local, master, false)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Conflicts) != 0 {
				t.Fatalf("independent edits conflict: %+v", plan.Conflicts)
			}
			local, err = plan.Commit(ctx, nil, master)
			if err != nil {
				t.Fatal(err)
			}
			if local.Profiles[0].Name != "Swift 修改" || local.Profiles[1].Name != "Go 修改" {
				t.Fatal("independent edit lost")
			}
			if local.Settings.FontSize != 20 || local.SessionDefaults.SSHPort != 2220 {
				t.Fatal("session merge applied legacy remote settings")
			}
			// A stale node editing the same session must not overwrite Swift's change.
			other.Profiles[0].Name = "stale node edit"
			if err = b.Store.Save(other); err != nil {
				t.Fatal(err)
			}
			conflict, err := b.Prepare(ctx, other, master, false)
			if err != nil {
				t.Fatal(err)
			}
			if len(conflict.Conflicts) != 1 {
				t.Fatalf("wanted conflict, got %+v", conflict.Conflicts)
			}
			before, err := transport.Fetch(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = conflict.Commit(ctx, nil, master); err == nil {
				t.Fatal("unresolved overwrite accepted")
			}
			after, err := transport.Fetch(ctx)
			if err != nil || after.ETag != before.ETag {
				t.Fatal("failed conflict changed shared object", err)
			}
			// No-change sync must not write again just because Swift normalized JSON.
			plan, err = a.Prepare(ctx, local, master, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = plan.Commit(ctx, nil, master); err != nil {
				t.Fatal(err)
			}
			stable, err := transport.Fetch(ctx)
			if err != nil || stable.ETag != after.ETag {
				t.Fatal("unchanged sync rewrote remote", err)
			}
			verify := filepath.Join(t.TempDir(), "final.json")
			if err = os.WriteFile(verify, stable.Data, 0600); err != nil {
				t.Fatal(err)
			}
			run("verify", verify)
			for _, p := range local.Profiles {
				secret, err := a.Vault.GetFor(p, p.Identity())
				if err != nil || secret != "fixture-"+p.Kind {
					t.Fatal("credential lost", err)
				}
			}
		})
	}
}

func TestSwiftReadsNewDesktopDocumentWithoutSpuriousChanges(t *testing.T) {
	helper := os.Getenv("OSHELL_SWIFT_INTEROP_HELPER")
	if helper == "" {
		t.Skip("Swift helper required")
	}
	cfg := DefaultConfig()
	p := NewProfile()
	p.Host = "fixture.invalid"
	p.Username = "fixture"
	cfg.Profiles = []Profile{p}
	v, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	master := []byte("interop-fixture-master-🔐")
	if err = v.SetMaster(string(master)); err != nil {
		t.Fatal(err)
	}
	if err = v.PutFor(p, "fixture-secret", p.Identity()); err != nil {
		t.Fatal(err)
	}
	doc, _, err := v.ExportSharedDocument(context.Background(), cfg, master, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := doc.Encode(master)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "input.json")
	output := filepath.Join(t.TempDir(), "output.json")
	if err = os.WriteFile(input, data, 0600); err != nil {
		t.Fatal(err)
	}
	if text, err := exec.Command(helper, "native-roundtrip", input, output).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, text)
	}
	data, err = os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeSharedDocument(data, master)
	if err != nil {
		t.Fatal(err)
	}
	if !sameShared(doc, back) {
		a, _ := sharedFields(doc)
		b, _ := sharedFields(back)
		for key, value := range a {
			if !equalSharedJSON(value, b[key]) {
				t.Errorf("native roundtrip changes section %s", key)
				if key == "preferences" {
					for k, v := range doc.Preferences {
						if !equalSharedJSON(v, back.Preferences[k]) {
							t.Logf("preference %s: %s -> %s", k, v, back.Preferences[k])
						}
					}
				}
			}
		}
	}
	secret, err := DecryptMacPassword(*back.Profiles[0].Password, master, back.Profiles[0].Definition)
	if err != nil || secret != "fixture-secret" {
		t.Fatal("Go-origin credential lost", err)
	}
}

func TestSwiftAllSharedFieldsAndUnknownRejection(t *testing.T) {
	helper := os.Getenv("OSHELL_SWIFT_INTEROP_HELPER")
	if helper == "" {
		t.Skip("Swift helper required")
	}
	dir := t.TempDir()
	contract := filepath.Join(dir, "contract.json")
	if out, err := exec.Command(helper, "contract", contract).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	actual, err := os.ReadFile(contract)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(actual), bytes.TrimSpace(sharedConfigurationSchema)) {
		t.Fatal("Swift and Go field contracts differ")
	}
	cfg := DefaultConfig()
	profile := NewProfile()
	profile.Host = "fixture.invalid"
	profile.Username = "tester"
	cfg.Profiles = []Profile{profile}
	cfg.Settings.FontSize = 13.123456789012345
	cfg.Settings.TerminalClockOpacity = .123456789012345
	style := DefaultTerminalAppearance()
	style.Left, style.LineSpacing, style.CursorShape = 17, -3, "bar"
	style.AsianFont, style.TabColor = "Fixture CJK", "#8967B5"
	cfg.Settings.TerminalAppearance = &style
	palette := ColorPresets()[0].Copy()
	palette.AutoCursor = true
	palette.BoldForeground = "#ABCDEF"
	cfg.Settings.CustomSchemes = []ColorScheme{palette}
	cfg.Settings.ColorSchemeID = palette.ID
	rule := &cfg.HighlightSets[0].Rules[0]
	rule.Name = "保留规则名称"
	rule.Bold = true
	rule.Background = "#123456"
	rule.Foreground = "#ABCDEF"
	cfg.Settings.KeyboardShortcuts.Overrides = map[string]ShortcutOverride{"find": {}, "terminalLineStart": {Shortcut: &KeyboardShortcut{KeyCode: 115, Modifiers: 0}}}
	vault, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	master := []byte("interop-fixture-master-🔐")
	if err = vault.SetMaster(string(master)); err != nil {
		t.Fatal(err)
	}
	document, _, err := vault.ExportSharedDocument(context.Background(), cfg, master, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := canonicalConfigurationJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	run := func(input []byte, mode string, valid bool) []byte {
		t.Helper()
		in, out := filepath.Join(dir, "in.json"), filepath.Join(dir, "out.json")
		if err := os.WriteFile(in, input, 0600); err != nil {
			t.Fatal(err)
		}
		result, err := exec.Command(helper, mode, in, out).CombinedOutput()
		if !valid {
			if err == nil {
				t.Fatal("Swift accepted unsupported data")
			}
			return nil
		}
		if err != nil {
			t.Fatalf("Swift %v %s", err, result)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	for i := 0; i < 3; i++ {
		back := run(canonical, "model-json", true)
		if !equalSharedJSON(canonical, back) {
			var a, b map[string]json.RawMessage
			json.Unmarshal(canonical, &a)
			json.Unmarshal(back, &b)
			for key, value := range a {
				if !equalSharedJSON(value, b[key]) {
					t.Errorf("model lost %s", key)
				}
			}
			t.FailNow()
		}
		doc, err := decodeNativeDocument(back)
		if err != nil {
			t.Fatal(err)
		}
		config, err := doc.Configuration()
		if err != nil {
			t.Fatal(err)
		}
		if config.Settings.TerminalAppearance == nil || *config.Settings.TerminalAppearance != style || config.Settings.FontSize != cfg.Settings.FontSize || !reflect.DeepEqual(config.Settings.CustomSchemes, cfg.Settings.CustomSchemes) || config.HighlightSets[0].Rules[0].Name != rule.Name || !config.HighlightSets[0].Rules[0].Bold || config.HighlightSets[0].Rules[0].Background != rule.Background {
			t.Fatal("model fields/precision lost")
		}
		exported, _, err := vault.ExportSharedDocument(context.Background(), config, master, &doc, nil)
		if err != nil {
			t.Fatal(err)
		}
		next, err := json.Marshal(exported)
		if err != nil {
			t.Fatal(err)
		}
		canonical, err = canonicalConfigurationJSON(next)
		if err != nil {
			t.Fatal(err)
		}
		if !equalSharedJSON(back, canonical) {
			t.Fatal("Go model changed shared configuration")
		}
	}
	// Exercise every object definition and reject additional fields before either
	// language can silently discard them, including nested rule/palette/shortcut data.
	var base map[string]any
	json.Unmarshal(canonical, &base)
	paths := [][]any{{}, {"preferences"}, {"profiles", 0}, {"profiles", 0, "proxy"}, {"profiles", 0, "keepAlive"}, {"preferences", "customColorSchemes", 0}, {"highlightSets", 0}, {"highlightSets", 0, "rules", 0}, {"preferences", "keyboardShortcuts", "overrides", "terminalLineStart", "shortcut"}, {"sessionLinks"}, {"sessionDefaults"}}
	for n, path := range paths {
		t.Run(fmt.Sprint("unknown-", n), func(t *testing.T) {
			var object map[string]any
			json.Unmarshal(canonical, &object)
			var target any = object
			for _, part := range path {
				switch key := part.(type) {
				case string:
					target = target.(map[string]any)[key]
				case int:
					target = target.([]any)[key]
				}
			}
			target.(map[string]any)["futureSensitiveField"] = "must-not-be-lost"
			raw, _ := json.Marshal(object)
			if _, err := canonicalConfigurationJSON(raw); !errors.Is(err, ErrSharedProtocol) {
				t.Fatal("Go accepted unsupported field", err)
			}
			run(raw, "canonical-json", false)
			if _, err := decodeNativeDocument(raw); err == nil {
				t.Fatal("Go decoded unsupported field")
			}
		})
	}
	// Identical handling of each legacy quick-send scope and of malformed values.
	for _, value := range []any{0, 1, 2, 3, 4, 5, 6, "current", "tab", "all", "visible", "selected", "currentGroup", "selectedGroups", true, 7, -1, "unknown", 1.5} {
		var object map[string]any
		json.Unmarshal(canonical, &object)
		object["preferences"].(map[string]any)["quickSendScope"] = value
		raw, _ := json.Marshal(object)
		goValue, err := canonicalConfigurationJSON(raw)
		swiftValue := run(raw, "canonical-json", err == nil)
		if err == nil && !equalSharedJSON(goValue, swiftValue) {
			t.Fatalf("normalization differs for %v", value)
		}
	}
}

func TestSwiftSessionArchiveUsesSameDefinitions(t *testing.T) {
	helper := os.Getenv("OSHELL_SWIFT_INTEROP_HELPER")
	if helper == "" {
		t.Skip("Swift helper required")
	}
	fixture, err := os.ReadFile("testdata/oshell-swift-shared-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	master := []byte("interop-fixture-master-🔐")
	doc, err := DecodeSharedDocument(fixture, master)
	if err != nil {
		t.Fatal(err)
	}
	archive := PortableArchive{Format: "OShell.sessions", Version: 4, Profiles: doc.Profiles, Proxies: doc.Proxies, Directories: doc.Directories, Links: doc.Links.Entries}
	data, err := archive.Encode()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	input, output := filepath.Join(dir, "archive.json"), filepath.Join(dir, "back.json")
	if err = os.WriteFile(input, data, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(helper, "archive-roundtrip", input, output).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	back, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !equalSharedJSON(data, back) {
		t.Fatal("archive wire definitions differ")
	}
	decoded, err := DecodePortableArchive(back)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range decoded.Profiles {
		secret, err := DecryptMacPassword(*p.Password, master, p.Definition)
		if err != nil || secret != "fixture-"+p.Definition.Kind {
			t.Fatal("archive credentials lost", err)
		}
	}
	var raw map[string]any
	json.Unmarshal(data, &raw)
	raw["unsupportedSecret"] = "fixture"
	bad, _ := json.Marshal(raw)
	if _, err = DecodePortableArchive(bad); !errors.Is(err, ErrSharedProtocol) {
		t.Fatal("unknown archive field accepted", err)
	}
	if err = os.WriteFile(input, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = exec.Command(helper, "archive-roundtrip", input, output).CombinedOutput(); err == nil {
		t.Fatal("native archive silently discarded unknown field")
	}
}

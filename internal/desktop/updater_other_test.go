//go:build !darwin

package desktop

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"io"
	"net/http"
	"os"
	"oshell-desktop/internal/core"
	"runtime"
	"strings"
	"testing"
	"time"
)

type updateHTTPFixture func(*http.Request) (*http.Response, error)

func (f updateHTTPFixture) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestToolbarChecksGitHubRelease(t *testing.T) {
	oldVersion, previous := mygo.App.Version(), http.DefaultTransport
	defer func() { mygo.App.SetVersion(oldVersion); http.DefaultTransport = previous }()
	mygo.App.SetVersion("0.4.1")
	for _, tc := range []struct{ tag, status string }{
		{"v0.3.2", "没有更高的正式版本"}, {"v0.4.1", "没有更高的正式版本"}, {"v0.4.2", "缺少已签名的 latest.json"},
	} {
		t.Run(tc.tag, func(t *testing.T) {
			http.DefaultTransport = updateHTTPFixture(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://api.github.com/repos/"+core.DefaultUpdateRepository+"/releases/latest" {
					return nil, fmt.Errorf("unexpected request: %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"tag_name":"` + tc.tag + `","assets":[]}`))}, nil
			})
			a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
			view := ui.NewTester(a.view, 1280, 800)
			if err := view.Click("更新"); err != nil {
				t.Fatal(err)
			}
			waitOperator(t, a, func() bool { return !a.updateState().busy })
			settings := ui.NewTester(a.settingsView, 800, 650)
			if !a.settingsOpen || !settings.HasText(tc.status) || a.updateState().candidate != nil {
				t.Fatalf("toolbar result not visible in update settings: %s", a.updateState().status)
			}
		})
	}
}

func TestToolbarDownloadsAndInstallsApprovedPackage(t *testing.T) {
	path := os.Getenv("OSHELL_P1_UPDATE_ARCHIVE")
	if path == "" || runtime.GOOS != "windows" {
		t.Skip("requires local Windows portable ZIP")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	oldVersion, oldKey, previous := mygo.App.Version(), core.UpdatePublicKey, http.DefaultTransport
	oldDialog, oldStart := showUpdateConfirmation, startPortableUpdate
	defer func() {
		mygo.App.SetVersion(oldVersion)
		core.UpdatePublicKey, http.DefaultTransport = oldKey, previous
		showUpdateConfirmation, startPortableUpdate = oldDialog, oldStart
	}()
	mygo.App.SetVersion("0.4.0")
	core.UpdatePublicKey = base64.StdEncoding.EncodeToString(pub)
	target := "windows-amd64"
	name := core.ReleaseAssetName(target)
	url := "https://github.com/" + core.DefaultUpdateRepository + "/releases/download/v0.4.1/"
	hash := sha256.Sum256(data)
	digest := hex.EncodeToString(hash[:])
	meta, err := core.SignUpdateManifest(core.UpdateManifest{
		Version: "0.4.1", Build: "100", ApplicationID: "dev.oshell.desktop", Repository: core.DefaultUpdateRepository,
		Platforms: map[string]core.UpdateArchive{target: {URL: url + name, Size: int64(len(data)), SHA256: digest, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, hash[:]))}},
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	release, err := json.Marshal(map[string]any{"tag_name": "v0.4.1", "assets": []map[string]any{
		{"id": 1, "state": "uploaded", "name": name, "size": len(data), "browser_download_url": url + name, "digest": "sha256:" + digest},
		{"id": 2, "state": "uploaded", "name": "latest.json", "size": len(meta), "browser_download_url": url + "latest.json"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	http.DefaultTransport = updateHTTPFixture(func(r *http.Request) (*http.Response, error) {
		var body []byte
		switch r.URL.String() {
		case "https://api.github.com/repos/" + core.DefaultUpdateRepository + "/releases/latest":
			body = release
		case "https://api.github.com/repos/" + core.DefaultUpdateRepository + "/releases/assets/2":
			body = meta
		case url + name:
			body = data
		default:
			return nil, fmt.Errorf("unexpected URL %s", r.URL)
		}
		return &http.Response{StatusCode: 200, ContentLength: int64(len(body)), Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	showUpdateConfirmation = func(o mygo.MessageOptions) (mygo.MessageResult, error) {
		if len(o.Buttons) != 2 || o.Buttons[1] != "下载并安装" {
			t.Errorf("unexpected dialog: %s", o.Title)
		}
		return mygo.MessageResult{Button: 1}, nil
	}
	startPortableUpdate = func(*core.PortableUpdate) error { return fmt.Errorf("test installer received verified package") }
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	view := ui.NewTester(a.view, 1280, 800)
	if err := view.Click("更新"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for state := a.updateState(); state.status != "test installer received verified package" && time.Now().Before(deadline); {
		a.drain()
		time.Sleep(time.Millisecond)
	}
	state := a.updateState()
	if state.prepared == nil {
		t.Fatalf("install did not receive package: %s", state.status)
	}
	defer state.prepared.Discard()
	if state.status != "test installer received verified package" {
		t.Fatal(state.status)
	}
}

func TestRetainedU01SourceBindingAndBusyGuard(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.prepareSettings()
	state := a.updateState()
	state.tokenDraft = "synthetic-token-retained"
	a.saveUpdateToken(false)
	a.features.settingsDraft.UpdateRepository = "fixture/other"
	state.busy = true
	if err := a.validateUpdateSettings(); err == nil {
		t.Fatal("source changed while busy")
	}
	state.busy = false
	state.tokenDraft = "synthetic-new-token"
	a.saveUpdateToken(false)
	if token, err := core.ReadUpdateToken(a.store.Dir, "fixture/other"); err != nil || token != "" {
		t.Fatal("token crossed unapplied source")
	}
	a.applySettings()
	a.prepareSettings()
	if state.hasToken || state.tokenDraft != "" {
		t.Fatal("old source token shown for new source")
	}
	state.tokenDraft = "synthetic-secret\r\ninvalid"
	a.saveUpdateToken(false)
	if strings.Contains(state.status, "synthetic-secret") {
		t.Fatal("error leaked secret")
	}
}

func TestP2U04DeferredInstallAndIdleSkipsConfirmation(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.Settings.UpdateRepository = "owner/repo"
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	state := a.updateState()
	state.prepared = &core.PortableUpdate{}
	state.source = "owner/repo"
	oldStart, oldConfirm := startPortableUpdate, showUpdateConfirmation
	defer func() { startPortableUpdate = oldStart; showUpdateConfirmation = oldConfirm }()
	started := make(chan struct{}, 2)
	startPortableUpdate = func(*core.PortableUpdate) error { started <- struct{}{}; return fmt.Errorf("synthetic start failure") }
	showUpdateConfirmation = func(mygo.MessageOptions) (mygo.MessageResult, error) {
		t.Error("idle install asked session confirmation")
		return mygo.MessageResult{}, nil
	}
	a.features.uploadDraft = &uploadConfirmation{open: true}
	a.installUpdate()
	if !state.pendingInstall || state.busy {
		t.Fatal("modal did not queue install")
	}
	a.features.uploadDraft = nil
	a.drain()
	waitOperator(t, a, func() bool { return !state.busy && state.status == "synthetic start failure" })
	select {
	case <-started:
	default:
		t.Fatal("deferred installation never resumed")
	}
	a.drain()
	select {
	case <-started:
		t.Fatal("failed install retried without user action")
	default:
	}
	if state.prepared == nil {
		t.Fatal("failure discarded prepared update")
	}
}

func TestUpdateSettingsDraftValidationAndView(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.prepareSettings()
	a.features.settingsTab = 4
	tt := ui.NewTester(a.settingsView, 800, 650)
	if !tt.HasText("签名更新") || !tt.HasText("检查更新") {
		t.Fatal(tt.Texts())
	}
	a.features.settingsDraft.AutomaticUpdateChecks = true
	if e := a.validateUpdateSettings(); e != nil || a.features.settingsDraft.UpdateRepository != core.DefaultUpdateRepository {
		t.Fatal("default repository unavailable", e)
	}
	a.features.settingsDraft.UpdateRepository = "https://github.com/owner/repo.git"
	if e := a.validateUpdateSettings(); e != nil || a.features.settingsDraft.UpdateRepository != "owner/repo" {
		t.Fatal("source normalization", e)
	}
	a.closeSettings()
	if a.config.Settings.UpdateRepository != "" || a.config.Settings.AutomaticUpdateChecks {
		t.Fatal("cancel saved update preferences")
	}
	a.prepareSettings()
	a.features.settingsDraft.UpdateRepository = "owner/repo"
	a.features.updates.busy = true
	if e := a.validateUpdateSettings(); e == nil {
		t.Fatal("changed repository during update")
	}
	a.features.updates.busy = false
	a.applySettings()
	if a.config.Settings.UpdateRepository != "owner/repo" {
		t.Fatal("apply lost update preference")
	}
}

func TestUpdateTokenSettingsSaveAndClear(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.prepareSettings()
	state := a.updateState()
	state.tokenDraft = "github_pat_synthetic_fixture"
	a.saveUpdateToken(false)
	if !state.hasToken || state.tokenDraft != "" {
		t.Fatal("token not saved", state.status)
	}
	if token, e := core.ReadUpdateToken(a.store.Dir, core.DefaultUpdateRepository); e != nil || token != "github_pat_synthetic_fixture" {
		t.Fatal("token read failed", e)
	}
	state.busy = true
	a.saveUpdateToken(true)
	if !state.hasToken {
		t.Fatal("token changed during update")
	}
	state.busy = false
	a.saveUpdateToken(true)
	if state.hasToken {
		t.Fatal("token not cleared")
	}
}
func TestUpdateSchedulerKeepsDailyDeadlineAndStopsWhenDisabled(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.Settings.UpdateRepository = "owner/repo"
	cfg.Settings.AutomaticUpdateChecks = true
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	h := &applicationHub{store: a.store, config: cfg, vault: a.vault, windows: []*App{a}}
	a.hub = h
	h.updates.scheduledSource = "owner/repo"
	h.updates.nextCheck = time.Now().Add(time.Hour)
	h.scheduleUpdates()
	timer := h.updates.timer
	deadline := h.updates.nextCheck
	if timer == nil {
		t.Fatal("enabled checks not scheduled")
	}
	defer h.stopUpdates()
	h.scheduleUpdates()
	if h.updates.timer != timer || !h.updates.nextCheck.Equal(deadline) {
		t.Fatal("ordinary config save postponed daily check")
	}
	h.config.Settings.AutomaticUpdateChecks = false
	h.scheduleUpdates()
	if h.updates.timer != nil {
		t.Fatal("disabled checks retained idle timer")
	}
	h.updates.source = "owner/old"
	h.updates.candidate = &core.ReleaseUpdate{}
	h.scheduleUpdates()
	if h.updates.candidate != nil {
		t.Fatal("disabled automatic checks retained candidate from old repository")
	}
}

func TestUpdateInstallationWaitsForAllWindowModals(t *testing.T) {
	cases := []struct {
		name string
		open func(*App)
	}{
		{"upload confirmation", func(a *App) { a.features.uploadDraft = &uploadConfirmation{open: true} }},
		{"standalone SCP", func(a *App) { a.features.scpDraft = &standaloneSCPDraft{open: true} }},
		{"sync diagnostics", func(a *App) { a.features.sharedSync.logsOpen = true }},
		{"vault operation", func(a *App) { a.features.vaultBusy = true }},
		{"shortcut capture", func(a *App) { a.features.settingsCapture = "copy" }},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			store := &core.Store{Dir: t.TempDir()}
			owner := newApp(store, core.DefaultConfig())
			other := newApp(store, core.DefaultConfig())
			files := newApp(store, core.DefaultConfig())
			hub := &applicationHub{windows: []*App{owner, other}, fileWindow: files}
			owner.hub = hub
			owner.settingsOpen = true // The updater's own preferences window is allowed.
			if owner.updateInstallBlocker() != "" {
				t.Fatal("preferences alone blocked updater")
			}
			for _, target := range []*App{owner, other, files} {
				item.open(target)
				if owner.updateInstallBlocker() == "" {
					t.Fatal("pending modal did not block install")
				}
				target.features = featureState{}
				if owner.updateInstallBlocker() != "" {
					t.Fatal("dismissed modal kept install blocked")
				}
			}
		})
	}
}

func TestInstallConfirmationSingleFlightAndRetainsPreparedUpdate(t *testing.T) {
	for _, mode := range []string{"cancel", "error", "changed", "quitting", "accept"} {
		t.Run(mode, func(t *testing.T) {
			cfg := core.DefaultConfig()
			cfg.Settings.UpdateRepository = "owner/repo"
			a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
			other := newApp(a.store, cfg)
			a.hub = &applicationHub{windows: []*App{a, other}, config: cfg}
			p, _ := operatorPane(t, other, "active update fixture")
			p.search.open = true
			prepared := &core.PortableUpdate{}
			state := a.updateState()
			state.prepared = prepared
			state.source = "owner/repo"
			entered := make(chan struct{}, 2)
			release := make(chan struct{})
			old, oldStart := showUpdateConfirmation, startPortableUpdate
			startPortableUpdate = func(update *core.PortableUpdate) error {
				if mode != "accept" || update != prepared {
					t.Error("unexpected installer invocation")
				}
				return fmt.Errorf("fixture installer started")
			}
			showUpdateConfirmation = func(mygo.MessageOptions) (mygo.MessageResult, error) {
				entered <- struct{}{}
				<-release
				if mode == "error" {
					return mygo.MessageResult{}, fmt.Errorf("fixture error")
				}
				button := 0
				if mode == "changed" || mode == "quitting" || mode == "accept" {
					button = 1
				}
				return mygo.MessageResult{Button: button}, nil
			}
			t.Cleanup(func() { showUpdateConfirmation, startPortableUpdate = old, oldStart })
			a.prepareSettings()
			a.features.settingsTab = 4
			view := ui.NewTester(a.settingsView, 1080, 760)
			if err := view.Click("安装并重新启动…"); err != nil {
				t.Fatal(err)
			}
			a.installUpdate()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("confirmation not opened")
			}
			select {
			case <-entered:
				t.Fatal("duplicate confirmation")
			default:
			}
			if !state.confirming || state.busy {
				t.Fatal("incorrect confirmation state")
			}
			if mode == "changed" {
				state.prepared = &core.PortableUpdate{}
			}
			if mode == "quitting" {
				a.hub.quitting = true
			}
			kept := state.prepared
			close(release)
			waitOperator(t, a, func() bool { return !state.confirming && !state.busy })
			if mode == "accept" && state.status != "fixture installer started" || mode == "cancel" && !strings.Contains(state.status, "已暂缓") {
				t.Fatal(state.status)
			}
			if state.prepared != kept || state.installing || state.busy || a.configurationBusy() {
				t.Fatal("confirmation lost staged update or started install")
			}
			if (mode == "error" || mode == "changed") && state.status == "" {
				t.Fatal("confirmation failure not reported")
			}
		})
	}
}

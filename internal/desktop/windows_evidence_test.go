package desktop

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/crypto/ssh"
	"oshell-desktop/internal/core"
)

func TestEvidenceS03DefaultsOnlyAffectNewProfiles(t *testing.T) {
	a := parityApp(t)
	p := core.NewProfile()
	p.Name = "existing"
	p.Host = "fixture.invalid"
	p.Port = 2233
	p.Group = ""
	if !a.changeConfig(func(c *core.Config) error { c.Profiles = []core.Profile{p}; return nil }) {
		t.Fatal(a.status)
	}
	a.showDefaults()
	a.features.defaultsSSHPort = "2299"
	a.features.defaults.SSHUsername = "new-user"
	view := ui.NewTester(a.defaultsDialog, 800, 650)
	if err := view.Click("取消"); err != nil {
		t.Fatal(err)
	}
	saved, _ := a.store.Load()
	if saved.SessionDefaults.SSHPort == 2299 || !reflect.DeepEqual(saved.Profiles[0], p) {
		t.Fatal("cancel changed data")
	}
	a.showDefaults()
	a.features.defaultsSSHPort = "2299"
	a.features.defaults.SSHUsername = "new-user"
	view.Frame()
	if err := view.Click("保存默认属性"); err != nil {
		t.Fatal(err)
	}
	fresh := a.newProfile("ssh")
	if fresh.Port != 2299 || fresh.Username != "new-user" {
		t.Fatal("new profile ignored defaults")
	}
	a.managerSelection.Add(p.ID)
	a.duplicateManagerSelection()
	waitOperator(t, a, func() bool { return !a.configurationBusy() })
	if len(a.config.Profiles) != 2 || a.config.Profiles[1].Port != p.Port || a.config.Profiles[1].Username != p.Username || !reflect.DeepEqual(a.config.Profiles[0], p) {
		t.Fatal("copy or existing profile adopted new defaults")
	}
	archive := core.PortableArchive{Format: "OShell.sessions", Version: 2, Profiles: []core.PortableProfile{{Definition: p}}}
	merged, _, err := archive.Merge(context.Background(), a.config, "imported", false, nil)
	if err != nil || merged.Profiles[len(merged.Profiles)-1].Port != p.Port || merged.Profiles[len(merged.Profiles)-1].Username != p.Username {
		t.Fatal("import adopted defaults", err)
	}
}

func TestEvidenceC10TwoWindowModelsSharePendingTunnelOwner(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var attempts atomic.Int32
	var released atomic.Bool
	defer func() {
		if released.CompareAndSwap(false, true) {
			close(release)
		}
	}()
	a, p, _ := savedConnectionFixture(t, func(c *ssh.ServerConfig, _ ssh.Signer) {
		previous := c.PasswordCallback
		c.PasswordCallback = func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if attempts.Add(1) == 1 {
				close(entered)
				select {
				case <-release:
				case <-time.After(8 * time.Second):
				}
			}
			return previous(meta, password)
		}
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	rule := core.NewTunnel()
	rule.ListenPort = port
	rule.DestinationHost = "127.0.0.1"
	rule.DestinationPort = p.Port
	p.Tunnels = []core.TunnelRule{rule}
	other := newApp(a.store, a.config)
	other.vault = a.vault
	hub := &applicationHub{store: a.store, config: a.config, vault: a.vault, windows: []*App{a, other}}
	a.hub = hub
	other.hub = hub
	t.Cleanup(func() {
		for _, tab := range other.tabs {
			other.walk(tab.root, other.dispose)
		}
	})
	cred := core.Credentials{Password: "saved-fixture-password", Method: "password"}
	a.open(p, cred, -2)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first window not authenticating")
	}
	other.open(p, cred, -2)
	waitOperator(t, other, func() bool { return other.activePane().conn != nil })
	if other.activePane().conn.TunnelPolicy() != "skipped" {
		t.Fatal("second window duplicated pending tunnels")
	}
	released.Store(true)
	close(release)
	waitOperator(t, a, func() bool { return a.activePane().conn != nil })
	first := a.activePane()
	if first.conn.TunnelPolicy() != "owned" {
		t.Fatal("first window lost claim")
	}
	thirdProfile := p
	thirdProfile.ID = strings.ToLower(p.ID)
	other.open(thirdProfile, cred, -2)
	waitOperator(t, other, func() bool { return other.activePane().conn != nil })
	if other.activePane().conn.TunnelPolicy() != "skipped" {
		t.Fatal("connected owner not deduplicated")
	}
	first.conn.Close()
	other.open(p, cred, -2)
	waitOperator(t, other, func() bool { return other.activePane().conn != nil })
	if other.activePane().conn.TunnelPolicy() != "owned" {
		t.Fatal("released owner not recreated across windows")
	}
}

func TestEvidenceT01T02TitleSettingsPropagation(t *testing.T) {
	a := parityApp(t)
	left, _ := operatorPane(t, a, "existing")
	left.profile = core.NewProfile()
	left.profile.Name = "existing"
	left.profile.Host = "fixture.invalid"
	left.profile.Group = ""
	left.profile.Port = 22
	left.profile.Username = "fixture"
	left.profile.InitialDirectory = "."
	left.profile.TitleMode = "passive"
	if !a.changeConfig(func(c *core.Config) error { c.Profiles = []core.Profile{left.profile}; return nil }) {
		t.Fatal(a.status)
	}
	other := newApp(a.store, a.config)
	right, _ := operatorPane(t, other, "other")
	right.profile = left.profile
	hub := &applicationHub{store: a.store, config: a.config, vault: a.vault, windows: []*App{a, other}}
	a.hub = hub
	other.hub = hub
	other.vault = a.vault
	a.initializeHostname(left)
	other.initializeHostname(right)
	original := a.config.Clone()
	a.prepareSettings()
	a.localUIDraft.DefaultTitleMode = "sessionName"
	a.titleOverwrite = true
	a.closeSettings()
	if a.paneTitleMode(left) != "passive" || other.paneTitleMode(right) != "passive" {
		t.Fatal("cancel changed modes")
	}
	a.prepareSettings()
	a.localUIDraft.DefaultTitleMode = "sessionName"
	a.titleOverwrite = false
	a.applySettings()
	if a.features.settingsError != "" || a.paneTitleMode(left) != "passive" || other.paneTitleMode(right) != "passive" {
		t.Fatal("new-only changed existing", a.features.settingsError)
	}
	a.draft = a.newProfile("ssh")
	a.initConnectionEditor()
	if a.draftTitleMode != "sessionName" {
		t.Fatal("new editor ignored local title default")
	}
	a.managerSelection.Add(left.profile.ID)
	a.duplicateManagerSelection()
	waitOperator(t, a, func() bool { return !a.configurationBusy() })
	if len(a.config.Profiles) != 2 || a.sessionTitleMode(a.config.Profiles[1]) != "passive" {
		t.Fatal("copy lost title mode")
	}
	for _, mode := range []string{"activeProbe", "sessionName", "shellIntegration", "passive"} {
		a.prepareSettings()
		a.localUIDraft.DefaultTitleMode = mode
		a.titleOverwrite = true
		a.applySettings()
		if a.features.settingsError != "" || a.paneTitleMode(left) != mode || other.paneTitleMode(right) != mode {
			t.Fatal("cross-window apply", mode, a.features.settingsError)
		}
		want := mode
		if mode == "sessionName" {
			want = "shellIntegration"
		}
		if left.hostObservation.Mode != want || right.hostObservation.Mode != want {
			t.Fatal("observation mode not refreshed", mode)
		}
		left.term.Feed([]byte("user@fixture:~$ "))
		a.scheduleActiveHostProbe(left, "user@fixture:~$")
		if mode == "activeProbe" {
			if left.hostProbe.work == nil {
				t.Fatal("active probe not scheduled")
			}
		} else if left.hostProbe.work != nil || left.hostProbe.timeout != nil || left.hostProbe.token != "" {
			t.Fatal("old probe retained", mode)
		}
		if mode == "sessionName" {
			a.receiveHostIdentity(left, core.HostIdentity{Hostname: "observed", Address: "192.0.2.7"})
			if a.paneDisplayTitle(left) != "existing" || !strings.Contains(paneConnectionDetails(left), "observed") {
				t.Fatal("name mode lost details or changed title")
			}
		}
	}
	if !reflect.DeepEqual(a.config.Profiles[0], original.Profiles[0]) {
		t.Fatal("local title setting changed shared profile")
	}
	a.cancelHostProbe(left)
	other.cancelHostProbe(right)
}

func TestEvidenceM02M06ImageConfirmationAndMemory(t *testing.T) {
	for _, mode := range []string{"cancel", "failure-retry", "success", "bad-preferences"} {
		t.Run(mode, func(t *testing.T) {
			a := parityApp(t)
			p, conn := operatorPane(t, a, "image")
			var encoded bytes.Buffer
			png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2)))
			s := &imagePasteState{pane: p, term: p.term, contextToken: p.term.PasteContextToken(), data: encoded.Bytes(), open: true, remember: true}
			a.imagePaste = s
			prefPath := filepath.Join(a.store.Dir, "desktop-preferences.json")
			if mode == "bad-preferences" {
				if err := os.WriteFile(prefPath, []byte(`{"version":999}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(prefPath)
			if mode == "cancel" {
				view := ui.NewTester(a.imagePasteDialog, 700, 600)
				if err := view.Click("取消"); err != nil {
					t.Fatal(err)
				}
				view.Frame()
				if a.imagePaste != nil || conn.text() != "" || a.localUI.ImagePasteLocal {
					t.Fatal("cancel sent or remembered")
				}
				if _, err := os.Stat(filepath.Join(a.store.Dir, "clipboard-images")); !os.IsNotExist(err) {
					t.Fatal("cancel created cache")
				}
				return
			}
			if mode == "failure-retry" {
				if err := os.WriteFile(filepath.Join(a.store.Dir, "clipboard-images"), []byte("block"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			a.startImagePaste(s)
			waitOperator(t, a, func() bool { return !s.busy })
			if mode == "failure-retry" {
				if s.error == "" || conn.text() != "" || a.localUI.ImagePasteLocal {
					t.Fatal("failed operation sent or remembered")
				}
				if err := os.Remove(filepath.Join(a.store.Dir, "clipboard-images")); err != nil {
					t.Fatal(err)
				}
				a.startImagePaste(s)
				waitOperator(t, a, func() bool { return !s.busy })
			}
			waitOperator(t, a, func() bool { return strings.Contains(conn.text(), "clipboard-images") })
			if a.imagePaste != nil {
				t.Fatal("completed operation retained retry dialog")
			}
			if mode == "bad-preferences" {
				after, _ := os.ReadFile(prefPath)
				if !bytes.Equal(before, after) || a.localUI.ImagePasteLocal {
					t.Fatal("unknown preferences overwritten")
				}
			} else if !a.localUI.ImagePasteLocal {
				t.Fatal("success not remembered")
			}
			// Outer preferences cancellation must not clear a successful remembered rule.
			remembered := a.localUI.clone()
			a.prepareSettings()
			a.localUIDraft.ImagePasteLocal = false
			a.closeSettings()
			if !reflect.DeepEqual(a.localUI, remembered) {
				t.Fatal("outer cancel changed reminder")
			}
		})
	}
	a := parityApp(t)
	p := core.NewProfile()
	p.Host = "fixture.invalid"
	p.Group = ""
	p.Name = "source"
	if !a.changeConfig(func(c *core.Config) error { c.Profiles = []core.Profile{p}; return nil }) {
		t.Fatal(a.status)
	}
	pref := a.localUI.clone()
	pref.ImagePasteSSH = map[string]imagePasteRule{strings.ToLower(p.ID): {Binding: imagePasteBinding(p), Directory: "/tmp", Name: p.Name}}
	if !a.saveLocalPreferences(pref) {
		t.Fatal(a.status)
	}
	a.managerSelection.Add(p.ID)
	a.duplicateManagerSelection()
	waitOperator(t, a, func() bool { return !a.configurationBusy() })
	if _, ok := a.localUI.ImagePasteSSH[strings.ToLower(a.config.Profiles[1].ID)]; ok {
		t.Fatal("copy inherited no-confirm reminder")
	}
	changed := p
	changed.Proxy.Kind = "socks5"
	changed.Proxy.Host = "proxy.invalid"
	changed.Proxy.Port = 1080
	if imagePasteBinding(changed) == a.localUI.ImagePasteSSH[strings.ToLower(p.ID)].Binding {
		t.Fatal("route change reused reminder")
	}
}

func TestEvidenceC01ReconnectCancellationKeepsLivePane(t *testing.T) {
	a := parityApp(t)
	p, conn := operatorPane(t, a, "existing")
	p.profile = core.NewProfile()
	p.profile.Host = "127.0.0.1"
	p.profile.Username = "fixture"
	original := p.term
	a.reconnectActive()
	waitOperator(t, a, func() bool { return !a.features.authPreparing })
	if !a.authOpen {
		t.Fatal("expected unsaved authentication prompt")
	}
	view := ui.NewTester(a.dialogs, 800, 650)
	if err := view.Click("取消"); err != nil {
		t.Fatal(err)
	}
	view.Frame()
	time.Sleep(20 * time.Millisecond)
	a.drain()
	if p.closed || a.activePane() != p || p.term != original || a.authOpen {
		t.Fatal("cancel replaced live pane")
	}
	p.term.Send([]byte("still-connected"))
	waitOperator(t, a, func() bool { return conn.text() == "still-connected" })
}

func TestEvidenceA05WidthSettingOnlyNewTerminal(t *testing.T) {
	a := parityApp(t)
	create := func() *pane {
		p := core.NewProfile()
		p.Kind = "blank"
		p.Name = "width fixture"
		a.open(p, core.Credentials{}, -2)
		target := a.activePane()
		waitOperator(t, a, func() bool { return target.term != nil })
		t.Cleanup(func() {
			target.term.Close()
			if target.cancel != nil {
				target.cancel()
			}
		})
		target.term.Resize(20, 5)
		return target
	}
	old := create()
	a.prepareSettings()
	a.features.settingsDraft.AmbiguousCharactersAreWide = true
	a.applySettings()
	if a.features.settingsError != "" || !a.config.Settings.AmbiguousCharactersAreWide {
		t.Fatal("settings not applied", a.features.settingsError)
	}
	next := create()
	for _, p := range []*pane{old, next} {
		p.term.Feed([]byte("\x1bc·X"))
		if got := p.term.Text(); got != "·X" {
			t.Fatal("copy text mutated", got)
		}
	}
	// Overwrite the second cell: it is X in the old narrow terminal, but the
	// wide character's continuation in the new terminal. This observes actual VT geometry.
	old.term.Feed([]byte("\x1b[1;2HQ"))
	next.term.Feed([]byte("\x1b[1;2HQ"))
	if old.term.Text() != "·Q" || next.term.Text() != " QX" {
		t.Fatal("width policy was reapplied to old terminal or absent on new", old.term.Text(), next.term.Text())
	}
}

func TestEvidenceM06RemoteReminderOnlyAfterSuccessfulUpload(t *testing.T) {
	a, profile, _ := savedConnectionFixtureWithHandlers(t, evidenceImagePermissions)
	a.open(profile, core.Credentials{Password: "saved-fixture-password", Method: "password"}, -2)
	waitOperator(t, a, func() bool { p := a.activePane(); return p != nil && p.conn != nil && p.term != nil })
	p := a.activePane()
	var encoded bytes.Buffer
	png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	state := &imagePasteState{pane: p, term: p.term, conn: p.conn, identity: p.hostObservation.Identity, hostEpoch: p.hostProbe.epoch, contextToken: p.term.PasteContextToken(), data: encoded.Bytes(), open: true, remember: true, remote: true, directory: "relative-not-allowed"}
	a.imagePaste = state
	a.startImagePaste(state)
	waitOperator(t, a, func() bool { return !state.busy })
	if state.error == "" || len(a.localUI.ImagePasteSSH) != 0 {
		t.Fatal("failed remote upload remembered")
	}
	state.directory = "/fixture-folder"
	a.startImagePaste(state)
	waitOperator(t, a, func() bool { return !state.busy })
	rule, ok := a.localUI.ImagePasteSSH[strings.ToLower(profile.ID)]
	if state.error != "" || a.imagePaste != nil || !ok || rule.Directory != "/fixture-folder" || rule.Binding != imagePasteBinding(profile) {
		t.Fatal("successful remote retry not remembered", state.error)
	}
}

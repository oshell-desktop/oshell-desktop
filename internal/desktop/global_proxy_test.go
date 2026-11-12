package desktop

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestGlobalProxyDialogPersistsOnlyLocalPolicy(t *testing.T) {
	a, p, _ := savedConnectionFixture(t)
	proxy := core.NewProxyProfile()
	proxy.Name = "测试跳板"
	proxy.Settings.Host = "127.0.0.1"
	a.config.Proxies = []core.ProxyProfile{proxy}
	before, _ := json.Marshal(a.config)
	storedBefore, err := os.ReadFile(filepath.Join(a.store.Dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	a.showGlobalProxy()
	tt := ui.NewTester(a.view, 880, 560)
	if !a.nonAuthenticationModalActive() {
		t.Fatal("dialog did not guard workspace")
	}
	if err := tt.Click("启用全局代理"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("应用全局代理"); err != nil {
		t.Fatal(err)
	}
	if !a.features.globalProxy.open || a.features.globalProxy.error == "" {
		t.Fatal("empty enabled route accepted")
	}
	if err := tt.Click("全局代理路线"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("测试跳板 · socks5"); err != nil {
		t.Fatal(err)
	}
	snapshot(t, tt, "global-proxy-settings.png")
	if err := tt.Click("应用全局代理"); err != nil {
		t.Fatal(err)
	}
	settings, err := core.LoadGlobalProxy(a.store.Dir)
	if err != nil || !settings.Enabled || settings.ProxyID != proxy.ID() || a.features.globalProxy.open {
		t.Fatal(settings, err)
	}
	other := newApp(a.store, a.config)
	route, err := other.connectionProfile(p)
	if err != nil || len(route.RuntimeProxyRoute) != 1 || route.RuntimeProxyRoute[0].ID != proxy.ID() {
		t.Fatal("other window did not load policy", err)
	}
	a.showGlobalProxy()
	a.features.globalProxy.draft.Enabled = false
	tt.Frame()
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	settings, err = core.LoadGlobalProxy(a.store.Dir)
	if err != nil || !settings.Enabled {
		t.Fatal("cancel changed policy")
	}
	tt.SetSize(1280, 720)
	tt.Frame()
	if err := tt.RightClick("全局代理"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("全局代理设置…"); err != nil || !a.features.globalProxy.open {
		t.Fatal("settings menu", err)
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("全局代理"); err != nil {
		t.Fatal(err)
	}
	route, err = other.connectionProfile(p)
	if err != nil || len(route.RuntimeProxyRoute) != 0 {
		t.Fatal("other window retained disabled override")
	}
	after, _ := json.Marshal(a.config)
	storedAfter, err := os.ReadFile(filepath.Join(a.store.Dir, "config.json"))
	if err != nil || !bytes.Equal(before, after) || !bytes.Equal(storedBefore, storedAfter) {
		t.Fatal("global setting changed shared configuration")
	}
	found := false
	for _, item := range a.headerItems("工具") {
		if item.Label == "全局代理设置…" {
			found = true
		}
	}
	if !found {
		t.Fatal("global proxy menu missing")
	}
}

func TestGlobalProxyDesktopNewConnectionsAndReusedChannels(t *testing.T) {
	a, p, accepted := savedConnectionFixture(t)
	gateway, hop, gatewayAccepted := savedConnectionFixture(t)
	key, err := os.ReadFile(gateway.trust.Path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(a.trust.Path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Write(key)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	proxy := core.NewProxyProfile()
	proxy.Settings.Kind = "jump"
	proxy.Settings.Host = hop.Host
	proxy.Settings.Port = hop.Port
	proxy.Settings.Username = hop.Username
	proxy.Settings.SSHAuthentication = "password"
	a.config.Proxies = []core.ProxyProfile{proxy}
	if err := a.vault.PutFor(p, "saved-fixture-password", p.Identity()); err != nil {
		t.Fatal(err)
	}
	if err := a.vault.PutFor(core.ProxyCredentialProfile(proxy.Settings), "saved-fixture-password", hop.Identity()); err != nil {
		t.Fatal(err)
	}
	a.request(p, 0)
	waitOperator(t, a, func() bool {
		return a.activePane() != nil && a.activePane().conn != nil && a.activePane().term != nil && strings.Contains(a.activePane().term.Text(), "SAVED_CREDENTIAL_READY")
	})
	original := a.activePane()
	_, port := original.conn.ObservedPeer()
	if port != p.Port {
		t.Fatal("initial route wrong")
	}
	policy := core.GlobalProxySettings{Version: 1, Enabled: true, ProxyID: proxy.ID()}
	if err := policy.Save(a.store.Dir); err != nil {
		t.Fatal(err)
	}
	a.request(p, 0)
	waitOperator(t, a, func() bool {
		return a.activePane() != original && a.activePane().conn != nil && a.activePane().term != nil && strings.Contains(a.activePane().term.Text(), "SAVED_CREDENTIAL_READY")
	})
	_, port = a.activePane().conn.ObservedPeer()
	if port != hop.Port || gatewayAccepted.Load() != 1 || accepted.Load() != 2 {
		t.Fatal("new SSH did not use global route", a.status)
	}
	// Corrupt local policy blocks new connections, but must not block clones.
	if err := os.WriteFile(filepath.Join(a.store.Dir, "global-proxy.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	count := len(a.tabs)
	a.request(p, 0)
	a.open(p, core.Credentials{Password: "saved-fixture-password"}, 0)
	if len(a.tabs) != count || accepted.Load() != 2 || !strings.Contains(a.status, "连接已停止") {
		t.Fatal("new connection bypassed invalid policy")
	}
	a.openConnection(p, core.Credentials{}, sessionOpenOptions{}, original.conn)
	waitOperator(t, a, func() bool { return len(a.tabs) == count+1 && a.activePane().conn != nil })
	_, port = a.activePane().conn.ObservedPeer()
	if port != p.Port || accepted.Load() != 2 || gatewayAccepted.Load() != 1 {
		t.Fatal("existing SSH clone rerouted")
	}
	files := p
	files.Kind = "sftp"
	files.InitialDirectory = "/"
	a.openConnection(files, core.Credentials{}, sessionOpenOptions{}, original.conn)
	waitOperator(t, a, func() bool {
		return a.activePane().files != nil && !a.activePane().files.busy && len(a.activePane().files.entries) == 1
	})
	_, port = a.activePane().conn.ObservedPeer()
	if port != p.Port || accepted.Load() != 2 {
		t.Fatal("existing SFTP clone rerouted")
	}
	if err := policy.Save(a.store.Dir); err != nil {
		t.Fatal(err)
	}
	files.ID = core.NewID()
	a.config.Profiles = append(a.config.Profiles, files)
	if err := a.vault.PutFor(files, "saved-fixture-password", files.Identity()); err != nil {
		t.Fatal(err)
	}
	a.request(files, 0)
	waitOperator(t, a, func() bool {
		return a.activePane().files != nil && !a.activePane().files.busy && len(a.activePane().files.entries) == 1 && accepted.Load() == 3
	})
	_, port = a.activePane().conn.ObservedPeer()
	if port != hop.Port || gatewayAccepted.Load() != 2 {
		t.Fatal("new SFTP did not use global route")
	}
}

package desktop

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestAdvancedEditorRoundtrip(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p := core.NewProfile()
	p.Host = "example.invalid"
	p.Username = "tester"
	a.edit(p)
	a.draft.Proxy = core.ProxySettings{Kind: "socks5", Host: "127.0.0.1", Port: 1080}
	a.features.proxyPort = "1081"
	a.draft.KeepAlive.IdleEnabled = true
	a.draft.KeepAlive.IdleText = `\n`
	a.features.idleInterval = "120"
	r := core.NewTunnel()
	a.features.tunnels = []tunnelEditor{{r, "8081", "443"}}
	tt := ui.NewTester(a.view, 1280, 820)
	a.features.editTab = 1
	tt.Frame()
	snapshot(t, tt, "proxy-settings.png")
	a.features.editTab = 2
	tt.Frame()
	snapshot(t, tt, "tunnel-settings.png")
	a.features.editTab = 3
	tt.Frame()
	snapshot(t, tt, "keepalive-settings.png")
	if e := tt.Click("保存"); e != nil {
		t.Fatal(e)
	}
	cfg, e := a.store.Load()
	if e != nil {
		t.Fatal(e)
	}
	saved := cfg.Profiles[0]
	resolved, err := cfg.ConnectionProfile(saved)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.RuntimeProxyRoute) != 1 || resolved.RuntimeProxyRoute[0].Port != 1081 || len(saved.Tunnels) != 1 || saved.Tunnels[0].DestinationPort != 443 || saved.KeepAlive.IdleInterval != 120 {
		t.Fatalf("%+v", saved)
	}
}
func TestQuickCommandsAndReferences(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p := core.NewProfile()
	p.Host = "example.invalid"
	p.Username = "tester"
	a.config.Profiles = []core.Profile{p}
	if !a.changeConfig(func(c *core.Config) error { return c.AddLink(p, "常用") }) {
		t.Fatal(a.status)
	}
	a.openCommands()
	a.features.commandEdit = &core.QuickCommand{ID: core.NewID(), Name: "磁盘空间", Text: "df -h"}
	tt := ui.NewTester(a.view, 1280, 820)
	snapshot(t, tt, "quick-commands.png")
	if e := tt.Click("保存命令"); e != nil {
		t.Fatal(e)
	}
	cfg, e := a.store.Load()
	if e != nil || len(cfg.QuickCommands) != 1 || len(cfg.Links) != 1 {
		t.Fatal(cfg, e)
	}
	archive, e := core.ExportArchive(cfg)
	if e != nil {
		t.Fatal(e)
	}
	imported, e := core.ImportArchive(archive)
	if e != nil {
		t.Fatal(e)
	}
	if len(imported.Links) != 1 || imported.Links[0].ProfileID != imported.Profiles[0].ID {
		t.Fatal("reference IDs not remapped")
	}
	a.managerGroup = "Links/常用"
	rows := a.managerRows()
	if len(rows) != 2 || !rows[0].parent || rows[1].linkID == "" {
		t.Fatal(rows)
	}
}
func TestSearchAndVaultViews(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, _ := operatorPane(t, a, "search-view")
	p.term.Feed([]byte("正常输出\r\nERROR: connection reset\r\nerror: disk full"))
	p.search.query = "error"
	a.openSearch()
	tt := ui.NewTester(a.view, 1280, 820)
	until := time.Now().Add(3 * time.Second)
	for p.search.busy && time.Now().Before(until) {
		tt.Frame()
		time.Sleep(time.Millisecond)
	}
	if len(p.search.matches) != 2 {
		t.Fatal(p.search.matches, p.search.err)
	}
	snapshot(t, tt, "terminal-search.png")
	a.closeTerminalSearch(p)
	a.showVault()
	tt.Frame()
	snapshot(t, tt, "password-protection.png")
	a.features.vaultAction = "set"
	a.features.master = "test-master-password"
	a.features.masterAgain = a.features.master
	a.runVaultAction()
	until = time.Now().Add(3 * time.Second)
	for a.features.vaultBusy && time.Now().Before(until) {
		tt.Frame()
		time.Sleep(time.Millisecond)
	}
	if a.features.vaultError != "" {
		t.Fatal(a.features.vaultError)
	}
	_, locked := a.vault.State()
	if locked {
		t.Fatal("new vault unexpectedly locked")
	}
	if _, e := core.ReadBounded(filepath.Join(a.store.Dir, "credentials.json")); e != nil {
		t.Fatal(e)
	}
}

func TestRearrangeDetachAndReorderPreservePanes(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, q := &pane{id: "p"}, &pane{id: "q"}
	first := &tab{id: "first", name: "first", active: p, root: &node{id: "root", first: &node{id: "p", pane: p}, second: &node{id: "q", pane: q}}}
	other := &tab{id: "other", active: &pane{id: "r"}, root: &node{id: "r", pane: &pane{id: "r"}}}
	a.tabs = []*tab{first, other}
	a.arrangeActive("vertical")
	if !first.root.vertical || first.root.first.pane != p || first.root.second.pane != q {
		t.Fatal("arrangement replaced the terminal models")
	}
	a.detachActive()
	if first.root.pane != q || a.activePane() != p || a.activeTab() == first {
		t.Fatal("detachment lost an existing pane")
	}
	active := a.activeTab()
	a.moveTabBefore(active.id, "first")
	if a.tabs[0] != active || a.activeTab() != active {
		t.Fatal("reordering changed active terminal")
	}
}

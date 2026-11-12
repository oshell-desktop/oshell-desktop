package desktop

import (
	"bytes"
	"image/png"
	"oshell-desktop/internal/core"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestQuickConnectionSessionMarkers(t *testing.T) {
	cfg := core.DefaultConfig()
	for _, kind := range []string{"ftp", "local", "sftp", "ssh"} {
		p := core.NewProfile()
		p.Kind, p.Name, p.Group, p.QuickConnect = kind, kind+" fixture", "group", true
		cfg.Profiles = append(cfg.Profiles, p)
	}
	for _, mode := range []string{"light", "dark"} {
		cfg.Settings.InterfaceTheme = mode
		hub := &applicationHub{config: cfg}
		group := hub.nativeQuickConnectionMenu().Submenu[0]
		if group.Label != "group" || group.Icon != sessionIconFor("", false, true, false).menuIcon(mode == "dark") {
			t.Fatalf("%s: missing folder icon", mode)
		}
		seen := map[string]bool{}
		for i, item := range group.Submenu {
			p := cfg.Profiles[i]
			if item.Label != p.Name || item.Icon == nil || item.Icon != sessionIconFor(p.Kind, false, false, false).menuIcon(mode == "dark") {
				t.Fatalf("%s: wrong label or icon for %s", mode, p.Kind)
			}
			img, err := png.Decode(bytes.NewReader(item.Icon.PNG))
			if err != nil || img.Bounds().Dx() != 2*item.Icon.Width || img.Bounds().Dy() != 2*item.Icon.Height {
				t.Fatalf("%s: missing high-density bitmap for %s", mode, p.Kind)
			}
			if _, _, _, alpha := img.At(0, 0).RGBA(); alpha != 0 {
				t.Fatalf("%s: %s marker covers native menu background", mode, p.Kind)
			}
			seen[string(item.Icon.PNG)] = true
		}
		if len(seen) != len(cfg.Profiles) {
			t.Fatalf("%s: sessions do not have distinct markers", mode)
		}
	}
}

func TestQuickConnectionChoiceWithEqualNames(t *testing.T) {
	cfg := core.DefaultConfig()
	for _, kind := range []string{"ssh", "sftp"} {
		p := core.NewProfile()
		p.Kind, p.Name, p.Group, p.Host = kind, "same", "", "fixture.invalid"
		cfg.Profiles = append(cfg.Profiles, p)
	}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	view := ui.NewTester(a.view, 1000, 560)
	if err := view.Click("快捷连接"); err != nil {
		t.Fatal(err)
	}
	// The first SSH entry disappears while its menu is open. The SFTP
	// session now at the same index has the same displayed name.
	a.config.Profiles = a.config.Profiles[1:]
	if err := view.ChooseMenuItem("same"); err != nil {
		t.Fatal(err)
	}
	if a.authOpen || len(a.tabs) != 0 {
		t.Fatal("stale selection opened the other same-name session")
	}
	if err := view.Click("快捷连接"); err != nil {
		t.Fatal(err)
	}
	if err := view.ChooseMenuItem("same"); err != nil || !a.authOpen || a.pendingProfile.ID != a.config.Profiles[0].ID {
		t.Fatalf("reopened menu failed to select the remaining SFTP session: %v", err)
	}
}

func TestQuickConnectionMenusShareNestedFilteredTree(t *testing.T) {
	makeProfile := func(name, group string, visible bool) core.Profile {
		p := core.NewProfile()
		p.Name = name
		p.Group = group
		p.Host = "fixture.invalid"
		p.QuickConnect = visible
		return p
	}
	profiles := []core.Profile{makeProfile("node10", "生产/数据库", true), makeProfile("node2", "生产/数据库", true), makeProfile("root", "", true), makeProfile("hidden", "不显示/子目录", false)}
	tree := quickConnectionTree(profiles, nil)
	if len(tree) != 2 || tree[0].label != "生产" || tree[1].id != profiles[2].ID {
		t.Fatal("root/folder hierarchy incorrect")
	}
	folder := tree[0].children
	if len(folder) != 1 || folder[0].label != "数据库" || len(folder[0].children) != 2 {
		t.Fatal("nested folder missing")
	}
	if folder[0].children[0].id != profiles[1].ID {
		t.Fatal("numeric ordering missing")
	}
	cfg := core.DefaultConfig()
	cfg.Profiles = profiles
	hub := &applicationHub{config: cfg}
	menu := hub.nativeQuickConnectionMenu()
	if menu.Label != "快捷连接" || menu.Submenu[0].Submenu[0].Submenu[0].Label != "node2" {
		t.Fatal("native menu differs from tree")
	}
	if menu.Submenu[len(menu.Submenu)-1].Label != "管理会话…" {
		t.Fatal("manager action missing")
	}
}

func TestQuickConnectionOrderMatchesManager(t *testing.T) {
	cfg := core.DefaultConfig()
	for _, row := range [][3]string{{"node10", "rack2", "ssh"}, {"node2", "rack2", "ssh"}, {"same", "rack2", "ssh"}, {"same", "rack2", "ftp"}, {"pinned", "", "ssh"}, {"leaf", "rack10", "ssh"}} {
		p := core.NewProfile()
		p.Name, p.Group, p.Kind, p.QuickConnect = row[0], row[1], row[2], true
		cfg.Profiles = append(cfg.Profiles, p)
	}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	pinned := cfg.Profiles[4]
	a.localUI.Pins = map[string]bool{"d:rack10": true, pinKey(sessionRow{profile: pinned}): true}
	tree := quickConnectionTree(cfg.Profiles, a.localUI.Pins)
	if len(tree) != 3 || tree[0].label != "rack10" || tree[1].id != pinned.ID || tree[2].label != "rack2" {
		t.Fatal("root did not honor pinned folders and sessions")
	}
	h := &applicationHub{config: cfg, windows: []*App{a}}
	menu := h.nativeQuickConnectionMenu()
	if menu.Submenu[0].Label != tree[0].label || menu.Submenu[1].Label != tree[1].label {
		t.Fatal("native quick connections lost pin order")
	}
	a.managerGroup = "rack2"
	var managerIDs, quickIDs []string
	for _, row := range a.buildManagerRows() {
		if !row.folder {
			managerIDs = append(managerIDs, row.id)
		}
	}
	for _, node := range tree[2].children {
		quickIDs = append(quickIDs, node.id)
	}
	if !slices.Equal(quickIDs, managerIDs) || quickIDs[0] != cfg.Profiles[1].ID || quickIDs[2] != cfg.Profiles[2].ID {
		t.Fatal("numeric or equal-name ordering differs from session manager")
	}
	a.features.switcher.scope = "saved"
	saved := a.switchEntries()
	if saved[0].Profile.ID != pinned.ID {
		t.Fatal("quick switch ignored root pin")
	}
	a.features.switcher.query = "node"
	saved = a.switchEntries()
	if len(saved) != 2 || saved[0].Title != "node2" || saved[1].Title != "node10" {
		t.Fatal("quick switch search lost numeric order")
	}
}
func TestStaleQuickConnectionCannotConnectDeletedOrDisabledProfile(t *testing.T) {
	cfg := core.DefaultConfig()
	p := core.NewProfile()
	p.Host = "fixture.invalid"
	p.QuickConnect = false
	cfg.Profiles = []core.Profile{p}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	for _, id := range []string{p.ID, "deleted"} {
		a.connectQuickID(id)
		if a.authOpen || len(a.tabs) != 0 || a.status != "快捷会话已被删除或停用，请重新选择" {
			t.Fatal("stale menu opened connection")
		}
	}
}

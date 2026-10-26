package desktop

import (
	"github.com/egoist/mygo/ui"
	"os"
	"oshell-desktop/internal/core"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLinkEditorAndEmptyBarMenu(t *testing.T) {
	cfg := core.DefaultConfig()
	profile := core.NewProfile()
	profile.Name = "Source"
	profile.Host = "example.invalid"
	profile.Username = "tester"
	cfg.Profiles = []core.Profile{profile}
	cfg.AddLink(profile, "")
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	tt := ui.NewTester(a.view, 1200, 800)
	bar, ok := tt.Find("快捷链接栏")
	if !ok {
		t.Fatal("missing links bar")
	}
	tt.RightClickAt(bar.X+bar.W-30, bar.Y+bar.H/2)
	want := []string{"添加链接", "添加已有会话链接…", "新增文件夹", "-", "打开 Links 文件夹", "隐藏链接栏"}
	if !reflect.DeepEqual(tt.Menu(), want) {
		t.Fatal(tt.Menu())
	}
	tt.CloseMenu()
	a.editLink(cfg.Links[0].ID)
	tt.Frame()
	if !tt.HasText("快捷引用属性") {
		t.Fatal(tt.Texts())
	}
	a.features.linkEditor.name = "Alias"
	a.applyLinkEditor()
	if a.config.Links[0].Name != "Alias" || a.config.Profiles[0].Name != "Source" {
		t.Fatal("alias editor changed source")
	}
	a.removeLink(cfg.Links[0].ID)
	if len(a.config.Links) != 0 || len(a.config.Profiles) != 1 {
		t.Fatal("removal changed source")
	}
}
func TestLockedWorkspaceHidesOpenLinkEditor(t *testing.T) {
	cfg := core.DefaultConfig()
	profile := core.NewProfile()
	profile.Name = "PRIVATE_LINK_SOURCE"
	profile.Host = "private.invalid"
	profile.Username = "tester"
	cfg.Profiles = []core.Profile{profile}
	cfg.AddLink(profile, "")
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	a.editLink(cfg.Links[0].ID)
	if e := a.vault.SetMaster("fixture-password"); e != nil {
		t.Fatal(e)
	}
	a.vault.Lock()
	a.workspaceAuthorized = false // Simulate a fresh, not-yet-authorized startup.
	tt := ui.NewTester(a.view, 1000, 700)
	if tt.HasText("快捷引用属性") || tt.HasText("PRIVATE_LINK_SOURCE") {
		t.Fatal("locked workspace exposed existing editor", tt.Texts())
	}
	a.request(profile, 0)
	if len(a.tabs) != 0 || a.authOpen {
		t.Fatal("stale menu opened a session while locked")
	}
	a.removeLink(cfg.Links[0].ID)
	if len(a.config.Links) != 1 {
		t.Fatal("stale menu action modified locked configuration")
	}
	if !tt.HasText("OShell 已锁定") {
		t.Fatal(tt.Texts())
	}
}

func TestCurrentLinkUsesLatestSavedProfileAndIsIdempotent(t *testing.T) {
	cfg := core.DefaultConfig()
	profile := core.NewProfile()
	profile.Host = "example.invalid"
	profile.Username = "tester"
	profile.Name = "Old name"
	saved := profile
	saved.Name = "Latest name"
	cfg.Profiles = []core.Profile{saved}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	p := &pane{profile: profile}
	a.tabs = []*tab{{active: p}}
	a.current = 0
	a.addCurrentLink()
	if len(a.config.Links) != 1 || a.config.Links[0].Name != "Latest name" {
		t.Fatal("used stale live-pane name")
	}
	a.changeConfig(func(config *core.Config) error { config.Links[0].Name = "Custom alias"; return nil })
	before, e := os.Stat(filepath.Join(a.store.Dir, "config.json"))
	if e != nil {
		t.Fatal(e)
	}
	a.addCurrentLink()
	after, e := os.Stat(filepath.Join(a.store.Dir, "config.json"))
	if e != nil {
		t.Fatal(e)
	}
	if !os.SameFile(before, after) || len(a.config.Links) != 1 || a.config.Links[0].Name != "Custom alias" {
		t.Fatal("duplicate addition rewrote or renamed reference")
	}
	a.config.Settings.LinksVisible = false
	a.addCurrentLink()
	if !a.config.Settings.LinksVisible || len(a.config.Links) != 1 {
		t.Fatal("existing link did not reveal bar")
	}
}
func TestLinkedSessionManagerSelectionUsesID(t *testing.T) {
	cfg := core.DefaultConfig()
	first := core.NewProfile()
	first.Name = "Duplicate"
	first.Host = "one.invalid"
	first.Username = "tester"
	first.Group = "Links"
	second := first
	second.ID = core.NewID()
	second.Host = "two.invalid"
	cfg.Profiles = []core.Profile{first, second}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	a.showLinkedSessionInManager(second)
	rows := a.selectedManagerRows()
	if len(rows) != 1 || rows[0].profile.ID != second.ID {
		t.Fatal("selected matching name instead of exact session")
	}
}

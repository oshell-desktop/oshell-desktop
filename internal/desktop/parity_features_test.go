package desktop

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestTabHoverCopiesWithoutChangingFocus(t *testing.T) {
	a := parityApp(t)
	p := core.NewProfile()
	p.Name = "hover fixture"
	p.Host = "192.0.2.9"
	p.Username = "tester"
	workspace := &tab{id: core.NewID(), name: p.Name, root: &node{pane: &pane{profile: p}}}
	workspace.active = workspace.root.pane
	a.tabs = []*tab{workspace}
	input := "draft"
	view := ui.NewTester(func(c *ui.Context) {
		a.observeTabDetails(c)
		ui.Column(c).Fill().Children(func() {
			anchor := ui.Button(c, "Hover tab")
			ui.TextInput(c, &input).Label("Original focus").AutoFocus()
			a.tabDetailsHover(c, anchor, workspace)
		})
	}, 600, 600)
	r, _ := view.Find("Hover tab")
	view.Move(r.X+r.W/2, r.Y+r.H/2)
	view.Frame()
	time.Sleep(520 * time.Millisecond)
	view.Frame()
	if err := view.Click("复制会话名称"); err != nil {
		t.Fatal(err)
	}
	if view.Clipboard() != p.Name {
		t.Fatal("wrong copied field", view.Clipboard())
	}
	snapshot(t, view, "tab-hover-details.png")
	view.Type("x")
	if !strings.Contains(input, "x") {
		t.Fatal("copy stole input focus")
	}
	if view.HasText("点击右侧按钮复制对应字段") || view.HasText("已复制会话名称") {
		t.Fatal("typing did not dismiss hover card")
	}
}

func TestParityPanelsAndBundledCascadia(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		a := parityApp(t)
		a.config.Settings.InterfaceTheme = theme
		a.features.sharedSync.options.Policy = core.DefaultSyncPolicy()
		view := ui.NewTester(func(c *ui.Context) {
			a.theme(c)
			ui.Column(c).Fill().Padding(18).Gap(9).Background(c.Theme().Background).Children(func() { a.syncPolicySettings(c) })
		}, 700, 680)
		snapshot(t, view, "sync-policy-"+theme+".png")
	}
	regular := ui.Shape("Aa01", ui.Font{Family: "Cascadia Code", Size: 18})
	for _, font := range []ui.Font{{Weight: 400}, {Weight: 700}, {Weight: 400, Italic: true}, {Weight: 700, Italic: true}} {
		font.Family = "Cascadia Code"
		font.Size = 18
		if len(ui.Shape("Aa01", font)) == 0 {
			t.Fatal("missing bundled Cascadia style", font)
		}
		if (font.Weight == 700 || font.Italic) && reflect.DeepEqual(regular, ui.Shape("Aa01", font)) {
			t.Fatal("Cascadia style fell back to regular", font)
		}
	}
}

func parityApp(t *testing.T) *App {
	t.Helper()
	s := &core.Store{Dir: t.TempDir()}
	cfg := core.DefaultConfig()
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}
	return newApp(s, cfg)
}
func TestSessionPinsRenameMoveAndSidebarMultiSelection(t *testing.T) {
	a := parityApp(t)
	p := core.NewProfile()
	p.Group = ""
	p.Name = "root"
	p.Host = "fixture.invalid"
	if !a.changeConfig(func(c *core.Config) error {
		c.Profiles = []core.Profile{p}
		c.Directories = []string{"group"}
		return nil
	}) {
		t.Fatal(a.status)
	}
	row, _ := a.sidebarRow(p.ID)
	dir, _ := a.sidebarRow("folder:group")
	if !a.setSessionPins([]sessionRow{row, dir}, true) {
		t.Fatal(a.status)
	}
	a.sidebar.visible = []string{dir.id, p.ID}
	a.selectSidebar(dir.id, 0)
	a.selectSidebar(p.ID, ui.Ctrl)
	a.withSidebarContext(p.ID, func() {
		if len(a.selectedManagerRows()) != 2 {
			t.Fatal("sidebar multi-selection discarded")
		}
	})
	if !a.renameSessionRow(dir, "renamed") || !a.localUI.Pins["d:renamed"] || a.localUI.Pins["d:group"] {
		t.Fatal("rename lost pin", a.status)
	}
	if !a.changeConfig(func(c *core.Config) error { c.Profiles[0].Group = "renamed"; return nil }) {
		t.Fatal(a.status)
	}
	if a.localUI.Pins["p:"+strings.ToLower(p.ID)] {
		t.Fatal("moved session kept root pin")
	}
	loaded := loadLocalPreferences(a.store.Dir)
	if !loaded.Pins["d:renamed"] {
		t.Fatal("pin not persisted")
	}
}
func TestLocalTitleAggregationAndStaleSettingsDraft(t *testing.T) {
	a := parityApp(t)
	p := core.NewProfile()
	p.Name = "one"
	p.Group = ""
	q := core.NewProfile()
	q.Name = "two"
	q.Group = ""
	a.config.Profiles = []core.Profile{p, q}
	left, right := &pane{profile: p}, &pane{profile: q}
	a.localUI.TitleModes = map[string]string{strings.ToLower(p.ID): "sessionName", strings.ToLower(q.ID): "sessionName"}
	workspace := &tab{root: &node{first: &node{pane: left}, second: &node{pane: right}}, active: left}
	a.tabs = []*tab{workspace}
	if got := a.displayTabTitle(workspace); got != "one · two" {
		t.Fatal(got)
	}
	workspace.active = right
	if a.displayTabTitle(workspace) != "one · two" {
		t.Fatal("focus changed aggregate")
	}
	right.profile = p
	if a.displayTabTitle(workspace) != "one ×2" {
		t.Fatal(a.displayTabTitle(workspace))
	}
	a.prepareSettings()
	changed := a.localUI.clone()
	changed.ReduceMotion = true
	if !a.saveLocalPreferences(changed) {
		t.Fatal(a.status)
	}
	a.applySettings()
	if !strings.Contains(a.features.settingsError, "其他窗口") {
		t.Fatal("stale local preference draft accepted", a.features.settingsError)
	}
}
func TestFactoryLocalPresentationDoesNotMutateStoredOrReferencedData(t *testing.T) {
	a := parityApp(t)
	p := core.NewProfile()
	p.Kind = "local"
	p.Name = "本地终端"
	p.Group = "本机"
	a.config.Profiles = []core.Profile{p}
	a.config.Directories = []string{"本机"}
	old := a.config.Clone()
	if len(a.presentationConfig().Profiles) != 0 || !reflect.DeepEqual(old, a.config) {
		t.Fatal("factory hiding mutated stored configuration")
	}
	a.config.Links = []core.SessionLink{{ID: core.NewID(), ProfileID: p.ID, Name: p.Name}}
	if len(a.presentationConfig().Profiles) != 1 {
		t.Fatal("referenced default hidden")
	}
}
func TestSyncPolicyUIEditsAndManualScheduling(t *testing.T) {
	a := parityApp(t)
	a.features.sharedSync.options.Policy = core.DefaultSyncPolicy()
	view := ui.NewTester(func(c *ui.Context) { ui.Column(c).Fill().Children(func() { a.syncPolicySettings(c) }) }, 650, 700)
	if err := view.Click("快速命令"); err != nil {
		t.Fatal(err)
	}
	if !a.features.sharedSync.options.Policy.Content.QuickCommands {
		t.Fatal("scope checkbox did not persist")
	}
	r, found := view.Find("修改后等待（秒）")
	if !found {
		t.Fatal("missing interval input")
	}
	view.ClickAt(r.X+20, r.Y+r.H/2)
	view.Key(ui.Ctrl, ui.KeyA)
	view.Type("17")
	if a.features.sharedSync.options.Policy.ChangeDelay != 17 {
		t.Fatal("numeric draft not updated", a.features.sharedSync.options.Policy.ChangeDelay)
	}
	if err := view.Click("自动同步"); err != nil {
		t.Fatal(err)
	}
	if a.features.sharedSync.options.Policy.Automatic {
		t.Fatal("manual policy not stored")
	}
}

func TestMalformedLocalPreferencesAreNotOverwritten(t *testing.T) {
	a := parityApp(t)
	p := filepath.Join(a.store.Dir, "desktop-preferences.json")
	for _, body := range []string{`{"version":2}`, `{"futureSetting":true}`, `{"imagePasteSSH":`} {
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if a.saveLocalPreferences(defaultLocalPreferences()) {
			t.Fatal("overwrote unreadable local settings")
		}
		got, _ := os.ReadFile(p)
		if string(got) != body {
			t.Fatal("local data changed")
		}
	}
}

func TestSidebarRevealAndFilteringLimitSelection(t *testing.T) {
	a := parityApp(t)
	p, q := core.NewProfile(), core.NewProfile()
	a.config.Profiles = []core.Profile{p, q}
	a.sidebar.selection.Add(p.ID)
	a.sidebar.selection.Add(q.ID)
	a.sidebar.visible = []string{p.ID}
	a.withSidebarContext(p.ID, func() {
		if len(a.selectedManagerRows()) != 1 {
			t.Fatal("hidden selection included")
		}
	})
	a.revealSidebar(q.ID)
	if a.sidebar.selection.Len() != 1 || !a.sidebar.selection.Has(q.ID) {
		t.Fatal("reveal kept old multiselection")
	}
}
func TestDeepFocusPreservesDraftAndPreviousFullscreen(t *testing.T) {
	a := parityApp(t)
	a.quick = "draft"
	a.focusFullscreen.requested = true
	a.toggleDeepFocus()
	if !a.focusFullscreen.deep {
		t.Fatal("deep focus not enabled")
	}
	a.toggleDeepFocus()
	if a.focusFullscreen.deep || !a.focusFullscreen.requested || a.quick != "draft" {
		t.Fatal("focus exit lost previous state")
	}
}
func TestImagePasteBindingIncludesIdentityAndRouting(t *testing.T) {
	p := core.NewProfile()
	p.Host = "example.invalid"
	p.Username = "a"
	base := imagePasteBinding(p)
	for _, change := range []func(*core.Profile){func(p *core.Profile) { p.Host = "other.invalid" }, func(p *core.Profile) { p.Port++ }, func(p *core.Profile) { p.Username = "b" }, func(p *core.Profile) { p.ProxyID = core.NewID() }} {
		q := p
		change(&q)
		if imagePasteBinding(q) == base {
			t.Fatal("remembered image choice survived identity change")
		}
	}
	a := parityApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	s := &imagePasteState{pane: &pane{}, cancel: cancel, open: false}
	a.imagePaste = s
	ui.NewTester(func(c *ui.Context) { a.imagePasteDialog(c) }, 600, 400)
	if a.imagePaste != nil || ctx.Err() == nil {
		t.Fatal("image cancel left pending work")
	}
}

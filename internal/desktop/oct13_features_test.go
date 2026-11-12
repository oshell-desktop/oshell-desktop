package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"testing"
)

func TestIdleReusePreservesTabSplitGroupAndZoom(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.newBlank()
	waitOperator(t, a, func() bool { return len(a.tabs) == 1 && a.tabs[0].active.term != nil })
	tab := a.tabs[0]
	old := tab.active
	id, group := tab.id, tab.group
	if !reusableIdlePane(old) {
		t.Fatal("blank not reusable")
	}
	other := &pane{id: core.NewID(), profile: core.NewProfile()}
	tab.root = &node{first: tab.root, second: &node{pane: other}, size: 240}
	tab.zoomed = old
	p := core.NewProfile()
	p.Kind = "local"
	p.Name = "replacement"
	a.open(p, core.Credentials{}, 0)
	if len(a.tabs) != 1 || a.tabs[0].id != id || tab.group != group || tab.root.first.pane == old || tab.root.second.pane != other || !old.closed || tab.zoomed != tab.active {
		t.Fatal("reuse changed tab/layout or lost zoom")
	}
	a.dispose(tab.active)
}
func TestBatchOnlyFirstTerminalCanReuse(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.newBlank()
	waitOperator(t, a, func() bool { return a.tabs[0].active.term != nil })
	p := core.NewProfile()
	p.Kind = "local"
	a.authQueue = []core.Profile{p, p}
	a.nextAuthentication()
	if len(a.tabs) != 2 {
		t.Fatalf("batch tabs=%d", len(a.tabs))
	}
	for _, tab := range a.tabs {
		a.dispose(tab.active)
	}
}
func TestClockPageDraftAndTransparentSubscription(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.prepareSettings()
	a.features.settingsTab = 7
	tt := ui.NewTester(a.settingsView, 800, 650)
	if !tt.HasText("时钟字号") || !tt.HasText("叠加预览") {
		t.Fatal("clock page missing")
	}
	a.features.settingsDraft.TerminalClockEnabled = true
	a.features.settingsDraft.TerminalClockOpacity = 0
	a.clockSizeDraft = "48"
	if err := a.validateClockDraft(); err != nil {
		t.Fatal(err)
	}
	if clockVisible(a.features.settingsDraft) {
		t.Fatal("transparent clock still visible")
	}
	a.closeSettings()
	if a.config.Settings.TerminalClockFontSize != 12 {
		t.Fatal("Cancel saved font size")
	}
	hub := &applicationHub{config: core.DefaultConfig()}
	ticker := &hub.clock
	ticker.update(map[*App]bool{a: true})
	hub.config.Settings.TerminalClockEnabled = true
	hub.config.Settings.TerminalClockOpacity = 0
	hub.refreshClock()
	ticker.mu.Lock()
	defer ticker.mu.Unlock()
	if ticker.stop != nil {
		t.Fatal("ticker did not stop")
	}
}
func TestTabPlusFollowsContentAndBackgroundMenu(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.ensureGroups()
	g := a.groups.root.group
	a.tabs = []*tab{{id: "one", name: "develop", group: g.id, active: &pane{profile: core.NewProfile(), status: "正在连接…"}}}
	g.active = "one"
	a.current = 0
	tt := ui.NewTester(func(c *ui.Context) { a.groupTabBar(c, g) }, 900, 40)
	plus, _ := tt.Find("新建标签")
	if plus.X > 220 {
		t.Fatal("plus did not follow last tab", plus)
	}
	if err := tt.RightClick("关闭 develop"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, label := range tt.Menu() {
		found = found || label == "新建窗口"
	}
	if !found {
		t.Fatal("close button context menu missing")
	}
	tt.CloseMenu()
	for i := 0; i < 10; i++ {
		a.tabs = append(a.tabs, &tab{id: core.NewID(), name: "long terminal", group: g.id, active: &pane{}})
	}
	tt.Frame()
	plus, _ = tt.Find("新建标签")
	menu, _ := tt.Find("所有标签")
	if plus.X < 700 || plus.X+plus.W > menu.X || menu.X+menu.W > 900 {
		t.Fatal("overflow plus not pinned", plus)
	}
	a.tabs = a.tabs[:1]
	tt.Frame()
	plus, _ = tt.Find("新建标签")
	if plus.X > 220 {
		t.Fatal("plus did not retract")
	}
	green, _ := tabConnectionStatus(a.tabs[0])
	if !green || canDuplicateConnectedTab(a.tabs[0]) {
		t.Fatal("connecting status confused with duplicate eligibility")
	}
}

func TestClockFollowColorAndApply(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.prepareSettings()
	a.features.settingsTab = 7
	tt := ui.NewTester(a.settingsView, 800, 650)
	if err := tt.Click("跟随终端文字颜色"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.features.settingsDraft.TerminalClockColor == "" || a.clockFollowsText {
		t.Fatal("could not enable custom clock color")
	}
	a.features.settingsDraft.TerminalClockColor = "#FF0000"
	a.clockSizeDraft = "96"
	a.features.settingsDraft.TerminalClockEnabled = true
	a.features.settingsDraft.TerminalClockOpacity = .75
	tt.Frame()
	if err := tt.Click("仅全屏时显示（含专注全屏）"); err != nil {
		t.Fatal(err)
	}
	snapshot(t, tt, "clock-settings-oct13.png")
	a.applySettings()
	if a.config.Settings.TerminalClockColor != "#FF0000" || a.config.Settings.TerminalClockFontSize != 96 || a.config.Settings.TerminalClockOpacity != .75 {
		t.Fatal("clock style not applied", a.features.settingsError)
	}
	disk, err := readLocalPreferences(a.store.Dir)
	if err != nil || !disk.ClockFullscreenOnly || a.windowClockVisible() {
		t.Fatal("normal window clock", err)
	}
	a.fullscreenSettled(true)
	if !a.windowClockVisible() {
		t.Fatal("clock missing in native fullscreen")
	}
	a.focusFullscreen.deep = true
	if !a.windowClockVisible() {
		t.Fatal("clock missing in deep fullscreen")
	}
	a.fullscreenSettled(false)
	if a.windowClockVisible() {
		t.Fatal("clock remained after exit")
	}
}
func TestEmptyGroupBackgroundMenuNewTabStaysInGroup(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.ensureGroups()
	g, err := a.createGroup("empty")
	if err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) { a.groupTabBar(c, g) }, 700, 40)
	if err = tt.RightClick("新建标签"); err != nil {
		t.Fatal(err)
	}
	if err = tt.ChooseMenuItem("新建空白标签页"); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 1 || a.tabs[0].group != g.id {
		t.Fatal("new tab escaped empty group")
	}
	a.dispose(a.tabs[0].active)
}

func TestExternalDuplicateUsesLiveChannelAndSilentlyRejectsDead(t *testing.T) {
	a, profile, accepted := savedConnectionFixture(t)
	a.open(profile, core.Credentials{Password: "saved-fixture-password", ExternalLaunch: true}, 0)
	waitOperator(t, a, func() bool { return len(a.tabs) == 1 && a.tabs[0].active.term != nil && a.tabs[0].active.conn != nil })
	original := a.tabs[0]
	a.duplicateConnectedTab(original)
	waitOperator(t, a, func() bool { return len(a.tabs) == 2 && a.tabs[1].active.term != nil && a.tabs[1].active.conn != nil })
	if accepted.Load() != 1 || a.authOpen || !a.tabs[1].active.externalLaunch {
		t.Fatal("external clone reauthenticated or lost external state")
	}
	if err := original.active.conn.Client.Close(); err != nil {
		t.Fatal(err)
	}
	waitOperator(t, a, func() bool { return !original.active.conn.Alive() })
	a.duplicateConnectedTab(original)
	if len(a.tabs) != 2 || a.authOpen {
		t.Fatal("invalid external channel opened another tab/dialog")
	}
	for _, tab := range a.tabs {
		a.dispose(tab.active)
	}
}

func TestMixedBatchReturnsTerminalToOriginalIdleGroup(t *testing.T) {
	a, profile, _ := savedConnectionFixture(t)
	file := profile
	file.ID = core.NewID()
	file.Kind = "sftp"
	file.InitialDirectory = "/"
	for _, p := range []core.Profile{profile, file} {
		if err := a.vault.PutFor(p, "saved-fixture-password", p.Identity()); err != nil {
			t.Fatal(err)
		}
	}
	a.newBlank()
	waitOperator(t, a, func() bool { return a.tabs[0].active.term != nil })
	original := a.tabs[0]
	old := original.active
	files := newApp(a.store, a.config)
	files.vault = a.vault
	files.trust = a.trust
	hub := &applicationHub{store: a.store, config: a.config, vault: a.vault, trust: a.trust, windows: []*App{a}, fileWindow: files}
	a.hub = hub
	files.hub = hub
	a.authQueue = []core.Profile{file, profile}
	a.batchTerminalOwner = a
	a.nextAuthentication()
	waitOperator(t, a, func() bool {
		files.drain()
		return len(a.tabs) == 1 && a.tabs[0].active != old && a.tabs[0].active.term != nil && len(files.tabs) == 1 && files.tabs[0].active.files != nil
	})
	if a.tabs[0] != original || !old.closed || files.tabs[0].active.term != nil {
		t.Fatal("mixed batch lost terminal owner or reused a file tab")
	}
	a.dispose(a.tabs[0].active)
	files.dispose(files.tabs[0].active)
}

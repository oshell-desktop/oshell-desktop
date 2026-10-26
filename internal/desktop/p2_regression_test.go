package desktop

import (
	"bytes"
	"fmt"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"image"
	"image/jpeg"
	"image/png"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
	"strings"
	"testing"
)

func TestP2W02NavigationCrossesVisibleGroups(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	for _, name := range []string{"one", "two", "three"} {
		operatorPane(t, a, name)
	}
	a.ensureGroups()
	g, err := a.createGroup("second")
	if err != nil {
		t.Fatal(err)
	}
	a.moveToGroup(a.tabs[1], g)
	a.selectTab(a.tabs[0])
	a.navigateTab(1)
	if a.activeTab() != a.tabs[1] {
		t.Fatal("navigation stayed in group")
	}
	a.navigateTab(1)
	if a.activeTab() != a.tabs[2] {
		t.Fatal("global order lost")
	}
	a.hideGroup(g, true)
	a.selectTab(a.tabs[0])
	a.navigateTab(1)
	if a.activeTab() != a.tabs[2] {
		t.Fatal("hidden group navigated")
	}
	a.navigateTab(1)
	if a.activeTab() != a.tabs[0] {
		t.Fatal("no wrap")
	}
	if !a.selectTabNumber(2) || a.activeTab() != a.tabs[2] {
		t.Fatal("number shortcut changed scope")
	}
}

func TestP2P08M05CacheClearKeepsWorkspaceAndPasteTarget(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, conn := operatorPane(t, a, "connected")
	if err := a.vault.SetMaster("synthetic-cache-master"); err != nil {
		t.Fatal(err)
	}
	s := &imagePasteState{pane: p, term: p.term, contextToken: p.term.PasteContextToken()}
	a.lockWorkspaceSecrets()
	if !a.passwordLocked() || a.workspaceLocked() || !a.imagePasteValid(s) || p.closed {
		t.Fatal("cache clear locked connected workspace")
	}
	p.term.Send([]byte("existing input"))
	waitOperator(t, a, func() bool { return conn.text() != "" })
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	s.data = encoded.Bytes()
	a.imagePaste = s
	_, otherConn := operatorPane(t, a, "other target")
	a.current = 1
	a.startImagePaste(s)
	waitOperator(t, a, func() bool { return a.imagePaste == nil || s.error != "" })
	waitOperator(t, a, func() bool { return strings.Contains(conn.text(), "clipboard-images") })
	if s.error != "" || otherConn.text() != "" {
		t.Fatal("image retargeted after cache clear", s.error)
	}
	other := newApp(a.store, a.config)
	if !other.workspaceLocked() {
		t.Fatal("fresh startup skipped master lock")
	}
	a.hub = &applicationHub{workspaceAuthorized: true}
	other.hub = a.hub
	if other.workspaceLocked() {
		t.Fatal("new window forgot authorized workspace")
	}
	p.closed = true
	if a.imagePasteValid(s) {
		t.Fatal("closed captured target accepted")
	}
}
func TestP2S05AliasAndTargetMetadataSearch(t *testing.T) {
	cfg := core.DefaultConfig()
	p := core.NewProfile()
	p.Name = "原会话"
	p.Host = "source.invalid"
	p.Username = "operator"
	cfg.Profiles = []core.Profile{p}
	cfg.Links = []core.SessionLink{{ID: core.NewID(), ProfileID: p.ID, Name: "Alias"}}
	for _, q := range []string{"alias", "原会话", "source.invalid", "alias operator"} {
		rows, _ := sidebarEntries(cfg, q)
		found := false
		for _, row := range rows {
			if strings.HasPrefix(row.id, "link:") {
				found = true
			}
		}
		if !found {
			t.Fatal("reference metadata missing", q)
		}
	}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	a.sidebar.query = strings.Repeat("中", 1366)
	tt := ui.NewTester(a.sidebarView, 400, 500)
	if !strings.Contains(strings.Join(tt.Texts(), " "), "4096") {
		t.Fatal("invalid query not explained", tt.Texts())
	}
	for _, tc := range []struct {
		query string
		valid bool
	}{{strings.Repeat("x", 4096), true}, {strings.Repeat("x", 4097), false}, {strings.TrimSpace(strings.Repeat("x ", 32)), true}, {strings.TrimSpace(strings.Repeat("x ", 33)), false}} {
		if core.NewSessionQuery(tc.query).Valid != tc.valid {
			t.Fatal("search boundary", len(tc.query))
		}
		a.sidebar.query = tc.query
		tt.Frame()
		if strings.Contains(strings.Join(tt.Texts(), " "), "4096") == tc.valid {
			t.Fatal("wrong boundary feedback", len(tc.query), tt.Texts())
		}
	}
	a.sidebar.query = "alias"
	tt.Frame()
	if !tt.HasText("Alias") {
		t.Fatal("search did not recover", tt.Texts())
	}
}
func TestP2O02QuickHistoryBoundsAndDraft(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	for i := 0; i < 60; i++ {
		a.rememberQuickSend(fmt.Sprint(i))
	}
	a.rememberQuickSend("59")
	if len(a.history) != 50 {
		t.Fatal(len(a.history))
	}
	a.quick = "draft"
	a.navigateQuickHistory(true)
	if a.quick != "59" {
		t.Fatal(a.quick)
	}
	a.navigateQuickHistory(false)
	if a.quick != "draft" {
		t.Fatal("draft lost")
	}
	a.history = nil
	for i := 0; i < 10; i++ {
		a.rememberQuickSend(fmt.Sprint(i) + strings.Repeat("x", 8191))
	}
	if len(a.history) != 8 {
		t.Fatal("64 KiB cap", len(a.history))
	}
	a.rememberQuickSend(strings.Repeat("x", 8193))
	if len(a.history) != 8 {
		t.Fatal("oversize history")
	}
}

func TestP2O02SuccessfulSendResetsAndFailureKeepsDraft(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, conn := operatorPane(t, a, "send")
	a.quick = "first"
	a.features.operators.noReturn = true
	a.prepareSend()
	waitOperator(t, a, func() bool { return conn.text() == "first" })
	if a.quick != "" || a.features.operators.noReturn {
		t.Fatal("successful send did not reset draft/return")
	}
	a.quick = "keep"
	a.features.operators.noReturn = true
	p.closed = true
	a.prepareSend()
	if a.quick != "keep" || !a.features.operators.noReturn {
		t.Fatal("failure discarded draft")
	}
}
func TestP2M01ImageRepresentationFallback(t *testing.T) {
	var jpg bytes.Buffer
	jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil)
	var read []transfer.Format
	raw, err := readClipboardImageRepresentations([]transfer.Format{"image/jpeg", transfer.PNG, "image/tiff"}, func(f transfer.Format) ([]byte, error) {
		read = append(read, f)
		if f == "image/jpeg" {
			return jpg.Bytes(), nil
		}
		return []byte("invalid image"), nil
	})
	if err != nil || len(read) != 3 || read[0] != transfer.PNG || read[1] != "image/tiff" {
		t.Fatal(read, err)
	}
	data, err := normalizeClipboardRepresentations(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatal("fallback is not PNG", err)
	}
}
func TestP2I20HostBellRoutesToCurrentOwner(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, _ := operatorPane(t, a, "bell")
	old := terminalBell
	defer func() { terminalBell = old }()
	calls := 0
	terminalBell = func() { calls++ }
	var opts terminal.Options
	a.configureTerminalStatus(p, &opts)
	opts.OnBell()
	a.drain()
	if calls != 1 {
		t.Fatal(calls)
	}
	opts.OnBell()
	p.closed = true
	a.drain()
	if calls != 1 {
		t.Fatal("stale bell")
	}
}
func TestP2O08SystemShortcutStates(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	old := systemShortcutProbe
	defer func() { systemShortcutProbe = old }()
	key := core.KeyboardShortcut{}
	systemShortcutProbe = func(core.KeyboardShortcut) systemShortcutResult {
		return systemShortcutResult{"fixture conflict", true}
	}
	if a.checkSystemShortcut(key) == nil {
		t.Fatal("known conflict accepted")
	}
	systemShortcutProbe = func(core.KeyboardShortcut) systemShortcutResult { return systemShortcutResult{} }
	if a.checkSystemShortcut(key) != nil || a.features.shortcutSystemNotice == "" {
		t.Fatal("unknown reported as verified")
	}
	systemShortcutProbe = func(core.KeyboardShortcut) systemShortcutResult { return systemShortcutResult{readable: true} }
	if a.checkSystemShortcut(key) != nil {
		t.Fatal("free shortcut blocked")
	}
}
func TestP2S14NestedLinkDropAndStaleSnapshot(t *testing.T) {
	cfg := core.DefaultConfig()
	p := core.NewProfile()
	p.Host = "fixture.invalid"
	p.Username = "test"
	cfg.Profiles = []core.Profile{p}
	cfg.Directories = []string{"Links/Nested", "Links/Destination"}
	link := core.SessionLink{ID: core.NewID(), ProfileID: p.ID, Name: "Nested link", Folder: "Nested"}
	cfg.Links = []core.SessionLink{link}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	d := a.linkDrag("link:" + link.ID)
	a.applyLinkDrop(d, "", "")
	if a.config.Links[0].Folder != "" {
		t.Fatal("menu leaf did not move to root", a.status)
	}
	a.applyLinkDrop(d, "", "Destination")
	if a.config.Links[0].Folder != "" {
		t.Fatal("stale snapshot accepted")
	}
	a.applyLinkDrop(a.linkDrag("link:"+link.ID), "", "Destination")
	if a.config.Links[0].Folder != "Destination" {
		t.Fatal(a.status)
	}
}

func TestP2S14MenuDragProductionSurface(t *testing.T) {
	for _, kind := range []string{"link", "session"} {
		t.Run(kind, func(t *testing.T) {
			cfg := core.DefaultConfig()
			p := core.NewProfile()
			p.Name = "nested session"
			p.Host = "fixture.invalid"
			p.Username = "test"
			cfg.Directories = []string{"Links/Folder"}
			name := "nested link"
			if kind == "session" {
				p.Group = "Links/Folder"
				name = p.Name
			} else {
				cfg.Links = []core.SessionLink{{ID: core.NewID(), ProfileID: p.ID, Name: name, Folder: "Folder"}}
			}
			cfg.Profiles = []core.Profile{p}
			a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
			tt := ui.NewTester(a.view, 1200, 800)
			folder, ok := tt.Find("快捷目录 Folder")
			if !ok {
				t.Fatal("no folder")
			}
			if err := tt.Click("快捷目录 Folder"); err != nil {
				t.Fatal(err)
			}
			entry, ok := tt.Find("拖动快捷项目 " + name)
			if !ok {
				t.Fatal("menu leaf unreachable", tt.Texts())
			}
			tt.Press(entry.X+entry.W/2, entry.Y+entry.H/2)
			tt.Move(entry.X+entry.W/2+12, entry.Y+entry.H/2)
			tt.Move(800, folder.Y+folder.H/2)
			tt.Release(800, folder.Y+folder.H/2)
			tt.Frame()
			if kind == "link" && a.config.Links[0].Folder != "" || kind == "session" && a.config.Profiles[0].Group != "Links" {
				t.Fatal("menu drag did not reach root", a.status)
			}
		})
	}
}

func TestP2S14GenerationAndEdgeScroll(t *testing.T) {
	cfg := core.DefaultConfig()
	p := core.NewProfile()
	p.Host = "fixture.invalid"
	p.Username = "test"
	cfg.Profiles = []core.Profile{p}
	for i := 0; i < 40; i++ {
		cfg.Links = append(cfg.Links, core.SessionLink{ID: core.NewID(), ProfileID: p.ID, Name: fmt.Sprintf("long shortcut %02d", i)})
	}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	if !a.save() {
		t.Fatal(a.status)
	}
	drag := a.linkDrag("link:" + cfg.Links[0].ID)
	original := a.config.Clone()
	a.config.Links[0].Name = "changed"
	if !a.save() {
		t.Fatal(a.status)
	}
	a.config = original
	if !a.save() {
		t.Fatal(a.status)
	}
	if a.acceptsLinkDrag(drag) {
		t.Fatal("restored snapshot bypassed generation")
	}
	foreign := a.linkDrag(drag.Key)
	foreign.Owner = newApp(&core.Store{Dir: t.TempDir()}, cfg)
	if a.acceptsLinkDrag(foreign) || a.acceptsLinkDrag(linkRootDrag{Key: drag.Key, Owner: a}) {
		t.Fatal("foreign/unbound payload accepted")
	}
	tt := ui.NewTester(a.view, 1000, 700)
	item, ok := tt.Find("快捷链接 long shortcut 00")
	if !ok {
		t.Fatal("no initial link")
	}
	bar, ok := tt.Find("快捷链接栏")
	if !ok {
		t.Fatal("no scroll bar")
	}
	tt.Press(item.X+item.W/2, item.Y+item.H/2)
	tt.Move(item.X+item.W/2+12, item.Y+item.H/2)
	tt.Move(bar.X+bar.W-2, bar.Y+bar.H/2)
	for i := 0; i < 6; i++ {
		tt.Frame()
	}
	if a.linkScroll.X <= 0 {
		t.Fatal("drag edge did not scroll", a.linkScroll)
	}
	tt.Release(bar.X+bar.W-2, bar.Y+bar.H/2)
}

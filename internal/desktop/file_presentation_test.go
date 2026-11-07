package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
)

func TestFileDockPreservesLiveConnectionTabsAndTransfer(t *testing.T) {
	files, p := connectedFileWorkflow(t)
	owner := newApp(files.store, files.config)
	operatorPane(t, owner, "workspace")
	enableTestClock(owner)
	hub := &applicationHub{store: files.store, config: files.config, vault: files.vault, trust: files.trust, windows: []*App{owner}, fileWindow: files}
	owner.hub = hub
	files.hub = hub
	tab, connection, client := files.activeTab(), p.conn, p.files.client
	started, finish := make(chan struct{}), make(chan struct{})
	defer close(finish)
	files.withFileOperation(p, false, func(ctx context.Context) error {
		close(started)
		select {
		case <-finish:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	<-started
	tt := ui.NewTester(owner.view, 1440, 850)
	for _, placement := range []string{"left", "right", "bottom"} {
		hub.presentFileWorkspace(owner, placement)
		tt.Frame()
		if _, ok := tt.Find("停靠文件管理"); !ok {
			t.Fatal("dock missing", placement)
		}
		if files.activeTab() != tab || p.conn != connection || p.files.client != client || !p.files.busy || p.closed {
			t.Fatal("presentation replaced active work")
		}
		region, _ := tt.Find("停靠文件管理")
		term, _ := tt.Find("终端工作区")
		if region.W < 200 || region.H < 120 || term.W < 100 || term.H < 100 {
			t.Fatal("unusable split", placement, region, term)
		}
		body, ok := tt.Find("Terminal")
		ink := clockPixels(t, tt)
		if !ok || !body.Contains(float32(ink.Min.X), float32(ink.Min.Y)) || !body.Contains(float32(ink.Max.X-1), float32(ink.Max.Y-1)) {
			t.Fatalf("%s dock: clock %v outside terminal %v", placement, ink, body)
		}
		list, ok := tt.Find("远程文件列表")
		if !ok || list.H < 36 {
			t.Fatal("file list collapsed", placement, list)
		}
		snapshot(t, tt, "file-dock-"+placement+".png")
		files.hideFileDock()
		tt.Frame()
		if _, ok := tt.Find("停靠文件管理"); ok || p.closed || !p.files.busy {
			t.Fatal("hide disposed work")
		}
	}
	hub.presentFileWorkspace(owner, "right")
	tt.SetSize(880, 560)
	snapshot(t, tt, "file-dock-narrow.png")
	hub.presentFileWorkspace(owner, "window")
	tt.Frame()
	if files.filePresentation.owner.Load() != nil || p.conn != connection || !p.conn.Alive() {
		t.Fatal("undock lost connection")
	}
}

func TestTerminalFileManagerConfirmsOneTargetAndRejectsReconnect(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, first := operatorPane(t, a, "first")
	_, second := operatorPane(t, a, "second")
	a.current = 0
	settings := core.DefaultFileManagementSettings()
	settings.Mode = "terminal"
	settings.Command = "ranger --selectfile 'a b'"
	if err := core.SaveFileManagementSettings(a.store.Dir, settings); err != nil {
		t.Fatal(err)
	}
	a.features.syncEnabled = true
	a.showFileManager()
	if !a.filePresentation.confirmOpen || first.text() != "" {
		t.Fatal("sent before confirmation")
	}
	tt := ui.NewTester(a.view, 1000, 700)
	if err := tt.Click("确认空闲并运行"); err != nil {
		t.Fatal(err)
	}
	waitOperator(t, a, func() bool { return first.text() == settings.Command+"\r" })
	if second.text() != "" {
		t.Fatal("file command broadcast")
	}
	a.showFileManager()
	original := p.term
	p.term = a.tabs[1].active.term
	a.runTerminalFileCommand()
	p.term = original
	if !strings.Contains(a.status, "已变化") || second.text() != "" {
		t.Fatal("reconnected target received command", a.status)
	}
	p.term.Feed([]byte("\x1b[?1049h"))
	a.showFileManager()
	if a.filePresentation.confirmOpen {
		t.Fatal("interactive program accepted")
	}
	p.term.Feed([]byte("\x1b[?1049l\r\nPassword:"))
	a.showFileManager()
	if a.filePresentation.confirmOpen {
		t.Fatal("authentication prompt accepted")
	}
}

func TestFileSettingsAndMCPShortcutDoNotChangeSharedSchema(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.prepareSettings()
	before, _ := json.Marshal(a.config)
	a.filePresentation.draft.Mode = "terminal"
	a.filePresentation.draft.Placement = "bottom"
	if err := a.validateFileSettingsDraft(); err != nil {
		t.Fatal(err)
	}
	if err := a.applyFileSettingsDraft(); err != nil {
		t.Fatal(err)
	}
	a.features.settingsCapture = "desktopMCP"
	if !a.acceptCapturedShortcut(core.KeyboardShortcut{KeyCode: 105, Modifiers: 6}) {
		t.Fatal(a.features.shortcutError)
	}
	if !a.saveLocalPreferences(a.localUIDraft) {
		t.Fatal(a.status)
	}
	after, _ := json.Marshal(a.config)
	if !bytes.Equal(before, after) {
		t.Fatal("local preferences changed shared config")
	}
	if _, ok := a.features.settingsDraft.KeyboardShortcuts.Overrides["desktopMCP"]; ok {
		t.Fatal("MCP leaked to shared overrides")
	}
	loaded := loadLocalPreferences(a.store.Dir)
	if loaded.MCPShortcut == nil || loaded.MCPShortcut.KeyCode != 105 {
		t.Fatal("local shortcut not saved")
	}
	a.features.settingsTab = 9
	tt := ui.NewTester(a.settingsView, 1080, 760)
	if !tt.HasText("打开方式与位置") {
		t.Fatal("missing file settings")
	}
	snapshot(t, tt, "file-settings.png")
}

func TestNewLocalAndMCPShortcutsDoNotReachPTY(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, conn := operatorPane(t, a, "original")
	tt := ui.NewTester(func(c *ui.Context) { terminal.View(c, p.term).Bind(&p.focus).Fill().AutoFocus(); a.shortcuts(c) }, 500, 240)
	tt.Key(ui.Ctrl|ui.Alt, ui.KeyA)
	if !a.mcp.open || conn.text() != "" {
		t.Fatal("MCP shortcut sent to PTY", conn.text())
	}
	a.closeMCPPanel()
	tt.Frame()
	localKey := core.KeyboardShortcut{KeyCode: 30, Modifiers: 6}
	if a.config.Settings.KeyboardShortcuts.Conflict(localKey, "", "windows") != "" {
		tt.Key(ui.Ctrl|ui.Alt, ui.KeyBracketRight)
		if len(a.tabs) != 1 || p.closed {
			t.Fatal("local shortcut stole existing binding")
		}
	}
	a.config.Settings.KeyboardShortcuts.Overrides = map[string]core.ShortcutOverride{"nextPane": {}}
	tt.Frame()
	tt.Key(ui.Ctrl|ui.Alt, ui.KeyBracketRight)
	if len(a.tabs) != 2 || p.closed || conn.text() != "" {
		t.Fatal("local shortcut failed after conflicting binding cleared", len(a.tabs), conn.text())
	}
	local := a.activePane()
	waitOperator(t, a, func() bool { return local.term != nil })
	a.dispose(local)
}

func TestLocalDirectorySelectionValidatesWholeBatch(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	got, err := localDirectories([]string{first, second, first}, false)
	if err != nil || len(got) != 2 || got[0] != first {
		t.Fatal(got, err)
	}
	file := filepath.Join(second, "file")
	os.WriteFile(file, []byte("fixture"), 0600)
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	if err = a.openLocalDirectories([]string{first, file}, false); err == nil || len(a.tabs) != 0 {
		t.Fatal("partially applied invalid selection")
	}
	for _, values := range [][]string{{"file://remote/tmp"}, {"https://example.com/path"}, {"echo hello"}} {
		if _, err = localDirectories(values, true); err == nil {
			t.Fatal("nonlocal URL accepted")
		}
	}
}

func enableTestClock(a *App) {
	a.clockText = "2026-10-30 12:34:56"
	a.config.Settings.TerminalClockEnabled = true
	a.config.Settings.TerminalClockColor = "#FF1234"
	a.config.Settings.TerminalClockOpacity = 1
}

func clockPixels(t *testing.T, tt *ui.Tester) image.Rectangle {
	t.Helper()
	img := tt.Image()
	var ink image.Rectangle
	for y := 0; y < img.Rect.Max.Y; y++ {
		for x := 0; x < img.Rect.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.R > 230 && c.G < 40 && c.B > 40 && c.B < 75 {
				ink = ink.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if ink.Empty() {
		t.Fatal("clock not rendered")
	}
	return ink
}

func TestWorkspaceClockRendersOnceAcrossSplitTabs(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, _ := operatorPane(t, a, "first")
	q, _ := operatorPane(t, a, "second")
	a.localUI.SidebarVisible = false
	a.arrangeTabs("horizontal")
	enableTestClock(a)
	var frame *ui.Context
	tt := ui.NewTester(func(c *ui.Context) { frame = c; a.view(c) }, 1200, 700)
	r, _ := tt.Find("终端工作区")
	body, ok := tt.Find("Terminal")
	if !ok {
		t.Fatal("terminal body missing")
	}
	ink := clockPixels(t, tt)
	if float32(ink.Min.Y) < body.Y || float32(ink.Min.X) < r.X+r.W/2 {
		t.Fatalf("clock %v overlaps tabs or repeats in the first split, terminal body %v", ink, body)
	}
	if strings.Contains(p.term.Text(), a.clockText) || strings.Contains(q.term.Text(), a.clockText) {
		t.Fatal("clock entered terminal history")
	}
	snapshot(t, tt, "workspace-single-clock.png")
	a.config.Settings.TerminalClockPosition = "topCenter"
	tt.Frame()
	ink = clockPixels(t, tt)
	if center := float32(ink.Min.X+ink.Max.X) / 2; center < r.X+r.W/2-3 || center > r.X+r.W/2+3 || float32(ink.Dx()) > r.W/2 {
		t.Fatalf("clock %v should span the split at the window center", ink)
	}
	a.arrangeTabs("vertical")
	a.config.Settings.TerminalClockPosition = "center"
	tt.Frame()
	ink = clockPixels(t, tt)
	inside := func(body ui.Rect) bool { return float32(ink.Min.Y) >= body.Y && float32(ink.Max.Y) <= body.Y+body.H }
	if !inside(p.focus.Bounds(frame)) && !inside(q.focus.Bounds(frame)) {
		t.Fatalf("clock %v crosses the middle tab strip", ink)
	}
	a.selectTab(a.tabs[0])
	a.arrangeTabs("tabs")
	tt.Frame()
	if p.closed || q.closed {
		t.Fatal("clock/layout closed terminal")
	}
}

func TestWorkspaceClockNinePositionsAfterResize(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	operatorPane(t, a, "clock tab")
	enableTestClock(a)
	tt := ui.NewTester(a.view, 1200, 700)
	tt.SetSize(880, 560)
	for _, position := range core.ClockPositions {
		a.config.Settings.TerminalClockPosition = position[0]
		tt.Frame()
		body, ok := tt.Find("Terminal")
		ink := clockPixels(t, tt)
		if !ok {
			t.Fatal("terminal body missing")
		}
		checkAxis := func(start, span float32, low, high, alignment int) {
			t.Helper()
			offset := float32(low) - start - 12
			if alignment == 1 {
				offset = float32(low+high)/2 - (start + span/2)
			}
			if alignment == 2 {
				offset = start + span - float32(high) - 12
			}
			if offset < -5 || offset > 5 {
				t.Fatalf("%s: clock %v misplaced relative to terminal %v", position[0], ink, body)
			}
		}
		col, row := core.ClockGrid(position[0])
		checkAxis(body.X, body.W, ink.Min.X, ink.Max.X, col)
		checkAxis(body.Y, body.H, ink.Min.Y, ink.Max.Y, row)
	}
}

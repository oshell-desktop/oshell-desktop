package desktop

import (
	"context"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type operatorConn struct {
	testConn
	mu   sync.Mutex
	sent []byte
}

func (c *operatorConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	c.sent = append(c.sent, b...)
	c.mu.Unlock()
	return len(b), nil
}
func (c *operatorConn) text() string { c.mu.Lock(); defer c.mu.Unlock(); return string(c.sent) }
func operatorPane(t *testing.T, a *App, name string) (*pane, *operatorConn) {
	c := &operatorConn{testConn: testConn{done: make(chan struct{})}}
	term, e := terminal.New(terminal.Options{Conn: c, NoBlink: true})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { term.Close() })
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	p := &pane{id: core.NewID(), profile: core.Profile{Name: name, Kind: "ssh", Host: name}, hostname: name, term: term, cancel: cancel}
	p.owner.Store(a)
	a.tabs = append(a.tabs, &tab{id: core.NewID(), name: name, active: p, root: &node{id: core.NewID(), pane: p}})
	return p, c
}
func waitOperator(t *testing.T, a *App, condition func() bool) {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for !condition() && time.Now().Before(until) {
		a.drain()
		time.Sleep(time.Millisecond)
	}
	if !condition() {
		t.Fatal("operation timed out")
	}
}
func TestCommandDispatchFreezesTargetsAndReturnPolicy(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, left := operatorPane(t, a, "left")
	_, right := operatorPane(t, a, "right")
	_, third := operatorPane(t, a, "third")
	a.current = 0
	a.loadQuickCommand(core.QuickCommand{Name: "无回车", Text: "prefix", AppendReturn: false})
	a.prepareSend()
	waitOperator(t, a, func() bool { return left.text() == "prefix" })
	if right.text() != "" || third.text() != "" {
		t.Fatal("sent to wrong pane")
	}
	a.quickSendScope = "selected"
	a.features.operators.selected = map[string]bool{p.id: true, a.tabs[1].active.id: true}
	a.quick = "echo ok"
	a.features.operators.noReturn = false
	a.prepareSend()
	if a.confirmSend {
		t.Fatal("quick send added an extra confirmation")
	}
	a.current = 2
	a.features.operators.selected = map[string]bool{a.tabs[2].active.id: true}
	// Terminal writers drain independently; one target completing does not
	// imply that the other target's queue has drained.
	waitOperator(t, a, func() bool { return right.text() == "echo ok\r" && left.text() == "prefixecho ok\r" })
	if left.text() != "prefixecho ok\r" || third.text() != "" {
		t.Fatal("dispatch retargeted")
	}
}
func TestComposerAndHighlightDraftUI(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, conn := operatorPane(t, a, "prod.example")
	_, excluded := operatorPane(t, a, "excluded")
	a.loadQuickCommand(core.QuickCommand{Name: "多行", Text: "echo one\n  echo two", AppendReturn: false})
	if !a.features.operators.composerOpen || conn.text() != "" {
		t.Fatal("multiline executed without composer")
	}
	tt := ui.NewTester(a.view, 1280, 900)
	snapshot(t, tt, "composer.png")
	if e := tt.Click("发送撰写内容"); e != nil {
		t.Fatal(e)
	}
	if a.confirmSend {
		t.Fatal("composer added an extra confirmation")
	}
	waitOperator(t, a, func() bool { return strings.Contains(conn.text(), "echo two") })
	if conn.text() != "echo one\r  echo two" {
		t.Fatalf("composer changed text: %q", conn.text())
	}
	if excluded.text() != "" {
		t.Fatal("composer sent to unselected pane")
	}
	a.showHighlights()
	a.newHighlightSet(true)
	set := a.editedHighlightSet()
	set.Name = "生产日志"
	a.features.highlightActive = set.ID
	tt.Frame()
	snapshot(t, tt, "highlight-sets.png")
	a.saveHighlights()
	if a.features.highlightOpen || len(a.config.HighlightSets) != 2 {
		t.Fatal(a.features.highlightError)
	}
	saved, e := a.store.Load()
	if e != nil || saved.Settings.HighlightSetID != set.ID {
		t.Fatal(e)
	}
	a.showHighlights()
	a.editedHighlightSet().Name = "未保存"
	a.features.highlightOpen = false
	if a.config.HighlightSets[1].Name == "未保存" {
		t.Fatal("cancel changed config")
	}
	a.applyTerminalPreferences(p)
}
func TestSearchNextPreviousRefreshesWithoutModal(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, _ := operatorPane(t, a, "search")
	tt := ui.NewTester(a.view, 900, 600)
	p.term.Feed([]byte("first hit\r\nsecond hit\r\nthird hit"))
	tt.Frame()
	p.search.query = "hit"
	a.navigateSearch(1)
	waitOperator(t, a, func() bool { return !p.search.busy })
	if p.search.index != 0 || p.search.open {
		t.Fatal(p.search.err)
	}
	a.navigateSearch(1)
	waitOperator(t, a, func() bool { return !p.search.busy })
	if p.search.index != 1 {
		t.Fatal("next failed")
	}
	p.term.Feed([]byte("\r\nfourth hit"))
	a.navigateSearch(-1)
	waitOperator(t, a, func() bool { return !p.search.busy })
	if p.search.index != 0 {
		t.Fatal("previous after output failed")
	}
	a.navigateSearch(-1)
	waitOperator(t, a, func() bool { return !p.search.busy })
	if p.search.index != 3 {
		t.Fatal("wrap failed")
	}
}

func TestComposerLoadsSingleLineSavedCommandAndExcludesMovedTarget(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, conn := operatorPane(t, a, "one")
	if a.features.operators.composerOpen {
		t.Fatal("composer not lazy")
	}
	q := core.NewQuickCommand()
	q.Text = "single line"
	q.AppendReturn = false
	a.loadCommandInComposer(q)
	if !a.features.operators.composerOpen || a.features.operators.composerText != q.Text || a.features.operators.composerReturn {
		t.Fatal("saved command not loaded faithfully")
	}
	other := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.transferTab(a.activeTab(), other)
	a.prepareDispatch(q.Text, false, []*pane{p}, true)
	if conn.text() != "" || !strings.Contains(a.status, "未发送") {
		t.Fatal("sent to moved target", a.status)
	}
}

func TestQuickSendPreferencesPersistWithoutHidingErrors(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	tt := ui.NewTester(a.view, 1100, 700)
	if !slices.Contains(tt.Texts(), "发送") {
		t.Fatal("quick send missing by default")
	}
	a.toggleQuickSend()
	a.setSendScope("visible")
	a.status = "错误：test"
	tt.Frame()
	if slices.Contains(tt.Texts(), "发送") || !slices.Contains(tt.Texts(), "错误：test") {
		t.Fatal("hiding the bar also hid errors")
	}
	saved, e := a.store.Load()
	if e != nil || saved.Settings.QuickSendVisible || saved.Settings.QuickSendScope != "visible" {
		t.Fatal(saved.Settings, e)
	}
}

func TestQuickSendScopeRemainsIndependentAcrossWindows(t *testing.T) {
	store := &core.Store{Dir: t.TempDir()}
	cfg := core.DefaultConfig()
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	first := newApp(store, cfg)
	second := newApp(store, cfg)
	hub := &applicationHub{store: store, config: cfg, windows: []*App{first, second}}
	first.hub = hub
	second.hub = hub
	operatorPane(t, second, "second-one")
	operatorPane(t, second, "second-two")
	first.setSendScope("all")
	if first.sendScope() != "all" || second.sendScope() != "current" {
		t.Fatal("scope leaked between existing windows")
	}
	if len(second.sendCandidates()) != 1 {
		t.Fatal("other window expanded its send targets")
	}
	saved, err := store.Load()
	if err != nil || saved.Settings.QuickSendScope != "all" {
		t.Fatal("default preference not saved", err)
	}
	third := newApp(store, saved)
	if third.sendScope() != "all" {
		t.Fatal("new window did not inherit preference")
	}
	second.setSendScope("visible")
	if first.sendScope() != "all" || second.sendScope() != "visible" {
		t.Fatal("later change altered other window")
	}
}

func TestQuickSendScopeInitializesAfterProtectedStartup(t *testing.T) {
	store := &core.Store{Dir: t.TempDir()}
	cfg := core.DefaultConfig()
	cfg.Settings.QuickSendScope = "selectedGroups"
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	rev, err := store.Revision()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ProtectConfiguration(cfg, []byte("fixture-master"), rev); err != nil {
		t.Fatal(err)
	}
	store.LockConfiguration()
	a := newApp(store, core.DefaultConfig())
	if a.sendScope() != "" {
		t.Fatal("locked placeholder initialized scope")
	}
	loaded, err := store.UnlockConfiguration([]byte("fixture-master"))
	if err != nil {
		t.Fatal(err)
	}
	a.applyCommittedConfig(loaded)
	if a.sendScope() != "selectedGroups" || len(a.sendCandidates()) != 0 {
		t.Fatal("saved scope or empty runtime target selection lost")
	}
}

func TestComposerRejectsPartialTargetsAndPreservesWhitespace(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	first, left := operatorPane(t, a, "ready")
	second, right := operatorPane(t, a, "unavailable")
	second.closed = true
	a.prepareDispatch("must-not-partially-send", true, []*pane{first, second}, true)
	first.term.Send([]byte("barrier"))
	second.term.Send([]byte("barrier"))
	waitOperator(t, a, func() bool { return left.text() == "barrier" && right.text() == "barrier" })
	second.closed = false
	a.prepareDispatch("  ", false, []*pane{first}, true)
	waitOperator(t, a, func() bool { return left.text() == "barrier  " })
	if a.confirmSend {
		t.Fatal("composer requested second confirmation")
	}
}

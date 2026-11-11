package desktop

import (
	"context"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
)

func TestSessionEditorPersistenceAndSearch(t *testing.T) {
	s := &core.Store{Dir: t.TempDir()}
	a := newApp(s, core.DefaultConfig())
	tt := ui.NewTester(a.view, 1280, 820)
	if !tt.HasText("OShell Desktop") {
		t.Fatal(tt.Texts())
	}
	if e := tt.Click("新建 SSH 会话"); e != nil {
		t.Fatal(e)
	}
	a.draft.Name = "测试服务器"
	a.draft.Host = "127.0.0.1"
	a.draft.Username = "tester"
	tt.Frame()
	if e := tt.Click("保存"); e != nil {
		t.Fatal(e)
	}
	c, e := s.Load()
	if e != nil || len(c.Profiles) != 1 {
		t.Fatalf("%+v %v", c, e)
	}
	manager := ui.NewTester(a.managerView, 900, 580)
	a.query = "no-match"
	manager.Frame()
	if slices.Contains(manager.Texts(), "测试服务器") {
		t.Fatal("search did not filter")
	}
	a.query = ""
	a.managerAll = true
	manager.Frame()
	snapshot(t, manager, "session-manager.png")
	tt.Frame()
	snapshot(t, tt, "welcome.png")
	a.edit(c.Profiles[0])
	tt.Frame()
	snapshot(t, tt, "session-editor.png")
}

type testConn struct {
	done chan struct{}
	once sync.Once
}

func (c *testConn) Read([]byte) (int, error)    { <-c.done; return 0, io.EOF }
func (c *testConn) Write(b []byte) (int, error) { return len(b), nil }
func (c *testConn) Close() error                { c.once.Do(func() { close(c.done) }); return nil }
func TestTerminalWorkspaceAndClose(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.config.Settings.Dark = true
	a.config.Profiles = []core.Profile{{ID: core.NewID(), Name: "生产环境", Group: "服务器", Kind: "ssh", Host: "prod.example.com", Port: 22, Username: "deploy"}}
	makePane := func(name string) *pane {
		ctx, cancel := context.WithCancel(context.Background())
		_ = ctx
		term, e := terminal.New(terminal.Options{Conn: &testConn{done: make(chan struct{})}, NoBlink: true, Scrollback: 1 << 20})
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { term.Close() })
		term.Feed([]byte("\x1b[32mdeploy@" + name + "\x1b[0m:~$ uname -a\r\nLinux " + name + " 6.8.0 x86_64 GNU/Linux\r\n\r\n\x1b[36mOShell Desktop\x1b[0m · 原生终端测试\r\n中文输入 / UTF-8 / SSH\r\n\r\n$ "))
		return &pane{id: core.NewID(), profile: core.Profile{Name: name}, term: term, cancel: cancel, status: "测试终端"}
	}
	p, q := makePane("production"), makePane("staging")
	workspace := &tab{id: core.NewID(), name: "终端工作区", active: p, root: &node{id: core.NewID(), size: 480, first: &node{id: core.NewID(), pane: p}, second: &node{id: core.NewID(), pane: q}}}
	a.tabs = []*tab{workspace}
	tt := ui.NewTester(a.view, 1280, 820)
	tt.Frame()
	splitMenu := "分屏排列"
	if runtime.GOOS == "darwin" {
		splitMenu = "分屏"
	}
	if err := tt.Click(splitMenu); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("左右分屏"); err != nil {
		t.Fatal(err)
	}
	if !a.authOpen || a.pendingOpen.split != 1 {
		t.Fatal("split menu did not route to a new connection")
	}
	a.authOpen = false
	tt.Frame()
	snapshot(t, tt, "workspace.png")
	a.config.Settings.Dark = false
	a.applyAppearance()
	tt.Frame()
	snapshot(t, tt, "workspace-light.png")
	tt.SetSize(880, 560)
	tt.Frame()
	snapshot(t, tt, "workspace-compact.png")
	a.closeTab(workspace)
	tt.Frame()
	if len(a.tabs) != 0 || !p.closed || !q.closed {
		t.Fatal("closing workspace did not close panes")
	}
	deadline := time.After(3 * time.Second)
	select {
	case <-p.term.Done():
	case <-deadline:
		t.Fatal("terminal did not stop")
	}
}
func snapshot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("OSHELL_TEST_ARTIFACTS")
	if dir == "" {
		return
	}
	if e := os.MkdirAll(dir, 0755); e != nil {
		t.Fatal(e)
	}
	f, e := os.Create(filepath.Join(dir, name))
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if e = png.Encode(f, tt.Image()); e != nil {
		t.Fatal(e)
	}
}

func TestManagerDirectoryNavigationAndMultiTermSearch(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	for _, spec := range []struct{ name, group, host, user string }{
		{"生产服务器", "运维/生产", "prod.example.com", "deploy"},
		{"测试服务器", "运维/测试", "test.example.com", "tester"},
		{"网关", "", "gateway.example.com", "admin"},
	} {
		p := core.NewProfile()
		p.Name = spec.name
		p.Group = spec.group
		p.Host = spec.host
		p.Username = spec.user
		a.config.Profiles = append(a.config.Profiles, p)
	}
	rows := a.managerRows()
	if len(rows) != 3 || rows[0].name != "Links" || !rows[1].folder || rows[1].name != "运维" || rows[2].name != "网关" {
		t.Fatalf("root: %+v", rows)
	}
	a.enterRow(rows[1])
	rows = a.managerRows()
	if len(rows) != 3 || !rows[0].parent || !rows[1].folder || !rows[2].folder {
		t.Fatalf("children: %+v", rows)
	}
	a.query = "prod deploy 22"
	rows = a.managerRows()
	if len(rows) != 2 || !rows[0].parent || rows[1].name != "生产服务器" {
		t.Fatalf("search: %+v", rows)
	}
	tt := ui.NewTester(a.managerView, 900, 580)
	if !tt.HasText("prod.example.com") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("生产服务器"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.selected != rows[1].profile.ID {
		t.Fatalf("selection: %q, row: %d", a.selected, a.managerRow)
	}
	if err := tt.Click("连接"); err != nil {
		t.Fatal(err)
	}
	if !a.authOpen || a.pendingProfile.ID != rows[1].profile.ID {
		t.Fatal("connect did not open authentication for the selected profile")
	}
}

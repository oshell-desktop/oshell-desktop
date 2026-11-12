//go:build windows || linux

package desktop

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestUnifiedHeaderFitsMinimumWindowAndRoutesMenus(t *testing.T) {
	for _, dark := range []bool{false, true} {
		cfg := core.DefaultConfig()
		if dark {
			cfg.Settings.InterfaceTheme = "dark"
		} else {
			cfg.Settings.InterfaceTheme = "light"
		}
		a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
		tt := ui.NewTester(a.view, 880, 560)
		tt.Frame()
		if dark {
			snapshot(t, tt, "unified-header-dark.png")
		} else {
			snapshot(t, tt, "unified-header-light.png")
		}
		if _, ok := tt.Find("主工具栏"); ok {
			t.Fatal("minimal window kept toolbar row")
		}
		for _, visible := range []bool{true, false} {
			if err := tt.Click("更多"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("会话侧栏"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if _, shown := tt.Find("搜索会话侧栏"); shown != visible {
				t.Fatal("minimal menu did not toggle sidebar")
			}
		}
		if err := tt.Click("更多"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem("会话", "默认会话属性"); err != nil {
			t.Fatal(err)
		}
		if !a.features.defaultsOpen {
			t.Fatal("session menu did not open defaults")
		}
		a.features.defaultsOpen = false
		tt.Frame()
		if err := tt.Click("更多"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem("工具", "显示 / 隐藏撰写窗"); err != nil {
			t.Fatal(err)
		}
		if !a.features.operators.composerOpen {
			t.Fatal("tools menu lost composer")
		}
	}
}

func TestUnifiedHeaderCommandMenuRefreshesSavedCommands(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	tt := ui.NewTester(a.view, 880, 560)
	tt.Frame()
	q := core.NewQuickCommand()
	q.Name, q.Group, q.Text = "新增命令", "运维", "echo refreshed"
	a.config.QuickCommands = []core.QuickCommand{q}
	if err := tt.Click("更多"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("快速命令", "运维", "新增命令"); err != nil {
		t.Fatal(err)
	}
	if !a.features.operators.composerOpen || a.features.operators.composerText != q.Text {
		t.Fatal("command menu was stale or bypassed composer")
	}
}

func TestToolbarMinimalRestoreMargin(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	tt := ui.NewTester(a.view, 1180, 760)
	for _, step := range []struct {
		w, h    int
		visible bool
	}{{1180, 760, true}, {979, 760, false}, {1027, 760, false}, {1028, 760, true}, {1180, 499, false}, {1180, 547, false}, {1180, 548, true}} {
		tt.SetSize(step.w, step.h)
		tt.Frame()
		if _, ok := tt.Find("主工具栏"); ok != step.visible {
			t.Fatalf("toolbar at %dx%d: visible=%t", step.w, step.h, ok)
		}
		if _, ok := tt.Find("更多"); !ok {
			t.Fatal("lost application menus")
		}
	}
}

func TestToolbarSessionPropertiesTargetsSource(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	source := core.NewProfile()
	source.Name, source.Group = "stage", "work/servers"
	a.config.Profiles = []core.Profile{source}
	a.config.Links = []core.SessionLink{{ID: core.NewID(), Name: "favorite", Folder: "daily", ProfileID: source.ID}}
	tt := ui.NewTester(a.view, 1280, 720)
	if err := tt.Click("当前会话属性"); err != nil {
		t.Fatal(err)
	}
	if a.editing {
		t.Fatal("opened current profile without a tab")
	}
	for _, path := range [][]string{{"work", "servers", "stage"}, {"Links", "daily", "favorite"}} {
		if err := tt.Click("选择会话属性"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(path...); err != nil {
			t.Fatal(err)
		}
		if !a.editing || a.draft.ID != source.ID || len(a.tabs) != 0 {
			t.Fatal("selection did not edit the source in place", path)
		}
		a.editing = false
		tt.Frame()
	}
	if err := tt.Click("选择会话属性"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("默认会话"); err != nil || !a.features.defaultsOpen {
		t.Fatal("defaults unavailable", err)
	}
	a.features.defaultsOpen = false
	p, _ := operatorPane(t, a, "connected")
	p.profile = source
	a.config.Profiles[0].Name = "updated name"
	tt.Frame()
	if err := tt.Click("当前会话属性"); err != nil || !a.editing || a.draft.Name != "updated name" || a.activePane() != p {
		t.Fatal("current segment did not edit the latest saved definition", err)
	}
}

func TestUnifiedHeaderResponsiveActions(t *testing.T) {
	for _, width := range []float32{980, 1028, 1179, 1180, 1440} {
		for _, theme := range []string{"light", "dark"} {
			t.Run(fmt.Sprintf("%s-%g", theme, width), func(t *testing.T) {
				cfg := core.DefaultConfig()
				cfg.Settings.InterfaceTheme = theme
				a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
				tt := ui.NewTester(a.view, int(width), 560)
				tt.Frame()
				header, ok := tt.Find("主工具栏")
				if !ok || header.H != 40 {
					t.Fatal("incorrect toolbar height", header)
				}
				labels := []string{"会话侧栏", "会话管理", "快捷连接", "新建会话", "本地终端", "文件管理", "分屏", "标签组", "分组排列", "分屏排列", "搜索终端", "记录终端日志", "工具", "MCP 控制中心", "当前会话属性", "选择会话属性", "全局代理", "多设备同步", "更新", "设置"}
				var end float32
				for _, label := range labels {
					r, ok := tt.Find(label)
					if !ok || r.X < end || r.X+r.W > width || r.W < 28 || r.H != 28 || r.Y < header.Y || r.Y+r.H > header.Y+header.H {
						t.Fatalf("toolbar action %q missing, overlapping or clipped: %+v (previous end %g)", label, r, end)
					}
					end = r.X + r.W
				}
				snapshot(t, tt, fmt.Sprintf("toolbar-%s-%g.png", theme, width))
				// No terminal: search/logging must not create sessions or dialogs.
				for _, label := range []string{"搜索终端", "记录终端日志"} {
					_ = tt.Click(label)
				}
				if len(a.tabs) != 0 || a.modalActive() {
					t.Fatal("inactive terminal actions changed the workspace")
				}
				if err := tt.Click("新建会话"); err != nil {
					t.Fatal(err)
				}
				if !a.modalActive() {
					t.Fatal("new session action did not open the editor")
				}
			})
		}
	}
}

func TestUnifiedHeaderActiveTerminalActions(t *testing.T) {
	for _, width := range []int{980, 1280} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			cfg := core.DefaultConfig()
			cfg.Settings.InterfaceTheme = "dark"
			a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
			p, _ := operatorPane(t, a, "develop")
			operatorPane(t, a, "staging")
			active := a.tabs[0]
			active.root = &node{id: core.NewID(), first: active.root, second: a.tabs[1].root}
			splitRoot := active.root
			a.tabs = a.tabs[:1]
			other, _ := operatorPane(t, a, "test")
			a.ensureGroups()
			a.selectTab(active)
			p.term.Feed([]byte("Ready for commands\r\n$ "))
			if err := p.term.StartLog(filepath.Join(t.TempDir(), "session.log")); err != nil {
				t.Fatal(err)
			}
			var left, right ui.Rect
			tt := ui.NewTester(func(c *ui.Context) {
				a.view(c)
				left, right = p.focus.Bounds(c), other.focus.Bounds(c)
			}, width, 560)
			tt.Frame()
			snapshot(t, tt, fmt.Sprintf("toolbar-active-%d.png", width))
			if err := tt.Click("搜索终端"); err != nil {
				t.Fatal(err)
			}
			if !p.search.open {
				t.Fatal("toolbar search did not target the active terminal")
			}
			a.closeTerminalSearch(p)
			tt.Frame()
			if err := tt.Click("记录终端日志"); err != nil {
				t.Fatal(err)
			}
			if _, logging, _ := p.term.LogState(); logging {
				t.Fatal("toolbar could not stop active logging")
			}
			choose := func(section, item string) {
				t.Helper()
				button, path := section, []string{item}
				if err := tt.Click(button); err != nil {
					t.Fatal(err)
				}
				if err := tt.ChooseMenuItem(path...); err != nil {
					t.Fatal(err)
				}
			}
			choose("分屏", "放大 / 还原当前分屏")
			if active.zoomed != p {
				t.Fatal("pane menu did not zoom active pane")
			}
			choose("分屏", "放大 / 还原当前分屏")
			if active.zoomed != nil || active.root != splitRoot {
				t.Fatal("pane menu did not restore split layout")
			}
			choose("分屏排列", "标签左右排列")
			if len(a.groups.root.leaves()) != 1 || len(a.visiblePanes()) != 3 || active.root != splitRoot {
				t.Fatal("split arrangement must share one tab strip and preserve all panes")
			}
			tt.Frame()
			if left.W <= 0 || right.W <= 0 || left.X+left.W > right.X || right.X+right.W > float32(width) {
				t.Fatal("shared-strip terminals not laid out side by side", left, right)
			}
			choose("分组排列", "标签左右排列")
			if len(a.groups.root.leaves()) != 2 || active.root != splitRoot || active.active != p {
				t.Fatal("group arrangement lost tabs or changed pane contents")
			}
		})
	}
}

func TestUnifiedHeaderEditRestoresComposerFocus(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.openComposer("keep text", false)
	var focused bool
	tt := ui.NewTester(func(c *ui.Context) {
		a.view(c)
		focused = a.features.operators.composerFocus.FocusWithin(c)
	}, 880, 560)
	tt.Frame()
	if !focused {
		t.Fatal("composer did not start focused")
	}
	if err := tt.Click("编辑"); err != nil {
		t.Fatal(err)
	}
	if !focused {
		t.Fatal("edit menu took focus away from composer")
	}
}

func TestUnifiedHeaderFileManagerWithoutTerminal(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	tt := ui.NewTester(a.view, 1180, 650)
	if err := tt.Click("文件管理"); err != nil {
		t.Fatal(err)
	}
	if !a.features.switcher.open || !a.features.switcher.filesOnly {
		t.Fatal("file manager entry requires an active terminal")
	}
	a.features.switcher.open = false
	files := newApp(a.store, a.config)
	a.hub = &applicationHub{fileWindow: files}
	a.performAction("fileManager")
	if a.features.switcher.open || len(files.tabs) != 0 {
		t.Fatal("existing file window was not reused")
	}
}

func TestUnifiedHeaderSyncWithoutTerminal(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	tt := ui.NewTester(a.view, 1180, 650)
	if err := tt.Click("多设备同步"); err != nil {
		t.Fatal(err)
	}
	if !a.syncMonitorOpen || a.settingsOpen || len(a.tabs) != 0 {
		t.Fatal("sync entry required a terminal or did not open progress")
	}
}

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
		header, ok := tt.Find("主工具栏")
		if !ok || header.H != 32 {
			t.Fatalf("unexpected header: %+v", header)
		}
		for _, label := range []string{"快捷连接", "新建会话", "本地终端", "编辑", "更多", "工具", "标签组", "排列", "会话管理", "文件管理", "会话侧栏", "多设备同步", "搜索终端", "记录终端日志", "设置"} {
			r, ok := tt.Find(label)
			if !ok || r.X < 0 || r.X+r.W > 880 || r.Y < header.Y || r.Y+r.H > header.Y+header.H {
				t.Fatalf("header entry %q clipped: %+v", label, r)
			}
		}
		for _, visible := range []bool{false, true} {
			if err := tt.Click("会话侧栏"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			_, sidebarShown := tt.Find("搜索会话侧栏")
			if sidebarShown != visible || loadLocalPreferences(a.store.Dir).SidebarVisible != visible {
				t.Fatal("sidebar button failed to toggle the view or persist its state")
			}
			if _, ok := tt.Find("会话侧栏"); !ok {
				t.Fatal("sidebar button disappeared with the sidebar")
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
		if err := tt.Click("工具"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem("显示 / 隐藏撰写窗"); err != nil {
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

func TestUnifiedHeaderResponsiveActions(t *testing.T) {
	for _, width := range []float32{880, 999, 1000, 1199, 1200, 1440} {
		for _, theme := range []string{"light", "dark"} {
			t.Run(fmt.Sprintf("%s-%g", theme, width), func(t *testing.T) {
				cfg := core.DefaultConfig()
				cfg.Settings.InterfaceTheme = theme
				a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
				tt := ui.NewTester(a.view, int(width), 560)
				tt.Frame()
				labels := []string{"会话侧栏", "会话管理", "快捷连接", "新建会话", "本地终端", "文件管理", "工具", "标签组"}
				if width < 1200 {
					labels = append(labels, "排列")
				} else {
					labels = append(labels, "分组排列", "分屏排列")
				}
				labels = append(labels, "编辑", "更多", "MCP 控制中心", "多设备同步", "更新", "搜索终端", "记录终端日志", "设置")
				var end float32
				for _, label := range labels {
					r, ok := tt.Find(label)
					if !ok || r.X < end || r.X+r.W > width || r.W <= 0 {
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
	for _, width := range []int{880, 1200} {
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
			operatorPane(t, a, "test")
			a.ensureGroups()
			a.selectTab(active)
			p.term.Feed([]byte("Ready for commands\r\n$ "))
			if err := p.term.StartLog(filepath.Join(t.TempDir(), "session.log")); err != nil {
				t.Fatal(err)
			}
			tt := ui.NewTester(a.view, width, 560)
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
				if width < 1200 {
					button, path = "排列", []string{section, item}
				}
				if err := tt.Click(button); err != nil {
					t.Fatal(err)
				}
				if err := tt.ChooseMenuItem(path...); err != nil {
					t.Fatal(err)
				}
			}
			choose("分屏排列", "放大 / 还原当前分屏")
			if active.zoomed != p {
				t.Fatal("pane menu did not zoom active pane")
			}
			choose("分屏排列", "放大 / 还原当前分屏")
			if active.zoomed != nil || active.root != splitRoot {
				t.Fatal("pane menu did not restore split layout")
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
	tt := ui.NewTester(a.view, 880, 560)
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
	tt := ui.NewTester(a.view, 880, 560)
	if err := tt.Click("多设备同步"); err != nil {
		t.Fatal(err)
	}
	if !a.syncMonitorOpen || a.settingsOpen || len(a.tabs) != 0 {
		t.Fatal("sync entry required a terminal or did not open progress")
	}
}

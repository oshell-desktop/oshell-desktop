//go:build windows || linux

package desktop

import (
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
		if !ok || header.H != 38 {
			t.Fatalf("unexpected header: %+v", header)
		}
		for _, label := range []string{"OShell", "会话", "快捷连接", "编辑", "视图", "窗口", "快速命令", "工具", "标签组", "排列", "会话管理", "文件管理", "会话侧栏", "多设备同步", "设置"} {
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
		if err := tt.Click("会话"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem("默认会话属性"); err != nil {
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
	if err := tt.Click("快速命令"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("运维", "新增命令"); err != nil {
		t.Fatal(err)
	}
	if !a.features.operators.composerOpen || a.features.operators.composerText != q.Text {
		t.Fatal("command menu was stale or bypassed composer")
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

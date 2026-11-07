package desktop

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

// SF-S14: exercise the actual popover, including its tree interaction and
// retained shortcut actions. All profiles are synthetic and never connected.
func TestLinkFolderTreeInteraction(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		t.Run(mode, func(t *testing.T) {
			cfg := core.DefaultConfig()
			cfg.Settings.InterfaceTheme = mode
			cfg.Directories = []string{"Links/pvg/FQS/生产", "Links/pvg/rancher", "Links/空目录"}
			p := core.NewProfile()
			p.Name, p.Host, p.Username = "源会话", "fixture.invalid", "tester"
			actual := core.NewProfile()
			actual.Name, actual.Group, actual.Host = "PVG-V1-PROXY-01", "Links/pvg", "proxy.invalid"
			cfg.Profiles = []core.Profile{p, actual}
			cfg.Links = []core.SessionLink{
				{ID: core.NewID(), ProfileID: p.ID, Name: "PVG-V1-PROXY-02", Folder: "pvg"},
				{ID: core.NewID(), ProfileID: p.ID, Name: "FQS 生产入口", Folder: "pvg/FQS/生产"},
				{ID: core.NewID(), ProfileID: p.ID, Name: "集群入口", Folder: "pvg/rancher"},
			}
			a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
			a.setSidebarExpanded(true)
			tt := ui.NewTester(a.view, 1000, 560)
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			visible := func(label string, want bool) {
				t.Helper()
				if _, ok := tt.Find(label); ok != want {
					t.Fatalf("visibility of %q = %v, want %v", label, ok, want)
				}
			}
			click("快捷目录 pvg")
			visible("返回上级", false)
			visible("拖动快捷项目 FQS 生产入口", false)
			click("快捷子目录 FQS")
			click("快捷子目录 生产")
			click("快捷子目录 rancher")
			visible("拖动快捷项目 FQS 生产入口", true)
			visible("拖动快捷项目 集群入口", true)
			visible("拖动快捷项目 PVG-V1-PROXY-01", true)
			if a.linkMenuFolder != "pvg" || !a.linkMenuOpen {
				t.Fatal("expansion navigated away from the original folder")
			}
			snapshot(t, tt, "link-tree-"+mode+".png")

			// The arrow toggles once; siblings and the anchored root remain visible.
			row, _ := tt.Find("快捷子目录 FQS")
			tt.ClickAt(row.X+12, row.Y+row.H/2)
			visible("拖动快捷项目 FQS 生产入口", false)
			visible("拖动快捷项目 集群入口", true)
			click("快捷子目录 FQS")
			tt.Key(0, ui.KeyLeft)
			visible("拖动快捷项目 FQS 生产入口", false)
			tt.Key(0, ui.KeyRight)
			visible("拖动快捷项目 FQS 生产入口", true)
			if !reflect.DeepEqual(a.config, cfg) {
				t.Fatal("tree interaction mutated shared configuration")
			}

			// A deeply nested entry still supports the existing drag to root.
			leaf, _ := tt.Find("拖动快捷项目 FQS 生产入口")
			anchor, _ := tt.Find("快捷目录 pvg")
			tt.Press(leaf.X+leaf.W/2, leaf.Y+leaf.H/2)
			tt.Move(leaf.X+leaf.W/2+12, leaf.Y+leaf.H/2)
			tt.Move(800, anchor.Y+anchor.H/2)
			tt.Release(800, anchor.Y+anchor.H/2)
			tt.Frame()
			if a.config.Links[1].Folder != "" || a.linkMenuOpen {
				t.Fatal("nested tree drag did not move to root", a.status)
			}
			click("快捷目录 pvg")
			click("快捷子目录 rancher")
			click("管理此快捷目录…")
			if a.managerGroup != "Links/pvg" || a.linkMenuOpen {
				t.Fatal("manage action lost the anchored directory")
			}
			a.closeManager()
			tt.Frame()
			click("快捷目录 空目录")
			visible("此目录暂无快捷项目", true)
			tt.Key(0, ui.KeyEscape)
			if a.linkMenuOpen {
				t.Fatal("Escape did not close the popover")
			}
			click("快捷目录 pvg")
			click("快捷子目录 rancher")
			a.config.Profiles[0].Host = "updated.invalid"
			tt.Frame()
			click("拖动快捷项目 集群入口")
			if a.linkMenuOpen || !a.authOpen || a.pendingProfile.ID != p.ID || a.pendingProfile.Host != "updated.invalid" {
				t.Fatal("shortcut did not request its latest source profile")
			}
			a.features.authCancel()
		})
	}
}

func TestLongShortcutLocationPicker(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		t.Run(mode, func(t *testing.T) {
			cfg := core.DefaultConfig()
			cfg.Settings.InterfaceTheme = mode
			p := core.NewProfile()
			p.Name, p.Host = "源会话", "fixture.invalid"
			cfg.Profiles = []core.Profile{p}
			link := core.SessionLink{ID: core.NewID(), ProfileID: p.ID, Name: "链接"}
			cfg.Links = []core.SessionLink{link}
			for i := 1; i <= 40; i++ {
				cfg.Directories = append(cfg.Directories, fmt.Sprintf("Links/目录%02d", i))
			}
			a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
			a.editLink(link.ID)
			tt := ui.NewTester(func(c *ui.Context) { a.theme(c); a.linkEditorDialog(c) }, 880, 560)
			if err := tt.Click("保存位置"); err != nil {
				t.Fatal(err)
			}
			snapshot(t, tt, "long-picker-"+mode+".png")
			first, visible := tt.Find("目录01")
			if !visible || first.H <= 0 || first.Y < 4 || first.Y+first.H > 556 {
				t.Fatalf("first option outside the window: %+v", first)
			}
			tt.Scroll(first.X+2, first.Y+first.H/2, 0, 180)
			if r, ok := tt.Find("目录01"); ok && r.H == first.H && r.Y == first.Y {
				t.Fatal("scrolling the list did not move the first option")
			}
			tt.Key(0, ui.KeyEnd)
			tt.Frame()
			end, ok := tt.Find("目录40")
			if !ok || end.H <= 0 || end.Y < 4 || end.Y+end.H > 556 {
				t.Fatalf("end of the directory list outside window: %+v", end)
			}
			tt.Key(0, ui.KeyEnter)
			if a.features.linkEditor.folder != "目录40" {
				t.Fatal("could not choose the last directory")
			}
			if err := tt.Click("保存位置"); err != nil {
				t.Fatal(err)
			}
			tt.Key(0, ui.KeyEscape)
			if !a.features.linkEditor.open || tt.HasText("快捷链接栏（平铺）") {
				t.Fatal("Escape should close only the location picker")
			}
		})
	}
}

func TestLinkDirectoryHoverExpansion(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		t.Run(mode, func(t *testing.T) { testLinkDirectoryHoverExpansion(t, mode) })
	}
}

func testLinkDirectoryHoverExpansion(t *testing.T, mode string) {
	cfg := core.DefaultConfig()
	cfg.Settings.InterfaceTheme = mode
	cfg.Directories = []string{"Links/A/Child/Grandchild", "Links/B"}
	p := core.NewProfile()
	p.Name, p.Group, p.Host = "Nested target", "Links/A/Child/Grandchild", "fixture.invalid"
	cfg.Profiles = []core.Profile{p}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	tt := ui.NewTester(a.view, 1000, 600)
	move := func(label string) ui.Rect {
		t.Helper()
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("missing %s", label)
		}
		tt.Move(r.X+r.W/2, r.Y+r.H/2)
		return r
	}
	dwell := func() {
		t.Helper()
		time.Sleep(linkHoverDelay + 40*time.Millisecond)
		tt.Frame()
		tt.Frame() // Draw the child rows scheduled by the hover state change.
	}
	move("快捷目录 A")
	if a.linkMenuOpen {
		t.Fatal("brief pointer crossing opened a directory")
	}
	tt.Move(700, 450)
	dwell()
	if a.linkMenuOpen {
		t.Fatal("delayed hover opened after pointer left")
	}
	move("快捷目录 A")
	dwell()
	if !a.linkMenuOpen || a.linkMenuFolder != "A" {
		t.Fatal("root directory did not open on hover")
	}
	move("快捷子目录 Child")
	dwell()
	move("快捷子目录 Grandchild")
	dwell()
	if _, ok := tt.Find("拖动快捷项目 Nested target"); !ok || a.authOpen {
		t.Fatal("nested hover failed or connected without a click")
	}
	// Collapsing under a stationary pointer must stay collapsed.
	if err := tt.Click("快捷子目录 Grandchild"); err != nil {
		t.Fatal(err)
	}
	dwell()
	if _, ok := tt.Find("拖动快捷项目 Nested target"); ok {
		t.Fatal("hover undid explicit collapse")
	}
	move("快捷子目录 Child")
	move("快捷子目录 Grandchild")
	dwell()
	if _, ok := tt.Find("拖动快捷项目 Nested target"); !ok {
		t.Fatal("returning pointer did not re-arm expansion")
	}
	move("快捷目录 B")
	dwell()
	if !a.linkMenuOpen || a.linkMenuFolder != "B" {
		t.Fatal("hover did not switch the anchored root directory")
	}
	tt.Key(0, ui.KeyEscape)
	dwell()
	if a.linkMenuOpen {
		t.Fatal("hover reopened directory after Escape")
	}
	tt.Move(700, 450)
	move("所有快捷链接")
	dwell()
	if !a.linkMenuOpen || a.linkMenuAnchor != "all" {
		t.Fatal("overflow menu did not open on hover")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Move(700, 450)
	r := move("快捷目录 A")
	tt.Press(r.X+r.W/2, r.Y+r.H/2)
	dwell()
	if a.linkMenuOpen {
		t.Fatal("holding a directory opened its popover")
	}
	tt.Move(r.X+r.W/2+12, r.Y+r.H/2)
	move("快捷目录 B")
	dwell()
	if a.linkMenuOpen {
		t.Fatal("dragging across a directory opened its popover")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Release(700, 450)
	if !reflect.DeepEqual(a.config, cfg) {
		t.Fatal("hover or canceled drag modified configuration")
	}
}

func TestShortcutTreeContextMenuAndPicker(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		t.Run(mode, func(t *testing.T) {
			cfg := core.DefaultConfig()
			cfg.Settings.InterfaceTheme = mode
			p := core.NewProfile()
			p.Name, p.Group, p.Host, p.QuickConnect = "开发主机", "运维/开发", "test.invalid", true
			cfg.Profiles = []core.Profile{p}
			cfg.Directories = []string{"Links/常用/开发"}
			link := core.SessionLink{ID: core.NewID(), ProfileID: p.ID, Name: "开发入口", Folder: "常用/开发"}
			cfg.Links = []core.SessionLink{link}
			a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
			tt := ui.NewTester(a.view, 1000, 560)
			click := func(name string) {
				t.Helper()
				if err := tt.Click(name); err != nil {
					t.Fatal(err)
				}
			}
			click("快捷目录 常用")
			click("快捷子目录 开发")
			if err := tt.RightClick("拖动快捷项目 开发入口"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("添加链接"); err == nil {
				t.Fatal("disabled action was accepted")
			}
			if err := tt.ChooseMenuItem("重命名链接…"); err != nil {
				t.Fatal(err)
			}
			if a.linkMenuOpen || a.features.linkEditor.id != link.ID || !a.features.linkEditor.open {
				t.Fatal("context action did not replace the tree with the selected link editor")
			}
			click("保存位置")
			option, ok := tt.Find("快捷链接栏（平铺）")
			if !ok || option.Y < 0 || option.Y+option.H > 560 {
				t.Fatalf("picker row outside the dialog: %+v", option)
			}
			tt.Move(option.X+2, option.Y+option.H/2)
			snapshot(t, tt, "link-picker-"+mode+".png")
			click("快捷链接栏（平铺）")
			if a.features.linkEditor.folder != "" {
				t.Fatal("picker did not select the root")
			}
			click("取消")
			click("快捷目录 常用")
			if err := tt.RightClick("快捷子目录 开发"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("新增文件夹"); err != nil {
				t.Fatal(err)
			}
			if a.linkMenuOpen || a.features.linkEditor.folder != link.Folder {
				t.Fatal("folder action targeted the wrong directory")
			}
			tt.Key(0, ui.KeyEscape)
			if a.features.linkEditor.open || !reflect.DeepEqual(a.config, cfg) {
				t.Fatal("canceling the modal changed configuration")
			}
			click("快捷连接")
			if err := tt.ChooseMenuItem("运维", "开发", "开发主机 · SSH"); err != nil {
				t.Fatal(err)
			}
			if !a.authOpen || a.pendingProfile.ID != p.ID {
				t.Fatal("cascaded quick connection did not reach its session")
			}
			a.features.authCancel()
		})
	}
}

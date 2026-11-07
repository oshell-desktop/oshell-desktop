package desktop

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestProfessionalSettingsGallery(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for page := 0; page < len(settingsPages); page++ {
			t.Run(fmt.Sprintf("%s-%d", theme, page), func(t *testing.T) {
				cfg := core.DefaultConfig()
				cfg.Settings.InterfaceTheme = theme
				a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
				a.prepareSettings()
				a.features.settingsTab = page
				for _, size := range [][2]int{{1080, 760}, {800, 650}, {700, 560}} {
					tt := ui.NewTester(a.settingsView, size[0], size[1])
					tt.Frame()
					for _, name := range []string{"应用", "取消", "选择与侧栏", "设置分类导航", "设置内容", "设置操作区"} {
						r, ok := tt.Find(name)
						if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > float32(size[0]) || r.Y+r.H > float32(size[1]) {
							t.Fatalf("settings control %q outside window: %+v", name, r)
						}
					}
					nav, _ := tt.Find("设置分类导航")
					content, _ := tt.Find("设置内容")
					footer, _ := tt.Find("设置操作区")
					heading, ok := tt.Find("设置页标题")
					if !ok || heading.Y < 0 || heading.H <= 0 || heading.Y+heading.H > content.Y {
						t.Fatalf("settings heading clipped or compressed: %+v", heading)
					}
					if nav.X+nav.W > content.X || content.Y+content.H > footer.Y {
						t.Fatal("settings sidebar or fixed footer overlaps content")
					}
					if size[0] == 1080 {
						snapshot(t, tt, fmt.Sprintf("settings-%s-%d.png", theme, page))
					}
					if size[0] == 700 && (page == 0 || page == 2 || page == 3) {
						snapshot(t, tt, fmt.Sprintf("settings-small-%s-%d.png", theme, page))
					}
				}
				a.closeSettings()
			})
		}
	}
}

func TestProfessionalDialogGallery(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, kind := range []string{"session", "commands", "proxy", "password", "paste"} {
			t.Run(theme+"-"+kind, func(t *testing.T) {
				cfg := core.DefaultConfig()
				cfg.Settings.InterfaceTheme = theme
				a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
				var draw func(*ui.Context)
				switch kind {
				case "session":
					p := core.NewProfile()
					p.Name, p.Host, p.Username = "生产应用服务器", "app.example.invalid", "deploy"
					a.edit(p)
					draw = a.connectionEditor
				case "commands":
					a.openCommands()
					a.editQuickCommand(nil)
					a.features.commandEdit.Name = "检查服务状态"
					a.features.commandEdit.Group = "日常维护"
					a.features.commandEdit.Text = "systemctl status app\ndf -h"
					draw = a.quickCommandDialog
				case "proxy":
					a.features.proxyManager.open = true
					a.editProxy(nil)
					draw = a.proxyManagerDialog
				case "password":
					a.showVault()
					a.features.vaultAction = "set"
					draw = a.vaultDialog
				case "paste":
					a.features.pasteOpen = true
					a.features.pasteText = "cd /srv/app\n./status.sh"
					draw = a.inputDialogs
				}
				tt := ui.NewTester(func(c *ui.Context) { a.theme(c); draw(c) }, 760, 560)
				tt.Frame()
				r, ok := tt.Find("对话框内容")
				if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > 760 || r.Y+r.H > 560 {
					t.Fatalf("dialog content escaped window: %+v", r)
				}
				snapshot(t, tt, "dialog-"+theme+"-"+kind+".png")
			})
		}
	}
}

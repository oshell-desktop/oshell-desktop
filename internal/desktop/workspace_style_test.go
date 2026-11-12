package desktop

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

// Synthetic sessions exercise the populated chrome, including long labels and
// selection, without contacting any hosts or reading the user's configuration.
func TestCompactWorkspacePresentation(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		for _, width := range []int{880, 1280} {
			t.Run(fmt.Sprintf("%s-%d", theme, width), func(t *testing.T) {
				cfg := core.DefaultConfig()
				cfg.Settings.InterfaceTheme = theme
				for i, name := range []string{"生产应用服务器", "数据库只读节点", "预发布环境", "文件归档", "开发环境与自动化构建服务器"} {
					p := core.NewProfile()
					p.Name, p.Host, p.Username = name, "fixture.invalid", "deploy"
					p.Group = "生产环境"
					if i > 1 {
						p.Group = "开发与测试"
					}
					if i == 3 {
						p.Kind = "sftp"
					}
					cfg.Profiles = append(cfg.Profiles, p)
				}
				a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
				a.toggleSidebar()
				a.setSidebarExpanded(true)
				p, _ := operatorPane(t, a, "production")
				operatorPane(t, a, "staging")
				operatorPane(t, a, "build-runner")
				a.ensureGroups()
				a.selectTab(a.tabs[0])
				p.term.Feed([]byte("\x1b[32mdeploy@production\x1b[0m:~$ systemctl status app\r\n\r\n  \x1b[32m●\x1b[0m app.service - Application server\r\n     Loaded: loaded; enabled\r\n     Active: \x1b[32mactive (running)\x1b[0m\r\n\r\n  All services healthy.\r\n\r\n\x1b[32mdeploy@production\x1b[0m:~$ "))
				tt := ui.NewTester(a.view, width, 720)
				tt.Frame()
				if err := tt.Click("生产应用服务器"); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if !a.sidebar.selection.Has(cfg.Profiles[0].ID) {
					t.Fatal("styled row did not retain session selection")
				}
				search, ok := tt.Find("搜索会话侧栏")
				if !ok || search.X < 8 || search.X+search.W > 227 || search.H < 28 {
					t.Fatalf("sidebar search lost its inset or input height: %+v", search)
				}
				for _, label := range []string{"生产应用服务器", "开发环境与自动化构建服务器"} {
					row, ok := tt.Find(label)
					if !ok || row.X < 0 || row.X+row.W > 235 || row.H != detailRowHeight {
						t.Fatalf("sidebar row clipped: %q %+v", label, row)
					}
				}
				snapshot(t, tt, fmt.Sprintf("compact-workspace-%s-%d.png", theme, width))
				if err := tt.Click("staging"); err != nil {
					t.Fatal(err)
				}
				if a.activeTab() != a.tabs[1] {
					t.Fatal("styled tab did not activate")
				}
			})
		}
	}
}

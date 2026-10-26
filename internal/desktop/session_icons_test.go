package desktop

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestSessionTypeIconsAndLinkOrigin(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		t.Run(mode, func(t *testing.T) {
			cfg := core.DefaultConfig()
			cfg.Settings.InterfaceTheme = mode
			cfg.Directories = []string{"服务器", "Links"}
			for _, kind := range []string{"ssh", "local", "sftp", "ftp"} {
				p := core.NewProfile()
				p.Kind, p.Name, p.Group, p.Host, p.Username = kind, strings.ToUpper(kind)+" 示例", "服务器", "192.0.2.9", "tester"
				cfg.Profiles = append(cfg.Profiles, p)
			}
			actual := core.NewProfile()
			actual.Name, actual.Group, actual.Host = "Links 中的实际会话", "Links", "192.0.2.10"
			cfg.Profiles = append(cfg.Profiles, actual)
			cfg.Links = []core.SessionLink{{ID: core.NewID(), ProfileID: cfg.Profiles[0].ID, Name: "服务器快捷引用"}}
			a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
			a.managerAll, a.managerRow = true, -1
			a.setSidebarExpanded(true)
			view := ui.NewTester(func(c *ui.Context) {
				a.theme(c)
				ui.Row(c).Fill().AlignItems(ui.Stretch).Background(c.Theme().Background).Children(func() {
					a.sidebarView(c)
					ui.Box(c).Grow(1).FillHeight().Children(func() { a.managerView(c) })
				})
			}, 1200, 500)
			snapshot(t, view, "session-types-"+mode+".png")
			for _, kind := range []string{"ssh", "local", "sftp", "ftp"} {
				label := sessionIconFor(kind, false, false, false).label
				if r, ok := view.Find(label); !ok || r.W != 36 || r.H != 20 {
					t.Fatalf("missing 36 × 20-point %s marker: %+v", kind, r)
				}
			}
			for _, badge := range []string{"SSH", "SFTP", "FTP"} {
				if _, ok := view.Find(badge); !ok {
					t.Fatalf("missing protocol text %s", badge)
				}
			}
			a.managerSelection.Add(cfg.Profiles[0].ID)
			a.sidebar.selected = cfg.Profiles[0].ID
			view.Frame()
			snapshot(t, view, "session-types-selected-"+mode+".png")
			entries, _ := sidebarEntries(cfg, "")
			for _, e := range entries {
				style := sidebarSessionStyle(e)
				if e.id == actual.ID && style.badge != "SSH" {
					t.Fatal("actual session in Links became a shortcut")
				}
				if strings.HasPrefix(e.id, "link:") {
					if !style.shortcut || style.badge != "SSH" || !strings.Contains(sessionListTooltip(e.profile, true), "来源：/服务器/SSH 示例") {
						t.Fatal("shortcut lost identity or origin")
					}
				}
			}
		})
	}
}

func TestHTTPWebDAVSettingsHideCertificateControls(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	f := &a.features.sharedSync
	f.options.Address = "http://dav.example.invalid/"
	f.options.TrustedServerCertificate = core.WebDAVCertificate{Host: "old.invalid", Port: 443}
	view := ui.NewTester(func(c *ui.Context) { ui.Column(c).Fill().Children(func() { a.webDAVCertificateSettings(c) }) }, 650, 140)
	if !strings.Contains(strings.Join(view.Texts(), ""), "账号密码会明文传输") {
		t.Fatal("HTTP warning missing")
	}
	if _, ok := view.Find("查看 / 信任服务器证书…"); ok {
		t.Fatal("HTTP shows certificate inspection")
	}
	if _, ok := view.Find("取消证书信任"); ok {
		t.Fatal("HTTP shows stale certificate control")
	}
	snapshot(t, view, "webdav-http-warning.png")
	f.options.Address = "https://dav.example.invalid/"
	view.Frame()
	if _, ok := view.Find("查看 / 信任服务器证书…"); !ok {
		t.Fatal("HTTPS did not restore certificate controls")
	}
}

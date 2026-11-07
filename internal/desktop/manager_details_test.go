package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strings"
	"testing"
)

func TestManagerParentConsumesActualClickSeriesAfterNavigation(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.Directories = []string{"outer/child"}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	a.navigateManager("outer/child")
	tt := ui.NewTester(a.managerView, 900, 580)
	r, ok := tt.Find("../")
	if !ok {
		t.Fatal("parent row missing")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	for count := 1; count <= 5; count++ {
		tt.ClickAt(x, y)
		if a.managerGroup != "outer" {
			t.Fatalf("click %d navigated rebuilt listing to %q", count, a.managerGroup)
		}
	}
	tt.CancelPointer()
	tt.ClickAt(x, y)
	if a.managerGroup != "" {
		t.Fatal("new series did not navigate to root", a.managerGroup)
	}
}

func TestManagerParentSingleClickAndDoubleClickSuppression(t *testing.T) {
	cfg := core.DefaultConfig()
	cfg.Directories = []string{"outer/child"}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	a.navigateManager("outer/child")
	tt := ui.NewTester(a.managerView, 900, 580)
	if err := tt.Click("../"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.managerGroup != "outer" {
		t.Fatal("single parent click did not navigate", a.managerGroup)
	}
	// The continuation can land on a different row, including a connection.
	other := sessionRow{profile: core.NewProfile()}
	if !a.managerCellInput(other, ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, Clicks: 2}) {
		t.Fatal("second click not consumed")
	}
	if a.authOpen || a.managerGroup != "outer" {
		t.Fatal("second click changed destination")
	}
	if !a.managerCellInput(other, ui.InputEvent{Kind: ui.InputPointerCancel}) || a.managerParentClick {
		t.Fatal("capture cancellation retained parent click suppression")
	}
	if a.managerCellInput(other, ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, Clicks: 1}) {
		t.Fatal("new ordinary click swallowed")
	}
	parent := a.managerRows()[0]
	if a.managerCellInput(parent, ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, Clicks: 1, Mods: ui.Shift}) {
		t.Fatal("modified selection navigated")
	}
}

func TestManagerHighlightPreservesUnicodeAndMatchesSearch(t *testing.T) {
	for _, tc := range []struct{ text, query, want string }{
		{"Café-server", "cafe", "Café"},
		{"ＣＡＦＥ-server", "cafe", "ＣＡＦＥ"},
		{"Cafe\u0301-server", "cafe", "Cafe\u0301"},
		{"Straße", "strasse", "Straße"},
		{"生产-root-2222", "root 2222", "root2222"},
		{"server", "missing", ""},
	} {
		spans := managerSearchSpans(tc.text, core.NewSessionQuery(tc.query))
		var joined, bold strings.Builder
		for _, span := range spans {
			joined.WriteString(span.Text)
			if span.Weight == 600 {
				bold.WriteString(span.Text)
			}
		}
		if joined.String() != tc.text || bold.String() != tc.want {
			t.Fatalf("%q / %q: text=%q emphasis=%q", tc.text, tc.query, joined.String(), bold.String())
		}
	}
}

func TestManagerRowsCacheInvalidatesInPlaceChanges(t *testing.T) {
	cfg := core.DefaultConfig()
	p := core.NewProfile()
	p.Group = ""
	p.Name = "before"
	cfg.Profiles = []core.Profile{p}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	a.managerRows()
	cached := a.managerCache
	a.managerRows()
	if a.managerCache != cached {
		t.Fatal("unchanged rows rebuilt")
	}
	a.config.Profiles[0].Name = "after"
	rows := a.managerRows()
	if a.managerCache == cached || rows[len(rows)-1].name != "after" {
		t.Fatal("in-place profile update stayed stale")
	}
	a.query = "missing"
	if len(a.managerRows()) != 0 {
		t.Fatal("query failed to invalidate cache")
	}
	a.query = ""
	a.config.Directories = append(a.config.Directories, "new-directory")
	found := false
	for _, row := range a.managerRows() {
		found = found || row.group == "new-directory"
	}
	if !found {
		t.Fatal("directory update stayed stale")
	}
	a.managerList.Columns.Widths = map[string]float32{"名称": 300}
	a.navigateManager("new-directory")
	if a.managerList.Columns.Widths["名称"] != 300 {
		t.Fatal("navigation discarded column width")
	}
}

func TestManagerSearchKeyboardAndCompactLayout(t *testing.T) {
	cfg := core.DefaultConfig()
	p := core.NewProfile()
	p.Group = ""
	p.Name = "生产 Café 2"
	p.Host = "server.example.invalid"
	p.Username = "root"
	cfg.Profiles = []core.Profile{p}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	tt := ui.NewTester(a.managerView, 740, 440)
	if err := tt.Click("会话搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if a.managerRow < 0 {
		t.Fatal("Down with empty search failed to select a result")
	}
	if err := tt.Click("会话搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Type("cafe")
	tt.Frame()
	snapshot(t, tt, "manager-details-compact.png")
	if a.managerRow < 0 || a.selected != p.ID {
		t.Fatalf("search did not select matching session: query=%q selected=%q index=%d rows=%+v", a.query, a.selected, a.managerRow, a.managerRows())
	}
}

func TestManagerDetailsDensity(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		t.Run(mode, func(t *testing.T) {
			cfg := core.DefaultConfig()
			cfg.Settings.InterfaceTheme = mode
			cfg.Directories = []string{"01 生产环境", "02 测试环境", "03 数据中心", "04 网络设备", "05 运维工具"}
			for i, name := range []string{"bj-gateway", "bj-jumpserver", "files-backup", "开发环境 · 本地终端"} {
				p := core.NewProfile()
				p.Name, p.Group = name, ""
				p.Host, p.Username = fmt.Sprintf("192.0.2.%d", i+10), "deploy"
				if i == 2 {
					p.Kind = "sftp"
				} else if i == 3 {
					p.Kind = "local"
				}
				cfg.Profiles = append(cfg.Profiles, p)
			}
			for _, size := range [][2]int{{900, 580}, {740, 440}} {
				a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
				view := ui.NewTester(a.managerView, size[0], size[1])
				first, ok := view.Find(cfg.Directories[0])
				second, found := view.Find(cfg.Directories[1])
				if !ok || !found || second.Y-first.Y < 22 || second.Y-first.Y > 26 {
					t.Fatalf("unexpected directory row spacing: %+v, %+v", first, second)
				}
				var right float32
				for _, label := range []string{"名称", "主机", "用户名", "协议", "端口", "操作"} {
					header, ok := view.Find(label)
					if !ok || header.X < right || header.X+header.W > float32(size[0]) {
						t.Fatalf("%dx%d column %s: %+v", size[0], size[1], label, header)
					}
					right = header.X + header.W
				}
				if err := view.Click("bj-jumpserver"); err != nil {
					t.Fatal(err)
				}
				view.Frame()
				if !a.managerSelection.Has(cfg.Profiles[1].ID) {
					t.Fatal("compact session row did not select the clicked profile")
				}
				if size[0] == 900 {
					snapshot(t, view, "manager-details-"+mode+".png")
				}
			}
		})
	}
}

func BenchmarkManagerRows(b *testing.B) {
	cfg := core.DefaultConfig()
	for i := 0; i < 1000; i++ {
		p := core.NewProfile()
		p.Name = fmt.Sprintf("server-%d", i)
		p.Group = "team"
		cfg.Profiles = append(cfg.Profiles, p)
	}
	a := newApp(&core.Store{Dir: b.TempDir()}, cfg)
	a.managerGroup = "team"
	a.query = "server"
	a.managerRows()
	b.Run("cached", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			a.managerRows()
		}
	})
	b.Run("rebuild", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			a.buildManagerRows()
		}
	})
}

package desktop

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestSidebarDirectoryDoubleClick(t *testing.T) {
	for _, kind := range []string{"root", "nested", "links", "link-nested", "pinned", "filtered"} {
		t.Run(kind, func(t *testing.T) {
			cfg := core.DefaultConfig()
			p := core.NewProfile()
			p.Name, p.Group, p.Host = "fixture", "team/dev", "fixture.invalid"
			cfg.Profiles = []core.Profile{p}
			cfg.Links = []core.SessionLink{{ID: "reference", ProfileID: p.ID, Name: "alias", Folder: "shortcuts"}}
			a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
			folder, label, child := "team", "team", "dev"
			switch kind {
			case "nested":
				a.setSidebarExpanded(true)
				folder, label, child = "team/dev", "dev", "fixture"
			case "links":
				folder, label, child = "Links", "Links", "shortcuts"
			case "link-nested":
				a.setSidebarExpanded(true)
				folder, label, child = "Links/shortcuts", "shortcuts", "alias"
			case "pinned":
				a.localUI.Pins = map[string]bool{"d:team": true}
				label = "↑ team"
			case "filtered":
				a.sidebar.query = "fixture"
			}
			collapsed := false
			a.sidebar.expansion()[folder] = &collapsed
			before, _ := json.Marshal(a.config)
			tt := sidebarContextTester(a)
			tt.Frame()
			if tt.HasText(child) {
				t.Fatal("fixture was not collapsed")
			}
			click := func() {
				t.Helper()
				r, ok := tt.Find(label)
				if !ok {
					t.Fatalf("missing folder %q", label)
				}
				// Click the label area, away from the independent disclosure arrow.
				tt.ClickAt(r.X+r.W-30, r.Y+r.H/2)
			}
			click()
			if collapsed || !a.sidebar.selection.Has("folder:"+folder) {
				t.Fatal("single click should only select the directory")
			}
			click()
			tt.Frame()
			if !collapsed || !tt.HasText(child) {
				t.Fatal("double click did not expand the directory")
			}
			// Start a separate click sequence, then double click to collapse.
			tt.ClickAt(480, 570)
			click()
			click()
			tt.Frame()
			if collapsed || tt.HasText(child) {
				t.Fatal("double click did not collapse the directory")
			}
			if kind == "root" {
				tt.Key(0, ui.KeyRight)
				tt.Frame()
				if !collapsed || !tt.HasText(child) {
					t.Fatal("keyboard expansion regressed")
				}
				tt.Key(0, ui.KeyLeft)
				tt.Frame()
				r, _ := tt.Find(label)
				tt.ClickAt(r.X+10, r.Y+r.H/2)
				tt.Frame()
				if !collapsed || !tt.HasText(child) {
					t.Fatal("single arrow click did not expand")
				}
			}
			if kind == "filtered" {
				if a.sidebar.query != "fixture" || len(a.sidebar.open) != 0 {
					t.Fatal("filtered toggle changed the unfiltered tree or query")
				}
			}
			after, _ := json.Marshal(a.config)
			if string(before) != string(after) || len(a.tabs) != 0 || a.authOpen || a.managerGroup != "" {
				t.Fatal("folder double click changed configuration, navigated the manager or connected")
			}
		})
	}
}

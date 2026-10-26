package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"slices"
	"testing"
)

func TestLinkBarAndMenuFollowPersistedRootOrder(t *testing.T) {
	cfg := core.DefaultConfig()
	one := core.NewProfile()
	one.Name = "Bookmark target"
	one.Host = "one.invalid"
	one.Username = "tester"
	two := core.NewProfile()
	two.Name = "Root session"
	two.Host = "two.invalid"
	two.Username = "tester"
	two.Group = "Links"
	cfg.Profiles = []core.Profile{one, two}
	cfg.Directories = []string{"Links/Folder"}
	cfg.Links = []core.SessionLink{{ID: core.NewID(), ProfileID: one.ID, Name: "Bookmark"}}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	tt := ui.NewTester(a.view, 1200, 800)
	items := cfg.OrderedLinkRootItems()
	a.moveLinkRoot(items[2].Key(), items[0].Key())
	tt.Frame()
	texts := tt.Texts()
	session := slices.Index(texts, "快捷会话 Root session")
	folder := slices.Index(texts, "快捷目录 Folder")
	link := slices.Index(texts, "快捷链接 Bookmark")
	if session < 0 || folder < 0 || link < 0 || session >= folder || folder >= link {
		t.Fatal("bar order differs from persisted model", texts)
	}
	if e := tt.Click("所有快捷链接"); e != nil {
		t.Fatal(e)
	}
	menu := tt.Texts()
	if slices.Index(menu, "拖动快捷项目 Root session") < 0 || slices.Index(menu, "拖动快捷项目 Root session") >= slices.Index(menu, "▸ Folder") {
		t.Fatal("overflow menu ignored order", menu)
	}
	saved, e := a.store.Load()
	if e != nil || len(saved.Profiles) != 2 || len(saved.Links) != 1 || saved.OrderedLinkRootItems()[0].ID != two.ID {
		t.Fatal("reorder not saved or duplicated sessions", e)
	}
}

func TestLinkFolderDragDoesNotOpenMenuAndBlankAreaAppends(t *testing.T) {
	cfg := core.DefaultConfig()
	profile := core.NewProfile()
	profile.Host = "fixture.invalid"
	profile.Username = "fixture"
	profile.Name = "Session"
	profile.Group = "Links"
	cfg.Profiles = []core.Profile{profile}
	cfg.Directories = []string{"Links/Folder"}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	tt := ui.NewTester(a.view, 1200, 800)
	folder, ok := tt.Find("快捷目录 Folder")
	if !ok {
		t.Fatal("missing folder")
	}
	session, ok := tt.Find("快捷会话 Session")
	if !ok {
		t.Fatal("missing session")
	}
	// Drag a folder menu surface into unused bar space, rather than clicking it.
	tt.Press(folder.X+folder.W/2, folder.Y+folder.H/2)
	tt.Move(folder.X+folder.W/2+12, folder.Y+folder.H/2)
	tt.Move(800, folder.Y+folder.H/2)
	tt.Release(800, folder.Y+folder.H/2)
	tt.Frame()
	if len(tt.Menu()) != 0 {
		t.Fatal("menu stole drag")
	}
	items := a.config.OrderedLinkRootItems()
	if len(items) != 2 || items[0].Kind != "session" || items[1].Kind != "folder" {
		t.Fatal("empty bar space did not append", items, session)
	}
}

func TestLinkDropEdgesAndFolderCenter(t *testing.T) {
	items := []core.LinkRootItem{{Kind: "folder", Folder: "Folder"}, {Kind: "link", ID: core.NewID()}}
	for _, tc := range []struct {
		fraction       float32
		before, folder string
	}{{0.1, items[0].Key(), ""}, {0.5, "", "Folder"}, {0.9, items[1].Key(), ""}} {
		before, folder := linkDropDestination(items[1].Key(), items[0], 0, items, tc.fraction)
		if before != tc.before || folder != tc.folder {
			t.Fatal("incorrect folder hit zone", tc, before, folder)
		}
	}
	if _, folder := linkDropDestination("folder:Other", items[0], 0, items, 0.5); folder != "" {
		t.Fatal("folder drag treated as leaf move")
	}
}

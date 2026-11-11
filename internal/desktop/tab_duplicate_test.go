package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"slices"
	"testing"
)

func TestTabMenuDuplicateInsertsEachCopyImmediatelyAfterSource(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	for _, name := range []string{"left", "source", "right"} {
		profile := blankProfile()
		profile.Name = name
		a.open(profile, core.Credentials{}, 0)
	}
	t.Cleanup(func() {
		for _, tab := range a.tabs {
			a.dispose(tab.active)
		}
	})
	left, source, right := a.tabs[0], a.tabs[1], a.tabs[2]
	g := a.groupByID(source.group)
	tt := ui.NewTester(func(c *ui.Context) { a.groupTabBar(c, g) }, 1100, 60)
	var copies []*tab
	for range 2 {
		if err := tt.RightClick(source.name); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem("复制会话"); err != nil {
			t.Fatal(err)
		}
		copies = append(copies, a.activeTab())
	}
	if !slices.Equal(a.groupTabs(g), []*tab{left, source, copies[1], copies[0], right}) ||
		copies[0] == copies[1] || copies[1].active.profile.ID == source.active.profile.ID {
		t.Fatal("repeated menu copies did not occupy the slot immediately after their source")
	}
	a.newBlank()
	if a.tabs[len(a.tabs)-1] != a.activeTab() || a.tabs[len(a.tabs)-2] != right {
		t.Fatal("ordinary new tab did not append")
	}
}

func TestConnectedTabDoubleClickCreatesIndependentSession(t *testing.T) {
	a, profile, accepted := savedConnectionFixture(t)
	if err := a.vault.PutFor(profile, "saved-fixture-password", profile.Identity()); err != nil {
		t.Fatal(err)
	}
	a.request(profile, 0)
	waitOperator(t, a, func() bool { return len(a.tabs) == 1 && a.tabs[0].active.conn != nil && a.tabs[0].active.term != nil })
	original := a.tabs[0]
	connection := original.active.conn
	term := original.active.term
	a.newBlank()
	tail := a.activeTab()
	other, err := a.createGroup("other")
	if err != nil {
		t.Fatal(err)
	}
	a.moveToGroup(tail, other)
	tt := ui.NewTester(func(c *ui.Context) { a.groupTabBar(c, a.groupByID(original.group)) }, 900, 60)
	if err := tt.Click(original.name); err != nil {
		t.Fatal(err)
	}
	if len(a.tabs) != 2 {
		t.Fatal("single click duplicated session")
	}
	if err := tt.Click(original.name); err != nil {
		t.Fatal(err)
	}
	// Authentication completes asynchronously; the current tab may already have changed.
	a.selectTab(tail)
	waitOperator(t, a, func() bool { return len(a.tabs) == 3 && a.tabs[1].active.conn != nil && a.tabs[1].active.term != nil })
	copy := a.tabs[1]
	if a.tabs[2] != tail || copy == original || copy.active.conn == connection || copy.active.term == term || copy.group != original.group || a.activeTab() != copy || a.groups.recent != tail.id || accepted.Load() != 2 {
		t.Fatal("double click did not create/select one independent session in source group")
	}
	if original.active.closed || !connection.Alive() || len(a.config.Profiles) != 1 {
		t.Fatal("original connection or saved profiles changed")
	}
	a.dispose(copy.active)
	a.dispose(original.active)
}
func TestTabDoubleClickIgnoresUnavailableStates(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	for _, p := range []*pane{nil, {profile: core.Profile{Kind: "ssh"}, status: "正在连接…"}, {profile: core.Profile{Kind: "ssh"}, closed: true}} {
		item := &tab{id: core.NewID(), active: p}
		a.tabs = []*tab{item}
		if canDuplicateConnectedTab(item) {
			t.Fatal("unavailable pane accepted")
		}
		a.duplicateConnectedTab(item)
		if len(a.tabs) != 1 || a.authOpen {
			t.Fatal("unavailable double click started connection")
		}
	}
}

func TestLocalDoubleClickKeepsNewTabSelected(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, _ := operatorPane(t, a, "local-fixture")
	p.profile.Kind = "local"
	a.ensureGroups()
	original := a.tabs[0]
	a.selectTab(original)
	tt := ui.NewTester(func(c *ui.Context) { a.groupTabBar(c, a.groupByID(original.group)) }, 900, 60)
	if err := tt.Click(original.name); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(original.name); err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, item := range a.tabs {
			a.dispose(item.active)
		}
	}()
	if len(a.tabs) != 2 || a.activeTab() != a.tabs[1] {
		t.Fatalf("duplicate result: count=%d active=%d status=%s eligible=%v", len(a.tabs), a.current, a.status, canDuplicateConnectedTab(original))
	}
}

func TestFileTitleDoubleClickCreatesNewFileTab(t *testing.T) {
	a, profile, _ := savedConnectionFixture(t)
	profile.Kind = "sftp"
	profile.InitialDirectory = "/"
	a.config.Profiles[0] = profile
	if err := a.vault.PutFor(profile, "saved-fixture-password", profile.Identity()); err != nil {
		t.Fatal(err)
	}
	a.request(profile, 0)
	waitOperator(t, a, func() bool {
		return len(a.tabs) == 1 && a.tabs[0].active.files != nil && !a.tabs[0].active.files.busy && a.tabs[0].active.files.client != nil
	})
	original := a.tabs[0]
	originalClient := original.active.files.client
	a.newBlank()
	tail := a.activeTab()
	a.selectTab(original)
	tt := ui.NewTester(a.fileWindowView, 1000, 700)
	if err := tt.Click(profile.Host); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(profile.Host); err != nil {
		t.Fatal(err)
	}
	waitOperator(t, a, func() bool {
		return len(a.tabs) == 3 && a.tabs[1].active.files != nil && !a.tabs[1].active.files.busy && a.tabs[1].active.files.client != nil
	})
	copy := a.tabs[1]
	if a.tabs[2] != tail || a.activeTab() != copy || copy.active.files.client == originalClient || original.active.closed {
		t.Fatal("file tab duplicate failed")
	}
	a.localUI.ConfirmTabClose = false
	a.requestSessionClose(copy, nil)
	if a.activeTab() != original {
		t.Fatal("closing a file tab copy did not select its source")
	}
	a.requestSessionClose(original, nil)
	if a.activeTab() != tail {
		t.Fatal("closing the first file tab did not select the new first tab")
	}
}

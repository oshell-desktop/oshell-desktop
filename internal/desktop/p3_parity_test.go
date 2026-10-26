package desktop

import (
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestP3S02IdleControlsAndProtocolDraft(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p := core.NewProfile()
	p.Host = "fixture.invalid"
	p.KeepAlive.IdleEnabled = true
	p.KeepAlive.IdleText = "pwd\\r"
	a.config.Profiles = []core.Profile{p}
	before := a.config.Clone()
	a.edit(p)
	a.features.editTab = 3
	tt := ui.NewTester(a.connectionEditor, 900, 800)
	edit := func(label, text string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Type(text)
	}
	edit("空闲字符串", "SSH")
	if a.draft.KeepAlive.IdleText == p.KeepAlive.IdleText {
		t.Fatal("SSH field cannot be edited")
	}
	edit("空闲间隔（秒）", "7")
	interval, text := a.features.idleInterval, a.draft.KeepAlive.IdleText
	if interval == "3600" {
		t.Fatal("SSH interval cannot be edited")
	}
	a.draft.Kind = "sftp"
	a.syncEditorProtocol()
	a.features.editTab = 2
	tt.Frame()
	tt.Click("空闲时发送字符串")
	edit("空闲间隔（秒）", "9")
	edit("空闲字符串", "SFTP")
	if !a.draft.KeepAlive.IdleEnabled || a.features.idleInterval != interval || a.draft.KeepAlive.IdleText != text {
		t.Fatal("SFTP changed disabled draft")
	}
	a.draft.Kind = "ssh"
	a.syncEditorProtocol()
	a.features.editTab = 3
	tt.Frame()
	if a.features.idleInterval != interval || a.draft.KeepAlive.IdleText != text {
		t.Fatal("protocol switch lost draft")
	}
	tt.Click("空闲时发送字符串")
	edit("空闲字符串", "OFF")
	if a.draft.KeepAlive.IdleEnabled || a.draft.KeepAlive.IdleText != text {
		t.Fatal("disabled SSH idle field changed")
	}
	tt.Click("空闲时发送字符串")
	edit("空闲字符串", "BACK")
	if a.draft.KeepAlive.IdleText == text {
		t.Fatal("SSH field did not re-enable")
	}
	tt.Click("取消")
	if a.editing || !reflect.DeepEqual(before, a.config) {
		t.Fatal("cancel changed saved profile")
	}
	// Saving SFTP suppresses only the enable flag, retaining hidden data.
	a.edit(p)
	a.draft.Kind = "sftp"
	a.features.idleInterval = "123"
	a.draft.KeepAlive.IdleText = "retained\\r"
	if err := a.applyConnectionEditor(); err != nil {
		t.Fatal(err)
	}
	if a.draft.KeepAlive.IdleEnabled || a.draft.KeepAlive.IdleInterval != 123 || a.draft.KeepAlive.IdleText != "retained\\r" {
		t.Fatal("SFTP save lost hidden fields")
	}
}

func TestP3T07CharacterLimitsAndAggregation(t *testing.T) {
	for _, cluster := range []string{"a", "界", "e\u0301", "👩‍💻", "🇨🇳"} {
		for _, n := range []int{31, 32, 33} {
			name := strings.Repeat(cluster, n)
			want := name
			if n > 32 {
				want = strings.Repeat(cluster, 32) + "…"
			}
			if got := splitTitleLabel(name); got != want {
				t.Fatalf("%q × %d: %q", cluster, n, got)
			}
		}
	}
	if got := sessionTitleName("\x00A\nB\u0085\u200eC"); got != "ABC" {
		t.Fatalf("control filter: %q", got)
	}
	if sessionTitleName("\x00\n") != "未命名会话" {
		t.Fatal("empty filtered name")
	}
	if sessionTitleName(strings.Repeat("界", 257)) != strings.Repeat("界", 256) {
		t.Fatal("name scalar limit")
	}
	a := parityApp(t)
	makePane := func(name string) *pane {
		p := core.NewProfile()
		p.Name, p.TitleMode = name, "sessionName"
		return &pane{profile: p}
	}
	prefix := strings.Repeat("界", 32)
	left, right, third := makePane(prefix+"甲"), makePane(prefix+"乙"), makePane("third")
	workspace := &tab{root: &node{first: &node{pane: left}, second: &node{pane: right}}, active: left}
	// Different full names remain distinct even when their shortened labels match.
	want := prefix + "… · " + prefix + "…"
	if got := a.displayTabTitle(workspace); got != want {
		t.Fatal(got)
	}
	workspace.root = &node{first: workspace.root, second: &node{first: &node{pane: third}, second: &node{pane: makePane("third")}}}
	want += " · +2"
	workspace.active = third
	if got := a.displayTabTitle(workspace); got != want {
		t.Fatal("order/remaining pane count", got)
	}
	detail := a.tabConnectionDetails(workspace)
	if !strings.Contains(detail, prefix+"甲") || !strings.Contains(detail, prefix+"乙") || !strings.Contains(detail, "分屏 3（当前）") || !strings.Contains(detail, "分屏 4") {
		t.Fatal("incomplete detail", detail)
	}
	right.profile.Name = left.profile.Name
	if got := a.displayTabTitle(workspace); got != prefix+"… ×2 · third ×2" {
		t.Fatal("same-name count", got)
	}
	workspace.root = &node{pane: left}
	if got := a.displayTabTitle(workspace); got != prefix+"甲" {
		t.Fatal("single pane was shortened", got)
	}
	a.config.Profiles = []core.Profile{left.profile}
	a.config.Profiles[0].Name = "saved\nname"
	if a.paneDisplayTitle(left) != "savedname" || a.config.Profiles[0].Name != "saved\nname" || left.profile.Name != prefix+"甲" {
		t.Fatal("name presentation changed source")
	}
}

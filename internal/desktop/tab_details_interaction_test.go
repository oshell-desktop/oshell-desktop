package desktop

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestTabHoverDismissInteractions(t *testing.T) {
	for _, action := range []string{"escape", "arrow", "text", "composition", "outside click", "outside right click", "outside scroll", "blur", "hide", "identity change"} {
		t.Run(action, func(t *testing.T) {
			a := parityApp(t)
			p := &pane{id: "hover-pane", profile: core.Profile{Name: "hover fixture", Kind: "ssh", Host: "example.invalid"}}
			workspace := &tab{id: "hover-tab", name: p.profile.Name, active: p, root: &node{pane: p}}
			a.tabs = []*tab{workspace}
			draft := "draft"
			view := ui.NewTester(func(c *ui.Context) {
				a.observeTabDetails(c)
				ui.Column(c).Fill().Children(func() {
					anchor := ui.Button(c, "Tab anchor")
					ui.TextInput(c, &draft).Label("Terminal input").AutoFocus()
					a.tabDetailsHover(c, anchor, workspace)
				})
			}, 700, 600)
			anchor, _ := view.Find("Tab anchor")
			view.Move(anchor.X+5, anchor.Y+5)
			workspace.hover.since = time.Now().Add(-time.Second)
			workspace.hover.last = time.Now()
			view.Frame()
			if _, ok := view.Find("复制会话名称"); !ok {
				t.Fatal("card did not appear")
			}
			switch action {
			case "escape":
				view.Key(0, ui.KeyEscape)
			case "arrow":
				view.Key(0, ui.KeyLeft)
			case "text":
				view.Type("typed")
				if !strings.Contains(draft, "typed") {
					t.Fatal("observer consumed input")
				}
			case "composition":
				view.Compose("中文", 2)
			case "outside click":
				view.ClickAt(650, 550)
			case "outside right click":
				view.RightClickAt(650, 550)
			case "outside scroll":
				view.Scroll(650, 550, 0, 30)
			case "blur":
				view.SetFocused(false)
			case "hide":
				a.dismissTabDetails()
			case "identity change":
				p.profile.Name = "changed"
			}
			view.Frame()
			if _, ok := view.Find("复制会话名称"); ok {
				t.Fatal("card survived", action)
			}
			view.SetFocused(true)
			view.Frame()
			if _, ok := view.Find("复制会话名称"); ok {
				t.Fatal("card reopened without a new hover")
			}
			view.Move(650, 550)
			view.Move(anchor.X+5, anchor.Y+5)
			workspace.hover.since = time.Now().Add(-time.Second)
			workspace.hover.last = time.Now()
			view.Frame()
			if _, ok := view.Find("复制会话名称"); !ok {
				t.Fatal("card could not reopen after re-entering")
			}
			copy, _ := view.Find("复制会话名称")
			view.Scroll(copy.X+2, copy.Y+2, 0, 30)
			if _, ok := view.Find("复制会话名称"); !ok {
				t.Fatal("scroll inside card dismissed it")
			}
		})
	}
}

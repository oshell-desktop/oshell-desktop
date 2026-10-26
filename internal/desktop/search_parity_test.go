package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"testing"
	"time"
)

func TestPaneSearchIndependentAndCompact(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	left, _ := operatorPane(t, a, "left")
	right, _ := operatorPane(t, a, "right")
	left.term.Feed([]byte("left token\r\nleft token"))
	right.term.Feed([]byte("right result"))
	a.activatePane(a.tabs[0], left)
	left.search.query = "token"
	a.openSearch()
	waitOperator(t, a, func() bool { return !left.search.busy })
	a.activatePane(a.tabs[1], right)
	right.search.query = "result"
	a.openSearch()
	waitOperator(t, a, func() bool { return !right.search.busy })
	if len(left.search.matches) != 2 || len(right.search.matches) != 1 || left.search.query != "token" {
		t.Fatal("pane search state leaked")
	}
	for _, width := range []int{220, 760} {
		a.refreshTerminalSearch(right, 0, true)
		waitOperator(t, a, func() bool { return !right.search.busy })
		right.search.focus = true
		tt := ui.NewTester(func(c *ui.Context) { ui.Column(c).Fill().Children(func() { a.terminalSearchBar(c, a.tabs[1], right) }) }, width, 120)
		for _, label := range []string{"搜索终端文本", "上一个匹配", "下一个匹配", "关闭搜索", "搜索选项"} {
			r, ok := tt.Find(label)
			if !ok || r.X < 0 || r.X+r.W > float32(width)+1 {
				t.Fatal("compact search overflow", width, label, r)
			}
		}
		if width == 760 {
			snapshot(t, tt, "terminal-search-compact.png")
		}
		tt.Key(0, ui.KeyEscape)
		if right.search.open {
			t.Fatal("escape did not close pane search")
		}
		right.search.open = true
	}
	a.closeTerminalSearch(right)
	if !left.search.open || right.term.SearchSelection() != "" {
		t.Fatal("close affected other pane or kept selection")
	}
}

func TestPaneSearchRefreshKeepsAnchorAndCancelsStaleQuery(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, _ := operatorPane(t, a, "search")
	p.term.Feed([]byte("hit hit\r\n"))
	p.search.query = "hit"
	a.openSearch()
	waitOperator(t, a, func() bool { return !p.search.busy })
	a.refreshTerminalSearch(p, 1, false)
	waitOperator(t, a, func() bool { return !p.search.busy })
	if p.search.index != 1 {
		t.Fatal("next")
	}
	p.term.Feed([]byte("hit\r\n"))
	a.refreshTerminalSearch(p, 0, false)
	waitOperator(t, a, func() bool { return !p.search.busy })
	if p.search.index != 1 || len(p.search.matches) != 3 {
		t.Fatal("output lost anchor")
	}
	p.search.query = "missing"
	p.search.due = time.Now().Add(time.Second)
	a.refreshTerminalSearch(p, 1, false)
	waitOperator(t, a, func() bool { return !p.search.busy })
	if p.search.index != -1 || len(p.search.matches) != 0 || p.term.SearchSelection() != "" {
		t.Fatal("stale match retained")
	}
}

func TestPaneSearchCompositionDoesNotNavigateOrClose(t *testing.T) {
	a := parityApp(t)
	p, _ := operatorPane(t, a, "search")
	p.term.Feed([]byte("中文 中文"))
	a.openSearch()
	view := ui.NewTester(func(c *ui.Context) { ui.Column(c).Fill().Children(func() { a.terminalSearchBar(c, a.tabs[0], p) }) }, 600, 120)
	if err := view.Click("搜索终端文本"); err != nil {
		t.Fatal(err)
	}
	view.Compose("中", 1)
	generation := p.search.generation
	view.Key(0, ui.KeyEnter)
	if p.search.generation != generation && p.search.due.IsZero() {
		t.Fatal("IME confirmation navigated search", p.search.query, p.search.generation, generation)
	}
	view.Compose("中", 1)
	view.Key(0, ui.KeyEscape)
	if !p.search.open {
		t.Fatal("IME cancellation closed search")
	}
	view.Compose("", 0)
	view.Key(0, ui.KeyEscape)
	if p.search.open {
		t.Fatal("escape did not close after composition")
	}
}

func TestPaneSearchInvalidExpressionClearsOldSelection(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, _ := operatorPane(t, a, "search")
	p.term.Feed([]byte("hit"))
	p.search.query = "hit"
	a.openSearch()
	waitOperator(t, a, func() bool { return !p.search.busy })
	if p.term.SearchSelection() != "hit" {
		t.Fatal("fixture not selected")
	}
	p.search.query, p.search.regex = "[", true
	a.refreshTerminalSearch(p, 1, false)
	waitOperator(t, a, func() bool { return !p.search.busy })
	if p.search.err == "" || p.term.SearchSelection() != "" || len(p.search.matches) != 0 {
		t.Fatal("invalid expression retained an old result")
	}
}

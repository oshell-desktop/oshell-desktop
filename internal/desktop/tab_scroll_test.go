package desktop

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTabBarHiddenScrollbarStillScrolls(t *testing.T) {
	a := parityApp(t)
	for i := 0; i < 12; i++ {
		operatorPane(t, a, fmt.Sprintf("scroll fixture %02d", i))
	}
	a.ensureGroups()
	g := a.groupForNewTab()
	view := ui.NewTester(func(c *ui.Context) { a.groupTabBar(c, g) }, 400, 80)
	if g.scroll.MaxX <= 0 {
		t.Fatal("fixture did not overflow")
	}
	before := g.scroll.X
	view.Scroll(120, 15, 90, 0)
	if g.scroll.X <= before || len(a.tabs) != 12 {
		t.Fatal("horizontal tab scrolling changed after hiding bar", g.scroll)
	}
	view.Scroll(120, 15, -10000, 0)
	if g.scroll.X != 0 {
		t.Fatal("tab strip did not clamp at beginning", g.scroll)
	}
}

func TestTabSelectionRevealsTitleAndKeepsManualScroll(t *testing.T) {
	for _, file := range []bool{false, true} {
		a := parityApp(t)
		for i := 1; i <= 18; i++ {
			operatorPane(t, a, fmt.Sprintf("session %02d", i))
		}
		a.selectTab(a.tabs[0])
		g := a.groupForNewTab()
		var frame *ui.Context
		view := ui.NewTester(func(c *ui.Context) {
			frame = c
			if file {
				a.fileWindowView(c)
			} else {
				a.groupTabBar(c, g)
			}
		}, 520, 360)
		selectAndCheck := func(index int) {
			t.Helper()
			tab := a.tabs[index]
			a.selectTab(tab)
			view.Frame()
			strip := g.scroll.Handle.Bounds(frame)
			left, ok := view.Find(tab.name)
			right, found := view.Find("关闭 " + tab.name)
			if file {
				left, ok = view.Find("文件标签 " + tab.id)
				right, found = left, ok
			}
			if !ok || !found || left.X < strip.X-1 || right.X+right.W > strip.X+strip.W+1 {
				t.Fatalf("file=%v tab %d outside strip %v: %v %v", file, index+1, strip, left, right)
			}
		}
		selectAndCheck(17)
		if g.scroll.X <= 0 {
			t.Fatal("last tab stayed off screen")
		}
		strip := g.scroll.Handle.Bounds(frame)
		view.Scroll(strip.X+10, strip.Y+10, -10000, 0)
		view.Frame()
		if g.scroll.X != 0 {
			t.Fatal("ordinary repaint stole manual scroll", g.scroll.X)
		}
		selectAndCheck(17)
		selectAndCheck(8)
		selectAndCheck(0)
	}
}

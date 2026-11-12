package desktop

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestWorkspaceArrangementGeometry(t *testing.T) {
	for _, menu := range []string{"分屏排列", "分组排列"} {
		t.Run(menu, func(t *testing.T) {
			cfg := core.DefaultConfig()
			cfg.Settings.MasterWarningAcknowledged = true
			cfg.Settings.InterfaceTheme = "dark"
			a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
			var panes []*pane
			var connections []*operatorConn
			for i := range 11 {
				name := fmt.Sprintf("session-%02d", i+1)
				p, conn := operatorPane(t, a, name)
				p.term.Feed([]byte("\x1b[48;2;20;95;120m " + name + " \x1b[0m\r\n$ hostname\r\n" + name + "\r\n$ "))
				panes = append(panes, p)
				connections = append(connections, conn)
			}
			a.ensureGroups()
			active := a.tabs[3]
			a.selectTab(active)
			var frame *ui.Context
			frames := 0
			tt := ui.NewTester(func(c *ui.Context) { frames++; frame = c; a.view(c) }, 1180, 760)
			for _, mode := range []string{"标签左右排列", "标签上下排列", "标签瓷砖排列", "合并为选项卡"} {
				if err := tt.Click(menu); err != nil {
					t.Fatal(err)
				}
				if err := tt.ChooseMenuItem(mode); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if a.activeTab() != active {
					t.Fatal(mode, "changed active session")
				}
				if mode == "合并为选项卡" {
					if len(a.visiblePanes()) != 1 || len(a.tabs) != len(panes) {
						t.Fatal("merge lost tabs or left extra terminals visible")
					}
					continue
				}
				if len(a.visiblePanes()) != len(panes) {
					t.Fatal(mode, "lost panes")
				}
				var rects []ui.Rect
				for i, p := range panes {
					r := p.focus.Bounds(frame)
					if r.W < 219 || r.H < 139 {
						t.Fatal(mode, i, "collapsed terminal", r)
					}
					for _, prior := range rects {
						if min(prior.X+prior.W, r.X+r.W)-max(prior.X, r.X) > .5 && min(prior.Y+prior.H, r.Y+r.H)-max(prior.Y, r.Y) > .5 {
							t.Fatal(mode, i, "overlap", prior, r)
						}
					}
					rects = append(rects, r)
				}
				if mode == "标签瓷砖排列" {
					for i := 1; i < len(rects); i++ {
						r, prev := rects[i], rects[i-1]
						if i == 4 || i == 8 {
							if r.Y <= prev.Y || math.Abs(float64(r.X-rects[0].X)) > 1 {
								t.Fatalf("tile row %d: %v", i, r)
							}
						} else if math.Abs(float64(r.Y-prev.Y)) > 1 || r.X <= prev.X {
							t.Fatal("tile order", rects)
						}
					}
					if menu == "分屏排列" {
						img := tt.Image()
						for i, r := range rects {
							pixels := 0
							for y := int(r.Y); y < int(r.Y+min(r.H, 50)); y++ {
								for x := int(r.X); x < int(r.X+r.W); x++ {
									col := img.RGBAAt(x, y)
									if col.R == 20 && col.G == 95 && col.B == 120 {
										pixels++
									}
								}
							}
							if pixels < 20 {
								t.Fatal("terminal output missing in tile", i, r, pixels)
							}
						}
						snapshot(t, tt, "tiled-11.png")
						last := rects[len(rects)-1]
						tt.ClickAt(last.X+last.W/2, last.Y+last.H/2)
						tt.Type("tile-input")
						waitOperator(t, a, func() bool { return connections[len(connections)-1].text() == "tile-input" })
						if a.activePane() != panes[len(panes)-1] || connections[3].text() != "" {
							t.Fatal("tile input reached the wrong session")
						}
						a.selectTab(active)
						tt.SetSize(880, 560)
						tt.Frame()
						for _, p := range panes {
							r := p.focus.Bounds(frame)
							if r.H < 139 || r.W < 219 {
								t.Fatal("resize collapsed tile", r)
							}
						}
						tt.SetSize(1180, 760)
					}
				} else {
					// Selecting the last tab must bring its terminal into the overflow viewport.
					a.selectTab(a.tabs[len(panes)-1])
					tt.Frame()
					r := panes[len(panes)-1].focus.Bounds(frame)
					area, _ := tt.Find("排列滚动区")
					if !area.Contains(r.X+r.W/2, r.Y+r.H/2) {
						t.Fatal(mode, "selected pane remained offscreen", r, area)
					}
					a.selectTab(active)
					tt.Frame()
				}
				before := frames
				tt.Frame()
				if frames-before > 3 {
					t.Fatalf("%s keeps rebuilding idle workspace (%d frames)", mode, frames-before)
				}
			}
		})
	}
}

func TestSplitZoomedPaneFromToolbar(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, _ := operatorPane(t, a, "initial")
	p.profile.Kind = "blank"
	a.selectTab(a.tabs[0])
	var frame *ui.Context
	tt := ui.NewTester(func(c *ui.Context) { frame = c; a.view(c) }, 1180, 760)
	choose := func(label string) {
		t.Helper()
		if err := tt.Click("分屏"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(label); err != nil {
			t.Fatal(err)
		}
	}
	choose("左右分屏")
	right := a.activePane()
	if right == p {
		t.Fatal("no new pane")
	}
	t.Cleanup(func() { a.dispose(right); right.disposal.Wait() })
	waitOperator(t, a, func() bool { return right.term != nil })
	tt.Frame()
	choose("放大 / 还原当前分屏")
	if a.activeTab().zoomed != right {
		t.Fatal("zoom did not target new pane")
	}
	choose("上下分屏")
	bottom := a.activePane()
	if bottom == right || bottom == p {
		t.Fatal("second split did not open a pane")
	}
	t.Cleanup(func() { a.dispose(bottom); bottom.disposal.Wait() })
	waitOperator(t, a, func() bool { return bottom.term != nil })
	tt.Frame()
	l, r, b := p.focus.Bounds(frame), right.focus.Bounds(frame), bottom.focus.Bounds(frame)
	if a.activeTab().zoomed != nil || len(a.visiblePanes()) != 3 || l.W < 1 || r.H < 1 || b.H < 1 || l.X+l.W > r.X+1 || r.Y+r.H > b.Y+1 {
		t.Fatal("new split stayed hidden", l, r, b)
	}
}

func TestTileAfterDetachingZoomedPane(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	first, _ := operatorPane(t, a, "first")
	second, _ := operatorPane(t, a, "second")
	source := a.tabs[0]
	source.root = &node{id: core.NewID(), first: source.root, second: a.tabs[1].root}
	a.tabs = a.tabs[:1]
	a.selectTab(source)
	tt := ui.NewTester(a.view, 1180, 760)
	for _, action := range []string{"放大 / 还原当前分屏", "当前分屏移到独立标签"} {
		if err := tt.Click("分屏"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(action); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.Click("分屏排列"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("标签瓷砖排列"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.tabs) != 2 || source.zoomed != nil || source.active != second || a.activePane() != first || len(a.visiblePanes()) != 2 {
		t.Fatal("detached pane still belongs to the old zoomed view")
	}
}

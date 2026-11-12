package desktop

import "github.com/egoist/mygo/ui"

var arrangementChoices = [][2]string{{"tabs", "合并为选项卡"}, {"horizontal", "标签左右排列"}, {"vertical", "标签上下排列"}, {"tile", "标签瓷砖排列"}}

func (a *App) arrangementMenu(menu *ui.Menu) {
	for _, choice := range arrangementChoices {
		if menu.Item(choice[1]).Chosen() {
			a.arrangeTabs(choice[0])
			return
		}
	}
}
func (a *App) splitActivePane(direction int) {
	if direction != 1 && direction != 2 {
		return
	}
	if a.activeTab() == nil {
		a.local()
		return
	}
	if p := a.activePane(); p != nil && !p.closed {
		a.request(p.profile, direction, p.terminalOnly)
	}
}

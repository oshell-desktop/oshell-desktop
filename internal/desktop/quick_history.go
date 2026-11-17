package desktop

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"path/filepath"
	"strings"
)

func (a *App) quickHistory() core.QuickHistoryStore {
	return core.QuickHistoryStore{Directory: filepath.Join(a.store.Dir, "quick-send-history")}
}
func (a *App) rememberQuickSend(text string) error {
	err := a.quickHistory().Remember(text, a.features.operators.pendingReturn, a.localUI.QuickHistoryLimit)
	if err == nil {
		a.quickHistoryChanged()
	}
	return err
}
func (a *App) quickHistoryChanged() {
	reset := func(target *App) {
		o := &target.features.operators
		o.historyID = ""
		o.historyPage = ""
		o.historyRecords = nil
		o.historyPages = nil
		o.historyBrowseOpen = false
		if target.win != nil {
			target.win.Invalidate()
		}
	}
	reset(a)
	if a.hub != nil {
		for _, target := range a.hub.windows {
			reset(target)
		}
	}
}
func (a *App) chooseQuickHistory(entry core.QuickHistoryEntry) {
	o := &a.features.operators
	o.historyID = ""
	o.historyBrowseOpen = false
	if strings.ContainsAny(entry.Text, "\r\n") {
		a.openComposer(entry.Text, entry.AppendReturn)
	} else {
		a.quick, o.noReturn = entry.Text, !entry.AppendReturn
	}
}
func (a *App) clearQuickHistory() {
	go func() {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.fileParent(), Title: "清空快速发送历史", Message: "删除本机所有窗口共用的快速发送历史？此操作无法撤销。", Buttons: []string{"取消", "清空历史"}, DefaultButton: 0, CancelButton: 0})
		a.post(func() {
			if err != nil {
				a.status = "无法显示历史清理确认：" + err.Error()
				return
			}
			if result.Button == 1 {
				if err := a.quickHistory().Trim(0); err != nil {
					a.status = "清空历史失败：" + err.Error()
				} else {
					a.quickHistoryChanged()
				}
			}
		})
	}()
}
func (a *App) loadQuickHistoryPage(before string) bool {
	page, more, err := a.quickHistory().Page(before)
	if err != nil {
		a.status = "历史读取失败：" + err.Error()
		return false
	}
	o := &a.features.operators
	o.historyRecords, o.historyPageMore, o.historyPage = page, more, before
	return true
}
func (a *App) quickHistoryBrowser(c *ui.Context, anchor ui.Element) {
	o := &a.features.operators
	ui.Popover(c, anchor, &o.historyBrowseOpen, func() {
		ui.Column(c).Width(460).Padding(8).Gap(6).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				windowHeading(c, "快速发送历史").Grow(1)
				flatButton(c, "关闭命令历史", closeIcon, "").OnClick(func() { o.historyBrowseOpen = false })
			})
			ui.Scroll(c).Height(300).FillWidth().Children(func() {
				for _, entry := range o.historyRecords {
					label := []rune(strings.NewReplacer("\n", " ", "\r", " ").Replace(entry.Text))
					if len(label) > 100 {
						label = append(label[:100], '…')
					}
					ui.Button(c.Key(entry.ID), string(label)).Height(24).FillWidth().Justify(ui.Start).OnClick(func() { a.chooseQuickHistory(entry) })
				}
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "上一页").Disabled(len(o.historyPages) == 0).OnClick(func() {
					i := len(o.historyPages) - 1
					if a.loadQuickHistoryPage(o.historyPages[i]) {
						o.historyPages = o.historyPages[:i]
					}
				})
				ui.Button(c, "下一页").Disabled(!o.historyPageMore || len(o.historyRecords) == 0).OnClick(func() {
					before := o.historyPage
					next := o.historyRecords[len(o.historyRecords)-1].ID
					if a.loadQuickHistoryPage(next) {
						o.historyPages = append(o.historyPages, before)
					}
				})
			})
		})
	})
}
func (a *App) navigateQuickHistory(older bool) {
	o := &a.features.operators
	if o.historyID != "" && (a.quick != o.historyValue || o.noReturn != o.historyValueNoReturn) {
		o.historyID = ""
	}
	if o.historyID == "" {
		o.historyDraft, o.historyDraftNoReturn = a.quick, o.noReturn
	}
	entry, found, err := a.quickHistory().Neighbor(o.historyID, older)
	if err != nil {
		a.status = "快速发送历史读取失败：" + err.Error()
		return
	}
	if found {
		a.chooseQuickHistory(entry)
		o.historyID = entry.ID
	} else if !older {
		a.quick, o.noReturn, o.historyID = o.historyDraft, o.historyDraftNoReturn, ""
	}
	o.historyValue, o.historyValueNoReturn = a.quick, o.noReturn
}
func (a *App) quickHistoryMenu(m *ui.Menu) {
	o := &a.features.operators
	page, more, err := a.quickHistory().Page("")
	if err != nil {
		m.Item("历史读取失败").Disabled(true)
		a.status = err.Error()
		return
	}
	if len(page) == 0 {
		m.Item("暂无历史命令").Disabled(true)
	}
	for _, entry := range page {
		label := []rune(strings.ReplaceAll(entry.Text, "\n", " "))
		if len(label) > 60 {
			label = append(label[:60], '…')
		}
		if m.ItemWithID(entry.ID, string(label)).Chosen() {
			a.chooseQuickHistory(entry)
		}
	}
	if more {
		if m.Item("更早的历史…").Chosen() {
			if a.loadQuickHistoryPage(page[len(page)-1].ID) {
				o.historyPages = []string{""}
				o.historyBrowseOpen = true
			}
		}
	}
	m.Separator()
	if m.Item("清空快速发送历史…").Chosen() {
		a.clearQuickHistory()
	}
}

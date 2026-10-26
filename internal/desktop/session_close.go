package desktop

import "github.com/egoist/mygo/ui"

type sessionCloseRequest struct {
	open bool
	tab  *tab
	pane *pane
}

func paneNeedsCloseConfirmation(p *pane) bool {
	if p == nil || p.closed {
		return false
	}
	if p.files != nil && p.files.busy {
		return true
	}
	if p.term != nil {
		return p.term.HasActiveProcess()
	}
	return p.conn != nil && p.conn.Alive() || p.ftp != nil || p.status == "正在连接…"
}
func (a *App) requestSessionClose(t *tab, p *pane) {
	if a.features.sessionClose != nil || t == nil || a.tabByID(t.id) != t || p != nil && a.ownerTab(p) != t {
		return
	}
	request := &sessionCloseRequest{tab: t, pane: p, open: true}
	active := false
	if p != nil {
		active = paneNeedsCloseConfirmation(p)
	} else {
		a.walk(t.root, func(p *pane) { active = active || paneNeedsCloseConfirmation(p) })
	}
	if active && a.localUI.ConfirmTabClose {
		a.features.sessionClose = request
		return
	}
	a.finishSessionClose(request)
}
func (a *App) finishSessionClose(request *sessionCloseRequest) {
	if request == nil {
		return
	}
	a.features.sessionClose = nil
	if a.tabByID(request.tab.id) != request.tab || request.pane != nil && a.ownerTab(request.pane) != request.tab {
		a.status = "会话位置已变化，请重新选择关闭目标"
		return
	}
	if request.pane != nil {
		a.closeEndedPane(request.tab, request.pane)
	} else {
		a.withCommandEditorFocus(func() { a.closeTab(request.tab) })
	}
}
func (a *App) sessionCloseDialog(c *ui.Context) {
	request := a.features.sessionClose
	if request == nil {
		return
	}
	ui.Modal(c, &request.open, func() {
		ui.Column(c).Width(440).Gap(12).Children(func() {
			title := "关闭此标签？"
			detail := "此标签中的连接、本机工具与文件传输会结束。"
			if request.pane != nil {
				title = "关闭此分屏会话？"
				detail = "此分屏中的连接、本机工具与文件传输会结束，其他分屏保持连接。"
			}
			ui.Text(c, title).FontSize(18).Bold()
			ui.Text(c, detail).Wrap()
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				ui.Button(c, "取消关闭").OnClick(func() { a.features.sessionClose = nil })
				ui.Button(c, "确认关闭会话").OnClick(func() { a.finishSessionClose(request) })
			})
		})
	})
	if !request.open {
		a.features.sessionClose = nil
	}
}

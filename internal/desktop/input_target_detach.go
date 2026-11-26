package desktop

// Moving a terminal never carries its old window's input selections with it.
// Invalidate previews as well, so moving it back cannot revive an old dispatch.
func (a *App) detachInputTarget(p *pane) {
	if d := a.bastionPassword; d != nil && d.pane == p {
		a.closeBastionPassword()
	}
	delete(a.features.syncTargets, p.id)
	delete(a.features.syncSelectionDraft, p.id)
	o := &a.features.operators
	delete(o.selected, p.id)
	delete(o.selectionDraft, p.id)
	delete(o.composerTargets, p.id)
	for _, target := range o.pendingTargets {
		if target == p {
			o.pendingTargets = nil
			o.pendingText = ""
			a.confirmSend = false
			o.error = "目标会话已移到其他窗口，请重新选择发送目标"
			a.status = o.error
			break
		}
	}
	for _, target := range a.features.pasteTargets {
		if target.pane == p {
			a.cancelPaste()
			a.status = "目标会话已移到其他窗口，已取消待确认粘贴"
			break
		}
	}
}

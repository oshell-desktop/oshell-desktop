package desktop

// SF-O02: window-local history, newest first; drafts are never persisted.
func (a *App) rememberQuickSend(text string) {
	if len(text) == 0 || len(text) > 8192 {
		return
	}
	if len(a.history) == 0 || a.history[0] != text {
		a.history = append([]string{text}, a.history...)
	}
	bytes, count := 0, 0
	for _, entry := range a.history {
		if count == 50 || bytes+len(entry) > 64<<10 {
			break
		}
		bytes += len(entry)
		count++
	}
	a.history = a.history[:count]
}
func (a *App) navigateQuickHistory(older bool) {
	o := &a.features.operators
	if o.historyOffset > 0 && a.quick != o.historyValue {
		o.historyOffset = 0
	}
	if o.historyOffset == 0 {
		o.historyDraft = a.quick
	}
	if older {
		o.historyOffset = min(o.historyOffset+1, len(a.history))
	} else {
		o.historyOffset = max(o.historyOffset-1, 0)
	}
	a.quick = o.historyDraft
	if o.historyOffset > 0 {
		a.quick = a.history[o.historyOffset-1]
	}
	o.historyValue = a.quick
}

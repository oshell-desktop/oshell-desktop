package terminal

import (
	"time"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/terminal/internal/vt"
)

type SemanticClickBehavior uint8

const (
	SemanticClickEnabled SemanticClickBehavior = iota
	SemanticClickDisabled
	SemanticClickRequireModifier
)

type semanticPress struct {
	eligible, selectionActive, dragged bool
	clicks                             int
	delay                              time.Duration
}

type pendingSemanticClick struct {
	target vt.SemanticClickTarget
	owner  *vt.Terminal
	column int
	mods   ui.Modifiers
	timer  *time.Timer
}

func (v *view) semanticModifiersAllow(mods ui.Modifiers) bool {
	switch v.t.opts.SemanticClickBehavior {
	case SemanticClickEnabled:
		return mods == 0
	case SemanticClickRequireModifier:
		return mods == v.t.opts.SemanticClickModifiers
	default:
		return false
	}
}

// The semantic gesture and pending timer are protected by t.mu. Unlike the
// other view fields, the timer may read/update pendingSemantic off the UI
// thread; it never touches selection, layout, or window services.
func (v *view) cancelSemanticClick() {
	if pending := v.pendingSemantic; pending != nil {
		pending.timer.Stop()
		v.pendingSemantic = nil
	}
}

func (v *view) beginSemanticPress(ev ui.InputEvent) {
	v.cancelSemanticClick()
	delay := 500 * time.Millisecond
	if ev.HasDoubleClickSettings || ev.DoubleClickInterval > 0 {
		delay = max(0, ev.DoubleClickInterval)
	}
	v.semanticPress = semanticPress{selectionActive: v.t.term.SelectionActive(), clicks: max(1, ev.Clicks), delay: delay}
}

// Called after link, remote mouse, and selection-release routes. Only the
// press snapshot decides whether this was a selection gesture.
func (v *view) deferSemanticClick(col, row int, mods ui.Modifiers) {
	press := v.semanticPress
	if !press.eligible || press.selectionActive || press.dragged || press.clicks != 1 || !v.semanticModifiersAllow(mods) {
		return
	}
	target, ok := v.t.term.CaptureSemanticClickTarget(row + int(v.t.term.Scrollbar().Offset))
	if !ok {
		return
	}
	v.cancelSemanticClick()
	pending := &pendingSemanticClick{target: target, owner: v.t.term, column: col, mods: mods}
	v.pendingSemantic = pending
	pending.timer = time.AfterFunc(press.delay, func() { v.fireSemanticClick(pending) })
}

func (v *view) fireSemanticClick(pending *pendingSemanticClick) {
	t := v.t
	t.mu.Lock()
	if v.pendingSemantic != pending {
		t.mu.Unlock()
		return
	}
	v.pendingSemantic = nil
	pending.timer.Stop()
	if t.closed || t.term == nil || t.term != pending.owner || !v.semanticModifiersAllow(pending.mods) || !t.Available() {
		t.mu.Unlock()
		return
	}
	row, ok := t.term.ResolveSemanticClickTarget(pending.target)
	var data []byte
	if ok {
		data = t.term.SemanticClickData(pending.column, row)
		t.sendInputLocked(data)
	}
	t.mu.Unlock()
	if len(data) > 0 {
		t.redraw()
	}
}

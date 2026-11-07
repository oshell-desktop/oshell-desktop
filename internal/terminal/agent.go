package terminal

import (
	"context"
	"fmt"

	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal/internal/vt"
)

func (t *Terminal) AgentSend(ctx context.Context, data []byte, ticket string) error {
	if !t.Available() || t.InLocalToolsMode() {
		return fmt.Errorf("终端已断开或正在传输")
	}
	_, err := t.in.rawTicket(ctx, data, ticket)
	return err
}

func (t *Terminal) FileCommandReady() bool {
	if !t.ManagedInputAvailable() || t.InLocalToolsMode() {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil || t.term.AltScreen() || core.IsAuthenticationPrompt(t.term.CursorLine()) {
		return false
	}
	state, known := t.term.SemanticInputState()
	return !known || state != vt.SemanticInputArmed && state != vt.SemanticInputSubmitted
}
func (t *Terminal) AgentSnapshot() (screen []byte, text string, cols, rows int, eligible bool) {
	available := t.Available() && !t.InLocalToolsMode()
	t.mu.Lock()
	defer t.mu.Unlock()
	cols, rows = t.size.cols, t.size.rows
	if t.term == nil {
		return
	}
	screen = t.term.ScreenVT()
	scroll := t.term.Scrollbar()
	text = t.term.FormattedTextFrom(max(0, int(scroll.Total)-int(scroll.Len)), false)
	eligible = available && !t.term.AltScreen() && !core.IsAuthenticationPrompt(t.term.CursorLine())
	return
}

package terminal

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/terminal/internal/vt"
)

func expectSemanticInput(t *testing.T, term *Terminal, want vt.SemanticInputState) {
	t.Helper()
	term.mu.Lock()
	defer term.mu.Unlock()
	got, ok := term.term.SemanticInputState()
	if !ok || got != want {
		t.Fatalf("semantic input = %v (supported %v), want %v", got, ok, want)
	}
}

func TestSemanticInputActualSendRoutes(t *testing.T) {
	term, tt, conn := bidiViewFixture(t)
	arm := func() { term.Feed([]byte("\x1b]133;B\a")) }
	arm()
	tt.Type("draft")
	expectSemanticInput(t, term, vt.SemanticInputArmed)
	tt.Key(0, ui.KeyEnter)
	expectSemanticInput(t, term, vt.SemanticInputSubmitted)
	if got := conn.take(6); got != "draft\r" {
		t.Fatalf("keyboard bytes %q", got)
	}
	arm()
	term.Feed([]byte("\x1b[?2004h"))
	term.Paste("a\nb")
	expectSemanticInput(t, term, vt.SemanticInputArmed)
	if got := conn.take(15); got != "\x1b[200~a\nb\x1b[201~" {
		t.Fatalf("paste bytes %q", got)
	}
	term.Send([]byte("\r"))
	expectSemanticInput(t, term, vt.SemanticInputSubmitted)
	conn.take(1)
	arm()
	// An automatic CPR reply must not consume this partial user CSI.
	term.Send([]byte("\x1b[13;"))
	term.Feed([]byte("\x1b[6n"))
	term.Send([]byte("1u"))
	expectSemanticInput(t, term, vt.SemanticInputSubmitted)
	// Kitty Enter release is emitted but must not submit a newly armed input.
	arm()
	term.Feed([]byte("\x1b[=3u"))
	term.v.encodeKey(vt.KeyEvent{Action: vt.KeyRelease, Key: vt.KeyEnter}, false)
	expectSemanticInput(t, term, vt.SemanticInputArmed)
	term.v.sendKey(vt.KeyEvent{Action: vt.KeyPress, Key: vt.KeyEnter})
	expectSemanticInput(t, term, vt.SemanticInputSubmitted)
}

func TestSemanticInputRejectedQueueDoesNotConsumeScanner(t *testing.T) {
	term, _, _ := bidiViewFixture(t)
	term.Feed([]byte("\x1b]133;B\a"))
	term.in.pause(true)
	term.Send([]byte("\r\x1b[200~"))
	expectSemanticInput(t, term, vt.SemanticInputArmed)
	term.in.pause(false)
	term.Send([]byte("\n"))
	expectSemanticInput(t, term, vt.SemanticInputSubmitted)
	term.Feed([]byte("\x1b]133;B\a"))
	term.in.close()
	term.Send([]byte("\r"))
	expectSemanticInput(t, term, vt.SemanticInputArmed)
}

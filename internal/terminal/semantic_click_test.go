package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func deferredSemanticFixture(t *testing.T) (*Terminal, *ui.Tester, *pipe) {
	t.Helper()
	term, tt, conn := bidiViewFixture(t)
	term.Feed([]byte("\x1b]133;A;cl=m\a\x1b]133;B\aabc"))
	tt.Frame()
	return term, tt, conn
}

// A marker drains earlier writes through the actual input queue; checking
// the transport immediately could miss an incorrectly enqueued packet.
func requireNoSemanticPacket(t *testing.T, term *Terminal, conn *pipe) {
	t.Helper()
	term.Send([]byte("~"))
	if got := conn.take(1); got != "~" {
		t.Fatalf("unexpected packet before queue marker: %q", got)
	}
}

// Exercise the real pointer handler with an explicit host interval. Most
// cases fire the captured callback deterministically; the first test below
// separately verifies the actual timer through the window input route.
func deferredSemanticPress(term *Terminal, mods ui.Modifiers, clicks int, drag bool) *pendingSemanticClick {
	x, y := cellCenter(term, 0, 0)
	term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y, Clicks: clicks, Mods: mods, HasDoubleClickSettings: true, DoubleClickInterval: time.Hour})
	if drag {
		term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerMove, X: x + 2, Y: y, Mods: mods})
	}
	term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y, Mods: mods})
	term.mu.Lock()
	defer term.mu.Unlock()
	return term.v.pendingSemantic
}

func TestSemanticClickRealWindowDelayAndPacket(t *testing.T) {
	term, tt, conn := deferredSemanticFixture(t)
	x, y := cellCenter(term, 0, 0)
	start := time.Now()
	tt.ClickAt(x, y)
	term.mu.Lock()
	pending := term.v.pendingSemantic
	delay := term.v.semanticPress.delay
	term.mu.Unlock()
	if pending == nil || delay != 500*time.Millisecond {
		t.Fatal("single window click was not deferred", pending, delay)
	}
	if got := conn.take(0); got != "" {
		t.Fatalf("click wrote immediately: %q", got)
	}
	if got := conn.take(9); got != strings.Repeat("\x1b[D", 3) {
		t.Fatalf("delayed packet %q", got)
	}
	if time.Since(start) < delay {
		t.Fatal("timer fired before host interval")
	}
}

func TestSemanticClickWindowSecondPressCancels(t *testing.T) {
	term, tt, conn := deferredSemanticFixture(t)
	x, y := cellCenter(term, 0, 0)
	tt.ClickAt(x, y)
	term.mu.Lock()
	first := term.v.pendingSemantic
	term.mu.Unlock()
	if first == nil {
		t.Fatal("missing first deferral")
	}
	tt.ClickAt(x, y)
	term.mu.Lock()
	pending := term.v.pendingSemantic
	term.mu.Unlock()
	if pending != nil {
		t.Fatal("double click scheduled semantic input")
	}
	term.v.fireSemanticClick(first) // a timer already queued when Stop ran
	requireNoSemanticPacket(t, term, conn)
}

func TestSemanticClickGestureArbitration(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		mods                    ui.Modifiers
		clicks                  int
		drag, selection, remote bool
		behavior                SemanticClickBehavior
		required                ui.Modifiers
		want                    bool
	}{
		{"plain", 0, 1, false, false, false, SemanticClickEnabled, 0, true},
		{"drag", 0, 1, true, false, false, SemanticClickEnabled, 0, false},
		{"dismiss-selection", 0, 1, false, true, false, SemanticClickEnabled, 0, false},
		{"double", 0, 2, false, false, false, SemanticClickEnabled, 0, false},
		{"triple", 0, 3, false, false, false, SemanticClickEnabled, 0, false},
		{"shift", ui.Shift, 1, false, false, false, SemanticClickEnabled, 0, false},
		{"alt", ui.Alt, 1, false, false, false, SemanticClickEnabled, 0, false},
		{"ctrl", ui.Ctrl, 1, false, false, false, SemanticClickEnabled, 0, false},
		{"super", ui.Super, 1, false, false, false, SemanticClickEnabled, 0, false},
		{"remote", 0, 1, false, false, true, SemanticClickEnabled, 0, false},
		{"disabled", 0, 1, false, false, false, SemanticClickDisabled, 0, false},
		{"exact-required", ui.Alt, 1, false, false, false, SemanticClickRequireModifier, ui.Alt, true},
		{"extra-modifier", ui.Alt | ui.Shift, 1, false, false, false, SemanticClickRequireModifier, ui.Alt, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, _, _ := deferredSemanticFixture(t)
			term.mu.Lock()
			term.opts.SemanticClickBehavior, term.opts.SemanticClickModifiers = tc.behavior, tc.required
			if tc.selection {
				selection, ok := term.term.SelectAll()
				if !ok {
					t.Fatal("missing selection")
				}
				term.term.SetSelection(&selection)
			}
			term.mu.Unlock()
			if tc.remote {
				term.Feed([]byte("\x1b[?1000h"))
			}
			pending := deferredSemanticPress(term, tc.mods, tc.clicks, tc.drag)
			if (pending != nil) != tc.want {
				t.Fatalf("scheduled %v, want %v", pending != nil, tc.want)
			}
		})
	}
}

func TestSemanticClickFireRevalidatesIdentityAndState(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		want         bool
	}{
		{"moved-line", "\x1b[H\x1b[L\x1b[2;4H", true},
		{"ordinary-erase-redraw", "\x1b[H\x1b[2K\x1b]133;B\aabc", true},
		{"deleted-line", "\x1b[H\x1b[M", false},
		{"submitted", "\x1b]133;C\a", false},
		{"alternate", "\x1b[?47h", false},
		{"reset", "\x1bc\x1b]133;A;cl=m\a\x1b]133;B\aabc", false},
		{"mode-cleared", "\x1b[H\x1b]133;P\a\x1b]133;B\a", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, _, conn := deferredSemanticFixture(t)
			pending := deferredSemanticPress(term, 0, 1, false)
			if pending == nil {
				t.Fatal("missing deferral")
			}
			term.Feed([]byte(tc.output))
			term.v.fireSemanticClick(pending)
			want, count := "", 0
			if tc.want {
				want, count = strings.Repeat("\x1b[D", 3), 9
			}
			if !tc.want {
				requireNoSemanticPacket(t, term, conn)
			} else if got := conn.take(count); got != want {
				t.Fatalf("packet %q, want %q", got, want)
			}
		})
	}
}

func TestSemanticClickCheapGateAndQueue(t *testing.T) {
	for _, output := range []string{"\x1bc", "\x1b]133;C\a", "\x1b[?47h", "\x1b[H\x1b]133;P\a\x1b]133;B\a"} {
		term, _, _ := deferredSemanticFixture(t)
		term.Feed([]byte(output))
		if deferredSemanticPress(term, 0, 1, false) != nil {
			t.Fatal("ineligible state scheduled timer", output)
		}
	}
	term, _, conn := deferredSemanticFixture(t)
	pending := deferredSemanticPress(term, 0, 1, false)
	if pending == nil {
		t.Fatal("missing deferral")
	}
	term.in.pause(true)
	term.v.fireSemanticClick(pending)
	term.in.pause(false)
	requireNoSemanticPacket(t, term, conn)
	pending = deferredSemanticPress(term, 0, 1, false)
	if pending == nil {
		t.Fatal("missing second deferral")
	}
	term.Close()
	term.v.fireSemanticClick(pending)
	if got := conn.take(0); got != "" {
		t.Fatal("closed terminal accepted input", got)
	}
}

func TestSemanticClickLinkPrecedesRequiredModifier(t *testing.T) {
	term, tt, _ := deferredSemanticFixture(t)
	term.mu.Lock()
	term.opts.SemanticClickBehavior = SemanticClickRequireModifier
	term.opts.SemanticClickModifiers = ui.Cmd
	term.mu.Unlock()
	term.Feed([]byte("\x1b[H\x1b]8;;https://example.test/\x1b\\abc\x1b]8;;\x1b\\"))
	tt.Frame()
	if deferredSemanticPress(term, ui.Cmd, 1, false) != nil {
		t.Fatal("link activation also scheduled semantic input")
	}
}

package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestPointerCancellationDuringTransfer(t *testing.T) {
	for _, end := range []string{"cancel", "release"} {
		t.Run(end, func(t *testing.T) {
			term, tt, conn := bidiViewFixture(t)
			term.Feed([]byte(strings.Repeat("abcdefghij\r\n", 120)))
			tt.Frame()
			term.mu.Lock()
			term.term.ScrollToRow(20)
			term.mu.Unlock()
			tt.Frame()
			v := term.v
			term.opts.CopyOnSelect = true
			v.services.WriteClipboard("keep clipboard")
			x, y := cellCenter(term, 2, 2)
			beginCharacterDrag(tt, x, y)
			y = float32(v.oy+(v.rows+3)*v.cellH) / v.scale
			tt.Move(x, y)
			if !v.selecting || !v.ticking {
				t.Fatal("fixture did not start selection autoscroll")
			}
			term.in.pause(true)
			if end == "cancel" {
				tt.CancelPointer()
			} else {
				tt.Release(x, y)
			}
			if v.selecting || v.ticking || v.linkPressed || v.selectionScrollDelta != 0 || v.semanticPress.eligible {
				t.Fatal("transfer swallowed pointer cancellation")
			}
			term.mu.Lock()
			before := term.term.Scrollbar().Offset
			v.autoscrollAt(time.Now().Add(time.Hour))
			after := term.term.Scrollbar().Offset
			term.mu.Unlock()
			if before != after {
				t.Fatal("finished gesture continued scrolling")
			}
			if end == "cancel" && tt.Clipboard() != "keep clipboard" {
				t.Fatal("cancel copied the selection")
			}
			if end == "release" && (tt.Clipboard() == "keep clipboard" || tt.Clipboard() == "") {
				t.Fatal("transfer suppressed copy-on-select at release")
			}
			term.in.pause(false)
			tt.Release(x, y)
			requireNoSemanticPacket(t, term, conn)
		})
	}
}

func TestRemotePointerCancellationDuringTransfer(t *testing.T) {
	for _, end := range []string{"cancel", "release"} {
		t.Run(end, func(t *testing.T) {
			term, tt, conn := bidiViewFixture(t)
			term.Feed([]byte("\x1b[?1002h\x1b[?1006h"))
			tt.Frame()
			x, y := cellCenter(term, 2, 3)
			tt.Press(x, y)
			if got := conn.take(9); got != "\x1b[<0;3;4M" {
				t.Fatal(got)
			}
			term.in.pause(true)
			if end == "cancel" {
				tt.CancelPointer()
			} else {
				tt.Release(x, y)
			}
			if term.v.reporting {
				t.Fatal("transfer retained remote pointer state")
			}
			term.in.pause(false)
			// A cancellation must not bypass the protocol's exclusive input gate or
			// replay its release after transfer completion.
			requireNoSemanticPacket(t, term, conn)
			tt.Press(x, y)
			tt.Release(x, y)
			if got := conn.take(18); got != "\x1b[<0;3;4M\x1b[<0;3;4m" {
				t.Fatalf("next gesture inherited old reporting state: %q", got)
			}
		})
	}
}

func TestKeyboardCancellationOnHiddenPausedView(t *testing.T) {
	loadLib(t)
	conn := newPipe()
	term, err := New(Options{Conn: conn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	visible := true
	tt := ui.NewTester(func(c *ui.Context) {
		if visible {
			View(c, term).Fill().AutoFocus()
		} else {
			ui.Text(c, "another tab")
		}
	}, 400, 200)
	term.Feed([]byte("\x1b[=31u"))
	v := term.v
	v.input(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyA, Unshifted: 'a', TextPending: true})
	if v.pending == nil {
		t.Fatal("fixture did not defer the key")
	}
	v.preedit, v.preeditCaret = "旧", 1
	v.pressedAliases = map[keyIdentity]KeyAlias{{key: ui.KeyHome}: {TargetKey: ui.KeyA}}
	v.composedKeys = map[keyIdentity]bool{{key: ui.KeyA}: true}
	forwarded := 0
	term.opts.OnUserInput = func(ui.InputEvent) { forwarded++ }
	term.in.pause(true)
	visible = false
	tt.Frame() // prune cancels focus without another terminal build
	if v.pending != nil || v.preedit != "" || v.preeditCaret != 0 || len(v.pressedAliases) != 0 || len(v.composedKeys) != 0 {
		t.Fatal("paused hidden view retained keyboard/composition state")
	}
	term.in.pause(false)
	visible = true
	tt.Frame()
	requireNoSemanticPacket(t, term, conn)
	if forwarded != 0 {
		t.Fatal("lifecycle cancellation was broadcast as user input")
	}
	v.input(ui.InputEvent{Kind: ui.InputText, Text: "新"})
	term.Send([]byte("END"))
	want := "\x1b[0;;26032uEND"
	if got := conn.take(len(want)); got != want {
		t.Fatalf("new text inherited old key: %q", got)
	}
}

func TestPointerCancellationWithoutUsableGrid(t *testing.T) {
	for _, mode := range []string{"width", "height", "scale", "closed"} {
		t.Run(mode, func(t *testing.T) {
			term, _, conn := deferredSemanticFixture(t)
			pending := deferredSemanticPress(term, 0, 1, false)
			if pending == nil {
				t.Fatal("fixture did not defer semantic click")
			}
			v := term.v
			v.selecting, v.reporting, v.overlay.dragging = true, true, true
			v.linkPressed, v.linkDragged = true, true
			v.setSelectionAutoscroll(3, time.Now())
			switch mode {
			case "width":
				v.cellW = 0
			case "height":
				v.cellH = 0
			case "scale":
				v.scale = 0
			case "closed":
				term.Close()
			}
			if !v.input(ui.InputEvent{Kind: ui.InputPointerCancel}) {
				t.Fatal("grid gate swallowed cancellation")
			}
			term.mu.Lock()
			pendingAfter := v.pendingSemantic
			term.mu.Unlock()
			if pendingAfter != nil || v.selecting || v.reporting || v.overlay.dragging || v.linkPressed || v.linkDragged || v.ticking || v.selectionScrollDelta != 0 || !v.selectionScrollNext.IsZero() {
				t.Fatal("cancel retained pointer state without a usable grid")
			}
			v.fireSemanticClick(pending) // already queued before Stop
			if mode != "closed" {
				requireNoSemanticPacket(t, term, conn)
			}
		})
	}
}

func TestPointerReleaseDuringTransferKeepsLinkSemantics(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		term, tt, conn := bidiViewFixture(t)
		term.Feed([]byte("\x1b]8;;https://example.test/\x1b\\link\x1b]8;;\x1b\\"))
		tt.Frame()
		x, y := cellCenter(term, 1, 0)
		term.v.handleInput(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y})
		term.in.pause(true)
		if cancel {
			term.v.handleInput(ui.InputEvent{Kind: ui.InputPointerCancel})
		} else {
			term.v.handleInput(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y, Mods: ui.Cmd})
		}
		opened := tt.OpenedURLs()
		if cancel && len(opened) != 0 || !cancel && (len(opened) != 1 || opened[0] != "https://example.test/") {
			t.Fatalf("cancel=%v URLs=%v", cancel, opened)
		}
		term.in.pause(false)
		requireNoSemanticPacket(t, term, conn)
	}
}

func TestPointerReleaseAfterConnectionClose(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	x, y := cellCenter(term, 0, 0)
	tt.Press(x, y)
	if !term.v.selecting {
		t.Fatal("fixture has no held pointer")
	}
	term.Close()
	tt.Release(x, y)
	if term.v.selecting || term.v.reporting || term.v.linkPressed || term.v.ticking {
		t.Fatal("release after close retained a held pointer")
	}
}

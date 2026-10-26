package terminal

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/zmodem"
)

func TestTransferKeepsLocalSelectionCopyAndScroll(t *testing.T) {
	term, tt, conn := bidiViewFixture(t)
	term.Feed([]byte(strings.Repeat("abcdefghij\r\n", 100)))
	tt.Frame()
	term.opts.CopyOnSelect = true
	forwarded := 0
	term.opts.OnUserInput = func(ui.InputEvent) { forwarded++ }
	term.in.pause(true)
	x, y := cellCenter(term, 0, 0)
	end, _ := cellCenter(term, 5, 0)
	beginCharacterDrag(tt, x-2, y)
	tt.Move(end+2, y)
	tt.Release(end+2, y)
	if got := tt.Clipboard(); got != "abcde" {
		t.Fatalf("paused transfer disabled selection/copy: %q", got)
	}
	term.v.services.WriteClipboard("old")
	tt.Command("copy")
	if tt.Clipboard() != "abcde" {
		t.Fatal("paused transfer disabled Copy command")
	}
	tt.Command("selectAll")
	if !strings.Contains(tt.Clipboard(), "abcdefghij\nabcdefghij") {
		t.Fatal("Select All did not apply copy-on-select during transfer")
	}
	term.mu.Lock()
	before := term.term.Scrollbar().Offset
	term.mu.Unlock()
	tt.Scroll(x, y, 0, -40)
	term.mu.Lock()
	wheel := term.term.Scrollbar().Offset
	term.mu.Unlock()
	tt.Key(0, ui.KeyPageUp)
	term.mu.Lock()
	page := term.term.Scrollbar().Offset
	term.mu.Unlock()
	if wheel >= before || page >= wheel {
		t.Fatalf("local scroll blocked: before=%d wheel=%d page=%d", before, wheel, page)
	}
	if forwarded != 0 {
		t.Fatal("paused input was broadcast", forwarded)
	}
	term.in.pause(false)
	requireNoSemanticPacket(t, term, conn)
}

func TestTransferDropsKeyboardAndMouseWithoutIdleOrBroadcast(t *testing.T) {
	term, tt, conn := bidiViewFixture(t)
	term.Feed([]byte("\x1b[=31u\x1b[?1002h\x1b[?1006h"))
	tt.Frame()
	forwarded := 0
	term.opts.OnUserInput = func(ui.InputEvent) { forwarded++ }
	term.in.pause(true)
	tt.TypeKey(0, ui.KeyA, "a")
	tt.Compose("拼", 1)
	tt.Type("拼音")
	term.v.handleInput(ui.InputEvent{Kind: ui.InputText, Text: "literal", Literal: true})
	x, y := cellCenter(term, 2, 3)
	tt.Press(x, y)
	if !term.v.reporting {
		t.Fatal("paused view skipped remote mouse gesture state")
	}
	tt.Move(x+4, y)
	tt.Release(x+4, y)
	tt.Scroll(x, y, 0, 40)
	if term.v.reporting || term.v.pending != nil || term.v.preedit != "" {
		t.Fatal("completed input retained transient state")
	}
	if term.idleEditing.Load() || forwarded != 0 {
		t.Fatal("rejected input changed idle editing or broadcast", forwarded)
	}
	term.in.pause(false)
	requireNoSemanticPacket(t, term, conn)
	tt.Press(x, y)
	tt.Release(x, y)
	if got := conn.take(18); got != "\x1b[<0;3;4M\x1b[<0;3;4m" {
		t.Fatalf("resumed mouse inherited a paused event: %q", got)
	}
}

func TestSelectAllHonorsCopyPolicyDuringTransfer(t *testing.T) {
	for _, paused := range []bool{false, true} {
		for _, copyOnSelect := range []bool{false, true} {
			term, tt, conn := bidiViewFixture(t)
			term.opts.CopyOnSelect = copyOnSelect
			term.opts.TrimCopyLeading, term.opts.TrimCopyTrailing = true, true
			term.Feed([]byte("  alpha  "))
			tt.Frame()
			term.v.services.WriteClipboard("keep")
			term.in.pause(paused)
			tt.Command("selectAll")
			want := "keep"
			if copyOnSelect {
				want = "alpha"
			}
			if got := tt.Clipboard(); got != want {
				t.Fatalf("paused=%v copy=%v clipboard=%q want=%q", paused, copyOnSelect, got, want)
			}
			term.in.pause(false)
			requireNoSemanticPacket(t, term, conn)
		}
	}
}

func TestTransferEncodedInterruptAndRawProtocolIsolation(t *testing.T) {
	for _, kitty := range []bool{false, true} {
		name := "legacy"
		if kitty {
			name = "kitty"
		}
		t.Run(name, func(t *testing.T) {
			loadLib(t)
			conn := newPipe()
			started, interrupted, finish := make(chan struct{}), make(chan struct{}), make(chan struct{})
			finished := make(chan error, 1)
			var transferContext context.Context
			forwarded := 0
			term, err := New(Options{Conn: conn, NoBlink: true, AutomaticTransfers: true,
				OnUserInput: func(ui.InputEvent) { forwarded++ },
				TransferHandler: func(ctx context.Context, _ bool, protocol net.Conn) error {
					transferContext = ctx
					// This is protocol data, not keyboard input. A payload byte 3
					// must not cancel its own transfer.
					if _, err := protocol.Write([]byte{3}); err != nil {
						return err
					}
					close(started)
					<-ctx.Done()
					close(interrupted)
					<-finish
					return ctx.Err()
				}, TransferFinished: func(err error) { finished <- err }})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				close(finish)
				term.CancelTransfer()
				select {
				case <-finished:
				case <-time.After(2 * time.Second):
					t.Error("transfer cleanup stalled")
				}
				term.Close()
			})
			tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 400, 200)
			if kitty {
				term.Feed([]byte("\x1b[=31u"))
			}
			term.transfers.feed(zmodem.BuildZhexHeader(zmodem.FrameZRINIT, 0))
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("transfer did not start")
			}
			if got := conn.take(1); got != "\x03" {
				t.Fatalf("raw protocol output %q", got)
			}
			select {
			case <-interrupted:
				t.Fatal("raw protocol byte interrupted transfer")
			default:
			}
			tt.Key(ui.Ctrl, ui.KeyC)
			if kitty {
				// Swift checks encoded bytes, not the physical shortcut: Kitty's
				// CSI-u Ctrl+C contains no literal byte 3.
				if transferContext.Err() != nil {
					t.Fatal("Kitty Ctrl+C was treated as a literal interrupt")
				}
				// Output parsing may own the controller while the UI sends a
				// key. Cancellation must not invert controller/terminal locks.
				term.transfers.mu.Lock()
				returned := make(chan struct{})
				go func() { term.Send([]byte("prefix\x03suffix")); close(returned) }()
				blocked := false
				select {
				case <-returned:
				case <-time.After(time.Second):
					blocked = true
				}
				term.transfers.mu.Unlock()
				if blocked {
					t.Fatal("interrupt waited for the transfer controller lock")
				}
			}
			select {
			case <-interrupted:
			case <-time.After(time.Second):
				t.Fatal("encoded interrupt did not cancel transfer")
			}
			if forwarded != 0 || term.idleEditing.Load() || term.Available() {
				t.Fatal("interruption bypassed paused input state", forwarded)
			}
			// Cleanup releases the handler; its abort sequence uses the raw
			// path. No keyboard packet was queued while paused.
			term.in.mu.Lock()
			for _, packet := range term.in.queue {
				if !packet.raw {
					t.Error("paused keyboard entered the transfer queue")
				}
			}
			term.in.mu.Unlock()
		})
	}
}

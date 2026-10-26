package terminal

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
)

// Expectations follow MacTerminalView.keyDown and AppleTerminalView.pageUp /
// pageDown in OShell 0.2.92, including full-page distance and alternate screen.
func TestPageKeyProtocolAndViewportMatrix(t *testing.T) {
	cases := 0
	for _, app := range []bool{false, true} {
		for _, alt := range []bool{false, true} {
			term, _, conn := bidiViewFixture(t)
			term.Feed([]byte(strings.Repeat("history\r\n", 240)))
			if alt {
				term.Feed([]byte("\x1b[?1049h"))
			}
			if app {
				term.Feed([]byte("\x1b[?1h"))
			}
			for flags := 0; flags < 32; flags++ {
				term.Feed([]byte(fmt.Sprintf("\x1b[=%du", flags)))
				for _, mods := range []ui.Modifiers{0, ui.Shift, ui.Ctrl, ui.Alt} {
					for _, keypad := range []bool{false, true} {
						for _, key := range []ui.Key{ui.KeyPageUp, ui.KeyPageDown} {
							term.mu.Lock()
							term.term.ScrollToRow(100)
							before := term.term.Scrollbar().Offset
							_, rows := term.term.Size()
							term.mu.Unlock()
							for i, kind := range []ui.InputKind{ui.InputKeyDown, ui.InputKeyDown, ui.InputKeyUp} {
								if !term.v.input(ui.InputEvent{Kind: kind, Key: key, Keypad: keypad, Mods: mods, Repeat: i == 1}) {
									t.Fatalf("unhandled event %v", kind)
								}
							}
							local := flags == 0 && !app && !alt && !keypad && mods == 0
							want := "END"
							if !local {
								want = pageKeyBytes(key, keypad, mods, flags) + want
							}
							term.Send([]byte("END"))
							if got := conn.take(len(want)); got != want {
								t.Fatalf("flags=%d app=%v alt=%v key=%d keypad=%v mods=%d: %q, want %q", flags, app, alt, key, keypad, mods, got, want)
							}
							term.mu.Lock()
							after := term.term.Scrollbar().Offset
							bottom := term.term.AtBottom()
							term.mu.Unlock()
							if local {
								step := 2 * rows
								if key == ui.KeyPageUp {
									step = -step
								}
								if int(after) != int(before)+step {
									t.Fatalf("local page distance %d -> %d, rows %d, key %d", before, after, rows, key)
								}
							} else if !bottom {
								t.Fatalf("encoded page key did not return to active screen")
							}
							cases++
						}
					}
				}
			}
		}
	}
	if cases != 2048 {
		t.Fatalf("matrix cases %d", cases)
	}
}

func pageKeyBytes(key ui.Key, keypad bool, mods ui.Modifiers, flags int) string {
	code, final := 5, "~"
	if key == ui.KeyPageDown {
		code = 6
	}
	if keypad && flags != 0 {
		code += 57416
		final = "u"
	}
	modifier := 1
	if mods&ui.Shift != 0 {
		modifier++
	}
	if mods&ui.Alt != 0 {
		modifier += 2
	}
	if mods&ui.Ctrl != 0 {
		modifier += 4
	}
	var out strings.Builder
	for event := 1; event <= 3; event++ {
		if event == 3 && flags&2 == 0 {
			break
		}
		params := fmt.Sprint(code)
		typed := event > 1 && flags&2 != 0
		if modifier != 1 || typed {
			params += fmt.Sprintf(";%d", modifier)
		}
		if typed {
			params += fmt.Sprintf(":%d", event)
		}
		out.WriteString("\x1b[" + params + final)
	}
	return out.String()
}

func TestPageKeysBoundariesSelectionAndAliases(t *testing.T) {
	term, input, conn := bidiViewFixture(t)
	// No history: a page key is consumed locally, rather than leaking to
	// readline or showing a scrollbar that has nowhere to move.
	for _, key := range []ui.Key{ui.KeyPageUp, ui.KeyPageDown} {
		input.Key(0, key)
		term.Send([]byte("END"))
		if got := conn.take(3); got != "END" {
			t.Fatalf("empty history: %q", got)
		}
		if !term.v.overlay.activity.IsZero() {
			t.Fatal("empty history revealed scrollbar")
		}
	}
	term.Feed([]byte(strings.Repeat("history\r\n", 240)))
	for _, top := range []bool{false, true} {
		term.mu.Lock()
		key := ui.KeyPageDown
		if top {
			term.term.ScrollToTop()
			key = ui.KeyPageUp
		} else {
			term.term.ScrollToBottom()
		}
		before := term.term.Scrollbar().Offset
		selection, ok := term.term.SelectAll()
		if !ok {
			term.mu.Unlock()
			t.Fatal("fixture selection failed")
		}
		term.term.SetSelection(&selection)
		term.mu.Unlock()
		term.v.overlay.hide()
		input.Key(0, key)
		term.Send([]byte("END"))
		if got := conn.take(3); got != "END" {
			t.Fatalf("boundary leaked page: %q", got)
		}
		term.mu.Lock()
		after := term.term.Scrollbar().Offset
		_, selected := term.term.SelectionText()
		term.mu.Unlock()
		if before != after || selected || !term.v.overlay.activity.IsZero() {
			t.Fatalf("boundary top=%v offset %d/%d selection=%v", top, before, after, selected)
		}
	}
	term.mu.Lock()
	term.term.ScrollToRow(100)
	_, rows := term.term.Size()
	term.mu.Unlock()
	term.SetKeyAliases([]KeyAlias{{Key: ui.KeyF6, TargetKey: ui.KeyPageUp}})
	input.Key(0, ui.KeyF6)
	term.Send([]byte("END"))
	if got := conn.take(3); got != "END" {
		t.Fatalf("alias bypassed local page routing: %q", got)
	}
	term.mu.Lock()
	offset := term.term.Scrollbar().Offset
	term.mu.Unlock()
	if int(offset) != 100-rows {
		t.Fatalf("alias page distance: %d, rows %d", offset, rows)
	}
}

func TestPageKeySynchronizationUsesEachTerminalState(t *testing.T) {
	modes := []string{"legacy", "application", "kitty", "alternate"}
	configure := func(term *Terminal, mode string) {
		term.Feed([]byte(strings.Repeat("history\r\n", 240)))
		switch mode {
		case "application":
			term.Feed([]byte("\x1b[?1h"))
		case "kitty":
			term.Feed([]byte("\x1b[=3u"))
		case "alternate":
			term.Feed([]byte("\x1b[?1049h"))
		}
	}
	for _, leftMode := range modes {
		for _, rightMode := range modes {
			left, input, lconn := bidiViewFixture(t)
			right, rightInput, rconn := bidiViewFixture(t)
			rightInput.SetSize(640, 160) // each target uses its own page height
			configure(left, leftMode)
			configure(right, rightMode)
			left.opts.OnUserInput = right.ReplayUserInput
			for _, key := range []ui.Key{ui.KeyPageUp, ui.KeyPageDown} {
				for _, term := range []*Terminal{left, right} {
					term.mu.Lock()
					term.term.ScrollToRow(100)
					term.mu.Unlock()
				}
				input.Key(0, key)
				for i, term := range []*Terminal{left, right} {
					mode, conn := leftMode, lconn
					if i == 1 {
						mode, conn = rightMode, rconn
					}
					code := 5
					if key == ui.KeyPageDown {
						code = 6
					}
					want := ""
					if mode != "legacy" {
						want = fmt.Sprintf("\x1b[%d~", code)
					}
					if mode == "kitty" {
						want += fmt.Sprintf("\x1b[%d;1:3~", code)
					}
					want += "END"
					term.Send([]byte("END"))
					if got := conn.take(len(want)); got != want {
						t.Fatalf("%s -> %s side %d key %d: %q, want %q", leftMode, rightMode, i, key, got, want)
					}
					term.mu.Lock()
					after := term.term.Scrollbar().Offset
					_, rows := term.term.Size()
					bottom := term.term.AtBottom()
					term.mu.Unlock()
					if mode == "legacy" {
						step := rows
						if key == ui.KeyPageUp {
							step = -step
						}
						if int(after) != 100+step {
							t.Fatalf("target %d did not use its own page height: %d/%d", i, after, rows)
						}
					} else if !bottom {
						t.Fatalf("remote target %d stayed in history", i)
					}
				}
			}
		}
	}
}

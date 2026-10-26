//go:build windows && (amd64 || arm64)

package windows

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestCompositionTargetBoundary(t *testing.T) {
	for _, mode := range []string{"switch", "disable", "same", "blur"} {
		t.Run(mode, func(t *testing.T) {
			h := &keypadRecorder{}
			s := &surface{w: &window{h: h}, input: platform.TextInputState{Active: true, FunctionalTarget: 1}, ime: true}
			s.high = 0xd83d
			s.layoutComposing = true
			s.layoutKeys = map[uint16]heldLayout{1: {base: 'a'}}
			s.keypadKeys = map[uint16]platform.Key{1: platform.Key1}
			s.reconvert = &[2]int{2, 3}
			next := s.input
			switch mode {
			case "switch":
				next.FunctionalTarget = 2
			case "disable":
				next.Active = false
			case "same":
				next.Caret.X = 20
			}
			if mode == "blur" {
				s.message(0, wmKillFocus, 0, 0)
			} else {
				s.SetTextInput(next)
			}
			if mode == "same" {
				if s.high == 0 || !s.layoutComposing || s.reconvert == nil || len(s.layoutKeys) != 1 || len(s.keypadKeys) != 1 {
					t.Fatal("caret movement discarded the current composition")
				}
				return
			}
			if s.high != 0 || s.layoutComposing || s.reconvert != nil || len(s.layoutKeys) != 0 || len(s.keypadKeys) != 0 {
				t.Fatalf("old input survived %s: surrogate=%x composing=%v reconvert=%v layouts=%d keypad=%d", mode, s.high, s.layoutComposing, s.reconvert, len(s.layoutKeys), len(s.keypadKeys))
			}
		})
	}
}

func TestCompositionSurrogateCannotSpanOrdinaryCharacter(t *testing.T) {
	h := &keypadRecorder{}
	s := &surface{w: &window{h: h}}
	s.message(0, wmChar, 0xd83d, 0)
	s.message(0, wmChar, 'x', 0)
	s.message(0, wmChar, 0xde00, 0)
	for _, ev := range h.events {
		if ev.Text == "😀" {
			t.Fatal("stale high surrogate combined across another character")
		}
	}
	h.events = nil
	s.message(0, wmChar, 0xd83d, 0)
	s.message(0, wmChar, 0xde00, 0)
	if len(h.events) != 1 || h.events[0].Text != "😀" {
		t.Fatalf("valid surrogate pair lost: %+v", h.events)
	}
}

type compositionClient struct {
	platform.TextInputClient
	changes []string
}

func (c *compositionClient) MarkedRange() (platform.TextRange, bool) {
	return platform.TextRange{Start: 2, End: 3}, true
}
func (c *compositionClient) UnmarkText() { c.changes = append(c.changes, "unmark") }
func (c *compositionClient) ReplaceText(_ *platform.TextRange, text string) {
	c.changes = append(c.changes, "replace:"+text)
}
func (c *compositionClient) SetMarkedText(_ *platform.TextRange, text string, _ platform.TextRange) {
	c.changes = append(c.changes, "mark:"+text)
}

func TestCompositionCancellationCannotDeliverIntoNewFocus(t *testing.T) {
	for _, mode := range []string{"terminal", "editor", "terminal-to-editor", "editor-to-terminal", "disable"} {
		t.Run(mode, func(t *testing.T) {
			h := &keypadRecorder{}
			oldClient, newClient := &compositionClient{}, &compositionClient{}
			old := platform.TextInputState{Active: true, FunctionalTarget: 1}
			next := platform.TextInputState{Active: true, FunctionalTarget: 2}
			if mode == "editor" || mode == "editor-to-terminal" {
				old.Client, old.FunctionalTarget = oldClient, 0
			}
			if mode == "editor" || mode == "terminal-to-editor" {
				next.Client, next.FunctionalTarget = newClient, 0
			}
			if mode == "disable" {
				next = old
				next.Active = false
			}
			s := &surface{w: &window{h: h}, input: old, ime: true}
			calls := 0
			s.setTextInput(next, func() {
				calls++
				// UI focus has already moved when the backend is notified. An IME
				// cancellation must not forward synchronous commits/end messages.
				s.composed(platform.SurfaceEvent{Kind: platform.TextInput, Text: "旧"})
				s.composed(platform.SurfaceEvent{Kind: platform.TextComposition, Text: "旧", Caret: 1})
				s.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "旧"})
				s.send(platform.SurfaceEvent{Kind: platform.TextComposition})
				for _, msg := range []uint32{wmChar, wmSysChar, wmDeadChar, wmSysDeadChar, wmImeStartComp, wmImeComposition, wmImeEndComp, wmImeChar, wmImeRequest} {
					if _, handled := s.message(0, msg, 'x', 0); !handled {
						t.Fatalf("cancellation message %x escaped", msg)
					}
				}
				s.cancelInput(func() { t.Error("recursive native cancellation") })
				s.composed(platform.SurfaceEvent{Kind: platform.TextInput, Text: "旧"})
			})
			if calls != 1 || len(h.events) != 0 || len(oldClient.changes) != 0 || len(newClient.changes) != 0 || s.cancelingInput || s.input != next {
				t.Fatalf("cancellation leaked: calls=%d events=%+v old=%v new=%v", calls, h.events, oldClient.changes, newClient.changes)
			}
			if mode == "disable" {
				next.Active = true
				s.setTextInput(next, func() { t.Fatal("inactive input cancelled") })
			}
			// Direct IME commits without STARTCOMPOSITION remain valid after the
			// transition; cancellation must not silence the next input session.
			s.composed(platform.SurfaceEvent{Kind: platform.TextInput, Text: "新😀"})
			if next.Client != nil {
				if len(newClient.changes) != 1 || newClient.changes[0] != "replace:新😀" {
					t.Fatalf("new editor commit: %v", newClient.changes)
				}
			} else if len(h.events) != 1 || h.events[0].Text != "新😀" || h.events[0].Replace {
				t.Fatalf("new terminal commit: %+v", h.events)
			}
		})
	}
}

func TestCompositionCaretUpdateDoesNotCancelIME(t *testing.T) {
	s := &surface{input: platform.TextInputState{Active: true, FunctionalTarget: 1}, ime: true}
	next := s.input
	next.Caret.X = 50
	s.setTextInput(next, func() { t.Fatal("caret update cancelled input") })
}

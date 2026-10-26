//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"testing"
	"unicode/utf16"

	"github.com/egoist/mygo/internal/platform"
)

func TestIMECancelMessageClearsPreviewWithoutContext(t *testing.T) {
	h := &keypadRecorder{}
	s := &surface{w: &window{h: h}, input: platform.TextInputState{Active: true, FunctionalTarget: 1}, layoutComposing: true}
	s.message(0, wmImeComposition, 0, 0)
	if len(h.events) != 1 || h.events[0].Kind != platform.TextComposition || h.events[0].Text != "" {
		t.Fatalf("cancel did not clear the preview: %+v", h.events)
	}
	if s.layoutComposing {
		t.Fatal("cancel retained composition ownership")
	}
}

type imeCaretClient struct {
	compositionClient
	caret platform.TextRange
}

func (c *imeCaretClient) SetMarkedText(_ *platform.TextRange, _ string, caret platform.TextRange) {
	c.caret = caret
}

func TestIMEClientCaretBounds(t *testing.T) {
	for _, tc := range []struct{ caret, want int }{{-2, 0}, {-1, 0}, {0, 0}, {1, 1}, {2, 3}, {3, 4}, {99, 4}} {
		t.Run(fmt.Sprint(tc.caret), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("caret %d panicked: %v", tc.caret, r)
				}
			}()
			c := &imeCaretClient{}
			s := &surface{input: platform.TextInputState{Active: true, Client: c}}
			s.composed(platform.SurfaceEvent{Kind: platform.TextComposition, Text: "a😀中", Caret: tc.caret})
			if c.caret != (platform.TextRange{Start: tc.want, End: tc.want}) {
				t.Fatalf("caret %d became %+v, want %d UTF-16 units", tc.caret, c.caret, tc.want)
			}
		})
	}
}

func TestIMEStringUsesActualSignedByteCount(t *testing.T) {
	for _, tc := range []struct {
		name         string
		size, copied int32
		units        []uint16
		want         string
		ok           bool
		calls        int
	}{
		{"no-data", -1, 0, nil, "", false, 1},
		{"general-error", -2, 0, nil, "", false, 1},
		{"empty", 0, 0, nil, "", true, 1},
		{"odd-size", 1, 0, nil, "", false, 1},
		{"oversize", maxIMEStringBytes + 2, 0, nil, "", false, 1},
		{"utf16", 10, 10, utf16.Encode([]rune("a😀中文")), "a😀中文", true, 2},
		{"shrunk", 10, 2, []uint16{'中'}, "中", true, 2},
		{"became-empty", 10, 0, nil, "", true, 2},
		{"read-no-data", 10, -1, nil, "", false, 2},
		{"read-error", 10, -2, nil, "", false, 2},
		{"invalid-count", 10, 12, nil, "", false, 2},
		{"odd-count", 10, 3, nil, "", false, 2},
		{"unpaired-surrogate", 2, 2, []uint16{0xd83d}, "�", true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			got, ok := readIMEString(func(buf []uint16) int32 {
				calls++
				if calls == 1 {
					if buf != nil {
						t.Fatal("size query supplied a buffer")
					}
					return tc.size
				}
				if len(buf)*2 != int(tc.size) {
					t.Fatalf("buffer has %d bytes, want %d", len(buf)*2, tc.size)
				}
				copy(buf, tc.units)
				return tc.copied
			})
			if got != tc.want || ok != tc.ok || calls != tc.calls {
				t.Fatalf("got %q ok=%v calls=%d; want %q ok=%v calls=%d", got, ok, calls, tc.want, tc.ok, tc.calls)
			}
		})
	}
}

func TestIMECompositionCursorOnlyAndUTF16Boundary(t *testing.T) {
	for _, tc := range []struct {
		pos  int32
		want int
	}{{-2, 3}, {-1, 3}, {0, 0}, {1, 1}, {2, 1}, {3, 2}, {4, 3}, {99, 3}, {1<<31 - 1, 3}} {
		for _, flags := range []uintptr{gcsCompStr, gcsCursorPos, gcsCompStr | gcsCursorPos} {
			h := &keypadRecorder{}
			s := &surface{w: &window{h: h}}
			s.imeComposition(flags, func(which uintptr) (string, bool) {
				if which != gcsCompStr {
					t.Fatalf("cursor update read %x", which)
				}
				return "a😀中", true
			}, func() int32 { return tc.pos })
			if len(h.events) != 1 || h.events[0].Kind != platform.TextComposition || h.events[0].Text != "a😀中" || h.events[0].Caret != tc.want || !s.layoutComposing {
				t.Fatalf("flags=%x pos=%d events=%+v composing=%v", flags, tc.pos, h.events, s.layoutComposing)
			}
		}
	}
}

func TestIMECompositionResultThenPreview(t *testing.T) {
	h := &keypadRecorder{}
	s := &surface{w: &window{h: h}, layoutComposing: true}
	s.imeComposition(gcsResultStr|gcsCompStr, func(which uintptr) (string, bool) {
		if which == gcsResultStr {
			return "中文", true
		}
		return "下一段", true
	}, func() int32 { return 2 })
	if len(h.events) != 2 || h.events[0].Kind != platform.TextInput || h.events[0].Text != "中文" || h.events[1].Kind != platform.TextComposition || h.events[1].Text != "下一段" || h.events[1].Caret != 2 {
		t.Fatalf("commit/preedit order: %+v", h.events)
	}
	h.events = nil
	s.imeComposition(gcsResultStr, func(uintptr) (string, bool) { return "", true }, nil)
	if len(h.events) != 1 || h.events[0].Kind != platform.TextInput || h.events[0].Text != "" || s.layoutComposing {
		t.Fatalf("valid empty result: %+v composing=%v", h.events, s.layoutComposing)
	}
	for _, flags := range []uintptr{gcsResultStr, gcsCompStr, gcsCursorPos} {
		h.events = nil
		s.layoutComposing = true
		s.imeComposition(flags, func(uintptr) (string, bool) { return "", false }, func() int32 { t.Fatal("queried caret after a failed string read"); return 0 })
		if len(h.events) != 0 || !s.layoutComposing {
			t.Fatalf("failed read was interpreted as empty text: %+v", h.events)
		}
	}
}

type imeFocusRecorder struct {
	keypadRecorder
	commit func()
}

func (r *imeFocusRecorder) SurfaceEvent(ev platform.SurfaceEvent) bool {
	r.events = append(r.events, ev)
	if ev.Kind == platform.TextInput {
		r.commit()
	}
	return true
}

func TestIMECompositionStopsAtReentrantFocusBoundary(t *testing.T) {
	for _, mode := range []string{"switch", "blur", "caret"} {
		t.Run(mode, func(t *testing.T) {
			h := &imeFocusRecorder{}
			s := &surface{w: &window{h: h}, ime: true, input: platform.TextInputState{Active: true, FunctionalTarget: 1}}
			h.commit = func() {
				next := s.input
				switch mode {
				case "switch":
					next.FunctionalTarget = 2
				case "blur":
					next.Active = false
				case "caret":
					next.Caret.X++
				}
				s.setTextInput(next, func() {})
			}
			reads := 0
			s.imeComposition(gcsResultStr|gcsCompStr, func(which uintptr) (string, bool) {
				reads++
				if which == gcsResultStr {
					return "提交", true
				}
				return "旧预编辑", true
			}, func() int32 { return 0 })
			want := 1
			if mode == "caret" {
				want = 2
			}
			if len(h.events) != want || reads != want {
				t.Fatalf("old preview crossed focus: events=%+v reads=%d", h.events, reads)
			}
		})
	}
}

func TestIMEUnavailableNativeContextDoesNotCommit(t *testing.T) {
	h := &keypadRecorder{}
	s := &surface{w: &window{h: h}, layoutComposing: true}
	s.message(0, wmImeComposition, 0, gcsResultStr|gcsCompStr)
	if len(h.events) != 0 || !s.layoutComposing {
		t.Fatalf("missing native context fabricated an empty commit: %+v", h.events)
	}
}

func TestIMEInsertedCharacterIsProvisional(t *testing.T) {
	const insert, noMove = uintptr(0x2000), uintptr(0x4000)
	for _, keepCaret := range []bool{false, true} {
		h := &keypadRecorder{}
		s := &surface{w: &window{h: h}, input: platform.TextInputState{Active: true, FunctionalTarget: 1}}
		flags := insert
		wantCaret := 1
		if keepCaret {
			flags |= noMove
			wantCaret = 0
		}
		// CS_INSERTCHAR carries its own UTF-16 character; it must work even
		// without an IMM context and must not send a premature text commit.
		s.message(0, wmImeComposition, '가', flags)
		if len(h.events) != 1 || h.events[0].Kind != platform.TextComposition || h.events[0].Text != "가" || h.events[0].Caret != wantCaret || !s.layoutComposing {
			t.Fatalf("keepCaret=%v provisional events=%+v", keepCaret, h.events)
		}
		s.imeComposition(gcsResultStr, func(uintptr) (string, bool) { return "각", true }, nil)
		s.message(0, wmImeEndComp, 0, 0)
		if len(h.events) != 3 || h.events[1].Kind != platform.TextInput || h.events[1].Text != "각" || h.events[2].Kind != platform.TextComposition || h.events[2].Text != "" {
			t.Fatalf("commit/end sequence: %+v", h.events)
		}
	}
}

func TestIMEInsertedCharacterUsesCompositionCaret(t *testing.T) {
	const insert, noMove = uintptr(0x2000), uintptr(0x4000)
	h := &keypadRecorder{}
	s := &surface{w: &window{h: h}}
	s.imeComposition(gcsCompStr, func(uintptr) (string, bool) { return "a😀z", true }, func() int32 { return 3 })
	s.message(0, wmImeComposition, '中', insert|noMove)
	s.message(0, wmImeComposition, 'x', insert)
	s.message(0, wmImeComposition, 'y', insert)
	want := []struct {
		text  string
		caret int
	}{{"a😀z", 2}, {"a😀中z", 2}, {"a😀x中z", 3}, {"a😀xy中z", 4}}
	if len(h.events) != len(want) {
		t.Fatalf("events: %+v", h.events)
	}
	for i, ev := range h.events {
		if ev.Kind != platform.TextComposition || ev.Text != want[i].text || ev.Caret != want[i].caret {
			t.Fatalf("event %d: %+v, want %+v", i, ev, want[i])
		}
	}
}

func TestIMEInsertedCharacterClearedAcrossLifecycle(t *testing.T) {
	const insert = uintptr(0x2000)
	for _, mode := range []string{"cancel", "end", "commit", "focus", "start"} {
		t.Run(mode, func(t *testing.T) {
			h := &keypadRecorder{}
			s := &surface{w: &window{h: h}}
			s.message(0, wmImeComposition, 'x', insert)
			switch mode {
			case "cancel":
				s.message(0, wmImeComposition, 0, 0)
			case "end":
				s.message(0, wmImeEndComp, 0, 0)
			case "commit":
				s.imeComposition(gcsResultStr, func(uintptr) (string, bool) { return "x", true }, nil)
			case "focus":
				s.cancelInput(func() { s.message(0, wmImeComposition, 'z', insert) })
			case "start":
				s.message(0, wmImeStartComp, 0, 0)
			}
			h.events = nil
			s.message(0, wmImeComposition, 'y', insert)
			if len(h.events) != 1 || h.events[0].Text != "y" || h.events[0].Kind != platform.TextComposition {
				t.Fatalf("old preview leaked: %+v", h.events)
			}
		})
	}
}

func TestIMEInsertedCharacterUsesClientMarkedRange(t *testing.T) {
	c := &compositionClient{}
	s := &surface{input: platform.TextInputState{Active: true, Client: c}}
	s.message(0, wmImeComposition, '가', csInsertChar|csNoMoveCaret)
	s.imeComposition(gcsResultStr, func(uintptr) (string, bool) { return "각", true }, nil)
	if len(c.changes) != 2 || c.changes[0] != "mark:가" || c.changes[1] != "replace:각" {
		t.Fatalf("provisional text was committed or duplicated: %v", c.changes)
	}
}

func TestIMEFullStringTakesPrecedenceOverCharacter(t *testing.T) {
	for _, flag := range []uintptr{gcsCompStr, gcsCursorPos, gcsResultStr, gcsResultStr | gcsCompStr} {
		h := &keypadRecorder{}
		s := &surface{w: &window{h: h}}
		// With no native context, a full-string read is unavailable. wParam
		// is only the latest change, not a fallback for the missing string.
		s.message(0, wmImeComposition, 'x', flag|csInsertChar|csNoMoveCaret)
		if len(h.events) != 0 {
			t.Fatalf("flags=%x fabricated text: %+v", flag, h.events)
		}
		s.imeComposition(flag|csInsertChar|csNoMoveCaret, func(uintptr) (string, bool) { return "完整文本", true }, func() int32 { return 2 })
		want := 1
		if flag == gcsResultStr|gcsCompStr {
			want = 2
		}
		if len(h.events) != want {
			t.Fatalf("flags=%x events=%+v", flag, h.events)
		}
		for _, ev := range h.events {
			if ev.Text != "完整文本" {
				t.Fatalf("full string replaced by change character: %+v", ev)
			}
		}
	}
}

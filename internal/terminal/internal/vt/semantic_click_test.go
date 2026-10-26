package vt

import (
	"strings"
	"testing"
)

func TestOShellSemanticClickData(t *testing.T) {
	const input = "\x1b]133;B\a"
	const ps2 = "\r\n\x1b]133;P;k=s\a> " + input
	for _, tc := range []struct {
		name, options, text, suffix string
		x, y                        int
		want                        string
	}{
		{"single-left", "cl=line", "abc", "", 3, 0, strings.Repeat("\x1b[D", 2)},
		{"prompt-is-offset-zero", "cl=m", "abc", "", 0, 0, strings.Repeat("\x1b[D", 3)},
		{"blank-is-offset-end", "cl=line", "abc", "\x1b[1;4H", 11, 0, strings.Repeat("\x1b[C", 2)},
		{"same-offset", "cl=line", "abc", "", 11, 0, ""},
		{"wide-tail", "cl=m", "界a", "", 3, 0, strings.Repeat("\x1b[D", 2)},
		{"after-wide", "cl=m", "界a", "", 4, 0, "\x1b[D"},
		{"combining", "cl=m", "e\u0301x", "", 2, 0, strings.Repeat("\x1b[D", 2)},
		{"soft-line", "cl=line", "0123456789abcd", "", 10, 0, strings.Repeat("\x1b[D", 6)},
		{"hard-line-rejected", "cl=line", "abc" + ps2 + "def", "", 3, 0, ""},
		{"hard-horizontal", "cl=m", "abc" + ps2 + "def", "", 3, 0, strings.Repeat("\x1b[D", 6)},
		{"hard-vertical", "cl=v", "abc" + ps2 + "def", "", 3, 0, strings.Repeat("\x1b[D", 3) + "\x1b[A\x1b[C"},
		{"hard-smart", "cl=w", "abc" + ps2 + "def", "", 3, 0, strings.Repeat("\x1b[D", 3) + "\x1b[A\x1b[C"},
		{"hard-down", "cl=v", "abc" + ps2 + "def", "\x1b[1;4H", 4, 1, "\x1b[D\x1b[B\x1b[C\x1b[C"},
		{"empty-hard-lines", "cl=v", "\r\n\r\n\r\nabc", "", 0, 0, strings.Repeat("\x1b[D", 3) + strings.Repeat("\x1b[A", 3)},
		{"application", "cl=w", "abc" + ps2 + "def", "\x1b[?1h", 3, 0, strings.Repeat("\x1bOD", 3) + "\x1bOA\x1bOC"},
		{"control-encoding-stays-escape", "cl=line", "abc", "\x1b G", 3, 0, strings.Repeat("\x1b[D", 2)},
		{"special-cross-line", "cl=v;special_key=1", "abc" + ps2 + "def", "\x1b[?1h", 3, 0, strings.Repeat("\x1b[1;1u", 6)},
		{"special-forward", "cl=m;special_key=1", "abc", "\x1b[1;3H", 4, 0, strings.Repeat("\x1b[1u", 2)},
		{"event-absolute", "cl=line;click_events=1", "abc" + ps2 + "def", "", 0, 1, "\x1b[<0;1;2M"},
		{"event-relative", "click_events=2", "abc" + ps2 + "def\r\nghi", "", 4, 2, "\x1b[<0;5;1M"},
		{"event-relative-before-ps2", "click_events=2", "abc\r\n\x1b[6G\x1b]133;P;k=s\a> " + input + "d", "", 3, 1, "\x1b[<0;4;1M"},
		{"event-relative-after-ps2", "click_events=2", "abc\r\n\x1b[6G\x1b]133;P;k=s\a> " + input + "d", "", 6, 1, "\x1b[<0;7;0M"},
		{"event-keeps-wide-tail-column", "click_events=1", "界a", "", 3, 0, "\x1b[<0;4;1M"},
		{"event-cursor-outside-group", "click_events=1", "abc", "\x1b[5;1H", 3, 0, "\x1b[<0;4;1M"},
		{"keys-cursor-outside-group", "cl=m", "abc", "\x1b[5;1H", 3, 0, ""},
		{"event-no-input", "click_events=1", "", "", 3, 0, ""},
		{"event-input-on-later-row", "click_events=1", "\r\nabc", "\x1b[H", 0, 0, "\x1b[<0;1;1M"},
		{"disabled", "", "abc", "", 3, 0, ""},
		{"submitted", "cl=m", "abc", "\x1b]133;C\a", 3, 0, ""},
		{"prompt-unarmed", "cl=m", "abc", "\x1b[H\x1b]133;A;cl=m\a", 3, 0, ""},
		{"alternate", "click_events=1", "abc", "\x1b[?47h", 3, 0, ""},
		{"outside-group", "click_events=1", "abc", "", 3, 2, ""},
		{"negative", "cl=m", "abc", "", -1, 0, ""},
		{"past-width", "cl=m", "abc", "", 12, 0, ""},
		{"past-history", "cl=m", "abc", "", 3, 500, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			term.Write([]byte("\x1b]133;A;" + tc.options + "\a$ " + input + tc.text + tc.suffix))
			if got := string(term.SemanticClickData(tc.x, tc.y)); got != tc.want {
				t.Fatalf("packet = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOShellSemanticClickPendingWrapAndHistory(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b]133;A;cl=m\a\x1b]133;B\aabcdefghijkl"))
	if got := string(term.SemanticClickData(0, 0)); got != strings.Repeat("\x1b[D", 12) {
		t.Fatalf("pending wrap packet = %q", got)
	}
	term.Reset()
	term.Write([]byte("\x1b]133;A;click_events=1\a\x1b]133;B\a" + strings.Repeat("a\r\n", 30)))
	for _, top := range []bool{true, false} {
		if top {
			term.ScrollToTop()
		} else {
			term.ScrollToBottom()
		}
		if got := string(term.SemanticClickData(2, 0)); got != "\x1b[<0;3;1M" {
			t.Fatalf("history report = %q", got)
		}
		if got := string(term.SemanticClickData(2, 30)); got != "\x1b[<0;3;5M" {
			t.Fatalf("active bottom report = %q", got)
		}
	}
}

func TestOShellSemanticClickExcludesStaleGroups(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b]133;A;click_events=2\a\x1b]133;B\aabcdefghijkl\x1b[2;1H\x1b]133;P;k=s\a"))
	term.Write([]byte("\x1b[H\x1b]133;N;click_events=2\a\x1b]133;B\aabcdefghijklm"))
	if got := string(term.SemanticClickData(3, 1)); got != "\x1b[<0;4;1M" {
		t.Fatalf("stale secondary changed relative origin: %q", got)
	}
	term.Reset()
	term.Write([]byte("\x1b]133;A;cl=m\a\x1b]133;B\aabc\r\ndef\x1b[H\x1b]133;N;cl=m\a\x1b]133;B\ax"))
	if got := term.SemanticClickData(0, 1); len(got) != 0 {
		t.Fatalf("stranded old-group hard line accepted: %q", got)
	}
	term.RegisterUserInput([]byte("\r"))
	if got := term.SemanticClickData(0, 0); len(got) != 0 {
		t.Fatalf("user submission did not suspend clicks: %q", got)
	}
}

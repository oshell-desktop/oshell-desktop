package core

import "testing"

func TestOutputActivitySplitControlStrings(t *testing.T) {
	var a OutputActivity
	for _, s := range []string{"\r\n \t", "\x1b]0;secret title", "\x1b", "\\", "\x1b[", "31m", "\x1b(B", "\x1bPignored\x1b\\"} {
		if a.Consume([]byte(s)) {
			t.Fatalf("control marked unread: %q", s)
		}
	}
	for _, s := range []string{"text", "中文", "\a"} {
		if !a.Consume([]byte(s)) {
			t.Fatalf("visible output ignored: %q", s)
		}
	}
}

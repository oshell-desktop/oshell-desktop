package vt

import (
	"strings"
	"testing"
)

func TestOShellExtendedOSC(t *testing.T) {
	for _, chunk := range []int{1, 17, 4096} {
		for _, end := range []string{"\a", "\x1b\\"} {
			var notification string
			term, _ := resetTerminal(t, Effects{Notify: func(title, body string) { notification = title + ":" + body }})
			longTitle := strings.Repeat("t", 65536) + "主机"
			writeControlChunks(term, []byte("\x1b]2;"+longTitle+end), chunk)
			if term.Title() != longTitle {
				t.Errorf("chunk=%d title bytes=%d want=%d", chunk, len(term.Title()), len(longTitle))
			}
			icon := strings.Repeat("i", 8192)
			writeControlChunks(term, []byte("\x1b]1;"+icon+end+"\x1b[22;0t"), chunk)
			term.Write([]byte("\x1b]2;short\a\x1b]1;short-icon\a\x1b[23;0t"))
			if term.Title() != longTitle || !strings.Contains(string(term.VT()), titleOSC(1, icon)) {
				t.Error("large title stack lost data")
			}
			pwd := "file://host/" + strings.Repeat("path/", 1800)
			writeControlChunks(term, []byte("\x1b]7;"+pwd+end), chunk)
			if term.Pwd() != pwd {
				t.Errorf("pwd bytes=%d want=%d", len(term.Pwd()), len(pwd))
			}
			link := "https://example.invalid/" + strings.Repeat("x", 8192)
			writeControlChunks(term, []byte("\x1b]8;id=long;"+link+end+"X\x1b]8;;"+end), chunk)
			ref, ok := term.CellAt(0, 0)
			if !ok || term.Hyperlink(ref) != link {
				t.Error("large hyperlink lost data")
			}
			body := strings.Repeat("message", 800)
			writeControlChunks(term, []byte("\x1b]777;notify;title;"+body+end), chunk)
			if notification != "title:"+body {
				t.Errorf("notification bytes=%d want=%d", len(notification), len(body)+6)
			}
		}
	}
}

func TestOShellExtendedOSCCancelAndOverflow(t *testing.T) {
	changes := 0
	term, _ := resetTerminal(t, Effects{TitleChanged: func() { changes++ }})
	term.Write([]byte("\x1b]2;keep\a"))
	for _, cancel := range []byte{0x18, 0x1a} {
		writeControlChunks(term, []byte("\x1b]2;"+strings.Repeat("x", 6000)), 17)
		term.Write([]byte{cancel})
		if term.Title() != "keep" || changes != 1 {
			t.Fatal("cancel committed partial title", changes)
		}
	}
	// The existing per-OSC 8 MiB budget remains in force. Overflow must not
	// publish a prefix, and the next complete sequence must still work.
	writeControlChunks(term, []byte("\x1b]2;"+strings.Repeat("x", 8<<20+1)+"\a"), 4096)
	if term.Title() != "keep" || changes != 1 {
		t.Fatal("oversized title changed state", changes)
	}
	term.Write([]byte("\x1b]2;recovered\aOK"))
	if term.Title() != "recovered" || term.FormattedText(false) != "OK" || changes != 2 {
		t.Fatal("OSC parser did not recover")
	}
}

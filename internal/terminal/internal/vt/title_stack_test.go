package vt

import (
	"fmt"
	"strings"
	"testing"
)

func titleOSC(kind int, value string) string { return fmt.Sprintf("\x1b]%d;%s\x1b\\", kind, value) }

func TestOShellTitleStackLifecycle(t *testing.T) {
	for _, chunk := range []int{1, 7, 4096} {
		for _, reset := range []string{"", "\x1b[!p", "\x1bc", "\x1b[?1049h\x1b[?1049l"} {
			changes := 0
			term, _ := resetTerminal(t, Effects{TitleChanged: func() { changes++ }})
			// Swift currently pops the window stack with 23;1 and icon with
			// 23;2, even though push selectors are the other way around.
			seq := titleOSC(2, "标题甲") + "\x1b[22;2t" + titleOSC(2, "标题乙") + reset + "\x1b[23;1t"
			writeControlChunks(term, []byte(seq), chunk)
			if term.Title() != "标题甲" || changes != 3 {
				t.Errorf("chunk=%d reset=%q title=%q changes=%d", chunk, reset, term.Title(), changes)
			}
			term.Write([]byte("\x1b[23;1t"))
			if changes != 3 {
				t.Fatal("empty stack emitted title callback", changes)
			}
		}
	}
}

func TestOShellTitleStackParameterShapes(t *testing.T) {
	for _, push := range []string{"\x1b[22;0t", "\x1b[22;t", "\x1b[22;0;0t", "\x1b[22;;t"} {
		term, _ := resetTerminal(t, Effects{})
		writeControlChunks(term, []byte(titleOSC(2, "saved")+push+titleOSC(2, "current")+"\x1b[23;0t"), 1)
		if term.Title() != "saved" {
			t.Errorf("push=%q title=%q", push, term.Title())
		}
	}
	for _, push := range []string{"\x1b[22t", "\x1b[22;2;t", "\x1b[22;1;0t", "\x1b[22;0;1t", "\x1b[22;0;0;0t"} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte(titleOSC(2, "saved") + push + titleOSC(2, "current") + "\x1b[23;0t"))
		if term.Title() != "current" {
			t.Errorf("invalid push=%q restored %q", push, term.Title())
		}
	}
}

func TestOShellTitleStackReplay(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte(titleOSC(2, "window-old") + titleOSC(1, "icon-old") + "\x1b[22;0t" + titleOSC(2, "window-now") + titleOSC(1, "icon-now") + "content"))
	restored, _ := resetTerminal(t, Effects{})
	restored.Write(term.VT())
	if restored.Title() != "window-now" || restored.FormattedText(false) != "content" || !strings.Contains(string(restored.VT()), titleOSC(1, "icon-now")) {
		t.Fatal("replay lost title/icon/content", restored.Title())
	}
	restored.Write([]byte("\x1b[23;1t\x1b[23;2t"))
	if restored.Title() != "window-old" || !strings.Contains(string(restored.VT()), titleOSC(1, "icon-old")) || strings.Contains(string(restored.VT()), "icon-now") {
		t.Fatal("replay lost independent title stacks", restored.Title())
	}
}

func TestOShellTitleStackDeepAndEmpty(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	for i := range 128 {
		value := fmt.Sprintf("title-%d", i)
		if i%7 == 0 {
			value = ""
		}
		term.Write([]byte(titleOSC(2, value) + "\x1b[22;2t"))
	}
	for i := 127; i >= 0; i-- {
		term.Write([]byte("\x1b[23;1t"))
		want := fmt.Sprintf("title-%d", i)
		if i%7 == 0 {
			want = ""
		}
		if term.Title() != want {
			t.Fatalf("pop=%d title=%q want=%q", i, term.Title(), want)
		}
	}
}

func TestOShellTitleUnicodeAndInvalidInput(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	for _, title := range []string{"主机 🚀", strings.Repeat("a", 1200) + " 主机"} {
		writeControlChunks(term, []byte(titleOSC(2, title)), 1)
		if term.Title() != title {
			t.Fatalf("title UTF-8/length changed: got=%d bytes want=%d", len(term.Title()), len(title))
		}
		restored, _ := resetTerminal(t, Effects{})
		restored.Write(term.VT())
		if restored.Title() != title {
			t.Fatal("title snapshot did not preserve Unicode", restored.Title())
		}
	}
	term.Write([]byte("\x1b]2;\xff\x1b\\"))
	if term.Title() != "" {
		t.Fatal("invalid UTF-8 did not use Swift's empty title fallback")
	}
}

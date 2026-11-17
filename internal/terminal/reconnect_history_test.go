package terminal

import (
	"oshell-desktop/internal/terminal/internal/vt"
	"strings"
	"testing"
	"time"
)

func TestReconnectPreservesNormalHistoryWithFreshModes(t *testing.T) {
	loadLib(t)
	old, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	old.Resize(40, 6)
	old.Feed([]byte(strings.Repeat("旧输出 中文 \x1b[31mred\x1b[0m\r\n", 20) + "last normal line\x1b[?2004h\x1b[?1h\x1b[?1049hALT-SCREEN"))
	history := old.HistoryForReconnect()
	restored, err := New(Options{Conn: newPipe(), ReconnectHistory: history})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if text := restored.Text(); !strings.Contains(text, "last normal line") || strings.Count(text, "旧输出") != 20 || strings.Contains(text, "ALT-SCREEN") {
		t.Fatal(text)
	}
	restored.mu.Lock()
	bad := restored.term.AltScreen() || restored.term.Mode(vt.ModeBracketedPaste) || restored.term.Mode(vt.ModeCursorKeys)
	restored.mu.Unlock()
	if bad {
		t.Fatal("previous parser modes crossed reconnect")
	}
	if !strings.Contains(string(restored.Snapshot()), "\x1b[38;5;1mred") {
		t.Fatal("color attributes lost")
	}
	restored.Feed([]byte("\x1b[2J\x1b[Hnew shell"))
	if text := restored.Text(); !strings.Contains(text, "last normal line") || !strings.Contains(text, "new shell") {
		t.Fatal("new shell erased archived viewport", text)
	}
}

func TestEndedSSHEmptySubmitReconnectOnly(t *testing.T) {
	called := make(chan struct{}, 2)
	console := newToolsConsole(func() { called <- struct{}{} })
	defer console.Close()
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := console.Read(buf); err != nil {
				return
			}
		}
	}()
	console.Write([]byte("\r"))
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("empty Enter did not reconnect")
	}
	console.Write([]byte("help\r"))
	select {
	case <-called:
		t.Fatal("nonempty input reconnected")
	case <-time.After(30 * time.Millisecond):
	}
}

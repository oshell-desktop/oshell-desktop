package terminal

import (
	"bufio"
	"io"
	"oshell-desktop/internal/core"
	"strings"
	"testing"
	"time"
)

func TestP3I08HelpAliasReturnsToPrompt(t *testing.T) {
	console := NewToolsConsole()
	defer console.Close()
	reader := bufio.NewReader(console)
	readPrompt := func(marker string) string {
		t.Helper()
		type result struct {
			text string
			err  error
		}
		done := make(chan result, 1)
		go func() {
			var out strings.Builder
			for {
				b, err := reader.ReadByte()
				if err != nil {
					done <- result{err: err}
					return
				}
				out.WriteByte(b)
				if strings.Contains(out.String(), marker) && strings.HasSuffix(out.String(), "oshell> ") {
					done <- result{text: out.String()}
					return
				}
			}
		}()
		select {
		case r := <-done:
			if r.err != nil {
				t.Fatal(r.err)
			}
			return r.text
		case <-time.After(3 * time.Second):
			t.Fatal("console did not return to prompt")
			return ""
		}
	}
	readPrompt("")
	command := func(line, marker string) string {
		t.Helper()
		go io.WriteString(console, line+"\r")
		return readPrompt(marker)
	}
	marker := strings.Join(core.LocalTools, "、")
	help, alias := command("help", marker), command("?", marker)
	// Ignore line-editor echo; compare the complete dispatched help and prompt.
	helpStart := strings.Index(help, marker)
	aliasStart := strings.Index(alias, marker)
	if helpStart < 0 || aliasStart < 0 || help[helpStart:] != alias[aliasStart:] {
		t.Fatalf("help alias mismatch: %q / %q", help, alias)
	}
	if got := command("not-an-allowed-tool", "不支持的工具"); !strings.Contains(got, "不支持的工具") {
		t.Fatal("unknown command accepted", got)
	}
}

func TestToolsConsoleEditingHelpAndInterrupt(t *testing.T) {
	loadLib(t)
	console := NewToolsConsole()
	terminal, e := New(Options{Conn: console, NoBlink: true, ManagedInputAllowed: console.ManagedInputAllowed})
	if e != nil {
		t.Fatal(e)
	}
	defer terminal.Close()
	wait := func(text string) {
		t.Helper()
		until := time.Now().Add(3 * time.Second)
		for time.Now().Before(until) {
			if strings.Contains(terminal.Text(), text) {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("missing %q in %q", text, terminal.Text())
	}
	wait("oshell>")
	terminal.Send([]byte("help\r"))
	wait("不解析管道")
	terminal.Send([]byte("cancel this\x03help\r"))
	wait("Ctrl+C 中断工具")
	if !terminal.ManagedInputAvailable() {
		t.Fatal("Ctrl+C closed blank console")
	}
	terminal.Send([]byte("exit\r"))
	select {
	case <-terminal.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("exit didn't close")
	}
}

func TestToolsConsoleReadlineControls(t *testing.T) {
	loadLib(t)
	console := NewToolsConsole()
	term, err := New(Options{Conn: console, NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	wait := func(text string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for !strings.Contains(term.Text(), text) && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if !strings.Contains(term.Text(), text) {
			t.Fatalf("missing %q: %q", text, term.Text())
		}
	}
	wait("oshell>")
	// Ctrl+A inserts the missing h at the beginning; Ctrl+E returns to the
	// end; Ctrl+W removes the extra word. The resulting command is "help ".
	term.Send([]byte("elp\x01h\x05 extra\x17\r"))
	wait("不解析管道")
	term.Send([]byte("exit\r"))
	select {
	case <-term.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("console no longer accepts commands after editing")
	}
}

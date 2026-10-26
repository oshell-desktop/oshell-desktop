package terminal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestOShellSearchRevealsHistoryAndUnicode(t *testing.T) {
	loadLib(t)
	term, e := New(Options{Conn: newPipe(), NoBlink: true})
	if e != nil {
		t.Fatal(e)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 640, 300)
	for i := 0; i < 100; i++ {
		term.Feed([]byte(fmt.Sprintf("line %d 中文结果\r\n", i)))
	}
	tt.Frame()
	matches, e := term.Search("中文结果", false, true)
	if e != nil || len(matches) != 100 {
		t.Fatal(len(matches), e)
	}
	if e = term.Reveal(matches[0]); e != nil {
		t.Fatal(e)
	}
	tt.Frame()
	term.mu.Lock()
	selected, ok := term.term.SelectionText()
	offset := term.term.Scrollbar().Offset
	term.mu.Unlock()
	if !ok || selected != "中文结果" || offset != 0 {
		t.Fatalf("selection %q offset %d", selected, offset)
	}
	term.Feed([]byte("changed"))
	if e = term.Reveal(matches[0]); e == nil {
		t.Fatal("accepted stale match")
	}
}
func TestOShellSemanticInputExcludesProtocolReplies(t *testing.T) {
	loadLib(t)
	left, right := newPipe(), newPipe()
	receiver, e := New(Options{Conn: right, NoBlink: true})
	if e != nil {
		t.Fatal(e)
	}
	defer receiver.Close()
	source, e := New(Options{Conn: left, NoBlink: true, OnUserInput: receiver.ReplayUserInput, OnUserPaste: receiver.Paste})
	if e != nil {
		t.Fatal(e)
	}
	defer source.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, source).Fill().AutoFocus() }, 480, 240)
	tt.Type("中文")
	if got := right.take(len("中文")); got != "中文" {
		t.Fatal(got)
	}
	left.take(len("中文"))
	source.Feed([]byte("\x1b[6n"))
	if got := left.take(1); !strings.Contains(got, "R") {
		t.Fatalf("device reply missing: %q", got)
	}
	source.Feed([]byte("\x1b[?1004h"))
	if got := left.take(3); got != "\x1b[I" {
		t.Fatalf("initial focus report missing: %q", got)
	}
	tt.SetFocused(false)
	if got := left.take(3); got != "\x1b[O" {
		t.Fatalf("blur report missing: %q", got)
	}
	right.mu.Lock()
	got := string(right.sent)
	right.mu.Unlock()
	if got != "" {
		t.Fatalf("protocol reply was broadcast: %q", got)
	}
	source.ConfirmedPaste("paste")
	if got := right.take(5); got != "paste" {
		t.Fatal(got)
	}
}
func TestOShellPastePolicyAndOutputLog(t *testing.T) {
	loadLib(t)
	pipe := newPipe()
	preview := ""
	term, e := New(Options{Conn: pipe, BeforePaste: func(s string) bool { preview = s; return false }, TrimCopyLeading: true, TrimCopyTrailing: true})
	if e != nil {
		t.Fatal(e)
	}
	defer term.Close()
	term.RequestPaste("a\nb")
	if preview != "a\nb" {
		t.Fatal("preview missing")
	}
	pipe.mu.Lock()
	n := len(pipe.sent)
	pipe.mu.Unlock()
	if n != 0 {
		t.Fatal("paste executed before confirmation")
	}
	term.ConfirmedPaste(preview)
	if got := pipe.take(3); got != "a\rb" && got != "a\nb" {
		t.Fatalf("unexpected paste %q", got)
	}
	if got := term.cleanCopy("  a\n  b \n"); got != "a\n  b" {
		t.Fatal(got)
	}
	file := filepath.Join(t.TempDir(), "output.log")
	if e = term.StartLog(file); e != nil {
		t.Fatal(e)
	}
	term.Feed([]byte("output"))
	term.StopLog()
 term.WaitLog()
	term.Feed([]byte("excluded"))
	data, e := os.ReadFile(file)
	if e != nil || string(data) != "output" {
		t.Fatal(string(data), e)
	}
}

func TestOShellHistoryLineLimitBoundsViewAndCopy(t *testing.T) {
	loadLib(t)
	term, e := New(Options{Conn: newPipe(), NoBlink: true})
	if e != nil {
		t.Fatal(e)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 640, 300)
	if e = term.SetHistoryLimit(500); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2000; i++ {
		term.Feed([]byte(fmt.Sprintf("row %04d\r\n", i)))
	}
	tt.Frame()
	_, rows := term.Size()
	if lines := strings.Count(term.Text(), "\n"); lines > 500+rows {
		t.Fatalf("history export exceeds limit: %d", lines)
	}
	term.mu.Lock()
	term.term.ScrollToTop()
	term.mu.Unlock()
	tt.Frame()
	term.mu.Lock()
	first := term.term.HistoryStart(500)
	offset := int(term.term.Scrollbar().Offset)
	term.mu.Unlock()
	if offset < first {
		t.Fatal("viewport exposed discarded rows")
	}
	term.v.selectAll()
	term.mu.Lock()
	copied, _ := term.term.SelectionText()
	term.mu.Unlock()
	if strings.Count(copied, "\n") > 500+rows {
		t.Fatal("copy exposed discarded history")
	}
	if e = term.SetHistoryLimit(499); e == nil {
		t.Fatal("accepted out-of-range history setting")
	}
}

func TestManagedComposerFramingAndNoKeyboardEcho(t *testing.T) {
	loadLib(t)
	conn := newPipe()
	echoed := false
	term, e := New(Options{Conn: conn, NoBlink: true, OnUserPaste: func(string) { echoed = true }})
	if e != nil {
		t.Fatal(e)
	}
	defer term.Close()
	term.Feed([]byte("\x1b[?2004h"))
	term.SendManagedText("a\r\n  b\x1b[201~", true)
	want := "a\r  b[201~\r"
	if got := conn.take(len(want)); got != want {
		t.Fatalf("%q != %q", got, want)
	}
	if echoed {
		t.Fatal("managed input was synchronized again")
	}
	// Ordinary clipboard paste must still honor the terminal's bracketed mode.
	term.Paste("clipboard\ntext")
	pasteWant := "\x1b[200~clipboard\ntext\x1b[201~"
	if got := conn.take(len(pasteWant)); got != pasteWant {
		t.Fatalf("clipboard framing changed: %q", got)
	}
	term.Feed([]byte("\x1b[?2004l"))
	term.SendManagedText("a\nb\n", true)
	if got := conn.take(4); got != "a\rb\r" {
		t.Fatalf("duplicate return: %q", got)
	}
}

func TestHighlightsChangeRenderingWithoutChangingBuffer(t *testing.T) {
	loadLib(t)
	term, e := New(Options{Conn: newPipe(), NoBlink: true})
	if e != nil {
		t.Fatal(e)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 700, 240)
	term.Feed([]byte("ERROR warning prod.example"))
	tt.Frame()
	before := term.Text()
	colors := []ui.Color{ui.Hex("#FF5F56"), ui.Hex("#FFAA33"), ui.Hex("#F5D547")}
	rules := []HighlightRule{{Pattern: `\b(error|fatal|failed|panic)\b`, Regex: true, Foreground: colors[0]}, {Pattern: `\b(warn|warning)\b`, Regex: true, Foreground: colors[1]}, {Pattern: "prod.example", Foreground: colors[2]}}
	if e = term.SetHighlights(rules); e != nil {
		t.Fatal(e)
	}
	tt.Frame()
	if term.Text() != before {
		t.Fatal("highlighter modified terminal history")
	}
	for _, color := range colors {
		found := false
		for _, row := range term.v.lines {
			for _, run := range row.runs {
				found = found || run.c == color
			}
		}
		if !found {
			t.Fatalf("highlight not painted: %+v", color)
		}
	}
	if e = term.SetHighlights(nil); e != nil {
		t.Fatal(e)
	}
	tt.Frame()
	if term.Text() != before {
		t.Fatal("disabling changed history")
	}
}

package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func wordClickFixture(t *testing.T, text string, width int) (*Terminal, *ui.Tester) {
	t.Helper()
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), CustomWordSelection: true, WordSeparators: "/.", ShiftWhitespace: true, CopyOnSelect: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, width, 200)
	term.Feed([]byte(text))
	tt.Frame()
	return term, tt
}

func clickWord(term *Terminal, tt *ui.Tester, col, row int) {
	x, y := cellCenter(term, col, row)
	tt.Press(x, y)
	tt.Release(x, y)
}

func TestProgressiveDoubleClickAndTripleLine(t *testing.T) {
	term, tt := wordClickFixture(t, "/home/file.txt /etc/name.conf", 500)
	for i, want := range []string{"", "file", "/home/file.txt /etc/name.conf", "/home/file.txt"} {
		clickWord(term, tt, 8, 0)
		if i > 0 && tt.Clipboard() != want {
			t.Fatalf("click %d selected %q, want %q", i+1, tt.Clipboard(), want)
		}
	}
	// The fifth press uses the row branch; its drag also clears the word seed.
	x, y := cellCenter(term, 8, 0)
	tt.Press(x, y)
	x, y = cellCenter(term, 21, 0)
	tt.Move(x, y)
	tt.Release(x, y)
	if got := tt.Clipboard(); got != "/home/file.txt /etc/name.conf" {
		t.Fatalf("fifth-click row drag selected %q", got)
	}
	for range 2 {
		clickWord(term, tt, 8, 0)
	}
	if tt.Clipboard() != "file" {
		t.Fatal("drag did not reset progressive selection", tt.Clipboard())
	}
}

// Explicit event times exercise the direct-caller fallback. Window-wide counts
// are covered separately through Tester, without mutating the terminal's clock.
func TestProgressiveDoubleClickAfterPauseAndReset(t *testing.T) {
	term, tt := wordClickFixture(t, "/home/file.txt /etc/name.conf", 500)
	clickWord(term, tt, 8, 0) // allocate the gesture
	term.v.wordClicks = wordClickState{}
	at := time.Second
	got := ""
	click := func(col int) { at += 100 * time.Millisecond; got = timedWordClick(t, term, col, 0, 0, at) }
	for range 2 {
		click(8)
	}
	if got != "file" {
		t.Fatal(got)
	}
	at += 600 * time.Millisecond
	for range 2 {
		click(8)
	}
	if got != "/home/file.txt" {
		t.Fatal("second double click after pause did not expand", got)
	}
	for range 2 {
		click(21)
	}
	if got != "name" {
		t.Fatal("another position inherited whitespace selection", got)
	}
	term.Feed([]byte(" changed"))
	for range 2 {
		click(21)
	}
	if got != "/etc/name.conf" {
		t.Fatal("unrelated output incorrectly reset word seed", got)
	}
	tt.Key(0, ui.KeyEscape)
	for range 2 {
		click(21)
	}
	if got != "name" {
		t.Fatal("keyboard input did not reset fallback selection progression", got)
	}
}

func TestWordClickHostCountSurvivesKeyboardSeedReset(t *testing.T) {
	term, tt := wordClickFixture(t, "/home/file.txt end", 500)
	for range 4 {
		clickWord(term, tt, 8, 0)
	}
	tt.Key(0, ui.KeyEscape)
	if term.v.wordClicks.seed != nil {
		t.Fatal("key retained word seed")
	}
	for range 2 {
		clickWord(term, tt, 8, 0)
	}
	if got := tt.Clipboard(); got != "/home/file.txt end" {
		t.Fatal("terminal replaced host fifth/sixth count after key", got)
	}
}

func TestProgressiveDoubleClickWideWrappedText(t *testing.T) {
	term, tt := wordClickFixture(t, "目录/file.txt end", 80)
	for range 2 {
		clickWord(term, tt, 0, 0)
	}
	if got := tt.Clipboard(); got != "目录" {
		t.Fatalf("first double click selected %q", got)
	}
	for range 2 {
		clickWord(term, tt, 0, 0)
	}
	if got := tt.Clipboard(); got != "目录/file.txt" {
		t.Fatalf("wrapped whitespace selection = %q", got)
	}
	save(t, tt, "progressive-word-selection")
}

func TestProgressiveShiftDoubleClickStillSelectsWhitespace(t *testing.T) {
	term, _ := wordClickFixture(t, "/home/file.txt end", 500)
	x, y := cellCenter(term, 8, 0)
	for range 2 {
		term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerDown, Mods: ui.Shift, X: x, Y: y})
		term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerUp, Mods: ui.Shift, X: x, Y: y})
	}
	term.mu.Lock()
	text, _ := term.term.SelectionText()
	term.mu.Unlock()
	if text != "/home/file.txt" {
		t.Fatal("Shift double click regressed", text)
	}
}

func TestTripleClickPhysicalWrappedRow(t *testing.T) {
	term, tt := wordClickFixture(t, "", 240)
	cols, _ := term.Size()
	term.Feed([]byte(strings.Repeat("A", cols) + strings.Repeat("B", cols)))
	tt.Frame()
	for range 3 {
		clickWord(term, tt, 2, 1)
	}
	if got := tt.Clipboard(); got != strings.Repeat("B", cols-1) {
		t.Fatalf("physical row copied %q, want %d B cells", got, cols-1)
	}
}

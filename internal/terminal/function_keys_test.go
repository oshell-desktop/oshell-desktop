package terminal

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestExtendedFunctionKeys(t *testing.T) {
	loadLib(t)
	conn := newPipe()
	term, err := New(Options{Conn: conn})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 400, 200)
	if err := tt.Click("Terminal"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		number int
		want   string
	}{{13, "\x1b[1;2P"}, {14, "\x1b[1;2Q"}, {15, "\x1b[1;2R"}, {16, "\x1b[1;2S"}, {17, "\x1b[15;2~"}, {18, "\x1b[17;2~"}, {19, "\x1b[18;2~"}, {20, "\x1b[19;2~"}} {
		if !term.SendFunctionKey(tc.number, 0, false, false) {
			t.Fatalf("F%d not handled", tc.number)
		}
		if got := conn.take(len(tc.want)); got != tc.want {
			t.Fatalf("F%d: %q != %q", tc.number, got, tc.want)
		}
	}
	if !term.SendFunctionKey(13, ui.Ctrl, false, false) {
		t.Fatal("modified function key lost")
	}
	if got := conn.take(6); got != "\x1b[1;2P" {
		t.Fatal("Ctrl-F13", got)
	}
	if !term.SendFunctionKey(13, 0, false, true) {
		t.Fatal("repeat lost")
	}
	if got := conn.take(6); got != "\x1b[1;2P" {
		t.Fatal("repeat", got)
	}
	if !term.SendFunctionKey(13, 0, true, false) {
		t.Fatal("release unhandled")
	}
	term.v.focused = false
	if term.SendFunctionKey(13, 0, false, false) {
		t.Fatal("unfocused terminal consumed key")
	}
}

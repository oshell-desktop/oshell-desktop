package terminal

import (
	"reflect"
	"testing"
)

func TestTitleStackRestorationNotifiesHost(t *testing.T) {
	loadLib(t)
	var titles []string
	term, err := New(Options{Conn: newPipe(), OnTitle: func(title string) { titles = append(titles, title) }})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.Feed([]byte("\x1b]2;original\a\x1b[22;2t"))
	term.Feed([]byte("\x1b]2;temporary\a"))
	term.Feed([]byte("\x1b[23;1t"))
	term.Feed([]byte("\x1b[23;1t"))
	if !reflect.DeepEqual(titles, []string{"original", "temporary", "original"}) || term.Title() != "original" {
		t.Fatal("restored title did not reach the host callback", titles, term.Title())
	}
}

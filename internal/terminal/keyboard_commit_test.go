package terminal

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestKeyboardCommittedTextUsesNegotiatedProtocol(t *testing.T) {
	for _, tc := range []struct {
		flags      int
		text, want string
	}{
		{0, "你", "你"}, {1, "你好", "你好"}, {8, "你", "\x1b[0u"},
		{24, "你好", "\x1b[0;;20320:22909u"}, {24, "😀", "\x1b[0;;128512u"},
		{24, "a\u0301", "\x1b[0;;97:769u"},
	} {
		t.Run(fmt.Sprintf("%d/%s", tc.flags, tc.text), func(t *testing.T) {
			term, tt, conn := bidiViewFixture(t)
			term.Feed([]byte(fmt.Sprintf("\x1b[=%du", tc.flags)))
			tt.Compose(tc.text, 1)
			tt.Type(tc.text)
			if got := conn.take(len(tc.want)); got != tc.want {
				t.Fatalf("commit %q, want %q", got, tc.want)
			}
			// Paste remains on the paste path, not the keyboard event path.
			term.Feed([]byte("\x1b[?2004h"))
			term.Paste("中文")
			want := "\x1b[200~中文\x1b[201~"
			if got := conn.take(len(want)); got != want {
				t.Fatalf("paste %q, want %q", got, want)
			}
		})
	}
}

func TestKeyboardHardwareIdentityAndEmptyCommit(t *testing.T) {
	term, tt, conn := bidiViewFixture(t)
	term.Feed([]byte("\x1b[=26u"))
	tt.Type("")
	tt.TypeKey(0, ui.KeyA, "a")
	want := "\x1b[97;;97u\x1b[97;1:3u"
	if got := conn.take(len(want)); got != want {
		t.Fatalf("hardware event %q, want %q", got, want)
	}
	term.Feed([]byte("\x1b[=2u"))
	tt.Key(ui.Ctrl, ui.KeyA)
	want = "\x01\x1b[97;5:3u"
	if got := conn.take(len(want)); got != want {
		t.Fatalf("partial flags control key %q, want %q", got, want)
	}
}

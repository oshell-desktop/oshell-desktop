package terminal

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Windows translates AltGr to Ctrl+Alt plus WM_CHAR. Like OShell's
// Option-composition path, the translated printable text must survive even
// when an application requests all keys without associated text.
func TestWindowsAltGrPrintableText(t *testing.T) {
	term, tt, conn := bidiViewFixture(t)
	for flags := 0; flags < 32; flags++ {
		term.Feed([]byte(fmt.Sprintf("\x1b[=%du", flags)))
		var want strings.Builder
		for _, tc := range []struct {
			key  ui.Key
			mods ui.Modifiers
			text string
		}{
			{ui.KeyQ, ui.Ctrl | ui.Alt, "@"},
			{ui.KeyE, ui.Ctrl | ui.Alt, "€"},
			{ui.Key7, ui.Ctrl | ui.Alt, "{"},
			{ui.KeyA, ui.Ctrl | ui.Alt | ui.Shift, "Ā"},
			{ui.KeyX, ui.Ctrl | ui.Alt, "a\u0301"},
		} {
			tt.TypeKey(tc.mods, tc.key, tc.text)
			want.WriteString(tc.text)
		}
		// A barrier on the same input queue makes stray release events visible.
		term.Send([]byte("END"))
		want.WriteString("END")
		if got := conn.take(want.Len()); got != want.String() {
			t.Fatalf("flags %d: got %q, want %q", flags, got, want.String())
		}
	}
}

func TestWindowsAltGrReleaseAndShortcutFallback(t *testing.T) {
	term, tt, conn := bidiViewFixture(t)
	term.Feed([]byte("\x1b[=26u"))
	v := term.v
	key := ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyQ, Mods: ui.Ctrl | ui.Alt}
	v.input(key)
	v.typed("@")
	key.Repeat = true
	v.input(key)
	v.typed("@")
	// Modifier release or protocol changes before key-up must not synthesize
	// a release for a key whose press was delivered as composed text.
	term.Feed([]byte("\x1b[=2u"))
	v.input(ui.InputEvent{Kind: ui.InputKeyUp, Key: ui.KeyQ})
	tt.Key(ui.Ctrl|ui.Alt, ui.KeyQ) // no WM_CHAR: real Ctrl+Alt shortcut
	term.Send([]byte("END"))
	want := "@@\x1b\x11\x1b[113;7:3uEND"
	if got := conn.take(len(want)); got != want {
		t.Fatalf("repeat/release/fallback %q, want %q", got, want)
	}

	v.input(key)
	v.typed("@")
	tt.SetFocused(false)
	if len(v.composedKeys) != 0 {
		t.Fatal("focus loss retained composed key releases")
	}
	tt.SetFocused(true)
	tt.Key(ui.Ctrl|ui.Alt, ui.KeyQ)
	term.Send([]byte("END"))
	want = "@\x1b\x11\x1b[113;7:3uEND"
	if got := conn.take(len(want)); got != want {
		t.Fatalf("focus reset %q, want %q", got, want)
	}
}

func TestWindowsAltGrDoesNotBypassControlFiltering(t *testing.T) {
	term, tt, conn := bidiViewFixture(t)
	term.Feed([]byte("\x1b[=26u"))
	for _, control := range []string{"\x1b", "\x7f", "\u0085", "\u009f"} {
		tt.TypeKey(ui.Ctrl|ui.Alt, ui.KeyQ, "@"+control)
		term.Send([]byte("END"))
		want := "\x1b[0;;64u\x1b[113;7:3uEND"
		if got := conn.take(len(want)); got != want {
			t.Fatalf("control %q: got %q, want %q", control, got, want)
		}
	}
}

package terminal

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAltWordArrowsFollowTerminalMode(t *testing.T) {
	term, tt, conn := bidiViewFixture(t)
	term.opts.OptionAsAlt = true
	for _, tc := range []struct{ name, setup, want string }{
		{"shell", "", "\x1bb\x1bf"},
		{"alternate screen", "\x1b[?1049h", "\x1b[1;3D\x1b[1;3C"},
		{"application cursor", "\x1b[?1049l\x1b[?1h", "\x1b[1;3D\x1b[1;3C"},
		{"kitty events", "\x1b[?1l\x1b[>3u", "\x1b[1;3D\x1b[1;3:3D\x1b[1;3C\x1b[1;3:3C"},
		{"back at shell", "\x1b[<u", "\x1bb\x1bf"},
	} {
		term.Feed([]byte(tc.setup))
		tt.Frame()
		tt.Key(ui.Alt, ui.KeyLeft)
		tt.Key(ui.Alt, ui.KeyRight)
		term.Send([]byte("|"))
		if got := conn.take(len(tc.want) + 1); got != tc.want+"|" {
			t.Fatalf("%s: got %q, want %q", tc.name, got, tc.want+"|")
		}
	}
}

func TestAltWordArrowsEditBashCommand(t *testing.T) {
	loadLib(t)
	bash := os.Getenv("OSHELL_TEST_BASH")
	if bash == "" {
		if runtime.GOOS == "windows" {
			t.Skip("set OSHELL_TEST_BASH for real Bash PTY acceptance")
		}
		bash = "/bin/bash"
	}
	term, err := New(Options{Command: []string{bash, "--noprofile", "--norc", "-i"}, Dir: t.TempDir(),
		Env: []string{"PS1=word-edit> ", "INPUTRC=/dev/null", "HISTFILE=/dev/null"}, OptionAsAlt: true, NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 720, 260)
	waitFor(t, tt, "Bash prompt", func() bool { return strings.Contains(term.Text(), "word-edit>") })
	tt.Type("printf '<%s>\\n' one two")
	tt.Key(ui.Alt, ui.KeyLeft)
	tt.Type("L")
	tt.Key(ui.Alt, ui.KeyRight)
	tt.Type("R")
	tt.Key(0, ui.KeyEnter)
	waitFor(t, tt, "edited arguments", func() bool { return strings.Contains(term.Text(), "<one>\n<LtwoR>") })
}

package terminal

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestPrintableReleaseUsesDesktopFlagRoute(t *testing.T) {
	term, _, conn := bidiViewFixture(t)
	for flags := 0; flags < 32; flags++ {
		for _, mods := range []ui.Modifiers{0, ui.Shift, ui.Ctrl, ui.Alt, ui.Ctrl | ui.Alt} {
			term.Feed([]byte(fmt.Sprintf("\x1b[=%du", flags)))
			if !term.v.input(ui.InputEvent{Kind: ui.InputKeyUp, Key: ui.KeyA, Mods: mods}) {
				t.Fatal("release must remain available for synchronization")
			}
			want := ""
			if flags&2 != 0 && (flags&8 != 0 || mods&(ui.Ctrl|ui.Alt) != 0) {
				m := 1
				if mods&ui.Shift != 0 {
					m++
				}
				if mods&ui.Alt != 0 {
					m += 2
				}
				if mods&ui.Ctrl != 0 {
					m += 4
				}
				want = fmt.Sprintf("\x1b[97;%d:3u", m)
			}
			term.Send([]byte("END"))
			want += "END"
			if got := conn.take(len(want)); got != want {
				t.Fatalf("flags=%d mods=%d: %q want %q", flags, mods, got, want)
			}
		}
	}
}

func TestPrintableReleaseBroadcastUsesTargetFlags(t *testing.T) {
	for _, leftAll := range []bool{false, true} {
		left, input, lconn := bidiViewFixture(t)
		right, _, rconn := bidiViewFixture(t)
		lflags, rflags := 2, 26
		if leftAll {
			lflags, rflags = rflags, lflags
		}
		left.Feed([]byte(fmt.Sprintf("\x1b[=%du", lflags)))
		right.Feed([]byte(fmt.Sprintf("\x1b[=%du", rflags)))
		left.opts.OnUserInput = right.ReplayUserInput
		input.TypeKey(0, ui.KeyA, "a")
		for i, term := range []*Terminal{left, right} {
			flags, conn := lflags, lconn
			if i == 1 {
				flags, conn = rflags, rconn
			}
			want := "aEND"
			if flags == 26 {
				want = "\x1b[97;;97u\x1b[97;1:3uEND"
			}
			term.Send([]byte("END"))
			if got := conn.take(len(want)); got != want {
				t.Fatalf("side=%d flags=%d: %q want %q", i, flags, got, want)
			}
		}
	}
}

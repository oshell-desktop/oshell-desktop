package vt

import (
	"bytes"
	"fmt"
	"testing"
)

func TestOShellModeReports(t *testing.T) {
	for _, chunk := range []int{1, 7, 4096} {
		var reply bytes.Buffer
		term, _ := resetTerminal(t, Effects{WritePTY: func(p []byte) { reply.Write(p) }})
		for _, c := range []struct{ query, want string }{
			{"$p", "0;0$y"}, {"?$p", "?0;0$y"},
			{"?7;25$p", "?7;1$y"}, {"?7;$p", "?7;1$y"},
			{"4;20$p", "4;2$y"}, {"?65535$p", "?65535;0$y"},
		} {
			reply.Reset()
			writeControlChunks(term, []byte("\x1b["+c.query), chunk)
			if reply.String() != "\x1b["+c.want {
				t.Errorf("chunk=%d query=%s got=%q want=%q", chunk, c.query, reply.String(), c.want)
			}
		}
		for _, c := range []struct {
			prefix string
			state  int
			modes  []int
		}{
			{"?", 1, []int{2, 8, 18, 19, 95}},
			{"?", 2, []int{13, 14, 30, 41, 1001}},
			{"?", 4, []int{10, 42, 44, 46, 67, 80, 1034}},
			{"", 4, []int{1, 5, 7, 10, 11, 13, 14, 15, 16, 17, 18, 19}},
			{"", 2, []int{2, 3}}, {"", 1, []int{12}},
			{"?", 0, []int{1047, 1048, 1049, 117, 2027, 65535}},
		} {
			for _, mode := range c.modes {
				for _, action := range []string{"h", "l"} {
					reply.Reset()
					seq := fmt.Sprintf("\x1b[%s%d%s\x1b[%s%d$p", c.prefix, mode, action, c.prefix, mode)
					writeControlChunks(term, []byte(seq), chunk)
					want := fmt.Sprintf("\x1b[%s%d;%d$y", c.prefix, mode, c.state)
					if reply.String() != want {
						t.Errorf("mode=%s%d%s got=%q want=%q", c.prefix, mode, action, reply.String(), want)
					}
				}
			}
		}
		for _, prefix := range []string{"", "?"} {
			modes := []int{4, 20}
			if prefix == "?" {
				modes = []int{1, 4, 5, 6, 7, 12, 25, 40, 45, 66, 69, 1004, 1007, 2004, 2026}
			}
			for _, mode := range modes {
				for i, action := range []string{"h", "l"} {
					term.Write([]byte("\x1b[?7h")) // reverse-wrap requires wraparound
					reply.Reset()
					writeControlChunks(term, []byte(fmt.Sprintf("\x1b[%s%d%s\x1b[%s%d$p", prefix, mode, action, prefix, mode)), chunk)
					want := fmt.Sprintf("\x1b[%s%d;%d$y", prefix, mode, i+1)
					if prefix == "?" && mode == 1004 && action == "h" {
						want = "\x1b[I" + want
					}
					if reply.String() != want {
						t.Errorf("dynamic mode=%s%d%s got=%q want=%q", prefix, mode, action, reply.String(), want)
					}
				}
			}
		}
		term.Write([]byte("\x1b[?1049h"))
		reply.Reset()
		term.Write([]byte("\x1b[?47$p"))
		if reply.String() != "\x1b[?47;1$y" {
			t.Error("alternate buffer query does not use active buffer", reply.String())
		}
		term.Write([]byte("\x1b[?1047l\x1b G"))
		reply.Reset()
		term.Write([]byte("\x1b[?47$p\x1b[2$p"))
		if reply.String() != "\x9b?47;2$y\x9b2;2$y" {
			t.Errorf("active buffer / 8-bit query=%q", reply.String())
		}
		// DECCOLM reports actual width, not the last value of DECSET 3.
		term.Resize(132, 5, 8, 16)
		reply.Reset()
		term.Write([]byte("\x1b[?3$p"))
		if reply.String() != "\x9b?3;1$y" {
			t.Errorf("132-column report=%q", reply.String())
		}
	}
}

func TestOShellMouseModeReportsAndReplay(t *testing.T) {
	var reply bytes.Buffer
	term, _ := resetTerminal(t, Effects{WritePTY: func(p []byte) { reply.Write(p) }})
	m, err := NewMouseEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Free()
	for _, tracking := range []int{9, 1000, 1002, 1003} {
		term.Write([]byte(fmt.Sprintf("\x1b[?%dh", tracking)))
		for _, q := range []int{9, 1000, 1002, 1003} {
			reply.Reset()
			term.Write([]byte(fmt.Sprintf("\x1b[?%d$p", q)))
			state := 2
			if q == tracking {
				state = 1
			}
			if want := fmt.Sprintf("\x1b[?%d;%d$y", q, state); reply.String() != want {
				t.Errorf("tracking=%d query=%d got=%q", tracking, q, reply.String())
			}
		}
	}
	for _, format := range []int{1005, 1006, 1015, 1016} {
		term.Write([]byte(fmt.Sprintf("\x1b[?%dh", format)))
		for _, q := range []int{1005, 1006, 1015, 1016} {
			reply.Reset()
			term.Write([]byte(fmt.Sprintf("\x1b[?%d$p", q)))
			state := 2
			if q == format {
				state = 1
			}
			if want := fmt.Sprintf("\x1b[?%d;%d$y", q, state); reply.String() != want {
				t.Errorf("format=%d query=%d got=%q", format, q, reply.String())
			}
		}
	}
	// The final mode is selected last, not by numeric enum order.
	term.Write([]byte("\x1b[?9h\x1b[?1006h"))
	replayed, _ := resetTerminal(t, Effects{})
	replayed.Write(term.VT())
	for _, candidate := range []*Terminal{term, replayed} {
		m.Sync(candidate, 96, 80, 8, 16, 0, 0, false)
		if got := string(m.Encode(MousePress, ButtonLeft, 0, 20, 24)); got != "\x1b[<0;3;2M" {
			t.Errorf("mouse format disagrees with selected mode: %q", got)
		}
		if got := m.Encode(MouseRelease, ButtonLeft, 0, 20, 24); len(got) != 0 {
			t.Errorf("X10 tracking emitted a release: %q", got)
		}
		// Resetting any encoding selects X10 but retains tracking.
		candidate.Write([]byte("\x1b[?1015l"))
		m.Sync(candidate, 96, 80, 8, 16, 0, 0, false)
		if got := string(m.Encode(MousePress, ButtonLeft, 0, 20, 24)); got != "\x1b[M #\"" {
			t.Errorf("encoding reset disabled tracking or used stale format: %q", got)
		}
		candidate.Write([]byte("\x1b[?1000l"))
		m.Sync(candidate, 96, 80, 8, 16, 0, 0, false)
		if got := m.Encode(MousePress, ButtonLeft, 0, 20, 24); len(got) != 0 {
			t.Errorf("reset of a tracking selector did not stop tracking: %q", got)
		}
	}
}

func TestOShellModeChangesPreserveInputAndCursor(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b[3;5H\x1b[?6h"))
	if x, y := term.Cursor(); x != 4 || y != 2 {
		t.Error("origin enable moved cursor", x, y)
	}
	term.Write([]byte("\x1b[?6l"))
	if x, y := term.Cursor(); x != 4 || y != 2 {
		t.Error("origin reset moved cursor", x, y)
	}
	term.Write([]byte("\x1b[?45l\x1b[?7l\x1b[?45h"))
	if term.Mode(45) {
		t.Error("reverse wrap enabled while wraparound disabled")
	}
	term.Write([]byte("\x1b[2h\x1b[12l\x1b[?67h"))
	if term.Mode(Mode(2|1<<15)) || !term.Mode(Mode(12|1<<15)) || term.Mode(67) {
		t.Error("unsupported keyboard/local echo/backarrow modes changed state")
	}
	k, err := NewKeyEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer k.Free()
	k.Sync(term, false)
	if got := string(k.Encode(KeyEvent{Action: KeyPress, Key: KeyBackspace})); got != "\x7f" {
		t.Errorf("unsupported backarrow request changed Backspace: %q", got)
	}
}

func TestOShellModeReportResetBoundaries(t *testing.T) {
	for _, reset := range []string{"\x1b[!p", "\x1bc"} {
		var reply bytes.Buffer
		term, _ := resetTerminal(t, Effects{WritePTY: func(p []byte) { reply.Write(p) }})
		term.Write([]byte("\x1b[?4;45;1003;1004;1006h" + reset))
		reply.Reset() // enabling 1004 immediately reported the current focus
		term.Write([]byte("\x1b[?4$p\x1b[?45$p\x1b[?1003$p\x1b[?1004$p\x1b[?1006$p"))
		want := "\x1b[?4;1$y\x1b[?45;2$y\x1b[?1003;1$y\x1b[?1004;1$y\x1b[?1006;1$y"
		if reset == "\x1bc" {
			want = "\x1b[?4;1$y\x1b[?45;1$y\x1b[?1003;2$y\x1b[?1004;1$y\x1b[?1006;1$y"
		}
		if reply.String() != want {
			t.Errorf("reset=%q got=%q want=%q", reset, reply.String(), want)
		}
		term.Write([]byte("\x1b[?9h"))
		m, err := NewMouseEncoder()
		if err != nil {
			t.Fatal(err)
		}
		m.Sync(term, 96, 80, 8, 16, 0, 0, false)
		if got := string(m.Encode(MousePress, ButtonLeft, 0, 20, 24)); got != "\x1b[<0;3;2M" {
			t.Errorf("reset=%q discarded retained SGR encoding: %q", reset, got)
		}
		m.Free()
	}
}

func TestOShellModeReplayReverseWrapWithoutWraparound(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b[?7h\x1b[?45h\x1b[?7l"))
	if !term.Mode(45) || term.Mode(7) {
		t.Fatal("setup lost independently retained reverse-wrap")
	}
	replayed, _ := resetTerminal(t, Effects{})
	replayed.Write(term.VT())
	if !replayed.Mode(45) || replayed.Mode(7) {
		t.Error("VT replay enabled reverse-wrap after disabling its prerequisite")
	}
}

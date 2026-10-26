package vt

import "testing"

func TestOShellLineDisplayModeState(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  LineMode
	}{
		{"\x1b#3", LineDoubleTop}, {"\x1b#4", LineDoubleBottom},
		{"\x1b#6", LineDoubleWidth}, {"\x1b#6\x1b#5", LineSingle},
	} {
		term, rs := resetTerminal(t, Effects{})
		term.Write([]byte("abcdefghijkl"))
		writeControlChunks(term, []byte(tc.input), 1)
		if got := term.LineModeAt(0); got != tc.want {
			t.Fatalf("%q mode=%d want=%d", tc.input, got, tc.want)
		}
		if err := rs.Update(term); err != nil {
			t.Fatal(err)
		}
		rs.Rows()
		rs.NextRow()
		if rs.RowLineMode() != tc.want {
			t.Fatal("render snapshot lost mode")
		}
		term.Write([]byte("Z"))
		if got := term.FormattedText(false); got != "abcdefghijkl\nZ" {
			t.Fatal("display mode changed columns or wrap", got)
		}
		if term.LineModeAt(1) != LineSingle {
			t.Fatal("line mode leaked to next row")
		}
	}
}

func TestOShellLineDisplayModeEraseAndReplay(t *testing.T) {
	for _, tc := range []struct {
		command string
		want    [3]LineMode
	}{
		{"\x1b[K", [3]LineMode{1, 2, 3}},
		{"\x1b[J", [3]LineMode{1, 2, 0}},
		{"\x1b[1J", [3]LineMode{0, 2, 3}},
		{"\x1b[2J", [3]LineMode{}},
		{"\x1b[!p", [3]LineMode{1, 2, 3}},
	} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("\x1b#6A\x1b[2;1H\x1b#3B\x1b[3;1H\x1b#4C\x1b[2;2H" + tc.command))
		for y, want := range tc.want {
			if got := term.LineModeAt(y); got != want {
				t.Fatalf("%q row %d got %d want %d", tc.command, y, got, want)
			}
		}
		other, _ := resetTerminal(t, Effects{})
		other.Write(term.VT())
		for y, want := range tc.want {
			if got := other.LineModeAt(y); got != want {
				t.Fatalf("replay %q row %d got %d want %d", tc.command, y, got, want)
			}
		}
	}
}

func TestOShellLineDisplayModeMoves(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		want          [5]LineMode
	}{
		{"insert", "\x1b[2;1H\x1b[L", [5]LineMode{1, 0, 2, 3, 0}},
		{"delete", "\x1b[2;1H\x1b[M", [5]LineMode{1, 3, 0, 1, 0}},
		{"up", "\x1b[S", [5]LineMode{2, 3, 0, 1, 0}},
		{"down", "\x1b[T", [5]LineMode{0, 1, 2, 3, 0}},
		{"explicit full margins", "\x1b[?69h\x1b[1;12s\x1b[S", [5]LineMode{1, 2, 3, 0, 1}},
		{"explicit narrow margins", "\x1b[?69h\x1b[2;10s\x1b[S", [5]LineMode{1, 2, 3, 0, 1}},
		{"implicit narrow up", "\x1b[?69h\x1b[2;10s\x1b[5;2H\n", [5]LineMode{}},
		{"implicit narrow down", "\x1b[?69h\x1b[2;10s\x1b[1;2H\x1bM", [5]LineMode{}},
		{"implicit full margins", "\x1b[?69h\x1b[1;12s\x1b[5;1H\n", [5]LineMode{2, 3, 0, 1, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			term.Write([]byte("\x1b#6A\x1b[2;1H\x1b#3B\x1b[3;1H\x1b#4C\x1b[4;1HD\x1b[5;1H\x1b#6E" + tc.command))
			for y, want := range tc.want {
				if got := term.LineModeAt(y); got != want {
					t.Fatalf("row %d mode=%d want=%d", y, got, want)
				}
			}
		})
	}
}

func TestOShellLineDisplayModeHistoryScreensAndResize(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b#6A\x1b[5;1H\n"))
	term.ScrollToTop()
	if term.LineModeAt(0) != LineDoubleWidth {
		t.Fatal("history lost row mode")
	}
	term.ScrollToBottom()
	term.Write([]byte("\x1b[H\x1b#3B\x1b[?1049h"))
	if term.LineModeAt(0) != LineSingle {
		t.Fatal("main mode leaked to alternate")
	}
	term.Write([]byte("\x1b#4C\x1b[?1049l"))
	if term.LineModeAt(0) != LineDoubleTop {
		t.Fatal("main mode not restored")
	}
	if err := term.Resize(14, 5, 8, 16); err != nil {
		t.Fatal(err)
	}
	if term.LineModeAt(0) != LineDoubleTop {
		t.Fatal("resize lost mode", term.LineModeAt(0), term.LineModeAt(1))
	}
	term.Write([]byte("\x1bc"))
	for y := 0; y < 5; y++ {
		if term.LineModeAt(y) != LineSingle {
			t.Fatal("RIS retained mode", y)
		}
	}
}

func TestOShellLineDisplayModeReflow(t *testing.T) {
	for _, text := range []string{"", "abcdefghijklmnopqr"} {
		term, _ := resetTerminal(t, Effects{})
		term.SetResizePullBack(false)
		term.Write([]byte("\x1b#6" + text + "\x1b[2;1H\x1b#4\x1b[5;1H"))
		if err := term.Resize(8, 5, 8, 16); err != nil {
			t.Fatal(err)
		}
		term.ScrollToTop()
		if got := term.LineModeAt(0); got != LineDoubleWidth {
			t.Fatal("first surviving row mode", text, got)
		}
		if got := term.LineModeAt(1); got != LineDoubleBottom {
			t.Fatal("second surviving row mode", text, got)
		}
		if got := term.LineModeAt(2); got != LineSingle {
			t.Fatal("new reflow row inherited mode", text, got)
		}
	}
}

package vt

import (
	"fmt"
	"strings"
	"testing"
)

func TestOShellEraseCommandParametersAndPendingWrap(t *testing.T) {
	for _, tc := range []struct{ command, want string }{
		{"0;99K", "abc"}, {"1;99K", "    efghijkl"}, {"2;99K", ""},
		{"?0K", "abc"}, {">1K", "    efghijkl"}, {"2$K", ""},
		{"0;99J", "abc"}, {"1;99J", "    efghijkl"}, {"2;99J", ""},
		{"?0J", "abc"}, {">1J", "    efghijkl"}, {"2$J", ""},
		{"3K", "abcdefghijkl"}, {"22J", "abcdefghijkl"},
	} {
		for _, chunk := range []int{1, 4096} {
			t.Run(fmt.Sprintf("%s/%d", tc.command, chunk), func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				term.Write([]byte("abcdefghijkl\x1b[1;4H"))
				writeControlChunks(term, []byte("\x1b["+tc.command), chunk)
				if got := term.FormattedText(false); got != tc.want {
					t.Fatalf("got %q want %q", got, tc.want)
				}
				layoutCursor(t, term, "", 3, 0)
			})
		}
	}
	for _, command := range []string{"K", "1K", "2K", "J", "1J", "2J", "3J"} {
		t.Run("pending/"+command, func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			term.Write([]byte("abcdefghijkl\x1b[" + command + "Z"))
			want := "\nZ"
			if command == "K" || command == "J" || command == "3J" {
				want = "abcdefghijkl\nZ"
			}
			if got := term.FormattedText(false); got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		})
	}
}

func TestOShellEraseLogicalLineBoundaries(t *testing.T) {
	for _, command := range []string{"K", "1K", "2K"} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("abcdefghijklmnopqrstuvwx\x1b[1;4H\x1b[" + command))
		if strings.Contains(term.Text(), "\n") {
			t.Fatal("EL cut logical continuation", command, term.Text())
		}
	}
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("abcdefghijklmnopqrstuvwx\x1b[2;4H\x1b[J"))
	if got := term.Text(); got != "abcdefghijklmno" {
		t.Fatal("partial ED below cut current continuation", got)
	}
}

func TestOShellErasedWrappedRowSearchAndSelection(t *testing.T) {
	for _, command := range []string{"2K", "K", "X"} {
		term, _ := resetTerminal(t, Effects{})
		erase := command
		if command == "X" {
			erase = "12X"
		}
		term.Write([]byte("abcdefghijklmnopqrstuvwx\x1b[1;1H\x1b[" + erase))
		rows, wrapped := term.SearchRows(0)
		if len(rows) < 2 || rows[0] != strings.Repeat(" ", 12) || !wrapped[0] {
			t.Fatal("erased logical row lost its width", command, rows, wrapped)
		}
		if !term.SelectSearchRange(0, 0, 1, 3) {
			t.Fatal("cannot select across erased wrapped row")
		}
		if got, ok := term.SelectionText(); !ok || got != strings.Repeat(" ", 12)+"mno" {
			t.Fatalf("copied selection %q, %v", got, ok)
		}
	}
}

func TestOShellErasedWrappedRowRetainsLongGrapheme(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	cluster := "A" + strings.Repeat("\u0301", 24)
	term.Write([]byte(cluster + "bcdefghijklmnopqrstuvwx\x1b[1;11H\x1b[K"))
	rows, wrapped := term.SearchRows(0)
	want := cluster + "bcdefghij  "
	if len(rows) < 2 || rows[0] != want || !wrapped[0] {
		t.Fatalf("long cluster lost in erased row: %q want %q", rows, want)
	}
	if !term.SelectSearchRange(0, 0, 1, 3) {
		t.Fatal("cannot select long cluster and erased cells")
	}
	if got, ok := term.SelectionText(); !ok || got != want+"mno" {
		t.Fatalf("long cluster selection %q, %v", got, ok)
	}
}

func TestOShellColumnEditsRegionAndCursor(t *testing.T) {
	for _, tc := range []struct{ command, want string }{
		{"2'}", "abcd  efijkl"}, {"2'~", "abcdgh  ijkl"},
		{"2;99'}", "abcd  efijkl"}, {"2;99~", "abcdgh  ijkl"},
		{"0'}", "abcd efgijkl"}, {"0~", "abcdfgh ijkl"},
	} {
		for _, chunk := range []int{1, 4096} {
			t.Run(fmt.Sprintf("%s/%d", tc.command, chunk), func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				term.Write([]byte(strings.Repeat("abcdefghijkl\r\n", 4) + "abcdefghijkl\x1b[?69h\x1b[3;8s\x1b[2;4r\x1b[3;5H"))
				writeControlChunks(term, []byte("\x1b["+tc.command), chunk)
				rows := strings.Split(term.FormattedText(false), "\n")
				if len(rows) != 5 || rows[0] != "abcdefghijkl" || rows[4] != "abcdefghijkl" {
					t.Fatal("column edit changed outside rows", rows)
				}
				for _, row := range rows[1:4] {
					if row != tc.want {
						t.Fatalf("got %q want %q", row, tc.want)
					}
				}
				layoutCursor(t, term, "", 4, 2)
			})
		}
	}
}

func TestOShellColumnEditsBoundaryRulesAndColors(t *testing.T) {
	for _, insert := range []bool{false, true} {
		command := "2'~"
		if insert {
			command = "2'}"
		}
		t.Run(command, func(t *testing.T) {
			term, rs := resetTerminal(t, Effects{})
			term.Write([]byte(strings.Repeat("abcdefghijkl\r\n", 4) + "abcdefghijkl\x1b[2;4r\x1b[1;5H\x1b[1;31;44m\x1b[" + command))
			rows := strings.Split(term.FormattedText(false), "\n")
			want := "abcdefghijkl"
			if insert {
				want = "abcd  efghij"
			}
			for _, row := range rows[1:4] {
				if row != want {
					t.Fatal("vertical eligibility differs", rows)
				}
			}
			// Move inside the region; DECIC uses default blanks, DECDC erase BG.
			term.Write([]byte("\x1b[3;5H\x1b[" + command))
			if err := rs.Update(term); err != nil {
				t.Fatal(err)
			}
			rs.Rows()
			for row := 0; rs.NextRow(); row++ {
				cells := rs.RowCells()
				if row != 2 {
					continue
				}
				col := 10
				if insert {
					col = 4
				}
				if insert && cells[col].HasBackground || !insert && (!cells[col].HasBackground || !cells[col].Palette || cells[col].Background.R != 4) {
					t.Fatal("wrong column blank background", cells[col])
				}
			}
			term.Write([]byte("Z"))
			if err := rs.Update(term); err != nil {
				t.Fatal(err)
			}
			rs.Rows()
			for row := 0; rs.NextRow(); row++ {
				rs.RowCells()
				if row == 2 {
					s := rs.CellStyle(4)
					if !s.Bold || s.FG.Index != 1 || s.BG.Index != 4 {
						t.Fatal("column edit changed current attributes", s)
					}
				}
			}
		})
	}
	for _, command := range []string{"'}", "'~"} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("abcdefghijkl\x1b[" + command + "Z"))
		if got := term.FormattedText(false); got != "abcdefghijkl\nZ" {
			t.Fatal("column edit at pending column changed text or wrap", command, got)
		}
	}
}

func TestOShellEraseIgnoresProtectionAndRetainsBackground(t *testing.T) {
	for _, command := range []string{"?2K", "?2J", "2K", "2J"} {
		term, rs := resetTerminal(t, Effects{})
		term.Write([]byte("\x1b[1\"qprotected\x1b[0\"q\x1b[1;31;44m\x1b[" + command))
		if term.FormattedText(false) != "" {
			t.Fatal("erase retained protected cells", command, term.Text())
		}
		if err := rs.Update(term); err != nil {
			t.Fatal(err)
		}
		rs.Rows()
		rs.NextRow()
		cell := rs.RowCells()[0]
		style := rs.CellStyle(0)
		if !cell.HasBackground || !cell.Palette || cell.Background.R != 4 || style.Bold || style.FG.Kind != 0 {
			t.Fatal("erase should keep background only", cell, style)
		}
	}
}

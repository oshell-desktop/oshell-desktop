package vt

import (
	"fmt"
	"strings"
	"testing"
)

func TestOShellCharacterEditParameters(t *testing.T) {
	for _, tc := range []struct{ command, want string }{
		{"2;99@", "abc  defghij"}, {"2;99P", "abcfghijkl"}, {"2;99X", "abc  fghijkl"},
		{"0@", "abc defghijk"}, {"0P", "abcefghijkl"}, {"0X", "abc efghijkl"},
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
}

func TestOShellCharacterEditsPreserveWrap(t *testing.T) {
	for _, command := range []string{"P", "X", "@"} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("abcdefghijkl\x1b[" + command + "Z"))
		if got := term.FormattedText(false); got != "abcdefghijkl\nZ" {
			t.Fatalf("pending %s: %q", command, got)
		}
	}
	for _, command := range []string{"P", "X"} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("abcdefghijklmnopqrstuvwx\x1b[1;3H\x1b[" + command))
		if got := term.Text(); strings.Contains(got, "\n") {
			t.Fatalf("edit broke logical line: %s %q", command, got)
		}
	}
}

func TestOShellLineEditsPreserveCursorAndOutsideRegion(t *testing.T) {
	for _, command := range []string{"0L", "1;99L", "0M", "1;99M"} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("AAAAAAAAAAAA\r\nBBBBBBBBBBBB\r\nCCCCCCCCCCCC\r\nDDDDDDDDDDDD\r\nEEEEEEEEEEEE\x1b[2;4r\x1b[3;5H\x1b[" + command))
		layoutCursor(t, term, "", 4, 2)
		rows := strings.Split(term.FormattedText(false), "\n")
		if len(rows) != 5 || rows[0] != "AAAAAAAAAAAA" || rows[1] != "BBBBBBBBBBBB" || rows[4] != "EEEEEEEEEEEE" {
			t.Fatalf("outside region changed: %q", rows)
		}
		if strings.HasSuffix(command, "L") {
			if rows[2] != "" || rows[3] != "CCCCCCCCCCCC" {
				t.Fatalf("insert: %q", rows)
			}
		} else if rows[2] != "DDDDDDDDDDDD" || rows[3] != "" {
			t.Fatalf("delete: %q", rows)
		}
	}
}

func TestOShellExplicitScrollDoesNotCreateHistoryOrUseEraseBackground(t *testing.T) {
	for _, command := range []string{"S", "0S", "1;99S", "T", "0T", "1;99T"} {
		term, rs := resetTerminal(t, Effects{})
		term.Write([]byte("A\r\nB\r\nC\r\nD\r\nE\x1b[3;5H\x1b[1;41m\x1b[" + command))
		layoutCursor(t, term, "", 4, 2)
		if strings.Contains(term.Text(), "A") && strings.HasSuffix(command, "S") {
			t.Fatal("scroll up retained discarded line", term.Text())
		}
		blankRow := 0
		if strings.HasSuffix(command, "S") {
			blankRow = 4
		}
		if err := rs.Update(term); err != nil {
			t.Fatal(err)
		}
		rs.Rows()
		for row := 0; rs.NextRow(); row++ {
			cells := rs.RowCells()
			if row == blankRow && cells[0].HasBackground {
				t.Fatal("scroll blank inherited background", cells[0])
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
				if !s.Bold || s.BG.Kind != 1 || s.BG.Index != 1 {
					t.Fatal("scroll changed current attributes", s)
				}
			}
		}
	}
}

func TestOShellEditMarginsAndImplicitHistory(t *testing.T) {
	for _, command := range []string{"L", "M", "S", "T"} {
		t.Run(command, func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			term.Write([]byte("AAAAAAAAAAAA\r\nBBBBBBBBBBBB\r\nCCCCCCCCCCCC\r\nDDDDDDDDDDDD\r\nEEEEEEEEEEEE\x1b[?69h\x1b[3;8s\x1b[2;4r\x1b[3;5H\x1b[" + command))
			layoutCursor(t, term, "", 4, 2)
			rows := strings.Split(term.FormattedText(false), "\n")
			if len(rows) != 5 || rows[0] != "AAAAAAAAAAAA" || rows[4] != "EEEEEEEEEEEE" {
				t.Fatalf("vertical region escaped: %q", rows)
			}
			for i := 1; i <= 3; i++ {
				if len(rows[i]) != 12 || rows[i][:2] != strings.Repeat(string(rune('A'+i)), 2) || rows[i][8:] != strings.Repeat(string(rune('A'+i)), 4) {
					t.Fatalf("horizontal region escaped: %q", rows)
				}
			}
		})
	}
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("history-A\r\nB\r\nC\r\nD\r\nE\r\nF"))
	if !strings.Contains(term.Text(), "history-A") {
		t.Fatal("implicit linefeed lost history", term.Text())
	}
}

func TestOShellLineShiftRetainsInternalSoftWraps(t *testing.T) {
	// Only new seams become hard line breaks. The B/C pair must stay joined.
	for _, tc := range []struct{ prefix, command, want string }{
		{"\x1b[2;1H", "L", "\nBBBBBBBBBBBBCCCCCCCCCCCC"},
		{"\x1b[1;1H", "M", "BBBBBBBBBBBBCCCCCCCCCCCC"},
		{"", "S", "BBBBBBBBBBBBCCCCCCCCCCCC"},
		{"", "T", "\nAAAAAAAAAAAABBBBBBBBBBBBCCCCCCCCCCCC"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			term.Write([]byte("AAAAAAAAAAAABBBBBBBBBBBBCCCCCCCCCCCC\r\nD\r\nE" + tc.prefix + "\x1b[" + tc.command))
			if got := term.Text(); !strings.Contains(got, tc.want) {
				t.Fatalf("lost moved soft wrap: %q, need %q", got, tc.want)
			}
		})
	}
}

func TestOShellCharacterEditsHonorMarginsAndEraseAttributes(t *testing.T) {
	for _, tc := range []struct{ command, want string }{
		{"2@", "abcd  efijkl"}, {"2P", "abcdgh  ijkl"}, {"99X", "abcd"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			term, rs := resetTerminal(t, Effects{})
			term.Write([]byte("abcdefghijkl\x1b[?69h\x1b[3;8s\x1b[1;5H\x1b[1;31;44m\x1b[" + tc.command))
			if got := term.FormattedText(false); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
			layoutCursor(t, term, "", 4, 0)
			if err := rs.Update(term); err != nil {
				t.Fatal(err)
			}
			rs.Rows()
			rs.NextRow()
			cells := rs.RowCells()
			col := 4
			if strings.HasSuffix(tc.command, "P") {
				col = 6
			}
			style := rs.CellStyle(col)
			if style.Bold || style.FG.Kind != 0 || !cells[col].HasBackground || !cells[col].Palette || cells[col].Background.R != 4 {
				t.Fatal("edit blank should retain background only", style, cells[col])
			}
		})
	}
	for _, command := range []string{"65535@", "65535P", "65535L", "65535M"} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("abcdefghijkl\x1b[?69h\x1b[3;8s\x1b[1;2H\x1b[" + command))
		if got := term.FormattedText(false); got != "abcdefghijkl" {
			t.Fatalf("out-of-margin %s changed cells: %q", command, got)
		}
		layoutCursor(t, term, "", 1, 0)
	}
}

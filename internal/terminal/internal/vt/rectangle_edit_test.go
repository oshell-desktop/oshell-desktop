package vt

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestOShellRectangleEdits(t *testing.T) {
	for _, chunk := range []int{1, 3, 4096} {
		for _, tc := range []struct{ command, want string }{
			{"90;2;3;3;5$x", "abcdefghijkl\nabZZZfghijkl\nabZZZfghijkl\nabcdefghijkl\nabcdefghijkl"},
			{"2;3;3;5$z", "abcdefghijkl\nab   fghijkl\nab   fghijkl\nabcdefghijkl\nabcdefghijkl"},
			{"2;3;3;5${", "abcdefghijkl\nab   fghijkl\nab   fghijkl\nabcdefghijkl\nabcdefghijkl"},
			{"90:2;3:3;5$x", "abcdefghijkl\nabZZZfghijkl\nabZZZfghijkl\nabcdefghijkl\nabcdefghijkl"},
			{"90;1;1;0;0$x", strings.TrimSuffix(strings.Repeat("abcdefghijkl\n", 5), "\n")},
			{"90;5;12;65535;65535$x", strings.Repeat("abcdefghijkl\n", 4) + "abcdefghijkZ"},
			{"90;1;1;1;$x", strings.TrimSuffix(strings.Repeat("abcdefghijkl\n", 5), "\n")},
			{"90;2;3;3;5x", strings.TrimSuffix(strings.Repeat("abcdefghijkl\n", 5), "\n")},
			{"?90;2;3;3;5$x", strings.TrimSuffix(strings.Repeat("abcdefghijkl\n", 5), "\n")},
		} {
			t.Run(fmt.Sprintf("%d/%s", chunk, tc.command), func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				term.Write([]byte(strings.Repeat("abcdefghijkl\r\n", 4) + "abcdefghijkl"))
				writeControlChunks(term, []byte("\x1b["+tc.command), chunk)
				if got := term.FormattedText(false); got != tc.want {
					t.Fatalf("got %q want %q", got, tc.want)
				}
				if x, y := term.Cursor(); x != 11 || y != 4 {
					t.Fatal("rectangle edit moved cursor", x, y)
				}
				term.Write([]byte("Q"))
				if x, y := term.Cursor(); x != 1 || y != 4 {
					t.Fatal("rectangle edit consumed pending wrap", x, y)
				}
			})
		}
	}
}

func TestOShellRectangleFillScalarsAndDefault(t *testing.T) {
	for _, tc := range []struct {
		parameter string
		cp        rune
	}{
		{"30028", '界'}, {"769", '\u0301'}, {"27", 27}, {"0", 0},
		{"55296", ' '}, {"999999999999999999", 65535},
	} {
		term, rs := resetTerminal(t, Effects{})
		term.Write([]byte("\x1b[" + tc.parameter + "$x"))
		if err := rs.Update(term); err != nil {
			t.Fatal(err)
		}
		rs.Rows()
		for rs.NextRow() {
			for _, c := range rs.RowCells() {
				if c.Codepoint != tc.cp || c.Wide != Narrow || c.Grapheme {
					t.Fatalf("scalar %s: %+v", tc.parameter, c)
				}
			}
		}
	}
}

func TestOShellRectangleEraseStylesAndLinks(t *testing.T) {
	for _, command := range []string{"1;2;1;2${", "1;2;1;2$z", "90;1;2;1;2$x"} {
		term, rs := resetTerminal(t, Effects{})
		term.Write([]byte("\x1b[1;31;44m\x1b]8;id=original;https://example.test/\aAe\u0301B\x1b]8;;\a\x1b[0;32;45m\x1b]8;;https://active.test/\a"))
		term.Write([]byte("\x1b[" + command))
		rs.Update(term)
		rs.Rows()
		rs.NextRow()
		cells := rs.RowCells()
		c, s := cells[1], rs.CellStyle(1)
		if c.Grapheme || c.Wide != Narrow {
			t.Fatal("rectangle retained replaced grapheme", c)
		}
		if strings.HasSuffix(command, "${") {
			if c.Codepoint != ' ' || !c.Hyperlink || !s.Bold || s.FG.Index != 1 || s.BG.Index != 4 {
				t.Fatal("selective erase lost original attributes", c, s)
			}
		} else {
			want := rune(' ')
			if strings.HasSuffix(command, "$x") {
				want = 'Z'
			}
			if c.Codepoint != want || c.Hyperlink || s.Bold || s.FG.Index != 2 || s.BG.Index != 5 {
				t.Fatal("fill/erase did not replace attributes and clear payload", c, s)
			}
		}
		if !cells[0].Hyperlink || !cells[2].Hyperlink || cells[0].Codepoint != 'A' || cells[2].Codepoint != 'B' {
			t.Fatal("rectangle changed neighbors", cells)
		}
		term.Write([]byte("C"))
		rs.Update(term)
		rs.Rows()
		rs.NextRow()
		if c := rs.RowCells()[3]; !c.Hyperlink || c.Codepoint != 'C' || rs.CellStyle(3).FG.Index != 2 {
			t.Fatal("rectangle consumed active style or link", c)
		}
	}
}

func TestOShellRectangleEditActivePageAndOrigin(t *testing.T) {
	var reply bytes.Buffer
	term, _ := resetTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }})
	term.Write([]byte(strings.Repeat("history\r\n", 20) + "\x1b[2J\x1b[?69h\x1b[3;9s\x1b[2;4r\x1b[?6h\x1b[?69l"))
	term.ScrollToTop()
	bar := term.Scrollbar()
	term.Write([]byte("\x1b[90;1;1;1;3$x\x1b[1;1;1;1;1;3*y"))
	if reply.String() != "\x1bP1!~010e\x1b\\" || term.Scrollbar() != bar {
		t.Fatal("rectangle did not use active page/origin", reply.String())
	}
	term.ScrollToBottom()
	term.Write([]byte("\x1b[?6l\x1b[?47h\x1b[90$x\x1b[${\x1b[?47l"))
	reply.Reset()
	term.Write([]byte("\x1b[1;1;2;3;2;5*y"))
	if reply.String() != "\x1bP1!~010e\x1b\\" {
		t.Fatal("alternate rectangle changed primary page", reply.String())
	}
}

func TestOShellRectangleEraseCompactBackgroundAndWideEdges(t *testing.T) {
	term, rs := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b[44m\x1b[2K\x1b[48;2;10;20;30m\x1b[2;1H\x1b[2K\x1b[0m\x1b[1;2;2;4${"))
	rs.Update(term)
	rs.Rows()
	for y := range 2 {
		rs.NextRow()
		cells := rs.RowCells()
		for x := 1; x < 4; x++ {
			s := rs.CellStyle(x)
			if cells[x].Codepoint != ' ' || (y == 0 && s.BG != (Color{Kind: 1, Index: 4})) || (y == 1 && s.BG != (Color{Kind: 2, RGB: RGB{10, 20, 30}})) {
				t.Fatal("selective erase lost compact background", y, x, cells[x], s)
			}
		}
	}
	for _, column := range []int{1, 2} {
		term.Reset()
		term.Write([]byte("\x1b[1\"q界B\x1b[0\"q\x1b[90;1;" + fmt.Sprint(column) + ";1;" + fmt.Sprint(column) + "$x"))
		rs.Update(term)
		rs.Rows()
		rs.NextRow()
		cells := rs.RowCells()
		if cells[column-1].Codepoint != 'Z' || cells[column-1].Wide != Narrow || cells[2].Codepoint != 'B' {
			t.Fatal("rectangle failed to replace one protected cell", column, cells)
		}
		if column == 2 && (cells[0].Codepoint != '界' || cells[0].Wide != WideChar) {
			t.Fatal("rectangle erased wide head outside bounds", cells)
		}
		if column == 1 && cells[1].Codepoint != 0 {
			t.Fatal("rectangle changed untouched zero-code tail", cells)
		}
	}
}

func TestOShellRectangleEditKeepsRowsAndImages(t *testing.T) {
	for _, suffix := range []string{"90;1;1;1;3$x", "1;1;1;3$z", "1;1;1;3${"} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("abcdefghijklmnopqrstuvwx\x1b[1;1H\x1b#6\x1b_Ga=T,f=24,s=1,v=1,i=1,c=1,r=1,C=1;/wAA\x1b\\"))
		_, wrapped := term.SearchRows(0)
		graphics := term.Graphics(8, 16)
		term.Write([]byte("\x1b[" + suffix))
		_, after := term.SearchRows(0)
		if fmt.Sprint(wrapped) != fmt.Sprint(after) || term.LineModeAt(0) != LineDoubleWidth || len(term.Graphics(8, 16)) != len(graphics) || len(graphics) == 0 {
			t.Fatal("rectangle changed row metadata or attached images", suffix)
		}
	}
}

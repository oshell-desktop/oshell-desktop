package vt

import (
	"fmt"
	"strings"
	"testing"
)

func TestOShellRectangleCopyGeometry(t *testing.T) {
	initial := []string{"ABCDEFGHIJKL", "abcdefghijkl", "0123456789!?", "mnopqrstuvwx", "YZ.........."}
	for _, tc := range []struct {
		query string
		rows  map[int]string
	}{
		{"1;2;2;4;1;3;6;1;0$v", map[int]string{2: "01234BCD89!?", 3: "mnopqbcduvwx"}},
		{"1;1;3;3;1;2;2;1;0$v", map[int]string{1: "aABCefghijkl", 2: "0abc456789!?", 3: "m012qrstuvwx"}},
		{"0;0;0;0;0;2;2;0;0$v", map[int]string{1: "aABCDEFGHIJK", 2: "0abcdefghijk", 3: "m0123456789!", 4: "Ymnopqrstuvw"}},
		{"1;11;1;12;1;2;1;1;0$v", map[int]string{1: "Kbcdefghijkl"}},
		{"1;12;1;12;1;2;1;1;0$v", nil},
		{"1;1;1;3;1;2;6$v", map[int]string{1: "ABCdefghijkl"}},
		{"1;1;1;3;1;2;6;2$v", map[int]string{1: "abcdeABCijkl"}},
		{"1;1;1;3;1;2;6;2;0$v", nil},
		{"1;1;1;3;2;2;6;1;0$v", nil},
		{"1;1;2;3;1;65535;65535;1;0$v", map[int]string{4: "YZ.........A"}},
		{"1:1;1:3;1:2;6:1;0$v", map[int]string{1: "abcdeABCijkl"}},
		{"1;1;1;3;1;2;6;1;0v", nil},
		{"?1;1;1;3;1;2;6;1;0$v", nil},
	} {
		for _, chunk := range []int{1, 3, 4096} {
			t.Run(fmt.Sprintf("%s/%d", tc.query, chunk), func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				term.Write([]byte(strings.Join(initial, "\r\n")))
				writeControlChunks(term, []byte("\x1b["+tc.query), chunk)
				want := append([]string(nil), initial...)
				for row, value := range tc.rows {
					want[row] = value
				}
				if got := term.FormattedText(false); got != strings.Join(want, "\n") {
					t.Fatalf("got %q want %q", got, strings.Join(want, "\n"))
				}
				if x, y := term.Cursor(); x != 11 || y != 4 {
					t.Fatal("copy moved cursor", x, y)
				}
				term.Write([]byte("Q"))
				if x, y := term.Cursor(); x != 1 || y != 4 {
					t.Fatal("copy consumed pending wrap", x, y)
				}
			})
		}
	}
}

func TestOShellRectangleCopyOriginScreensAndRows(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte(strings.Repeat("history\r\n", 20) + "\x1b[2J\x1b[2;3HABC\x1b#6\x1b[4;1Htarget\x1b_Ga=T,f=24,s=1,v=1,i=1,c=1,r=1,C=1;/wAA\x1b\\\x1b[?69h\x1b[3;9s\x1b[2;4r\x1b[?6h\x1b[?69l"))
	graphics := term.Graphics(8, 16)
	term.ScrollToTop()
	bar := term.Scrollbar()
	term.Write([]byte("\x1b[1;1;1;3;1;4;1;1;0$v"))
	if term.Scrollbar() != bar {
		t.Fatal("copy changed viewport")
	}
	term.ScrollToBottom()
	text, _ := term.RowText(3)
	if !strings.HasPrefix(string(text), "ABCget") || term.LineModeAt(3) != LineSingle || len(term.Graphics(8, 16)) != len(graphics) {
		t.Fatal("copy incorrectly applied target origin or copied row metadata", string(text))
	}
	term.Write([]byte("\x1b[?6l\x1b[?47h\x1b[HDEF\x1b[?2026h\x1b[1;1;1;3;1;2;1;1;0$v\x1b[?2026l"))
	text, _ = term.RowText(1)
	if !strings.HasPrefix(string(text), "DEF") {
		t.Fatal("copy did not use alternate active screen", string(text))
	}
	term.Write([]byte("\x1b[?47l"))
	text, _ = term.RowText(3)
	if !strings.HasPrefix(string(text), "ABCget") {
		t.Fatal("alternate copy changed primary screen", string(text))
	}
}

func TestOShellRectangleCopyOwnsLinksAndGraphemes(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	if err := term.Resize(64, 80, 8, 16); err != nil {
		t.Fatal(err)
	}
	for y := range 32 {
		uri := fmt.Sprintf("https://copy.test/%d/%s", y, strings.Repeat("path/", 180))
		term.Write(fmt.Appendf(nil, "\x1b[%d;1H\x1b]8;id=row-%d;%s\aAe\u0301界Z\x1b]8;;\a", y+1, y, uri))
	}
	for range 3 {
		term.Write([]byte("\x1b[1;1;32;5;1;49;10;1;0$v"))
	}
	term.Write([]byte("\x1b[1;1;32;64$z"))
	for y := range 32 {
		ref, ok := term.CellAt(10, 48+y)
		want := fmt.Sprintf("https://copy.test/%d/%s", y, strings.Repeat("path/", 180))
		if !ok || term.Hyperlink(ref) != want {
			t.Fatal("copy did not retain independent hyperlink bytes", y)
		}
		text, _ := term.RowText(48 + y)
		if !strings.Contains(string(text), "Ae\u0301界Z") {
			t.Fatal("copy did not retain independent grapheme bytes", y, string(text))
		}
	}
	term.Reset()
	term.Write([]byte("\x1b]8;id=empty;\aABC\x1b]8;;\a\x1b[1;1;1;3;1;1;4;1;0$v"))
	if a, b, ok := term.HyperlinkRange(4, 0); !ok || a != 0 || b != 6 {
		t.Fatal("copy did not retain original empty-link identity", a, b, ok)
	}
}

func TestOShellRectangleCopyAttributes(t *testing.T) {
	term, rs := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b[1;31;44m\x1b]8;id=source;https://source.test/\aA界e\u0301\x1b]8;;\a\x1b[0;32m\x1b[3;1Htarget\x1b]8;;https://current.test/\a"))
	term.Write([]byte("\x1b[1;1;1;4;1;3;2;1;0$v"))
	rs.Update(term)
	rs.Rows()
	for range 3 {
		rs.NextRow()
	}
	cells := rs.RowCells()
	for x, cp := range []rune{'A', '界', 0, 'e'} {
		cell, style := cells[x+1], rs.CellStyle(x+1)
		if cell.Codepoint != cp || !cell.Hyperlink || !style.Bold || style.FG.Index != 1 || style.BG.Index != 4 {
			t.Fatal("copy lost cell identity", x, cell, style)
		}
	}
	if cells[2].Wide != WideChar || cells[3].Wide != SpacerTail || !cells[4].Grapheme {
		t.Fatal("copy lost width or grapheme", cells)
	}
	if cps := rs.CellGraphemes(4, nil); len(cps) != 2 || cps[0] != 'e' || cps[1] != '\u0301' {
		t.Fatal("copy lost combining scalar", cps)
	}
	term.Write([]byte("X"))
	rs.Update(term)
	rs.Rows()
	for range 3 {
		rs.NextRow()
	}
	if c := rs.RowCells()[6]; c.Codepoint != 'X' || !c.Hyperlink || rs.CellStyle(6).FG.Index != 2 {
		t.Fatal("copy consumed active attributes", c)
	}
}

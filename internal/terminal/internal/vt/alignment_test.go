package vt

import (
	"strings"
	"testing"
)

func TestOShellAlignmentRetainsModesAndAttributes(t *testing.T) {
	for _, chunk := range []int{1, 4096} {
		term, rs := resetTerminal(t, Effects{})
		term.Write([]byte("\x1b#6abcdefghijklmnop\x1b[2;1H\x1b#3\x1b[3;1H\x1b#4\x1b[?69h\x1b[3;9s\x1b[2;4r\x1b[1;3;4;31;44;58;5;2m"))
		writeControlChunks(term, []byte("\x1b#8"), chunk)
		layoutCursor(t, term, "", 0, 0)
		want := strings.TrimSuffix(strings.Repeat("EEEEEEEEEEEE\n", 5), "\n")
		if got := term.FormattedText(false); got != want {
			t.Fatalf("alignment=%q", got)
		}
		for y, want := range []LineMode{1, 2, 3, 0, 0} {
			if got := term.LineModeAt(y); got != want {
				t.Fatalf("row %d mode=%d want=%d", y, got, want)
			}
		}
		if err := rs.Update(term); err != nil {
			t.Fatal(err)
		}
		rs.Rows()
		rs.NextRow()
		rs.RowCells()
		s := rs.CellStyle(0)
		if s.Bold || s.Italic || s.Underline != 0 || s.FG.Index != 1 || s.BG.Index != 4 || s.UnderlineColor.Index != 2 {
			t.Fatal("alignment colors/style", s)
		}
		term.Write([]byte("Z"))
		if err := rs.Update(term); err != nil {
			t.Fatal(err)
		}
		rs.Rows()
		rs.NextRow()
		rs.RowCells()
		s = rs.CellStyle(0)
		if !s.Bold || !s.Italic || s.Underline == 0 || s.FG.Index != 1 || s.BG.Index != 4 || s.UnderlineColor.Index != 2 {
			t.Fatal("alignment modified current SGR", s)
		}
		term.Write([]byte("\x1b[?6h\x1b[H"))
		layoutCursor(t, term, "", 2, 1)
	}
}

func TestOShellAlignmentOriginAndHistory(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("history\r\n\r\n\r\n\r\n\r\n\x1b[HTOP\x1b[?69h\x1b[3;9s\x1b[2;4r\x1b[?6h\x1b#8"))
	if !term.Mode(6) || !term.Mode(69) {
		t.Fatal("alignment disabled origin/margins")
	}
	layoutCursor(t, term, "", 2, 1)
	if text, _ := term.RowText(0); strings.TrimSpace(string(text)) != "TOP" {
		t.Fatal("calibration changed row before origin", string(text))
	}
	for y := 1; y < 5; y++ {
		if text, _ := term.RowText(y); string(text) != strings.Repeat("E", 12) {
			t.Fatal("calibration skipped visible row", y, string(text))
		}
	}
	rows, _ := term.SearchRows(0)
	if !strings.Contains(strings.Join(rows, "\n"), "history") {
		t.Fatal("alignment overwrote history", rows)
	}
	term.ScrollToTop()
	if !strings.HasPrefix(term.FormattedText(false), "history") {
		t.Fatal("history missing", term.FormattedText(false))
	}
}

func TestOShellAlignmentPreservesImagesAndRemovesCellLinks(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=3,c=1,r=1,C=1", []byte{255, 0, 0}))
	term.Write([]byte("\x1b]8;;https://example.invalid/\aÁ界\x1b[1\"qprotected\x1b#8"))
	if len(term.Graphics(8, 16)) != 1 {
		t.Fatal("alignment deleted image")
	}
	ref, ok := term.CellAt(0, 0)
	if !ok || term.Hyperlink(ref) != "" {
		t.Fatal("alignment retained cell link")
	}
	term.Write([]byte("Z"))
	ref, _ = term.CellAt(0, 0)
	if term.Hyperlink(ref) != "https://example.invalid/" {
		t.Fatal("alignment reset current link")
	}
}

func TestOShellAlignmentAcrossPagesAndSavedCursor(t *testing.T) {
	loadLib(t)
	term, err := NewTerminal(97, 160, Effects{})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Free()
	for i := 0; i < 8; i++ {
		term.Write([]byte("\x1b[H\x1b[1;31;44mÁ界\x1b[123;12H\x1b7\x1b#8"))
		rows := strings.Split(term.FormattedText(false), "\n")
		if len(rows) != 160 {
			t.Fatal("alignment row count", len(rows))
		}
		for y, row := range rows {
			if row != strings.Repeat("E", 97) {
				t.Fatal("alignment cells", y, len(row))
			}
		}
		term.Write([]byte("\x1b8Z"))
		layoutCursor(t, term, "", 12, 122)
	}
}

package vt

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unsafe"
)

func resetTerminal(t *testing.T, effects Effects) (*Terminal, *RenderState) {
	t.Helper()
	loadLib(t)
	if oshellABI < 3 || oshellABI > 62 {
		if os.Getenv("OSHELL_REQUIRE_WIDTH_EXTENSION") == "1" {
			t.Fatal("required OShell reset extension is absent")
		}
		t.Skip("requires OShell native extension ABI 3")
	}
	term, err := NewTerminal(12, 5, effects)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(term.Free)
	if err := term.Resize(12, 5, 8, 16); err != nil {
		t.Fatal(err)
	}
	rs, err := NewRenderState()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rs.Free)
	return term, rs
}

func TestOShellSoftResetPreservesContentAndPosition(t *testing.T) {
	for _, fragmented := range []bool{false, true} {
		t.Run(fmt.Sprint(fragmented), func(t *testing.T) {
			term, rs := resetTerminal(t, Effects{})
			for i := range 15 {
				term.Write(fmt.Appendf(nil, "line-%02d\r\n", i))
			}
			term.Write([]byte("\x1b]2;kept title\x07\x1b]7;file://host/work\x07\x1b[3;4H\x1b[1;31m\x1b)0\x0e\x1b]8;;https://example.test/\x07"))
			before, bar := term.Text(), term.Scrollbar()
			x, y := term.Cursor()
			sequence := []byte("\x1b[!p")
			if fragmented {
				for _, b := range sequence {
					term.Write([]byte{b})
				}
			} else {
				term.Write(sequence)
			}
			x2, y2 := term.Cursor()
			if term.Text() != before || term.Scrollbar() != bar || x != x2 || y != y2 {
				t.Fatal("soft reset changed content/history/current cursor")
			}
			if term.Title() != "kept title" || !strings.Contains(term.Pwd(), "/work") {
				t.Fatal("soft reset discarded title or current directory")
			}
			term.Write([]byte("q"))
			rs.Update(term)
			rs.Rows()
			for row := 0; row <= y; row++ {
				rs.NextRow()
			}
			cell := rs.RowCells()[x]
			if cell.Codepoint != 'q' || cell.Style != 0 || cell.Hyperlink {
				t.Fatalf("new text retained charset/style/link: %+v", cell)
			}
		})
	}
}

func TestOShellSoftResetModesMarginsAndSavedCursor(t *testing.T) {
	const insert Mode = 0x8000 | 4
	const linefeed Mode = 0x8000 | 20
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b[?69h\x1b[3;9s\x1b[2;4r\x1b[?6h\x1b[2;3H\x1b7\x1b[?25l\x1b[4h\x1b[20h\x1b[?45h\x1b[?7l\x1b[?1h\x1b=\x1b[?1000;1004;2004;2026;2027h"))
	x, y := term.Cursor()
	term.Write([]byte("\x1b[0!p"))
	for _, mode := range []Mode{insert, linefeed, 6, 45, 66, ModeCursorKeys} {
		if term.Mode(mode) {
			t.Errorf("mode %d was not cleared", mode)
		}
	}
	for _, mode := range []Mode{7, 69, ModeCursorVisible, 1000, ModeFocusEvent, ModeBracketedPaste, ModeSyncOutput, ModeGraphemes} {
		if !term.Mode(mode) {
			t.Errorf("mode %d was not set/preserved", mode)
		}
	}
	if xx, yy := term.Cursor(); xx != x || yy != y {
		t.Fatal("resetting origin or margins moved the cursor")
	}
	term.Write([]byte("\x1b[99;99H"))
	if x, y := term.Cursor(); x != 11 || y != 4 {
		t.Fatalf("margins not reset: cursor %d,%d", x, y)
	}
	term.Write([]byte("\x1b8"))
	if x, y := term.Cursor(); x != 0 || y != 0 || term.Mode(7) || term.Mode(69) || term.Mode(45) {
		t.Fatal("saved cursor/modes survived DECSTR")
	}
	// DECSC/DECRC must retain these SwiftTerm saved flags in ordinary use too.
	term.Write([]byte("\x1b[?7;45;69h\x1b7\x1b[?7;45;69l\x1b8"))
	if !term.Mode(7) || !term.Mode(45) || !term.Mode(69) {
		t.Fatal("saved wrap/margin/reverse-wrap flags were not restored")
	}
	term.SetModeDefault(linefeed, true)
	term.Write([]byte("\x1b[20l\x1b[!p"))
	if !term.Mode(linefeed) {
		t.Fatal("linefeed did not restore configured default")
	}
}

func TestOShellSoftResetPreservesAlternateScreenAndImages(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("primary\x1b[?1049halternate"))
	term.Write(kittyCommand("a=T,f=32,s=1,v=1,i=9,C=1", []byte{255, 0, 0, 255}))
	term.Write([]byte("\x1b[!p"))
	if !strings.Contains(term.Text(), "alternate") || len(term.Graphics(8, 16)) != 1 {
		t.Fatal("soft reset cleared alternate screen/image")
	}
	term.Write([]byte("\x1b[?1049l"))
	if !strings.Contains(term.Text(), "primary") {
		t.Fatal("soft reset damaged primary screen")
	}
}

func TestOShellResetColorAndCursorBoundaries(t *testing.T) {
	var holds []bool
	term, rs := resetTerminal(t, Effects{RenderHold: func(v bool) { holds = append(holds, v) }})
	var palette [256]RGB
	palette[42] = RGB{R: 12, G: 34, B: 56}
	term.SetColors(RGB{R: 200}, RGB{B: 10}, nil, &palette)
	term.Write([]byte("content\x1b]2;retained\x07\x1b]4;42;#123456\x07\x1b]10;#abcdef\x07\x1b[?25l\x1b[6 q\x1b[?2026h"))
	term.Write([]byte("\x1bc"))
	rs.Update(term)
	if term.Text() != "" || rs.Colors().Palette[42] != (RGB{R: 0x12, G: 0x34, B: 0x56}) || term.Title() != "retained" {
		t.Fatal("RIS content/color/title boundary differs from OShell")
	}
	if rs.Cursor().Visible || rs.Cursor().Style != CursorBar || rs.Cursor().Blinking || term.Mode(ModeSyncOutput) {
		t.Fatalf("RIS cursor/update state = %+v", rs.Cursor())
	}
	if len(holds) != 2 || !holds[0] || holds[1] {
		t.Fatalf("RIS did not end synchronized output: %v", holds)
	}
	term.Write([]byte("\x1b[!p"))
	rs.Update(term)
	if rs.Colors().Palette[42] != palette[42] || !rs.Cursor().Visible || rs.Cursor().Style != CursorBar {
		t.Fatal("DECSTR did not restore palette and cursor visibility")
	}
	if rs.Colors().Foreground != (RGB{R: 0xab, G: 0xcd, B: 0xef}) {
		t.Fatal("reset discarded dynamic foreground")
	}
}

func TestOShellFullResetRetainsAlternateTextAndClearsBothKeyboards(t *testing.T) {
	for _, active := range []string{"primary", "alternate"} {
		t.Run(active, func(t *testing.T) {
			var replies strings.Builder
			term, _ := resetTerminal(t, Effects{WritePTY: func(data []byte) { replies.Write(data) }})
			term.Write([]byte(strings.Repeat("history\r\n", 20) + "\x1b[>3u\x1b[?47h\x1b[HALTERNATE\x1b[3;4H\x1b7\x1b[>7u"))
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=1,c=1,r=1,C=1", []byte{255, 0, 0}))
			if active == "primary" {
				term.Write([]byte("\x1b[?47l\x1b[1;1H"))
			}
			term.Write([]byte("\x1bc\x1b[?u"))
			if term.Text() != "" || term.Scrollbar().Total != 5 {
				t.Fatal("RIS did not rebuild primary buffer/history", term.Scrollbar())
			}
			if x, y := term.Cursor(); x != 3 || y != 2 {
				t.Fatalf("RIS did not copy alternate cursor: %d,%d", x, y)
			}
			if !strings.Contains(replies.String(), "\x1b[?0u") {
				t.Fatal("primary keyboard not reset", replies.String())
			}
			replies.Reset()
			term.Write([]byte("\x1b[?47h\x1b[?u"))
			if !strings.Contains(term.Text(), "ALTERNATE") {
				t.Fatal("RIS destroyed alternate text")
			}
			if len(term.Graphics(8, 16)) != 0 {
				t.Fatal("RIS preserved alternate image")
			}
			if !strings.Contains(replies.String(), "\x1b[?0u") {
				t.Fatal("alternate keyboard not reset", replies.String())
			}
			term.Write([]byte("\x1b[H\x1b8"))
			if x, y := term.Cursor(); x != 3 || y != 2 {
				t.Fatal("RIS destroyed alternate saved cursor", x, y)
			}
		})
	}
}

func TestOShellAlternateModeClearBoundaries(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("PRIMARY\x1b[?47h\x1b[HALTERNATE\x1b[?47l\x1b[?1049h"))
	if !strings.Contains(term.Text(), "ALTERNATE") {
		t.Fatal("1049 entry cleared retained alternate text")
	}
	term.Write([]byte("\x1b[?25l\x1b[?1049h"))
	if !strings.Contains(term.Text(), "ALTERNATE") || !term.Mode(ModeCursorVisible) {
		t.Fatal("repeated entry cleared text or kept cursor hidden")
	}
	term.Write([]byte("\x1b[?1049l\x1b[?47h"))
	if term.Text() != "" {
		t.Fatal("1049 exit did not clear alternate text")
	}
	term.Write([]byte("SECOND\x1b[?1047l\x1b[?47h"))
	if term.Text() != "" {
		t.Fatal("1047 exit did not clear alternate text")
	}
	term.Write([]byte("\x1b[?47l"))
	if !strings.Contains(term.Text(), "PRIMARY") {
		t.Fatal("alternate clear damaged primary text")
	}
}

func TestOShellAlternateExitPreservesSavedCursorAndKeyboard(t *testing.T) {
	for _, mode := range []string{"1047", "1049"} {
		t.Run(mode, func(t *testing.T) {
			var replies strings.Builder
			term, _ := resetTerminal(t, Effects{WritePTY: func(data []byte) { replies.Write(data) }})
			term.Write([]byte("\x1b[?47h\x1b[4;5H\x1b[31m\x1b7\x1b[>7u\x1b[?" + mode + "l\x1b[?47h\x1b[?u\x1b8"))
			if !strings.Contains(replies.String(), "\x1b[?7u") {
				t.Fatal("alternate clear reset keyboard", replies.String())
			}
			if x, y := term.Cursor(); x != 4 || y != 3 {
				t.Fatal("alternate clear reset saved cursor", x, y)
			}
			if term.Text() != "" {
				t.Fatal("alternate text not cleared")
			}
		})
	}
}

func TestOShellRepeatedAlternateEntryPreservesImages(t *testing.T) {
	for _, mode := range []string{"47", "1047", "1049"} {
		t.Run(mode, func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			term.Write([]byte("\x1b[?" + mode + "h"))
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=1,p=1,c=1,r=1,C=1", []byte{255, 0, 0}))
			term.Write([]byte("\x1b[?" + mode + "h"))
			if g := term.Graphics(8, 16); len(g) != 1 || g[0].Image.Pix[0] != 255 {
				t.Fatal("repeated entry cleared image")
			}
		})
	}
}

func TestOShellFullResetRetainsAlternateTextWithoutPromptMarks(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	prompt := func() int32 {
		t.Helper()
		ref, found := term.screenCell(0, 0)
		if !found {
			t.Fatal("missing prompt row")
		}
		var raw uint64
		var mark int32
		if !ok(call(fnGridRefRow, uintptr(unsafe.Pointer(&ref)), uintptr(unsafe.Pointer(&raw)))) || !ok(call(fnRawRowGet, uintptr(raw), 6, uintptr(unsafe.Pointer(&mark)))) {
			t.Fatal("cannot read prompt mark")
		}
		return mark
	}
	term.Write([]byte("\x1b]133;A\x07primary"))
	if prompt() != 1 {
		t.Fatal("primary fixture did not mark a prompt")
	}
	// OSC 133 is ignored in the alternate buffer, including its row marks.
	term.Write([]byte("\x1b[?47h\x1b[H\x1b]133;A\x07prompt"))
	if prompt() != 0 {
		t.Fatal("alternate screen accepted a prompt mark")
	}
	term.Write([]byte("\x1bc\x1b[?47h"))
	if !strings.Contains(term.Text(), "prompt") || prompt() != 0 {
		t.Fatal("RIS must retain alternate text without introducing prompt metadata")
	}
}

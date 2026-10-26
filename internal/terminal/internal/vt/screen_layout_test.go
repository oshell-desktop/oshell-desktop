package vt

import "testing"

func layoutCursor(t *testing.T, term *Terminal, command string, x, y int) {
	t.Helper()
	term.Write([]byte(command))
	if xx, yy := term.Cursor(); xx != x || yy != y {
		t.Fatalf("cursor after %q = %d,%d, want %d,%d", command, xx, yy, x, y)
	}
}

func TestOShellTabStopsBelongToEachScreen(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b[3g\x1b[1;4H\x1bH"))
	layoutCursor(t, term, "\x1b[?47h\x1b[H\t", 8, 0)
	term.Write([]byte("\x1b[3g\x1b[1;7H\x1bH"))
	layoutCursor(t, term, "\x1b[?47l\x1b[H\t", 3, 0)
	layoutCursor(t, term, "\x1b[?47h\x1b[H\t", 6, 0)
	layoutCursor(t, term, "\x1b[?1047l\x1b[?47h\x1b[H\t", 6, 0)
	layoutCursor(t, term, "\x1bc\x1b[H\t", 8, 0)
	layoutCursor(t, term, "\x1b[?47h\x1b[H\t", 6, 0)
}

func TestOShellResizeExtendsEachTabTableFromItsLastStop(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b[3g\x1b[1;4H\x1bH\x1b[?47h\x1b[3g\x1b[1;6H\x1bH"))
	if err := term.Resize(20, 7, 8, 16); err != nil {
		t.Fatal(err)
	}
	layoutCursor(t, term, "\x1b[H\t", 5, 0)
	layoutCursor(t, term, "\t", 13, 0)
	layoutCursor(t, term, "\x1b[?47l\x1b[H\t", 3, 0)
	layoutCursor(t, term, "\t", 11, 0)
	if err := term.Resize(10, 5, 8, 16); err != nil {
		t.Fatal(err)
	}
	layoutCursor(t, term, "\x1b[H\t", 3, 0)
	layoutCursor(t, term, "\x1b[?47h\x1b[H\t", 5, 0)
	term.Write([]byte("\x1b[3g"))
	if err := term.Resize(10, 6, 8, 16); err != nil {
		t.Fatal(err)
	}
	layoutCursor(t, term, "\x1b[H\t", 8, 0)
}

func TestOShellScreenScrollRegionsAndMarginsAreIndependent(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b[?69h\x1b[3;9s\x1b[2;4r\x1b[?6h"))
	layoutCursor(t, term, "\x1b[99;99H\t", 8, 3)
	layoutCursor(t, term, "\x1b[?47h\x1b[99;99H\t", 11, 4)
	term.Write([]byte("\x1b[5;10s\x1b[3;5r"))
	layoutCursor(t, term, "\x1b[99;99H\t", 9, 4)
	layoutCursor(t, term, "\x1b[?69l\x1b[99;99H\t", 11, 4)
	layoutCursor(t, term, "\x1b[?69h\x1b[99;99H\t", 9, 4)
	layoutCursor(t, term, "\x1b[?47l\x1b[99;99H\t", 8, 3)
	layoutCursor(t, term, "\x1b[?47h\x1b[?1047l\x1b[?47h\x1b[99;99H\t", 11, 4)
	layoutCursor(t, term, "\x1b[?47l\x1b[99;99H\t", 8, 3)
}

func TestOShellHorizontalMarginChangesDoNotMoveCursor(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	layoutCursor(t, term, "\x1b[3;6H\x1b[?69h\x1b[3;9s", 5, 2)
}

func TestOShellResizeAndResetKeepInactiveLayout(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b[?69h\x1b[3;9s\x1b[2;4r\x1b[?47h\x1b[5;10s\x1b[3;5r\x1b[?6h"))
	if err := term.Resize(20, 7, 8, 16); err != nil {
		t.Fatal(err)
	}
	layoutCursor(t, term, "\x1b[99;99H\t", 9, 6)
	layoutCursor(t, term, "\x1b[?47l\x1b[99;99H\t", 8, 6)
	layoutCursor(t, term, "\x1b[?69l\x1b[?47h\x1b[99;99H\t", 19, 6)
	layoutCursor(t, term, "\x1b[?69h\x1b[99;99H\t", 9, 6)
	layoutCursor(t, term, "\x1bc\x1b[?69;6h\x1b[99;99H\t", 19, 6)
	layoutCursor(t, term, "\x1b[?47h\x1b[99;99H\t", 9, 6)
	layoutCursor(t, term, "\x1b[!p\x1b[?6h\x1b[99;99H\t", 19, 6)
}

func TestOShellLargeTabTablesOwnTheirAllocations(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	if err := term.Resize(800, 8, 1, 1); err != nil {
		t.Fatal(err)
	}
	term.Write([]byte("\x1b[3g\x1b[1;701H\x1bH\x1b[?47h\x1b[3g\x1b[1;601H\x1bH"))
	if err := term.Resize(1200, 10, 1, 1); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		layoutCursor(t, term, "\x1b[?47l\x1b[H\t", 700, 0)
		layoutCursor(t, term, "\t", 708, 0)
		layoutCursor(t, term, "\x1b[?47h\x1b[H\t", 600, 0)
		layoutCursor(t, term, "\t", 608, 0)
	}
	layoutCursor(t, term, "\x1bc\x1b[H\t", 8, 0)
	layoutCursor(t, term, "\x1b[?47h\x1b[H\t", 600, 0)
}

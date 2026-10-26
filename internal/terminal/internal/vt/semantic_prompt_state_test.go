package vt

import (
	"fmt"
	"testing"
	"unsafe"
)

func TestOShellSemanticOutputKeepsAuthoredPromptMark(t *testing.T) {
	for _, prompt := range []string{"A", "P"} {
		for _, end := range []string{"C", "D"} {
			t.Run(prompt+"/"+end, func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				term.Write([]byte("\x1b]133;" + prompt + "\a\x1b[2K\x1b]133;" + end + "\a"))
				ref, found := term.screenCell(0, 0)
				if !found {
					t.Fatal("missing row")
				}
				var raw uint64
				var mark int32
				if !ok(call(fnGridRefRow, uintptr(unsafe.Pointer(&ref)), uintptr(unsafe.Pointer(&raw)))) || !ok(call(fnRawRowGet, uintptr(raw), 6, uintptr(unsafe.Pointer(&mark)))) {
					t.Fatal("cannot read prompt mark")
				}
				if mark != 1 {
					t.Fatalf("output discarded authored prompt mark: %d", mark)
				}
			})
		}
	}
}

// A/N right prompts are marks at the current cursor, never fresh-line
// requests. The alternate buffer ignores every OSC 133 action entirely.
func TestOShellSemanticPromptScreenAndRightPrompt(t *testing.T) {
	for _, chunk := range []int{1, 7, 4096} {
		for _, action := range []string{"A", "N"} {
			t.Run(fmt.Sprintf("right/%s/%d", action, chunk), func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				term.Write([]byte("\x1b[2;5H"))
				writeControlChunks(term, []byte("\x1b]133;"+action+";k=r;cl=m\a"), chunk)
				layoutCursor(t, term, "", 4, 1)
			})
		}
		for _, action := range []string{"A", "N", "L", "P", "B", "I", "C", "D"} {
			t.Run(fmt.Sprintf("alternate/%s/%d", action, chunk), func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				term.Write([]byte("\x1b[?1049h\x1b[2;5H"))
				writeControlChunks(term, []byte("\x1b]133;"+action+"\a"), chunk)
				layoutCursor(t, term, "", 4, 1)
			})
		}
	}
}

func TestOShellSemanticFreshLineLeftOfMargin(t *testing.T) {
	for _, action := range []string{"A", "N", "L"} {
		for _, column := range []int{1, 2, 3, 5} {
			t.Run(fmt.Sprintf("%s/column%d", action, column), func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				term.Write([]byte(fmt.Sprintf("\x1b[?69h\x1b[3;9s\x1b[2;%dH\x1b]133;%s\a", column, action)))
				x, y := 0, 2
				if column >= 3 {
					x = 2
				}
				if column == 3 {
					y = 1
				}
				layoutCursor(t, term, "", x, y)
			})
		}
	}
}

func TestOShellSemanticFreshLineNewlineMode(t *testing.T) {
	for _, action := range []string{"A", "N", "L"} {
		for _, mode := range []string{"h", "l"} {
			t.Run(action+"/LNM-"+mode, func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				term.Write([]byte("\x1b[?69h\x1b[3;9s\x1b[20" + mode + "\x1b[2;5H\x1b]133;" + action + "\a"))
				x := 2
				if mode == "h" {
					x = 0
				}
				layoutCursor(t, term, "", x, 2)
			})
		}
	}
}

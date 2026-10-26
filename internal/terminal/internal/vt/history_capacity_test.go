package vt

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"
)

func TestOShellOutputHistoryKeepsExactCapacity(t *testing.T) {
	for _, limit := range []int{0, 1, 500} {
		for _, chunk := range []int{1, 4096} {
			t.Run(fmt.Sprintf("%d/%d", limit, chunk), func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				if err := term.SetHistoryLines(limit); err != nil {
					t.Fatal(err)
				}
				var text strings.Builder
				for i := 0; i < 620; i++ {
					fmt.Fprintf(&text, "%03d\r\n", i)
				}
				writeControlChunks(term, []byte(text.String()), chunk)
				if got := int(term.Scrollbar().Total - term.Scrollbar().Len); got != limit {
					t.Fatal("output retained wrong history count", got, limit)
				}
				term.ScrollToTop()
				row, _ := term.RowText(0)
				if want := fmt.Sprintf("%03d", 616-limit); strings.TrimSpace(string(row)) != want {
					t.Fatal("wrong first row", string(row), want)
				}
			})
		}
	}
}

func TestOShellOutputHistoryViewportAndSavedCursor(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.SetHistoryLines(4)
	for i := 0; i < 8; i++ {
		term.Write([]byte(fmt.Sprintf("%03d\r\n", i)))
	}
	term.Write([]byte("\x1b7"))
	term.ScrollToRow(2)
	term.Write([]byte("008\r\n"))
	if bar := term.Scrollbar(); bar.Offset != 1 || bar.Total != 9 {
		t.Fatal("scrolled viewport did not follow retained text", bar)
	}
	row, _ := term.RowText(0)
	if strings.TrimSpace(string(row)) != "002" {
		t.Fatal("viewport text jumped", string(row))
	}
	term.Write([]byte("009\r\n010\r\n"))
	if bar := term.Scrollbar(); bar.Offset != 0 || bar.Total != 9 {
		t.Fatal("oldest viewport did not clamp", bar)
	}
	term.ScrollToBottom()
	term.Write([]byte("\x1b8"))
	if x, y := term.Cursor(); x != 0 || y != 4 {
		t.Fatal("natural history recycling adjusted saved row", x, y)
	}
}

func TestOShellOutputHistoryWrapAndPartialRegion(t *testing.T) {
	for _, suffix := range []string{"abcdefghijklmnopq", "\x1b[1;3r\x1b[3;1H" + strings.Repeat("x\r\n", 12)} {
		term, _ := resetTerminal(t, Effects{})
		rows := 3
		if strings.HasPrefix(suffix, "\x1b") {
			rows = 5
		}
		term.Resize(4, rows, 8, 16)
		term.SetHistoryLines(1)
		term.Write([]byte(suffix))
		if bar := term.Scrollbar(); int(bar.Total) != rows+1 {
			t.Fatal("wrapped or top-aligned region exceeded history", bar)
		}
	}
}

func TestOShellOutputHistoryReleasesOnlyDiscardedImage(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.SetHistoryLines(1)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=1,p=1,c=1,r=1,C=1", []byte{255, 0, 0}))
	term.Write(kittyCommand("a=t,f=24,s=1,v=1,i=1", []byte{0, 0, 255}))
	g := term.Graphics(8, 16)
	if len(g) != 1 {
		t.Fatal("missing image fixture")
	}
	oldID := g[0].ImageID
	term.Write([]byte(strings.Repeat("\r\n", 5)))
	term.ScrollToTop()
	if len(term.Graphics(8, 16)) != 1 {
		t.Fatal("last history image released too early")
	}
	term.Write([]byte("\r\n"))
	if len(term.Graphics(8, 16)) != 0 {
		t.Fatal("discarded history image remained")
	}
	var storage uintptr
	if err := term.get(30, unsafe.Pointer(&storage)); err != nil {
		t.Fatal(err)
	}
	if call(fnGraphicsImage, storage, uintptr(oldID)) != 0 {
		t.Fatal("discarded retained pixels were not released")
	}
	term.ScrollToBottom()
	term.Write(kittyCommand("a=p,i=1,p=2,c=1,r=1,C=1", nil))
	if g := term.Graphics(8, 16); len(g) != 1 || g[0].Image.Pix[2] != 255 {
		t.Fatal("history recycle lost current cache")
	}
}

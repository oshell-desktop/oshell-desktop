package vt

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Paragraph boundaries affect copying, reflow, and BiDi resolution even when
// the initial pixels look identical. Compare the logical rows, then continue
// writing and resize both terminals instead of only comparing visible text.
func TestOShellSnapshotSoftWrap(t *testing.T) {
	for _, tc := range []struct{ name, text string }{
		{"ascii", "abcdefghijklmnopqr\r\nhard\r\nlast"},
		{"rtl_context", "A.....אב...Z\r\nnew\r\nlast"},
		{"wide_boundary", "abcde界e\u0301אבxyz\r\nnext"},
		{"spaces", "abc         next\r\nlast"},
		{"erased_row", "abcdefghijklmnopqr\x1b[2;1H\x1b[2K\x1b[3;1H"},
		{"history", strings.Repeat("A.....אב...Z\r\n", 15) + "tail"},
		{"pending_wrap", "abcdefghijkl"},
		{"wrap_disabled", "abcdefghijklmnopqr\r\nlast\x1b[?7l"},
		{"reverse_wrap_without_wrap", "abcdefghijklmnopqr\r\nlast\x1b[?45h\x1b[?7l"},
		{"styled_continuation", "\x1b[31mabcdef\x1b[34mghijkl\x1b[0mmnop\r\nend"},
		{"line_mode_continuation", "abcdef\x1b#6ghijkl\x1b#3mnopqr\r\nend"},
		{"erased_line_mode", "abcdefghijklmnopqr\x1b[2;1H\x1b#4\x1b[2K\x1b[3;1H"},
		{"wide_pending_wrap", "abcd界"},
		{"history_pages", strings.Repeat("A.....אב...Z\r\n", 1100) + "tail"},
	} {
		for _, chunk := range []int{1, 7, 4096} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, chunk), func(t *testing.T) {
				a, _ := bidiTerminal(t, Effects{})
				b, _ := bidiTerminal(t, Effects{})
				for _, term := range []*Terminal{a, b} {
					if err := term.Resize(6, 4, 8, 16); err != nil {
						t.Fatal(err)
					}
				}
				a.Write([]byte(tc.text))
				snapshot := a.VT()
				writeControlChunks(b, snapshot, chunk)
				compare := func(stage string) {
					t.Helper()
					ar, aw := a.SearchRows(0)
					br, bw := b.SearchRows(0)
					if !reflect.DeepEqual(ar, br) || !reflect.DeepEqual(aw, bw) {
						t.Fatalf("%s rows %q/%v want %q/%v; snapshot %q", stage, br, bw, ar, aw, snapshot)
					}
					if a.FormattedText(true) != b.FormattedText(true) {
						t.Fatalf("%s logical text differs", stage)
					}
					ap := a.BidiRows(0, int(a.Scrollbar().Total))
					bp := b.BidiRows(0, int(b.Scrollbar().Total))
					if !reflect.DeepEqual(ap, bp) {
						t.Fatalf("%s paragraph layout differs", stage)
					}
					for row := range int(a.Scrollbar().Total) {
						ar, aok := a.screenCell(0, row)
						br, bok := b.screenCell(0, row)
						if lineModeRef(ar, aok) != lineModeRef(br, bok) {
							t.Fatalf("%s line mode at %d differs", stage, row)
						}
					}
					if a.Mode(7) != b.Mode(7) || a.Mode(45) != b.Mode(45) {
						t.Fatalf("%s wrap modes differ", stage)
					}
					ax, ay := a.Cursor()
					bx, by := b.Cursor()
					if ax != bx || ay != by {
						t.Fatalf("%s cursor %d,%d want %d,%d", stage, bx, by, ax, ay)
					}
				}
				compare("replay")
				for _, term := range []*Terminal{a, b} {
					term.Write([]byte("XYZ"))
				}
				compare("continued output")
				for _, width := range []int{4, 12, 6} {
					for _, term := range []*Terminal{a, b} {
						if err := term.Resize(width, 4, 8, 16); err != nil {
							t.Fatal(err)
						}
					}
					compare(fmt.Sprint("resize ", width))
				}
			})
		}
	}
}

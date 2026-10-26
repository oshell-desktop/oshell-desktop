package terminal

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestImplicitLinkWrappedRows(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		row, col    int
		want        string
	}{
		{"soft-first", "https://example.test/path", 0, 2, "https://example.test/path"},
		{"soft-middle", "https://example.test/path", 1, 2, "https://example.test/path"},
		{"soft-last", "https://example.test/path", 1, 11, "https://example.test/path"},
		{"indented-soft", "https://exam  ple.test/a", 1, 3, "https://example.test/a"},
		{"indent-not-hit", "https://exam  ple.test/a", 1, 0, ""},
		{"editor-wrap", "https://exam\r\n  ple.test/a", 1, 3, "https://example.test/a"},
		{"short-line-break", "https://\r\nexample.test/a", 0, 2, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, _, _ := bidiViewFixture(t)
			term.Resize(12, 6)
			term.Feed([]byte(tc.input))
			term.mu.Lock()
			got := term.v.linkAt(tc.col, tc.row)
			term.mu.Unlock()
			if got != tc.want {
				t.Fatalf("link=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestImplicitLinkVisualClick(t *testing.T) {
	for _, tc := range []struct {
		text          string
		logical, tail int
		want          string
	}{
		{"אב https://example.test/", 5, 0, "https://example.test/"},
		{"https://界.test/", 8, 0, "https://界.test/"},
		{"https://界.test/", 8, 1, "https://界.test/"},
		{"https://e\u0301.test/", 8, 0, "https://e\u0301.test/"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			term, tt, _ := bidiViewFixture(t)
			term.Feed([]byte(tc.text))
			tt.Frame()
			x, y := cellCenter(term, term.v.visualColumn(0, tc.logical)+tc.tail, 0)
			tt.ClickAtWith(ui.Cmd, x, y)
			if got := tt.OpenedURLs(); len(got) != 1 || got[0] != tc.want {
				t.Fatalf("visual click opened %q", got)
			}
		})
	}
}

func TestImplicitLinkWideCombiningAndPrintedSpaces(t *testing.T) {
	for _, tc := range []struct {
		input    string
		row, col int
		want     string
	}{
		{"https://界/", 0, 8, "https://界/"},
		{"https://界/", 0, 9, "https://界/"},
		{"https://界/", 0, 11, ""},
		{"https://e\u0301/", 0, 8, "https://e\u0301/"},
		{"https://界/路径", 1, 1, "https://界/路径"},
		{"./file.txt  ", 0, 11, "./file.txt  "},
		{"./file.txt", 0, 11, ""},
		{"https://x/a\r\n  /b", 1, 2, "https://x/a/b"},
		{"https://x/a \r\n  /b", 1, 2, "/b"},
		{"https://x/a\r\n\r\n/b", 2, 0, "/b"},
		{"\x1b]8;;https://explicit.test\x1b\\https://x/a\x1b]8;;\x1b\\", 0, 8, "https://explicit.test"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			term, _, _ := bidiViewFixture(t)
			term.Resize(12, 6)
			term.Feed([]byte(tc.input))
			term.mu.Lock()
			got := term.v.linkAt(tc.col, tc.row)
			term.mu.Unlock()
			if got != tc.want {
				t.Fatalf("link %q want %q", got, tc.want)
			}
		})
	}
}

func TestImplicitLinkHistoryReadAndReflow(t *testing.T) {
	term, _, _ := bidiViewFixture(t)
	term.Resize(20, 3)
	url := "https://example.test/" + strings.Repeat("x", 35)
	term.Feed([]byte(url + "\r\nend\r\n"))
	term.mu.Lock()
	defer term.mu.Unlock()
	before := term.term.Scrollbar()
	if before.Offset < 1 {
		t.Fatalf("fixture has no history: %+v", before)
	}
	if got := term.v.linkAt(1, 0); got != url {
		t.Fatalf("offscreen prefix: %q", got)
	}
	if after := term.term.Scrollbar(); after != before {
		t.Fatalf("read moved viewport: %+v -> %+v", before, after)
	}
	term.term.ScrollToRow(0)
	before = term.term.Scrollbar()
	if got := term.v.linkAt(1, 0); got != url {
		t.Fatalf("scrolled link: %q", got)
	}
	if after := term.term.Scrollbar(); after != before {
		t.Fatal("read moved scrolled viewport")
	}
	// A resize rebuilds native rows. No cache from the previous lookup survives.
	if err := term.term.Resize(16, 6, 0, 0); err != nil {
		t.Fatal(err)
	}
	term.term.ScrollToTop()
	if got := term.v.linkAt(1, 0); got != url {
		t.Fatalf("reflow link: %q", got)
	}
}

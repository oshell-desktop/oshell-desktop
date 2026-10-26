package terminal

import (
	"fmt"
	"strings"
	"testing"
)

func TestSearchNavigationBeyondSummaryLimit(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	if err := term.SetHistoryLimit(10000); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	for i := 0; i < 6005; i++ {
		fmt.Fprintf(&output, "hit %04d\r\n", i)
	}
	term.Feed([]byte(output.String()))
	page, err := term.NavigateSearch("hit", false, true, false, -1, true)
	if err != nil || len(page.Matches) != 1001 || page.Current == nil || page.Current.Text != "hit 6004" || page.Index != -1 {
		t.Fatalf("last result lost beyond count cap: %+v %v", page.Current, err)
	}
	for _, step := range []struct {
		direction int
		text      string
	}{{-1, "hit 6003"}, {1, "hit 6004"}, {1, "hit 0000"}, {-1, "hit 6004"}} {
		if err := term.Reveal(*page.Current); err != nil {
			t.Fatal(err)
		}
		page, err = term.NavigateSearch("hit", false, true, false, step.direction, false)
		if err != nil || page.Current == nil || page.Current.Text != step.text {
			t.Fatalf("direction %d: %+v %v", step.direction, page.Current, err)
		}
	}
}

func TestSearchNavigationUsesReflowedAndTrimmedSelection(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	if err := term.SetHistoryLimit(500); err != nil {
		t.Fatal(err)
	}
	term.mu.Lock()
	term.resize(gridSize{cols: 30, rows: 5, cellW: 8, cellH: 16})
	term.mu.Unlock()
	term.Feed([]byte(strings.Repeat("filler\r\n", 495) + "0123456789 hit one\r\n0123456789 hit two\r\n0123456789 hit three\r\n"))
	page, err := term.NavigateSearch("hit", false, true, false, 1, true)
	if err != nil || page.Current == nil {
		t.Fatal(err)
	}
	if err := term.Reveal(*page.Current); err != nil {
		t.Fatal(err)
	}
	term.mu.Lock()
	term.resize(gridSize{cols: 12, rows: 5, cellW: 8, cellH: 16})
	term.mu.Unlock()
	// Additional output trims old history while the selected hit is retained.
	term.Feed([]byte(strings.Repeat("new\r\n", 30)))
	if got := term.SearchSelection(); got != "hit" {
		t.Fatalf("fixture selection lost on reflow: %q", got)
	}
	page, err = term.NavigateSearch("hit", false, true, false, 1, false)
	if err != nil || page.Current == nil || page.Current.Text != "0123456789 hit two" {
		t.Fatalf("stale row anchor: %+v %v", page.Current, err)
	}
	if err := term.Reveal(*page.Current); err != nil {
		t.Fatal(err)
	}
	term.Feed([]byte(strings.Repeat("replacement\r\n", 550) + "hit new\r\n"))
	page, err = term.NavigateSearch("hit", false, true, false, 1, false)
	if err != nil || page.Current == nil || page.Current.Text != "hit new" {
		t.Fatalf("trimmed anchor reused: %+v %v", page.Current, err)
	}
}

func TestSearchLogicalLinesAndUnicodeBoundaries(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.mu.Lock()
	term.term.Resize(12, 5, 8, 16)
	term.mu.Unlock()
	term.Feed([]byte("1234567890中文e\u0301🙂tail\r\nword sword word_ word\r\n\r\nlast"))
	for _, tc := range []struct {
		q     string
		whole bool
		count int
	}{
		{"中文e\u0301🙂tail", false, 1}, {"90中文", false, 1}, {"word", true, 2}, {"word", false, 4}, {"\u0301", true, 0},
	} {
		matches, err := term.Search(tc.q, false, true, tc.whole)
		if err != nil || len(matches) != tc.count {
			t.Fatalf("%q count=%d err=%v rows=%q", tc.q, len(matches), err, term.Text())
		}
		if tc.count == 0 {
			continue
		}
		if err := term.Reveal(matches[0]); err != nil {
			t.Fatal(err)
		}
		if got := term.SearchSelection(); got != tc.q {
			t.Fatalf("%q selected %q; %+v", tc.q, got, matches[0])
		}
	}
	term.ClearSearchSelection()
	if term.SearchSelection() != "" {
		t.Fatal("selection retained")
	}
}

func TestSearchPreservesWrappedSpacesAndHardLineBoundaries(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.mu.Lock()
	term.term.Resize(8, 4, 8, 16)
	term.mu.Unlock()
	term.Feed([]byte("abc     def\r\nxyz\r\nend"))
	for _, tc := range []struct {
		q     string
		count int
	}{{"c     de", 1}, {"defxyz", 0}, {"^abc     def$", 1}, {"^$", 0}, {"end", 1}} {
		matches, err := term.Search(tc.q, true, true)
		if err != nil || len(matches) != tc.count {
			t.Fatalf("%q count=%d err=%v", tc.q, len(matches), err)
		}
	}
	if _, err := term.Search(strings.Repeat("a", 4097), false, false); err == nil {
		t.Fatal("unbounded query")
	}
}

func TestSearchAcrossErasedWrappedCells(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.mu.Lock()
	term.term.Resize(8, 4, 8, 16)
	term.mu.Unlock()
	for _, erase := range []string{"\x1b[1;1H\x1b[2K", "\x1b[1;4H\x1b[K"} {
		term.Feed([]byte("\x1bc\x1b[Habcdefghijklmnop" + erase))
		query := strings.Repeat(" ", 8) + "ij"
		if strings.Contains(erase, "1;4H") {
			query = "abc" + strings.Repeat(" ", 5) + "ij"
		}
		matches, err := term.Search(query, false, true)
		if err != nil || len(matches) != 1 {
			t.Fatalf("erased cells lost from search: %q matches=%v err=%v", query, matches, err)
		}
		if err := term.Reveal(matches[0]); err != nil {
			t.Fatal(err)
		}
		if got := term.SearchSelection(); got != query {
			t.Fatalf("search selected %q want %q", got, query)
		}
	}
}

func TestSearchUnicodeFoldingKeepsOriginalSelection(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.Feed([]byte("Straße café 👨‍👩‍👧‍👦"))
	for _, tc := range []struct {
		query, selection        string
		regex, sensitive, whole bool
	}{
		{"STRASSE", "Straße", false, false, true},
		{"cafe\u0301", "café", false, true, true},
		{"ß", "ß", false, true, false},
		{"👩", "👨‍👩‍👧‍👦", true, true, false},
	} {
		matches, err := term.Search(tc.query, tc.regex, tc.sensitive, tc.whole)
		if err != nil || len(matches) != 1 {
			t.Fatalf("%q: %v %v", tc.query, matches, err)
		}
		if err := term.Reveal(matches[0]); err != nil {
			t.Fatal(err)
		}
		if got := term.SearchSelection(); got != tc.selection {
			t.Fatalf("%q: selected %q", tc.query, got)
		}
	}
	for _, query := range []string{"STRASSE", "👩"} {
		matches, err := term.Search(query, query == "👩", true, true)
		if err != nil || len(matches) != 0 {
			t.Fatal("partial grapheme or case-sensitive false match", query, matches, err)
		}
	}
}

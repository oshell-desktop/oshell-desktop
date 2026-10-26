package terminal

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"oshell-desktop/internal/core"
)

type Match struct {
	Row, Start, End int
	EndRow          int
	Text            string
	Revision        uint64
}

func wordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_'
}

func (t *Terminal) Search(query string, regex, caseSensitive bool, wholeWord ...bool) ([]Match, error) {
	page, err := t.NavigateSearch(query, regex, caseSensitive, len(wholeWord) > 0 && wholeWord[0], 0, true)
	return page.Matches, err
}

type SearchPage struct {
	Matches  []Match // At most 1001 entries, solely for the count summary.
	Current  *Match
	Index    int // -1 when the selected match is outside the summary or absent.
	Revision uint64
}

// NavigateSearch scans beyond the count summary without retaining every match.
// Its anchor comes from the live selection, including scrollback trim/reflow.
func (t *Terminal) NavigateSearch(query string, regex, sensitive, whole bool, direction int, reset bool) (SearchPage, error) {
	page := SearchPage{Index: -1}
	var anchor *Match
	var first, last, chosen Match
	var haveFirst, haveChosen bool
	before := func(ar, ac, br, bc int) bool { return ar < br || ar == br && ac < bc }
	err := t.search(query, regex, sensitive, whole, func(current *Match, revision uint64) {
		page.Revision = revision
		if !reset {
			anchor = current
		}
	}, func(m Match) bool {
		if len(page.Matches) < 1001 {
			page.Matches = append(page.Matches, m)
		}
		if !haveFirst {
			first, haveFirst = m, true
		}
		last = m
		if anchor != nil {
			switch {
			case direction == 0 && m.Row == anchor.Row && m.Start == anchor.Start && m.EndRow == anchor.EndRow && m.End == anchor.End:
				chosen, haveChosen = m, true
			case direction > 0 && !haveChosen && !before(m.Row, m.Start, anchor.EndRow, anchor.End):
				chosen, haveChosen = m, true
			case direction < 0 && !before(anchor.Row, anchor.Start, m.EndRow, m.End):
				chosen, haveChosen = m, true
			}
		}
		if len(page.Matches) == 1001 {
			if direction >= 0 && (haveChosen || anchor == nil) {
				return false
			}
			if direction < 0 && haveChosen && !before(m.Row, m.Start, anchor.Row, anchor.Start) {
				return false
			}
		}
		return true
	})
	if err != nil {
		return SearchPage{Index: -1, Revision: page.Revision}, err
	}
	if !haveChosen && haveFirst && (reset || direction != 0) {
		chosen = first
		if direction < 0 {
			chosen = last
		}
		haveChosen = true
	}
	if haveChosen {
		page.Current = &chosen
		for i, m := range page.Matches {
			if m.Row == chosen.Row && m.Start == chosen.Start {
				page.Index = i
				break
			}
		}
	}
	return page, nil
}

func (t *Terminal) search(query string, regex, caseSensitive, wholeWord bool, snapshot func(*Match, uint64), visit func(Match) bool) error {
	if query == "" {
		return nil
	}
	if len(query) > 4096 {
		return fmt.Errorf("搜索内容不能超过 4 KB")
	}
	var compiled *core.Pattern
	if regex {
		pattern := query
		if !caseSensitive {
			pattern = "(?i)" + pattern
		}
		var err error
		compiled, err = core.CompilePattern(pattern, 10*time.Millisecond)
		if err != nil {
			return err
		}
		defer compiled.Close()
	}
	t.mu.Lock()
	if t.term == nil {
		t.mu.Unlock()
		return nil
	}
	first := t.term.HistoryStart(t.historyLines)
	rows, wraps := t.term.SearchRows(first)
	var widths []int
	if regex {
		widths = t.term.SearchRowWidths(first, rows, wraps)
	}
	revision := t.revision
	var anchor *Match
	if row, start, endRow, end, found := t.term.SearchSelectionRange(); found && row >= first {
		anchor = &Match{Row: row, Start: start, EndRow: endRow, End: end, Revision: revision}
	}
	t.mu.Unlock()
	snapshot(anchor, revision)
	deadline := time.Now().Add(time.Second)
	for row := 0; row < len(rows); {
		startRow := row
		var line strings.Builder
		var lengths []int
		cells := 0
		for {
			if regex {
				cells += widths[row]
			}
			line.WriteString(rows[row])
			lengths = append(lengths, utf8.RuneCountInString(rows[row]))
			row++
			if row == len(rows) || !wraps[row-1] {
				break
			}
		}
		text := line.String()
		if regex && cells > 65536 {
			return fmt.Errorf("正则搜索的单行过长，请使用普通文本搜索")
		}
		var indices [][]int
		var boundaries []int
		if regex {
			var err error
			indices, err = compiled.FindAllStringIndex(text, 100001)
			if err != nil {
				return err
			}
			boundaries = searchGraphemeBoundaries(text)
		} else {
			indices = literalSearchIndices(text, query, caseSensitive, 100001)
		}
		if len(indices) > 100000 {
			return fmt.Errorf("匹配候选过多，统计受限；请缩小搜索范围")
		}
		offsets := make([]int, len(lengths)+1)
		for i, n := range lengths {
			offsets[i+1] = offsets[i] + n
		}
		previousByte, previousRune := 0, 0
		for _, m := range indices {
			if m[0] == m[1] {
				continue
			}
			if regex {
				left, right := sort.SearchInts(boundaries, m[0]), sort.SearchInts(boundaries, m[1])
				if boundaries[left] != m[0] {
					left--
				}
				if wholeWord && (boundaries[left] != m[0] || boundaries[right] != m[1]) {
					continue
				}
				m[0], m[1] = boundaries[left], boundaries[right]
				if m[0] < previousByte {
					continue
				}
			}
			if wholeWord {
				before, _ := utf8.DecodeLastRuneInString(text[:m[0]])
				after, _ := utf8.DecodeRuneInString(text[m[1]:])
				if wordRune(before) || wordRune(after) {
					continue
				}
			}
			start := previousRune + utf8.RuneCountInString(text[previousByte:m[0]])
			end := start + utf8.RuneCountInString(text[m[0]:m[1]])
			previousByte, previousRune = m[1], end
			i := sort.Search(len(lengths), func(i int) bool { return offsets[i+1] > start })
			j := sort.Search(len(lengths), func(i int) bool { return offsets[i+1] >= end })
			r1, r2 := startRow+i, startRow+j
			start -= offsets[i]
			end -= offsets[j]
			if !visit(Match{Row: first + r1, Start: start, End: end, EndRow: first + r2, Text: text, Revision: revision}) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("搜索耗时过长，请缩小搜索范围或简化表达式")
		}
	}
	return nil
}

func (t *Terminal) SearchRevision() uint64 { t.mu.Lock(); defer t.mu.Unlock(); return t.revision }

func (t *Terminal) SearchSelection() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return ""
	}
	text, _ := t.term.SelectionText()
	return text
}

func (t *Terminal) ClearSearchSelection() {
	t.mu.Lock()
	if t.term != nil {
		t.term.SetSelection(nil)
	}
	t.mu.Unlock()
	t.redraw()
}

func (t *Terminal) Reveal(match Match) error {
	t.mu.Lock()
	err := func() error {
		if t.term == nil {
			return fmt.Errorf("终端已关闭")
		}
		if match.Revision != t.revision {
			return fmt.Errorf("终端输出已变化，请刷新匹配")
		}
		if !t.term.SelectSearchRange(match.Row, match.Start, match.EndRow, match.End) {
			return fmt.Errorf("匹配位置已失效")
		}
		return nil
	}()
	t.mu.Unlock()
	if err == nil {
		t.redraw()
	}
	return err
}

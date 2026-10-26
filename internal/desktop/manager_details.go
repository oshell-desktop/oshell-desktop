package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"reflect"
	"strings"
	"unicode"
)

type managerRowsCache struct {
	source                        core.Config
	group, query, protocol, scope string
	all                           bool
	rows                          []sessionRow
}

func (a *App) managerRows() []sessionRow {
	source := core.Config{Profiles: a.config.Profiles, Directories: a.config.Directories, Links: a.config.Links}
	old := a.managerCache
	if old != nil && old.group == a.managerGroup && old.query == a.query && old.protocol == a.managerProtocol && old.scope == a.managerSearchScope && old.all == a.managerAll && reflect.DeepEqual(old.source, source) {
		return old.rows
	}
	rows := a.buildManagerRows()
	a.managerCache = &managerRowsCache{source: source.Clone(), group: a.managerGroup, query: a.query, protocol: a.managerProtocol, scope: a.managerSearchScope, all: a.managerAll, rows: rows}
	return rows
}

// Consume the entire parent click sequence. Its second click arrives over a
// different row after navigation and must never open that row's connection.
func (a *App) managerCellInput(row sessionRow, ev ui.InputEvent) bool {
	if ev.Kind == ui.InputPointerCancel {
		a.managerParentClick = false
		return true
	}
	if ev.Button != 0 {
		return false
	}
	if ev.Kind == ui.InputPointerUp && a.managerParentClick {
		return true
	}
	if ev.Kind != ui.InputPointerDown {
		return false
	}
	if ev.Clicks > 1 && a.managerParentClick {
		return true
	}
	a.managerParentClick = false
	if row.parent && ev.Mods&(ui.Super|ui.Ctrl|ui.Shift) == 0 {
		a.managerParentClick = true
		a.navigateManager(row.group)
		return true
	}
	return false
}

// Match folded text while retaining the original spelling and combining marks.
// Byte origins also cover folds that expand one character (for example ß → ss).
func managerSearchSpans(text string, query core.SessionQuery) []ui.Span {
	if len(query.Terms) == 0 || !query.Valid {
		return []ui.Span{{Text: text}}
	}
	type origin struct{ start, end int }
	starts := []int{}
	for i, r := range text {
		if len(starts) == 0 || !unicode.Is(unicode.Mn, r) && !unicode.Is(unicode.Mc, r) {
			starts = append(starts, i)
		}
	}
	starts = append(starts, len(text))
	var folded strings.Builder
	var origins []origin
	for i := 0; i+1 < len(starts); i++ {
		part := core.NormalizeSessionSearch(text[starts[i]:starts[i+1]])
		folded.WriteString(part)
		for range len(part) {
			origins = append(origins, origin{starts[i], starts[i+1]})
		}
	}
	normalized := folded.String()
	marked := make([]bool, len(text))
	for _, term := range query.Terms {
		if term == "" {
			continue
		}
		for offset := 0; offset < len(normalized); {
			index := strings.Index(normalized[offset:], term)
			if index < 0 {
				break
			}
			start := offset + index
			end := start + len(term)
			for j := origins[start].start; j < origins[end-1].end; j++ {
				marked[j] = true
			}
			offset = end
		}
	}
	spans := []ui.Span{}
	for start := 0; start < len(text); {
		end := start + 1
		for end < len(text) && marked[end] == marked[start] {
			end++
		}
		weight := 0
		if marked[start] {
			weight = 600
		}
		spans = append(spans, ui.Span{Text: text[start:end], Weight: weight})
		start = end
	}
	return spans
}

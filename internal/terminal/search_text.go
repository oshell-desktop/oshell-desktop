package terminal

import (
	"strings"
	"unicode/utf8"

	"github.com/go-text/typesetting/segmenter"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// Keep byte boundaries in the original text: case folding can expand a single
// grapheme (ß -> ss), and canonical normalization can change byte lengths.
func literalSearchIndices(text, query string, sensitive bool, limit int) [][]int {
	fold := cases.Fold()
	transform := func(s string) string {
		if !sensitive {
			s = fold.String(s)
		}
		return norm.NFD.String(s)
	}
	needle := transform(query)
	if needle == "" {
		return nil
	}
	// The common log-search path needs no grapheme-to-byte mapping.
	if isASCII(text) && isASCII(needle) {
		haystack := text
		if !sensitive {
			haystack = strings.ToLower(text)
		}
		var found [][]int
		for start := 0; start < len(haystack) && len(found) < limit; {
			at := strings.Index(haystack[start:], needle)
			if at < 0 {
				break
			}
			at += start
			start = at + len(needle)
			found = append(found, []int{at, start})
		}
		return found
	}
	var segment segmenter.Segmenter
	segment.Init([]rune(text))
	it := segment.GraphemeIterator()
	var normalized strings.Builder
	boundaries := map[int]int{0: 0}
	original := 0
	for it.Next() {
		cluster := string(it.Grapheme().Text)
		normalized.WriteString(transform(cluster))
		original += len(cluster)
		boundaries[normalized.Len()] = original
	}
	haystack := normalized.String()
	var found [][]int
	for start := 0; start < len(haystack) && len(found) < limit; {
		at := strings.Index(haystack[start:], needle)
		if at < 0 {
			break
		}
		at += start
		end := at + len(needle)
		left, leftOK := boundaries[at]
		right, rightOK := boundaries[end]
		if leftOK && rightOK {
			found = append(found, []int{left, right})
			start = end
		} else {
			_, size := utf8.DecodeRuneInString(haystack[at:])
			start = at + size
		}
	}
	return found
}

func isASCII(text string) bool {
	for i := range text {
		if text[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func searchGraphemeBoundaries(text string) []int {
	var segment segmenter.Segmenter
	segment.Init([]rune(text))
	it := segment.GraphemeIterator()
	boundaries := []int{0}
	for it.Next() {
		boundaries = append(boundaries, boundaries[len(boundaries)-1]+len(string(it.Grapheme().Text)))
	}
	return boundaries
}

package bidilayout

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type joiningKind uint8

const (
	joiningNone joiningKind = iota
	joiningTransparent
	joiningCausing
	joiningRight
	joiningLeft
	joiningDual
)

func joining(r rune) joiningKind {
	if r == 0x200c {
		return joiningNone
	}
	i := sort.Search(len(joiningRanges), func(i int) bool { return joiningRanges[i].hi >= r })
	if i < len(joiningRanges) && joiningRanges[i].lo <= r {
		return joiningRanges[i].kind
	}
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) {
		return joiningTransparent
	}
	return joiningNone
}

// Shape across the paragraph, independent of cell colors and soft wraps.
// Mandatory lam/alef keeps two logical cells: the lam displays the ligature,
// the alef becomes a blank, and marks from both cells follow the ligature.
func shapeArabic(cells []Cell) []string {
	result := make([]string, len(cells))
	base := func(i int) rune { r, _ := utf8.DecodeRuneInString(cells[i].Text); return r }
	neighbor := func(i, step int) joiningKind {
		for j := i + step; j >= 0 && j < len(cells); j += step {
			if k := joining(base(j)); k != joiningTransparent {
				return k
			}
		}
		return joiningNone
	}
	withMarks := func(r rune, from ...Cell) string {
		var s strings.Builder
		s.WriteRune(r)
		for _, c := range from {
			_, n := utf8.DecodeRuneInString(c.Text)
			s.WriteString(c.Text[n:])
		}
		return s.String()
	}
	for i := 0; i < len(cells); i++ {
		r, kind := base(i), joining(base(i))
		form, hasForm := arabicForms[r]
		if kind != joiningDual && kind != joiningRight {
			if kind == joiningNone && hasForm {
				result[i] = withMarks(form.iso, cells[i])
			}
			continue
		}
		prev := neighbor(i, -1)
		joinedBefore := prev == joiningDual || prev == joiningLeft || prev == joiningCausing
		if r == 0x644 && i+1 < len(cells) {
			var ligature rune
			switch base(i + 1) {
			case 0x622:
				ligature = 0xfef5
			case 0x623:
				ligature = 0xfef7
			case 0x625:
				ligature = 0xfef9
			case 0x627:
				ligature = 0xfefb
			}
			if ligature != 0 {
				if joinedBefore {
					ligature++
				}
				result[i] = withMarks(ligature, cells[i], cells[i+1])
				result[i+1] = " "
				i++
				continue
			}
		}
		if !hasForm {
			continue
		}
		next := neighbor(i, 1)
		joinedAfter := kind == joiningDual && (next == joiningDual || next == joiningRight || next == joiningCausing)
		shaped := form.iso
		if kind == joiningRight {
			if joinedBefore {
				shaped = form.fin
			}
		} else if joinedBefore && joinedAfter && form.med != 0 {
			shaped = form.med
		} else if joinedBefore {
			shaped = form.fin
		} else if joinedAfter && form.ini != 0 {
			shaped = form.ini
		}
		result[i] = withMarks(shaped, cells[i])
	}
	return result
}

// Isolate asks the renderer to shape this grapheme alone at its visual column,
// preventing a font's mandatory ligatures from merging terminal cells again.
func Isolate(text string) bool {
	r, n := utf8.DecodeRuneInString(text)
	return n < len(text) || r >= 0x600 && r <= 0x6ff || r >= 0x750 && r <= 0x77f ||
		r >= 0x870 && r <= 0x8ff || r >= 0xfb50 && r <= 0xfdff || r >= 0xfe70 && r <= 0xfefc
}

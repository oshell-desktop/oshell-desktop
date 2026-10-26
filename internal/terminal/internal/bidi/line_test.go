package bidi

import (
	"bufio"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestParagraphContextAcrossSoftWrap(t *testing.T) {
	var p Paragraph
	// The neutral at row 2's start belongs between R runs. Resolving row 2
	// alone would incorrectly treat it as a leading LTR neutral.
	if n := p.Resolve([]rune("אב-גד xyz"), LeftToRight); n != 9 {
		t.Fatal(n)
	}
	row, ok := p.Line(2, 9)
	if !ok || !slices.Equal(row.Order, []int{4, 3, 2, 5, 6, 7, 8}) {
		t.Fatal(row, ok)
	}
	// Trailing row whitespace resets to the paragraph level, independently
	// of the next row, without modifying another call's level state.
	p.Resolve([]rune("אב גד"), LeftToRight)
	first, _ := p.Line(0, 3)
	if !slices.Equal(first.Levels, []Level{1, 1, 0}) || !slices.Equal(first.Order, []int{1, 0, 2}) {
		t.Fatal(first)
	}
	whole, _ := p.Line(0, 5)
	if !slices.Equal(whole.Order, []int{4, 3, 2, 1, 0}) {
		t.Fatal(whole)
	}
	if _, ok := p.Line(-1, 2); ok {
		t.Fatal("accepted invalid range")
	}
	p.Resolve([]rune("plain"), LeftToRight)
	if !slices.Equal(first.Levels, []Level{1, 1, 0}) {
		t.Fatal("result borrowed mutable paragraph storage")
	}
}

// The full Unicode 17 character corpus is already shipped by our go-text
// dependency. Run it when OSHELL_BIDI_CHARACTER_TEST names that file; keep a
// representative fixture in the repository for ordinary offline regression.
func TestUnicodeCharacterConformance(t *testing.T) {
	path := os.Getenv("OSHELL_BIDI_CHARACTER_TEST")
	if path == "" {
		path = "testdata/characters.txt"
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var p Paragraph
	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "#", 2)[0])
		if line == "" {
			continue
		}
		fields := strings.Split(line, ";")
		if len(fields) != 5 {
			t.Fatal("invalid fixture row")
		}
		var text []rune
		for _, s := range strings.Fields(fields[0]) {
			v, err := strconv.ParseInt(s, 16, 32)
			if err != nil {
				t.Fatal(err)
			}
			text = append(text, rune(v))
		}
		direction := map[string]Direction{"0": LeftToRight, "1": RightToLeft, "2": Neutral}[strings.TrimSpace(fields[1])]
		if p.Resolve(text, direction) != len(text) {
			t.Fatal("truncated test paragraph", count)
		}
		wantBase, _ := strconv.Atoi(strings.TrimSpace(fields[2]))
		if int(p.BaseLevel()) != wantBase {
			t.Fatalf("case %d base=%d want=%d", count, p.BaseLevel(), wantBase)
		}
		result, _ := p.Line(0, len(text))
		levelStrings := strings.Fields(fields[3])
		if len(levelStrings) != len(text) {
			t.Fatal("invalid fixture levels")
		}
		for i, s := range levelStrings {
			if s == "x" {
				continue
			}
			want, _ := strconv.Atoi(s)
			if int(result.Levels[i]) != want {
				t.Fatalf("case %d index=%d level=%d want=%d text=%x", count, i, result.Levels[i], want, text)
			}
		}
		var got, want []int
		for _, i := range result.Order {
			if levelStrings[i] != "x" {
				got = append(got, i)
			}
		}
		for _, s := range strings.Fields(fields[4]) {
			i, _ := strconv.Atoi(s)
			want = append(want, i)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("case %d order=%v want=%v text=%x", count, got, want, text)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("empty corpus")
	}
	t.Logf("verified %d Unicode character cases", count)
}

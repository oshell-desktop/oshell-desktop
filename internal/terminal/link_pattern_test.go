package terminal

import (
	"slices"
	"strings"
	"testing"
)

func TestImplicitLinkSourcePattern(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"http://x/ https://y/ mailto:a@b ftp://x/ file:/a ssh:x git://x/ tel:123 magnet:?xt=1 ipfs://x/ ipns://x/ gemini://x/ gopher://x/ news:comp.test", []string{"http://x/", "https://y/", "mailto:a@b", "ftp://x/", "file:/a", "ssh:x", "git://x/", "tel:123", "magnet:?xt=1", "ipfs://x/", "ipns://x/", "gemini://x/", "gopher://x/", "news:comp.test"}},
		{"HTTPS://example.test", nil},
		{"https://[::1]:8080/a", []string{"https://[::1]:8080/a"}},
		{"https://x/a_(word)?q=1,...", []string{"https://x/a_(word)?q=1"}},
		{"https://x/a(word)(next)", []string{"https://x/a(word)"}},
		{"https://[::1](word)", []string{"https://[::1]"}},
		{"https://x/a!;:?", []string{"https://x/a!;:?"}},
		{"https://例.test/界e\u0301Ⅳ\u200c", []string{"https://例.test/界e\u0301Ⅳ\u200c"}},
		{"$https://example.test", nil},
		{"$1/dir.txt", nil},
		{"./src/main.go", []string{"./src/main.go"}},
		{"../src/main.go", []string{"../src/main.go"}},
		{"~/Documents/a.txt", []string{"~/Documents/a.txt"}},
		{"$HOME/dir/file", []string{"$HOME/dir/file"}},
		{"src/$ROOT/file.txt", []string{"src/$ROOT/file.txt"}},
		{".config/app/file", []string{".config/app/file"}},
		{"/usr/local/bin", []string{"/usr/local/bin"}},
		{"src/main.go", []string{"src/main.go"}},
		{"src/main", nil},
		{"./some folder/file", []string{"./some folder/file"}},
		{"./file.txt prose", []string{"./file.txt"}},
		{"./file.txt other/", []string{"./file.txt other/"}},
		{"./file.txt   ", []string{"./file.txt   "}},
		{"./file:", []string{"./file"}},
		{"./dir https://example.test", []string{"./dir", "https://example.test"}},
		{"https://[" + strings.Repeat("a", 128), nil},
		{"https://x/a" + strings.Repeat(".", 4096), []string{"https://x/a"}},
	} {
		t.Run(tc.text[:min(len(tc.text), 70)], func(t *testing.T) {
			_, err := implicitLinkPattern().FindAllStringIndex(tc.text, -1)
			if err != nil {
				t.Fatalf("pattern timed out: %v", err)
			}
			var got []string
			for _, m := range implicitLinkRanges(tc.text) {
				got = append(got, tc.text[m[0]:m[1]])
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("matches %q, want %q", got, tc.want)
			}
		})
	}
}

func TestImplicitLinkTimeoutDoesNotExposePartialResults(t *testing.T) {
	// A pathological path candidate follows a valid URL. The bounded engine
	// must discard the earlier match if it cannot finish the whole lookup.
	text := "https://valid.test/ https://[" + strings.Repeat("a", 100000)
	if got := implicitLinkRanges(text); len(got) != 0 {
		t.Fatalf("partial matches: %v", got)
	}
}

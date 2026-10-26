package terminal

import (
	"strings"
	"sync"
	"time"

	"oshell-desktop/internal/core"
)

// This is Terminal.ghosttyImplicitLinkRegex, including its non-nested URL
// tokens and path branches. Spell out ICU's Unicode word class for the Go
// fallback engine; its default \w omits letter numbers and join controls.
var implicitLinkPattern = sync.OnceValue(func() *core.Pattern {
	schemes := `https?://|mailto:|ftp://|file:|ssh:|git://|ssh://|tel:|magnet:|ipfs://|ipns://|gemini://|gopher://|news:`
	ipv6 := `(?:\[[:0-9a-fA-F]+\](?::[0-9]+)?)`
	urlChars := `[{WC}\-.~:/?#@!$&*+,;=%]`
	pathChars := `[{WC}\-.~:\/?#@!$&*+;=%]`
	suffix := `(?:[\(\[]{WORD}*[\)\]])`
	url := `(?:` + schemes + `)(?:` + ipv6 + `|` + urlChars + `(?:` + suffix + `)?)+(?<![,.])`
	dotted := `(?=` + pathChars + `*\.)`
	nondotted := `(?!` + pathChars + `*\.)`
	dottedSpaces := `(?:(?<!:) (?!{WORD}+:\/\/)` + pathChars + `*[\/.])*`
	anySpaces := `(?:(?<!:) (?!{WORD}+:\/\/)` + pathChars + `+)*`
	end := `(?<!:)(?: +(?= *$))?`
	prefix := `(?:\.\.\/|\.\/|(?<!{WORD})~\/|(?:[{WC}][{WC}\-.]*\/)*(?<!{WORD})\$[A-Za-z_]{WORD}*\/|\.[{WC}][{WC}\-.]*\/|(?<![{WC}~\/])\/(?!\/))`
	rooted := prefix + `(?:` + dotted + pathChars + `+` + dottedSpaces + end + `|` + nondotted + pathChars + `+` + anySpaces + end + `)`
	bare := dotted + `(?<!{WORD})[{WC}][{WC}\-.]*\/` + pathChars + `+` + dottedSpaces + end
	word := `\p{L}\p{M}\p{Nl}\p{Nd}\p{Pc}\u200c\u200d`
	pattern := strings.NewReplacer("{WORD}", "["+word+"]", "{WC}", word).Replace(url + `|` + rooted + `|` + bare)
	p, err := core.CompilePattern(pattern, 100*time.Millisecond)
	if err != nil {
		panic("invalid terminal link pattern: " + err.Error())
	}
	return p
})

func implicitLinkRanges(text string) [][]int {
	ranges, err := implicitLinkPattern().FindAllStringIndex(text, -1)
	if err != nil {
		return nil // Never publish partial matches on timeout.
	}
	out := ranges[:0]
	for _, r := range ranges {
		if r[1] > r[0] && (r[0] == 0 || text[r[0]-1] != '$') {
			out = append(out, r)
		}
	}
	return out
}

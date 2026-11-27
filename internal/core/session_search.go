package core

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/width"
	"strconv"
	"strings"
	"unicode"
)

// SessionQuery searches display metadata only, never credentials or output.
type SessionQuery struct {
	Terms []string
	Valid bool
}

func NewSessionQuery(text string) SessionQuery {
	if len(text) > 4096 {
		return SessionQuery{}
	}
	words := strings.Fields(text)
	if len(words) > 32 {
		return SessionQuery{}
	}
	for i := range words {
		words[i] = NormalizeSessionSearch(words[i])
	}
	return SessionQuery{Terms: words, Valid: true}
}
func NormalizeSessionSearch(text string) string {
	ascii := true
	for i := 0; i < len(text); i++ {
		if text[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return strings.ToLower(text)
	}
	text = norm.NFD.String(cases.Fold().String(width.Fold.String(text)))
	return norm.NFC.String(strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, text))
}
func (q SessionQuery) Matches(normalized string) bool {
	if !q.Valid {
		return false
	}
	for _, term := range q.Terms {
		if !strings.Contains(normalized, term) {
			return false
		}
	}
	return true
}
func SessionSearchMetadata(p Profile) string {
	protocol := strings.ToUpper(p.Kind)
	port := strconv.Itoa(p.Port)
	if p.Kind == "local" {
		protocol = "本地终端 local"
		port = ""
	}
	group := p.Group
	if !strings.HasPrefix(group, "/") {
		group = "/" + group
	}
	return NormalizeSessionSearch(strings.Join([]string{p.Name, p.Host, p.Username, group, protocol, port}, " "))
}

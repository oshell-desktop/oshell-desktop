package core

import (
	"strings"
	"testing"
)

func TestSessionQueryNormalizationAndBounds(t *testing.T) {
	p := NewProfile()
	p.Name = "Café ＰＲＯＤ"
	p.Host = "db.example"
	p.Username = "Admin"
	p.Group = "生产"
	p.Port = 2222
	metadata := SessionSearchMetadata(p)
	for _, text := range []string{"cafe prod", "ＣＡＦＥ admin ２２２２", "cafe\u0301 /生产 db.example"} {
		if !NewSessionQuery(text).Matches(metadata) {
			t.Fatal(text)
		}
	}
	for _, text := range []string{"cafe absent", strings.Repeat("a", 4097), strings.Repeat("prod ", 33)} {
		if NewSessionQuery(text).Matches(metadata) {
			t.Fatal("invalid query matched")
		}
	}
	if !NewSessionQuery(strings.Repeat("a", 4096)).Valid || !NewSessionQuery(strings.Repeat("prod ", 32)).Valid {
		t.Fatal("boundary rejected")
	}
	p.Kind = "local"
	if !NewSessionQuery("本地终端").Matches(SessionSearchMetadata(p)) {
		t.Fatal("local title absent")
	}
}

func TestSessionSearchASCIIAndUnicodeEquivalence(t *testing.T) {
	var ascii, folded strings.Builder
	for i := byte(0); i < 128; i++ {
		ascii.WriteByte(i)
		if i >= 'A' && i <= 'Z' {
			folded.WriteByte(i + 'a' - 'A')
		} else {
			folded.WriteByte(i)
		}
	}
	for _, tc := range []struct{ input, want string }{
		{"", ""}, {ascii.String(), folded.String()},
		{"Straße CAFÉ ＰＲＯＤ", "strasse cafe prod"},
		{"A\u0301BC", "abc"}, {"生产 SSH", "生产 ssh"},
	} {
		if got := NormalizeSessionSearch(tc.input); got != tc.want {
			t.Fatalf("normalize %q: %q != %q", tc.input, got, tc.want)
		}
	}
}

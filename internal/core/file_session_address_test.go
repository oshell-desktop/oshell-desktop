package core

import "testing"

func TestFileSessionAddressDoesNotGuessAmbiguousHost(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want string
	}{
		{[]string{"192.0.2.1", "::ffff:192.0.2.1"}, "192.0.2.1"},
		{[]string{"192.0.2.1", "2001:db8::1"}, ""}, {[]string{"[2001:db8::1]"}, "2001:db8::1"}, {[]string{"host.invalid"}, ""},
	} {
		if got := uniqueFileAddress(tc.in); got != tc.want {
			t.Fatal(tc.in, got)
		}
	}
}

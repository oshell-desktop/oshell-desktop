package terminal

import "testing"

func TestInitialTargetLogin(t *testing.T) {
	for _, test := range []struct {
		opening string
		ready   bool
	}{
		{"\r\nLogin: fixture\r\nPassword: ", true},
		{"Login: fixture\nPassword:", true},
		{"Login: another\r\nPassword:", false},
		{"$ command\r\nLogin: fixture\r\nPassword:", false},
		{"Login: fixture\r\nOTP:", false},
		{"Login: fixture\r\nPassword:\r\n$ ", false},
	} {
		state := initialTargetLogin{account: "fixture"}
		for _, b := range []byte(test.opening) {
			state.observe([]byte{b})
		}
		if state.ready != test.ready {
			t.Errorf("opening %q: ready=%v", test.opening, state.ready)
		}
		if test.ready {
			state.observe([]byte("\r\nWelcome back\r\n$ "))
			state.observe([]byte("Login: fixture\r\nPassword:"))
			if state.ready || state.retry {
				t.Fatal("accepted a password prompt after login")
			}
		}
	}
}

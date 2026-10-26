package core

import "testing"

func TestLoginPasswordPromptMatchesOnlyResolvedAccount(t *testing.T) {
	identity := SSHIdentity{User: "alice", Host: "example.test", Port: 22}
	for _, prompt := range []string{"alice@example.test's password:", " (alice@example.test) Password:\n", "(alice@example.test) password:"} {
		if !loginPasswordPrompt(identity, prompt) {
			t.Errorf("login prompt not recognized: %q", prompt)
		}
	}
	for _, prompt := range []string{"Password:", "password", "密码：", "密码:", "bob@example.test's password:", "alice@jump.test's password:", "ALICE@example.test's password:", "Enter passphrase for key '/tmp/key':", "One-time password:", "New password:", "Retype password:", "Verification code:", "Enter your password for another service:"} {
		if loginPasswordPrompt(identity, prompt) {
			t.Errorf("unrelated prompt recognized as target login: %q", prompt)
		}
	}
	identity.Host = "[2001:db8::1]"
	if !loginPasswordPrompt(identity, "alice@2001:db8::1's password:") {
		t.Fatal("bracketed IPv6 identity not normalized")
	}
}

func TestLoginPasswordQuestionRequiresUniqueHiddenField(t *testing.T) {
	identity := SSHIdentity{User: "alice", Host: "example.test", Port: 22}
	for _, tc := range []struct {
		questions []string
		echo      []bool
		want      int
	}{
		{[]string{"Account:", "Password:", "OTP:"}, []bool{true, false, false}, -1},
		{[]string{"alice@example.test's password:", "OTP:"}, []bool{false, false}, -1},
		{[]string{"alice@example.test's password:"}, []bool{false}, 0},
		{[]string{"Password:", "Password:"}, []bool{false, false}, -1},
		{[]string{"Password:"}, []bool{true}, -1},
		{[]string{"Password:"}, nil, -1},
		{[]string{"bob@example.test's password:", "OTP:"}, []bool{false, false}, -1},
	} {
		if got := loginPasswordQuestion(identity, tc.questions, tc.echo); got != tc.want {
			t.Errorf("question classification=%d want=%d", got, tc.want)
		}
	}
}

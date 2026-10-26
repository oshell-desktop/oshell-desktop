package vt

import (
	"fmt"
	"strings"
	"testing"
)

func semanticState(t *testing.T, term *Terminal, want SemanticInputState) {
	t.Helper()
	if got, ok := term.SemanticInputState(); !ok || got != want {
		t.Fatalf("semantic input = %v (supported %v), want %v", got, ok, want)
	}
}

func TestOShellSemanticInputLifecycle(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	semanticState(t, term, SemanticInputIdle)
	for _, step := range []struct {
		seq   string
		state SemanticInputState
	}{
		{"P", SemanticInputIdle}, {"B", SemanticInputArmed}, {"P;k=r", SemanticInputArmed},
		{"A;k=r", SemanticInputArmed}, {"N;k=r", SemanticInputArmed}, {"L", SemanticInputArmed},
		{"C", SemanticInputSubmitted}, {"P", SemanticInputSubmitted}, {"I", SemanticInputArmed},
		{"A;k=c", SemanticInputPrompt}, {"B", SemanticInputArmed}, {"N;k=s", SemanticInputPrompt},
		{"I", SemanticInputArmed}, {"D", SemanticInputSubmitted}, {"A", SemanticInputPrompt},
		{"B", SemanticInputArmed}, {"N;cl=INVALID", SemanticInputArmed},
	} {
		term.Write([]byte("\x1b]133;" + step.seq + "\a"))
		semanticState(t, term, step.state)
	}
	term.RegisterUserInput([]byte("\r"))
	semanticState(t, term, SemanticInputSubmitted)
	term.RegisterUserInput([]byte("ordinary text"))
	semanticState(t, term, SemanticInputSubmitted)
	term.Write([]byte("\x1b]133;B\a"))
	semanticState(t, term, SemanticInputArmed)
}

func TestOShellSemanticInputSubmissionChunks(t *testing.T) {
	for _, tc := range []struct {
		text   string
		submit bool
	}{
		{"abc", false}, {"\r", true}, {"\n", true}, {"\x1b\r", true}, {"\x1b[\n", true},
		{"\x1bO\r", true}, {"\x1bOM", true}, {"\x1bOA", false},
		{"\x1b[13u", true}, {"\x1b[13;1:2u", true}, {"\x1b[13;1:3u", false},
		{"\x1b[13:99;2u", true}, {"\x1b[013u", false}, {"\x1b[97u", false},
		{"\x1b[200~one\r\ntwo\x1b[201~", false}, {"\x1b[200~\n\x1b[201~\r", true},
		{"\x1b[200~\x1b[13u", true}, {"\x1b[200~\x1bOM", true},
		{"\x1b[12\x1b[200~\n\x1b[201~", false},
		{"\x1b\x1b[13u", true}, {"\x1b[" + strings.Repeat("0", 100) + "u", false},
	} {
		for split := 0; split <= len(tc.text); split++ {
			t.Run(fmt.Sprintf("%q/split%d", tc.text, split), func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				term.Write([]byte("\x1b]133;B\a"))
				term.RegisterUserInput([]byte(tc.text[:split]))
				term.RegisterUserInput([]byte(tc.text[split:]))
				want := SemanticInputArmed
				if tc.submit {
					want = SemanticInputSubmitted
				}
				semanticState(t, term, want)
			})
		}
	}
}

func TestOShellSemanticInputResetAndScreens(t *testing.T) {
	for _, reset := range []string{"\x1bc", "\x1b[!p"} {
		term, _ := resetTerminal(t, Effects{})
		term.RegisterUserInput([]byte("\x1b[200~")) // Scanner runs even while idle.
		term.Write([]byte("\x1b]133;B\a"))
		term.RegisterUserInput([]byte("\n"))
		semanticState(t, term, SemanticInputArmed)
		term.Write([]byte(reset))
		semanticState(t, term, SemanticInputIdle)
		term.Write([]byte("\x1b]133;B\a"))
		term.RegisterUserInput([]byte("\n"))
		semanticState(t, term, SemanticInputSubmitted)
	}
	for _, mode := range []string{"47", "1047", "1049"} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("\x1b]133;B\a\x1b[?" + mode + "h\x1b]133;B\a"))
		semanticState(t, term, SemanticInputSubmitted)
		term.Write([]byte("\x1b[?" + mode + "l"))
		semanticState(t, term, SemanticInputSubmitted)
		term.Write([]byte("\x1b]133;B\a\x1b[?" + mode + "l"))
		semanticState(t, term, SemanticInputArmed)
	}
}

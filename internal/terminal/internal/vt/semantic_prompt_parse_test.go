package vt

import (
	"fmt"
	"testing"
)

// Terminal.oscSemanticPrompt rejects an entire malformed command before
// fresh-line or classification. Cursor movement makes this externally visible.
func TestOShellSemanticPromptOptionValidation(t *testing.T) {
	for _, action := range []string{"A", "N", "L"} {
		for _, option := range []string{"k=", "k=z", "k=ii", "cl=LINE", "cl=", "click_events=3", "click_events=01", "special_key=2", "special_key=-1", "k=z;k=i", "k=i;k=z", "unknown=\xff"} {
			for _, chunk := range []int{1, 7, 4096} {
				t.Run(fmt.Sprintf("%s/%q/%d", action, option, chunk), func(t *testing.T) {
					term, _ := resetTerminal(t, Effects{})
					term.Write([]byte("\x1b[2;5H"))
					writeControlChunks(term, []byte("\x1b]133;"+action+";"+option+"\a"), chunk)
					layoutCursor(t, term, "", 4, 1)
				})
			}
		}
	}
}

func TestOShellSemanticFreshLineAcceptsOptions(t *testing.T) {
	for _, action := range []string{"A", "N", "L"} {
		for _, option := range []string{"", ";;;", "bare", "unknown=value", "unknown=中文", "k=i", "cl=m", "special_key=1", "click_events=2"} {
			for _, chunk := range []int{1, 4096} {
				t.Run(fmt.Sprintf("%s/%s/%d", action, option, chunk), func(t *testing.T) {
					term, _ := resetTerminal(t, Effects{})
					term.Write([]byte("\x1b[2;5H"))
					writeControlChunks(term, []byte("\x1b]133;"+action+";"+option+"\x1b\\"), chunk)
					layoutCursor(t, term, "", 0, 2)
				})
			}
		}
	}
}

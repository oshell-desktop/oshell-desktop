package vt

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"
)

var testWriteUntilGround, testContinuationBuf uintptr

func init() {
	symbols = append(symbols, []struct {
		name string
		addr *uintptr
	}{
		{"ghostty_terminal_vt_write_until_ground", &testWriteUntilGround},
		{"ghostty_terminal_continuation_buf", &testContinuationBuf},
	}...)
}

// SwiftTerm decodes a printable run with a Latin-1 fallback for its first
// byte, consuming the full expected sequence even when a later byte is bad.
func TestOShellUTF8FallbackAndFragments(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"valid", "A中文🙂éZ", "A中文🙂éZ"},
		{"combining", "e\u0301", "e\u0301"},
		{"invalid-leads", "\xc0\xc1\xf5\xffZ", "ÀÁõÿZ"},
		{"stray-continuations", "\xa1\xbfZ", "¡¿Z"},
		{"invalid-middle", "\xe2ABZ", "âZ"},
		{"invalid-tail", "\xe2\x82AZ", "âZ"},
		{"consumed-next-lead", "\xe2\xc3\xa9Z", "âZ"},
		{"overlong", "\xe0\x80\x80Z", "àZ"},
		{"surrogate", "\xed\xa0\x80Z", "íZ"},
		{"out-of-range", "\xf4\x90\x80\x80Z", "ôZ"},
		{"raw-c1", "\x9b31mZ", "31mZ"},
		{"encoded-c1", "\xc2\x9b31mZ", "31mZ"},
		{"del-is-control", "A\x7fZ", "AZ"},
		{"incomplete", "A\xf0\x9f", "A"},
		{"escape-discards-partial", "\xe2\x1b[32mZ", "Z"},
	} {
		for _, chunk := range []int{1, 2, 3, 7, 1024} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, chunk), func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				for input := []byte(tc.input); len(input) > 0; {
					n := min(chunk, len(input))
					term.Write(input[:n])
					input = input[n:]
				}
				if got := strings.TrimSpace(term.Text()); got != tc.want {
					t.Fatalf("decoded %x: got %q, want %q", tc.input, got, tc.want)
				}
			})
		}
	}
}

func TestOShellUTF8ContinuationAndGroundBoundary(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	limit := uintptr(8)
	if err := term.set(31, unsafe.Pointer(&limit)); err != nil {
		t.Fatal(err)
	}
	read := func(want string, wantErr error) {
		t.Helper()
		var buf [8]byte
		var written uintptr
		err := result(call(testContinuationBuf, term.h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&written))))
		if err != wantErr || (err == nil && string(buf[:written]) != want) {
			t.Fatalf("continuation=%x error=%v, want %x %v", buf[:written], err, want, wantErr)
		}
	}
	term.Write([]byte("\xe2\n\r\x82"))
	read("\xe2\x82", nil) // Do not replay already executed line/cursor movement.
	input := []byte("\xacZ")
	var consumed uintptr
	if err := result(call(testWriteUntilGround, term.h, uintptr(unsafe.Pointer(&input[0])), uintptr(len(input)), uintptr(unsafe.Pointer(&consumed)))); err != nil || consumed != 1 {
		t.Fatalf("ground boundary consumed=%d error=%v", consumed, err)
	}
	if got := term.FormattedText(false); got != "\n€" {
		t.Fatalf("write-until-ground printed suffix: %q", got)
	}
	read("", nil)
	term.Write([]byte("\xe2A"))
	read("", Invalid) // Strict public snapshots cannot represent this prefix.
	term.Write([]byte("B"))
	read("", nil)
	limit = 1
	if err := term.set(31, unsafe.Pointer(&limit)); err != nil {
		t.Fatal(err)
	}
	term.Write([]byte("\xe2\x82"))
	read("", Invalid)
	term.Write([]byte("\xac"))
	read("", nil)
}

func TestOShellUTF8LargePrintableRuns(t *testing.T) {
	for _, chunk := range []int{1, 31, 4095, 4096, 8192, 65536} {
		t.Run(fmt.Sprint(chunk), func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			line := strings.Repeat("a", 4095) + "中" + strings.Repeat("b", 4095) + "🙂Z"
			if err := term.Resize(8300, 5, 8, 16); err != nil {
				t.Fatal(err)
			}
			data := []byte(line)
			for len(data) > 0 {
				n := min(chunk, len(data))
				term.Write(data[:n])
				data = data[n:]
			}
			if got := strings.TrimSpace(term.Text()); got != line {
				t.Fatalf("large printable run differs: got %d bytes, want %d", len(got), len(line))
			}
		})
	}
}

func TestOShellUTF8PartialAcrossExecutedControls(t *testing.T) {
	for _, chunk := range []int{1, 2, 4096} {
		t.Run(fmt.Sprint(chunk), func(t *testing.T) {
			bells := 0
			term, _ := resetTerminal(t, Effects{Bell: func() { bells++ }})
			data := []byte("A\xe2\n\r\a\x82\xacZ")
			for len(data) > 0 {
				n := min(chunk, len(data))
				term.Write(data[:n])
				data = data[n:]
			}
			if got := term.FormattedText(false); got != "A\n€Z" || bells != 1 {
				t.Fatalf("pending sequence across C0: text=%q bells=%d", got, bells)
			}
		})
	}
}

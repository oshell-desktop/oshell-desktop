package vt

import (
	"bytes"
	"testing"
)

// Byte expectations come from SwiftTerm Attribute.toSgr, including its
// current limited attribute list and indexed-color separator conventions.
// This is the status reply only; rendering and VT state export stay complete.
func TestOShellAttributeStatusReport(t *testing.T) {
	for _, chunk := range []int{1, 7, 4096} {
		for _, tc := range []struct{ attributes, report string }{
			{"0", "0m"},
			{"1;2;3;4:3;5;7;8;9;53", "0;1;4;5;7;8m"},
			{"31;44", "0;31;;44;m"},
			{"97;100", "0;97;;100;m"},
			{"38;5;16;48;5;16", "0;98;;108;m"},
			{"38;5;17;48;5;255", "0;38;5;17;48;5;255m"},
			{"38;2;1;2;3;48;2;250;251;252", "0;38;2;1;2;3;48;2;250;251;252m"},
			{"4:2;58;2;7;8;9", "0;4m"},
			{"31;44;39;49", "0m"},
		} {
			var reply bytes.Buffer
			term, _ := resetTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }})
			writeControlChunks(term, []byte("\x1b["+tc.attributes+"mX"), chunk)
			before := append([]byte(nil), term.VT()...)
			writeControlChunks(term, []byte("\x1bP$qm\x1b\\"), chunk)
			if got, want := reply.String(), "\x1bP1$r"+tc.report+"\x1b\\"; got != want {
				t.Errorf("chunk=%d SGR=%s response=%q want=%q", chunk, tc.attributes, got, want)
			}
			if !bytes.Equal(before, term.VT()) {
				t.Fatal("attribute query changed terminal state", tc.attributes)
			}
			reply.Reset()
			writeControlChunks(term, []byte("\x1b G\x1bP$qm\x1b\\"), chunk)
			if got, want := reply.String(), "\x901$r"+tc.report+"\x9c"; got != want {
				t.Errorf("8-bit SGR=%s response=%q want=%q", tc.attributes, got, want)
			}
		}
	}
}

func TestOShellWindowTitleQueries(t *testing.T) {
	for _, chunk := range []int{1, 7, 4096} {
		var reply bytes.Buffer
		term, _ := resetTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }})
		term.Write([]byte("\x1b]2;do-not-return-this-title\a"))
		for _, tc := range []struct{ request, want string }{
			{"\x1b[20t\x1b[21t", "\x1b]L\x1b\\\x1b]l\x1b\\"},
			{"\x1b[20;t\x1b[21;t\x1b[20;0t\x1b[21;1t", ""},
			{"\x1b G\x1b[20t\x1b[21t", "\x9dL\x9c\x9dl\x9c"},
		} {
			reply.Reset()
			writeControlChunks(term, []byte(tc.request), chunk)
			if got := reply.String(); got != tc.want {
				t.Errorf("chunk=%d query=%q response=%q want=%q", chunk, tc.request, got, tc.want)
			}
			if term.Title() != "do-not-return-this-title" {
				t.Fatal("query changed title", term.Title())
			}
		}
	}
}

// MacTerminalView returns nil for screen/window dimensions, so OShell uses
// Terminal.cmdWindowOptions' text-grid fallback for 14;2, 15 and 19.
func TestOShellWindowSizeQueries(t *testing.T) {
	for _, chunk := range []int{1, 7, 4096} {
		var reply bytes.Buffer
		cols, rows, cw, ch := 12, 5, 8, 16
		term, _ := resetTerminal(t, Effects{
			WritePTY: func(b []byte) { reply.Write(b) },
			Size:     func() (int, int, int, int) { return cols, rows, cw, ch },
		})
		request := "\x1b[14t\x1b[14;2t\x1b[15t\x1b[16t\x1b[18t\x1b[19t"
		writeControlChunks(term, []byte(request), chunk)
		want := "\x1b[4;80;96t\x1b[4;80;96t\x1b[5;80;96t\x1b[6;16;8t\x1b[8;5;12t\x1b[9;5;12t"
		if reply.String() != want {
			t.Errorf("chunk=%d sizes=%q want=%q", chunk, reply.String(), want)
		}
		reply.Reset()
		writeControlChunks(term, []byte("\x1b[14;t\x1b[14;2;t\x1b[14;0t\x1b[15;t\x1b[16;1t\x1b[18;t\x1b[19;0t"), chunk)
		if reply.Len() != 0 {
			t.Fatal("unsupported query replied", reply.String())
		}
		cols, rows, cw, ch = 40, 10, 9, 17
		reply.Reset()
		writeControlChunks(term, []byte("\x1b G"+request), chunk)
		want = "\x9b4;170;360t\x9b4;170;360t\x9b5;170;360t\x9b6;17;9t\x9b8;10;40t\x9b9;10;40t"
		if reply.String() != want {
			t.Errorf("updated sizes=%q want=%q", reply.String(), want)
		}
	}
}

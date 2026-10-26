package vt

import (
	"bytes"
	"strings"
	"testing"
)

func TestOShellControlEncodingStateAndReplies(t *testing.T) {
	for _, chunk := range []int{1, 2, 4096} {
		var reply bytes.Buffer
		term, _ := resetTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }})
		writeControlChunks(term, []byte("\x1b G\x1b[5n\x1b F\x1b[5n\x1b G\x1b[5n"), chunk)
		if got, want := reply.String(), "\x9b0n\x1b[0n\x9b0n"; got != want {
			t.Fatalf("chunk=%d reply=%q want=%q", chunk, got, want)
		}
		for _, tc := range []struct {
			seq   string
			eight bool
			level uint8
		}{
			{"\x1b[62\"p", true, 62}, {"\x1b[63;1\"p", false, 63},
			{"\x1b[64;\"p", true, 64}, {"\x1b[61;0\"p", false, 61}, {"\x1b[\"p", false, 65},
			{"\x1b[64;0\"p", true, 64}, {"\x1b[65;2\"p", true, 65},
			{"\x1b[61;2\"p", false, 61}, {"\x1b G", true, 61},
			{"\x1b[62\"p", true, 62}, {"\x1b[!p", true, 65},
			{"\x1b[?1049h", true, 65}, {"\x1b[?1049l", true, 65},
			{"\x1b[1;1\"p", false, 65}, {"\x1b G\x1bc", false, 65},
		} {
			writeControlChunks(term, []byte(tc.seq), chunk)
			eight, level := term.ControlEncoding()
			if eight != tc.eight || level != tc.level {
				t.Fatalf("%q state=%v/%d want=%v/%d", tc.seq, eight, level, tc.eight, tc.level)
			}
			reply.Reset()
			term.Write([]byte("\x1b[5n"))
			want := "\x1b[0n"
			if tc.eight {
				want = "\x9b0n"
			}
			if reply.String() != want {
				t.Fatalf("%q response=%q want=%q", tc.seq, reply.String(), want)
			}
		}
	}
}

func TestOShellControlEncodingResponseFamilies(t *testing.T) {
	var reply bytes.Buffer
	term, _ := resetTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }, Size: func() (int, int, int, int) { return 12, 5, 8, 16 }})
	term.Write([]byte("\x1b G"))
	for _, tc := range []struct{ request, want string }{
		{"\x1b[3;4H\x1b[6n", "\x9b3;4R"},
		{"\x1b[?25$p", "\x9b?25;1$y"},
		{"\x1b[?u", "\x9b?0u"},
		{"\x1b[16t", "\x9b6;16;8t"},
		{"\x1b[18t", "\x9b8;5;12t"},
		{"\x1bP$q\"p\x1b\\", "\x901$r65;1\"p\x9c"},
		{"\x1bP$q\"q\x1b\\", "\x901$r\"q\x9c"},
		{"\x1bP$qBAD\x1b\\", "\x900$r\x9c"},
	} {
		reply.Reset()
		term.Write([]byte(tc.request))
		if got := reply.String(); got != tc.want {
			t.Errorf("%q => %q want=%q", tc.request, got, tc.want)
		}
	}
	term.Write([]byte("\x1b]4;1;#010203;2;#040506\a"))
	for _, prefix := range []string{"\x1b G", "\x1b F"} {
		reply.Reset()
		term.Write([]byte(prefix + "\x1b]4;1;?;2;?\a"))
		start, end := "\x9d", "\x9c"
		if prefix == "\x1b F" {
			start, end = "\x1b]", "\x1b\\"
		}
		want := start + "4;1;rgb:0101/0202/0303" + end + start + "4;2;rgb:0404/0505/0606" + end
		if got := reply.String(); got != want {
			t.Fatalf("color batch=%q want=%q", got, want)
		}
	}
	term.Write([]byte("\x1b G"))
	reply.Reset()
	term.Write([]byte("\x1b[>q"))
	if !strings.HasPrefix(reply.String(), "\x1bP>|") || !strings.HasSuffix(reply.String(), "\x1b\\") {
		t.Fatal("version reply changed encoding", reply.String())
	}
	reply.Reset()
	term.Write(kittyCommand("a=q,f=24,s=1,v=1,i=31", []byte{255, 0, 0}))
	if !strings.HasPrefix(reply.String(), "\x1b_G") {
		t.Fatal("Kitty reply changed encoding", reply.String())
	}
}

func TestOShellControlEncodingInputBoundaries(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b G\x1b[?1000h\x1b[?1006h"))
	if got := string(term.EncodeControlReport(EncodeFocus(true))); got != "\x9bI" {
		t.Fatalf("focus=%q", got)
	}
	m, err := NewMouseEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Free()
	m.Sync(term, 96, 80, 8, 16, 0, 0, false)
	if got := string(m.Encode(MousePress, ButtonLeft, 0, 20, 24)); got != "\x9b<0;3;2M" {
		t.Fatalf("mouse=%q", got)
	}
	raw := []byte{27, '[', 'M', 27, '[', 0x9b}
	got := controlReport(raw, true)
	if !bytes.Equal(got, []byte{0x9b, 'M', 27, '[', 0x9b}) {
		t.Fatal("mouse payload changed", got)
	}
	if got := string(EncodePaste("abc", true)); got != "\x1b[200~abc\x1b[201~" {
		t.Fatalf("paste changed=%q", got)
	}
	term.Write([]byte("\x1b F"))
	m.Sync(term, 96, 80, 8, 16, 0, 0, false)
	if got := string(m.Encode(MousePress, ButtonLeft, 0, 20, 24)); got != "\x1b[<0;3;2M" {
		t.Fatalf("mouse reset=%q", got)
	}
}

func TestOShellControlEncodingUnknownQueryRecovery(t *testing.T) {
	for _, chunk := range []int{1, 7, 4096} {
		var reply bytes.Buffer
		term, _ := resetTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }})
		request := "\x1b G\x1bP$q" + strings.Repeat("m", 16384) + "\x1b\\\x1bP$q\"p\x1b\\\x1b[5n"
		writeControlChunks(term, []byte(request), chunk)
		if got, want := reply.String(), "\x900$r\x9c\x901$r65;1\"p\x9c\x9b0n"; got != want {
			t.Fatalf("chunk=%d response=%q want=%q", chunk, got, want)
		}
	}
}

func TestOShellControlEncodingReplayAndKeys(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b[61\"p\x1b Ghello"))
	restored, _ := resetTerminal(t, Effects{})
	restored.Write(term.VT())
	if eight, level := restored.ControlEncoding(); !eight || level != 61 {
		t.Fatal("replay lost control encoding", eight, level)
	}
	if restored.FormattedText(false) != "hello" {
		t.Fatal("replay changed content", restored.FormattedText(false))
	}
	k, err := NewKeyEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer k.Free()
	k.Sync(term, false)
	if got := string(k.Encode(KeyEvent{Key: KeyArrowUp, Action: KeyPress})); got != "\x1b[A" {
		t.Fatalf("ordinary key changed=%q", got)
	}
}

package terminal

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLinkPayloadASCIIAndEmptyURL(t *testing.T) {
	for _, tc := range []struct {
		payload, label, url string
		explicit            bool
	}{
		{";", "label", "", false},
		{"", "label", "", false},
		{"id=broken", "label", "", false},
		{";https://例.test/", "label", "", false},
		{"id=例;https://example.test/", "label", "", false},
		{"x=\xff;https://example.test/", "label", "", false},
		{";https://例.test/", "https://text.test/", "https://text.test/", false},
		{"id=empty;", "label", "", true},
		{"x=value;", "https://text.test/", "", true},
		{":;", "label", "", true},
		{";;", "label", ";", true},
		{"id=x;https://example.test/a;b", "label", "https://example.test/a;b", true},
	} {
		for _, chunk := range []int{1, 17, 4096} {
			for _, end := range []string{"\a", "\x1b\\"} {
				t.Run(fmt.Sprintf("%q/%d/%q", tc.payload, chunk, end), func(t *testing.T) {
					term, _, _ := bidiViewFixture(t)
					input := "\x1b]8;;https://old.test/" + end + "old" + "\x1b]8;" + tc.payload + end + tc.label + "\x1b]8;;" + end
					for len(input) > 0 {
						n := min(chunk, len(input))
						term.Feed([]byte(input[:n]))
						input = input[n:]
					}
					term.mu.Lock()
					got := term.v.linkMatchAt(4, 0)
					old := term.v.linkMatchAt(1, 0)
					term.mu.Unlock()
					if got.text != tc.url || got.explicit != tc.explicit {
						t.Fatalf("match %+v want %q explicit=%v", got, tc.url, tc.explicit)
					}
					if old.text != "https://old.test/" || !old.explicit {
						t.Fatal("new payload changed existing cells", old)
					}
				})
			}
		}
	}
}

func TestLinkPayloadEmptySuppressesImplicitClick(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte("\x1b]8;id=empty;\x1b\\https://text.test/\x1b]8;;\x1b\\"))
	tt.Frame()
	x, y := cellCenter(term, 2, 0)
	tt.Move(x, y)
	tt.HoldModifiers(ui.Cmd)
	if m := term.v.linkHover.match; !m.explicit || m.text != "" || len(m.ranges) != 1 {
		t.Fatalf("empty payload hover: %+v", m)
	}
	if !term.v.linkHover.previewSet || term.v.linkHover.preview != "" {
		t.Fatal("empty explicit preview lost its presence")
	}
	tt.ClickAtWith(ui.Cmd, x, y)
	if got := tt.OpenedURLs(); len(got) != 0 {
		t.Fatal("empty explicit payload opened implicit text", got)
	}
}

func TestLinkPayloadCancellationAndRecovery(t *testing.T) {
	for _, cancel := range []byte{0x18, 0x1a} {
		term, _, _ := bidiViewFixture(t)
		term.Feed([]byte("\x1b]8;;https://old.test/\x1b\\old\x1b]8;id=broken"))
		term.Feed([]byte{cancel})
		term.Feed([]byte("kept\x1b]8;;https://new.test/\x1b\\new\x1b]8;;\x1b\\"))
		term.mu.Lock()
		kept, newLink := term.v.linkMatchAt(4, 0), term.v.linkMatchAt(8, 0)
		term.mu.Unlock()
		if kept.text != "https://old.test/" || newLink.text != "https://new.test/" {
			t.Fatal("cancel/recovery changed payloads", kept, newLink)
		}
	}
}

func TestLinkPayloadInvalidFallsBackToPrintedURL(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte("\x1b]8;;https://old.test/\x1b\\old \x1b]8;id=例;https://ignored.test/\x1b\\https://printed.test/\x1b]8;;\x1b\\"))
	tt.Frame()
	x, y := cellCenter(term, 8, 0)
	tt.ClickAtWith(ui.Cmd, x, y)
	if got := tt.OpenedURLs(); len(got) != 1 || got[0] != "https://printed.test/" {
		t.Fatal("invalid payload did not release old click target", got)
	}
}

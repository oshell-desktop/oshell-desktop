package vt

import (
	"fmt"
	"strings"
	"testing"
)

func TestKittyQueryWithoutIDAndWithBothIdentifiers(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	for _, query := range []struct{ control, reply string }{
		{"a=q,f=24,s=1,v=1", "\x1b_G;OK\x1b\\"},
		{"a=q,f=24,s=1,v=1,i=7,I=8,p=9", "\x1b_Gi=7,I=8,p=9;OK\x1b\\"},
	} {
		replies.Reset()
		term.Write(kittyCommand(query.control, []byte{255, 0, 0}))
		if replies.String() != query.reply {
			t.Fatalf("query %s: got %q, want %q", query.control, replies.String(), query.reply)
		}
	}
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("query displayed an image")
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=7", nil))
	if !strings.Contains(replies.String(), "ENOENT") {
		t.Fatal("query stored a cache entry", replies.String())
	}
}

func TestKittyQuietSuppressesSuccessAndFailure(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	for _, quiet := range []int{1, 2} {
		for _, control := range []string{
			"a=q,f=24,s=1,v=1,i=7",     // Valid query.
			"a=q,f=24,s=2,v=2,i=7",     // Invalid payload.
			"a=t,f=24,s=1,v=1,i=7,I=8", // Conflicting IDs.
			"a=p,i=123",                // Missing image.
		} {
			replies.Reset()
			term.Write(kittyCommand(fmt.Sprintf("%s,q=%d", control, quiet), []byte{255, 0, 0}))
			if replies.Len() != 0 {
				t.Fatalf("q=%d did not suppress %s: %q", quiet, control, replies.String())
			}
		}
	}
}

func TestKittyChunkResponseUsesInitialQuietSetting(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	for _, settings := range [][2]int{{0, 1}, {0, 2}, {1, 0}, {2, 0}} {
		replies.Reset()
		term.Write(kittyEncodedCommand(fmt.Sprintf("a=t,f=24,s=1,v=1,i=17,m=1,q=%d", settings[0]), "/"))
		if replies.Len() != 0 {
			t.Fatal("partial upload responded")
		}
		term.Write(kittyEncodedCommand(fmt.Sprintf("m=0,q=%d", settings[1]), "wAA"))
		if settings[0] == 0 {
			if !strings.Contains(replies.String(), ";OK") {
				t.Fatal("final chunk changed initial quiet setting", settings, replies.String())
			}
		} else if replies.Len() != 0 {
			t.Fatal("quiet upload responded", settings, replies.String())
		}
	}
}

func TestKittyPutPrefersNumberThenFallsBackToID(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyCommand("a=t,f=24,s=1,v=1,I=51", []byte{0, 0, 255}))
	term.Write(kittyCommand("a=t,f=24,s=1,v=1,i=7", []byte{255, 0, 0}))
	for _, item := range []struct {
		number  int
		channel int
	}{{51, 2}, {52, 0}} {
		replies.Reset()
		term.Write(kittyCommand(fmt.Sprintf("a=p,i=7,I=%d,p=1,c=1,r=1,C=1", item.number), nil))
		if !strings.Contains(replies.String(), ";OK") {
			t.Fatal("put rejected both ID forms", replies.String())
		}
		if item.number == 52 && replies.String() != "\x1b_Gi=7,p=1;OK\x1b\\" {
			t.Fatal("fallback response retained the missing number", replies.String())
		}
		found := false
		for _, g := range term.Graphics(10, 20) {
			if g.Image.Pix[item.channel] == 255 {
				found = true
			}
		}
		if !found {
			t.Fatal("put selected the wrong image", item.number)
		}
	}
}

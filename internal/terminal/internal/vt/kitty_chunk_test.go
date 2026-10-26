package vt

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func kittyEncodedCommand(control, payload string) []byte {
	return []byte("\x1b_G" + control + ";" + payload + "\x1b\\")
}

// Swift appends encoded bytes before decoding the entire transmission. An APC
// boundary can occur anywhere in a Base64 quartet, not just between RGB pixels.
func TestKittyEncodedChunkBoundaries(t *testing.T) {
	for _, fragment := range []bool{false, true} {
		for split := 1; split < 8; split++ {
			t.Run(fmt.Sprintf("split%d/bytes%v", split, fragment), func(t *testing.T) {
				term, replies := kittyCacheTerminal(t)
				payload := base64.StdEncoding.EncodeToString([]byte{7, 8, 9, 10, 11, 12})
				write := func(data []byte) {
					if fragment {
						for _, b := range data {
							term.Write([]byte{b})
						}
					} else {
						term.Write(data)
					}
				}
				write(kittyEncodedCommand("a=T,f=24,s=2,v=1,i=71,p=1,c=2,r=1,C=1,m=1", payload[:split]))
				if replies.Len() != 0 || len(term.Graphics(10, 20)) != 0 {
					t.Fatal("partial transfer committed", replies.String())
				}
				write(kittyEncodedCommand("m=0", payload[split:]))
				g := term.Graphics(10, 20)
				if replies.String() != "\x1b_Gi=71,p=1;OK\x1b\\" || len(g) != 1 || !bytes.Equal(g[0].Image.Pix, []byte{7, 8, 9, 255, 10, 11, 12, 255}) {
					t.Fatal("encoded chunk boundary changed image", replies.String(), g)
				}
			})
		}
	}
}

func TestKittyPendingUsesFirstControl(t *testing.T) {
	for _, continuation := range []string{"a=q,i=999", "a=p,i=999", "a=Z,i=999", "a=t,i=999,I=998", "a=T,f=13,o=X,t=X,i=999", "m=2", "m=4294967295", "i=invalid,m=0", "m=1,m=garbage", "m=-1", "m=+0", "m=9223372036854775808", "m=1,m=", "unknownkeyofanylength=value,broken,m=0"} {
		t.Run(continuation, func(t *testing.T) {
			term, replies := kittyCacheTerminal(t)
			term.Write(kittyEncodedCommand("a=T,f=24,s=2,v=1,i=71,c=2,r=1,C=1,m=1", "BwgJ"))
			term.Write(kittyEncodedCommand(continuation, "CgsM"))
			g := term.Graphics(10, 20)
			if replies.String() != "\x1b_Gi=71;OK\x1b\\" || len(g) != 1 || g[0].Image.Pix[4] != 10 {
				t.Fatal("continuation dispatched independently", replies.String(), g)
			}
		})
	}
}

func TestKittyPaddingAcrossFragments(t *testing.T) {
	raw := []byte{7, 8, 9, 255}
	encoded := base64.StdEncoding.EncodeToString(raw)
	for split := 1; split < len(encoded); split++ {
		term, replies := kittyCacheTerminal(t)
		term.Write(kittyEncodedCommand("a=T,f=32,s=1,v=1,i=71,c=1,r=1,C=1,m=1", encoded[:split]))
		term.Write(kittyEncodedCommand("m=1", "!"))
		term.Write(kittyEncodedCommand("m=0", encoded[split:]))
		g := term.Graphics(10, 20)
		if replies.String() != "\x1b_Gi=71;OK\x1b\\" || len(g) != 1 || !bytes.Equal(g[0].Image.Pix, raw) {
			t.Fatal("padding state lost at fragment boundary", split, replies.String(), g)
		}
	}
	// A pending display never decodes payload, including illegal padding.
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyCommand("a=t,f=32,s=1,v=1,i=71", raw))
	replies.Reset()
	term.Write(kittyEncodedCommand("a=p,i=71,c=1,r=1,C=1,m=1", "=A"))
	if replies.Len() != 0 || len(term.Graphics(10, 20)) != 0 {
		t.Fatal("display ran before completion")
	}
	term.Write(kittyEncodedCommand("a=q", "A="))
	if replies.String() != "\x1b_Gi=71;OK\x1b\\" || len(term.Graphics(10, 20)) != 1 {
		t.Fatal("display decoded unused payload", replies.String())
	}
}

func TestKittyMoreFlagAndControlParsing(t *testing.T) {
	for _, more := range []string{"m=1", "m=+0001", "m=" + strings.Repeat("0", 300) + "1", "m=0,m=1", "m=1,m=\xff", "m=1,extra=ignored", "broken,m=1,"} {
		t.Run(more[:min(len(more), 32)], func(t *testing.T) {
			term, replies := kittyCacheTerminal(t)
			term.Write(kittyEncodedCommand("a=Transmit,f=24,s=2,v=1,i=71,c=1,r=1,C=1,"+more, "BwgJ"))
			if replies.Len() != 0 {
				t.Fatal("more flag not retained", replies.String())
			}
			term.Write(kittyEncodedCommand("m=0", "CgsM"))
			if replies.String() != "\x1b_Gi=71;OK\x1b\\" || len(term.Graphics(10, 20)) != 1 {
				t.Fatal("control parsing changed transfer", replies.String())
			}
		})
	}
	// A non-direct final fragment ends the initial direct transmission, ignoring
	// the final fragment's medium and payload interpretation.
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyEncodedCommand("a=q,f=24,s=2,v=1,i=71,m=1", "BwgJ"))
	term.Write(kittyEncodedCommand("t=file,m=1", "CgsM"))
	if replies.String() != "\x1b_Gi=71;OK\x1b\\" {
		t.Fatal("non-direct fragment kept pending", replies.String())
	}
}

func TestKittyDeferredValidationAndQuiet(t *testing.T) {
	for _, control := range []string{"a=Z", "a=t,f=13", "a=t,f=24,s=1,v=1,i=71,I=72"} {
		for _, quiet := range []string{"0", "-1", "+2", "9223372036854775807", "9223372036854775808"} {
			t.Run(control+"/q"+quiet, func(t *testing.T) {
				term, replies := kittyCacheTerminal(t)
				term.Write(kittyEncodedCommand(control+",q="+quiet+",m=1", "B"))
				if replies.Len() != 0 {
					t.Fatal("validation ran before final fragment", replies.String())
				}
				term.Write(kittyEncodedCommand("q=0", "wgJ"))
				wantReply := quiet == "0" || quiet == "9223372036854775808"
				if wantReply != strings.Contains(replies.String(), "EINVAL") {
					t.Fatal("first quiet or error not retained", replies.String())
				}
			})
		}
	}
}

func TestKittyEncodedTransferBudget(t *testing.T) {
	for _, singleAPC := range []bool{false, true} {
		t.Run(fmt.Sprintf("singleAPC%v", singleAPC), func(t *testing.T) {
			testKittyEncodedTransferBudget(t, singleAPC)
		})
	}
}

func TestKittyChunkedPayloadExceedsSingleAPCBudget(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	// 54 MiB RGB expands to 72 MiB Base64, larger than one APC's 65 MiB
	// input limit but inside Swift's existing 400 MiB decoded-data limit.
	term.Write(kittyEncodedCommand("a=q,f=24,s=4096,v=4608,i=71,m=1", ""))
	chunk := strings.Repeat("A", 1<<20)
	for range 71 {
		term.Write(kittyEncodedCommand("m=1", chunk))
	}
	term.Write(kittyEncodedCommand("m=0", chunk))
	if replies.String() != "\x1b_Gi=71;OK\x1b\\" || len(term.Graphics(10, 20)) != 0 {
		t.Fatal("chunk assembler reduced the decoded payload budget", replies.String())
	}
}

func testKittyEncodedTransferBudget(t *testing.T, singleAPC bool) {
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=71,c=1,r=1,C=1", []byte{255, 0, 0}))
	replies.Reset()
	// Each APC fits the input budget but their aggregate exceeds it. The
	// failed first control must remain until termination, without its bytes.
	term.Write(kittyEncodedCommand("a=T,f=24,s=1,v=1,i=71,m=1", ""))
	if singleAPC {
		term.Write([]byte("\x1b_Gm=1;"))
		chunk := []byte(strings.Repeat("A", 1<<20))
		for range 66 {
			term.Write(chunk)
		}
		term.Write([]byte("\x1b\\"))
	} else {
		chunk := kittyEncodedCommand("m=1", strings.Repeat("A", 1<<20))
		for range 534 {
			term.Write(chunk)
		}
	}
	if replies.Len() != 0 {
		t.Fatal("budget failure replied before terminator", replies.String())
	}
	term.Write(kittyEncodedCommand("a=q,i=999", "BwgJ"))
	if replies.String() != "\x1b_Gi=71;EINVAL: bad payload\x1b\\" {
		t.Fatal("budget failure forgot first command", replies.String())
	}
	g := term.Graphics(10, 20)
	if len(g) != 1 || g[0].Image.Pix[0] != 255 {
		t.Fatal("overflow replaced existing pixels", g)
	}
	replies.Reset()
	term.Write(kittyCommand("a=q,f=24,s=1,v=1,i=72", []byte{1, 2, 3}))
	if replies.String() != "\x1b_Gi=72;OK\x1b\\" {
		t.Fatal("budget failure did not recover", replies.String())
	}
}

func TestKittyPendingQueryAndDelete(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyEncodedCommand("a=q,f=24,s=2,v=1,i=71,m=1", "BwgJ"))
	if replies.Len() != 0 {
		t.Fatal("query replied before final chunk", replies.String())
	}
	term.Write(kittyEncodedCommand("a=T,m=0", "CgsM"))
	if replies.String() != "\x1b_Gi=71;OK\x1b\\" || len(term.Graphics(10, 20)) != 0 {
		t.Fatal("pending query changed action", replies.String())
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=71", nil))
	if !strings.Contains(replies.String(), "ENOENT") {
		t.Fatal("query populated cache", replies.String())
	}
	for _, action := range []string{"d", "D"} {
		for _, more := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/m%d", action, more), func(t *testing.T) {
				term, replies := kittyCacheTerminal(t)
				term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,c=1,r=1,C=1", []byte{255, 0, 0}))
				replies.Reset()
				term.Write(kittyEncodedCommand("a=T,f=24,s=2,v=1,i=71,m=1", "BwgJ"))
				term.Write(kittyEncodedCommand(fmt.Sprintf("a=%s,d=I,i=7,m=%d", action, more), "!"))
				if more == 1 {
					if len(term.Graphics(10, 20)) != 1 {
						t.Fatal("pending delete ran early")
					}
					term.Write(kittyEncodedCommand("a=T,i=99", "?"))
				}
				if len(term.Graphics(10, 20)) != 0 || replies.Len() != 0 {
					t.Fatal("delete did not replace pending upload", replies.String())
				}
			})
		}
	}
}

func TestKittyBase64WholeTransfer(t *testing.T) {
	for _, tc := range []struct {
		name, first, last string
		valid             bool
	}{
		{"ignored punctuation", "B!w", "gJ\tC?gsM", true},
		{"padding only at end", "Bwg", "J", true},
		{"intermediate padding", "Bw==", "CAk=", false},
		{"unfinished quartet", "B", "w", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, replies := kittyCacheTerminal(t)
			width := 1
			if tc.name == "ignored punctuation" {
				width = 2
			}
			term.Write(kittyEncodedCommand(fmt.Sprintf("a=T,f=24,s=%d,v=1,i=71,c=1,r=1,C=1,m=1", width), tc.first))
			term.Write(kittyEncodedCommand("m=0", tc.last))
			if tc.valid {
				if !strings.Contains(replies.String(), ";OK") || len(term.Graphics(10, 20)) != 1 {
					t.Fatal("valid whole payload failed", replies.String())
				}
			} else if !strings.Contains(replies.String(), "EINVAL") || len(term.Graphics(10, 20)) != 0 {
				t.Fatal("invalid whole payload accepted or silently dropped", replies.String())
			}
			// Failure must release the pending command and leave later input usable.
			replies.Reset()
			term.Write(kittyCommand("a=q,f=24,s=1,v=1,i=72", []byte{1, 2, 3}))
			if replies.String() != "\x1b_Gi=72;OK\x1b\\" {
				t.Fatal("pending payload leaked into next query", replies.String())
			}
		})
	}
}

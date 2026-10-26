package vt

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"unsafe"
)

func TestKittyOutOfRangeReplacesOnlySelectedPlacement(t *testing.T) {
	for _, format := range []string{"rgb", "png"} {
		for _, transmit := range []bool{false, true} {
			for _, crop := range []bool{false, true} {
				for _, quiet := range []bool{false, true} {
					for _, offset := range []int{-100, 100} {
						t.Run(fmt.Sprintf("%s/transmit%v/crop%v/quiet%v/V%d", format, transmit, crop, quiet, offset), func(t *testing.T) {
							term, replies := graphicTerminal(t)
							term.Write([]byte("\x1b[3;4H"))
							term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0}))
							control, data := "f=24,s=2,v=1", []byte{0, 255, 0, 0, 255, 0}
							if format == "png" {
								control = "f=100"
								data, _ = kittyFormatFixture(t, "png")
							}
							term.Write(kittyCommand("a=T,i=8,p=2,c=1,r=1,C=1,"+control, data))
							term.Write(kittyCommand("a=p,i=8,p=3,c=1,r=1,C=1", nil))
							term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=9,p=1,P=8,Q=2,H=1,V=1,C=1", []byte{0, 0, 255}))
							before := term.Graphics(10, 20)
							if len(before) != 3 {
								t.Fatal("fixture missing placements")
							}
							x, y := term.Cursor()
							command := fmt.Sprintf("a=p,i=8,p=2,P=7,Q=1,V=%d", offset)
							var payload []byte
							if transmit {
								command = strings.Replace(command, "a=p", "a=T", 1) + "," + control
								payload = data
							}
							if crop {
								command += ",x=1,w=1,h=1"
							}
							if quiet {
								command += ",q=1"
							}
							replies.Reset()
							for _, b := range kittyCommand(command, payload) {
								term.Write([]byte{b})
							}
							want := "\x1b_Gi=8;EINVAL: placement out of range\x1b\\"
							if quiet {
								want = ""
							}
							if replies.String() != want {
								t.Fatal("wrong range reply", replies.String())
							}
							after := term.Graphics(10, 20)
							if len(after) != 2 {
								t.Fatal("range failure did not remove exactly one placement", len(after))
							}
							for _, g := range after {
								if g.logicalImageID == 8 && g.PlacementID == 2 {
									t.Fatal("old selected raster retained")
								}
								found := false
								for _, old := range before {
									if g.ImageID == old.ImageID {
										found = true
										if g.Image != old.Image || g.X != old.X || g.Y != old.Y {
											t.Fatal("sibling or descendant changed")
										}
									}
								}
								if !found {
									t.Fatal("range failure created a raster")
								}
							}
							if cx, cy := term.Cursor(); cx != x || cy != y {
								t.Fatal("range failure changed cursor")
							}
							replies.Reset()
							term.Write(kittyCommand("a=p,i=9,p=4,P=8,Q=2", nil))
							if replies.String() != "\x1b_Gi=9;ENOPARENT: parent placement not found\x1b\\" {
								t.Fatal("old logical record survived", replies.String())
							}
							term.Write(kittyCommand("a=p,i=8,p=2,c=1,r=1,C=1", nil))
							if len(term.Graphics(10, 20)) != 3 {
								t.Fatal("range failure removed cached payload")
							}
						})
					}
				}
			}
		}
	}
}

func TestKittyOutOfRangeUnaddressedPlacementAndNumber(t *testing.T) {
	for _, tc := range []struct {
		identity, placement, reply string
		remains                    bool
	}{
		{"i=8", "", "i=8", true}, {"i=8", ",p=0", "i=8", true},
		{"I=70", ",p=2", "I=70", false},
	} {
		term, replies := graphicTerminal(t)
		term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0}))
		term.Write(kittyCommand("a=T,f=24,s=1,v=1,"+tc.identity+",p=2,C=1", []byte{0, 255, 0}))
		old := term.Graphics(10, 20)
		replies.Reset()
		term.Write(kittyCommand("a=p,"+tc.identity+tc.placement+",P=7,Q=1,V=-100", nil))
		if replies.String() != "\x1b_G"+tc.reply+";EINVAL: placement out of range\x1b\\" {
			t.Fatal("identity changed", replies.String())
		}
		g := term.Graphics(10, 20)
		if tc.remains {
			if len(g) != 1 || g[0].Image != old[0].Image {
				t.Fatal("unaddressed failure removed existing placement")
			}
		} else if len(g) != 0 {
			t.Fatal("number lookup did not remove resolved placement")
		}
	}
}

func TestKittyOutOfRangeChecksBeforeUncroppedPixelDecode(t *testing.T) {
	for _, crop := range []bool{false, true} {
		term, replies := graphicTerminal(t)
		memory := func() uint64 {
			var usage struct {
				size       uintptr
				compressed bool
				fields     [12]uint64
			}
			usage.size = unsafe.Sizeof(usage)
			if err := term.get(42, unsafe.Pointer(&usage)); err != nil {
				t.Fatal(err)
			}
			return usage.fields[5] + usage.fields[11]
		}
		term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0}))
		data, raw := kittyFormatFixture(t, "png")
		term.Write(kittyCommand("a=T,f=100,i=8,p=2,U=1", data))
		before := memory()
		control := "a=p,i=8,p=2,P=7,Q=1,V=-100"
		if crop {
			control += ",x=1,w=1,h=1"
		}
		replies.Reset()
		term.Write(kittyCommand(control, nil))
		if replies.String() != "\x1b_Gi=8;EINVAL: placement out of range\x1b\\" {
			t.Fatal(replies.String())
		}
		want := before
		if crop {
			want += uint64(raw.Bounds().Dx() * raw.Bounds().Dy() * 4)
		}
		if memory() != want {
			t.Fatal("wrong decode stage", crop, memory(), want)
		}
	}
}

func TestKittyOutOfRangeRemovesHistoryPlacement(t *testing.T) {
	term, replies := graphicTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0}))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=2,C=1", []byte{0, 255, 0}))
	term.Write([]byte("\x1b[10;1H" + strings.Repeat("\n", 20)))
	term.ScrollToTop()
	if len(term.Graphics(10, 20)) != 1 {
		t.Fatal("fixture not retained in history")
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=8,p=2,P=7,Q=1", nil))
	if replies.String() != "\x1b_Gi=8;EINVAL: placement out of range\x1b\\" {
		t.Fatal("historical origin not rejected", replies.String())
	}
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("failed replacement left historical raster")
	}
}

func TestKittyOutOfRangeFailurePrecedence(t *testing.T) {
	for _, tc := range []struct {
		control, message string
		data             []byte
		removed          bool
	}{
		{"a=p,i=8,p=2,P=7,Q=1,V=-100,x=99", "EINVAL: bad crop", nil, false},
		{"a=T,f=100,i=8,p=2,P=7,Q=1,V=-100,x=1", "ENOTSUP: cannot crop png", kittyMetadataPNG(30, 10), false},
		{"a=T,f=100,i=8,p=2,P=7,Q=1,V=-100", "EINVAL: placement out of range", kittyMetadataPNG(30, 10), true},
		{"a=T,f=100,i=8,p=2,P=99,Q=1,V=-100,x=1", "ENOPARENT: parent placement not found", kittyMetadataPNG(30, 10), false},
		{"a=p,i=8,p=2,P=7,Q=1,V=-100,U=1", "EINVAL: virtual placement cannot refer to parent", nil, false},
	} {
		t.Run(tc.control, func(t *testing.T) {
			term, replies := graphicTerminal(t)
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0}))
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=2,C=1", []byte{0, 255, 0}))
			old := term.Graphics(10, 20)
			replies.Reset()
			term.Write(kittyCommand(tc.control, tc.data))
			if replies.String() != "\x1b_Gi=8;"+tc.message+"\x1b\\" {
				t.Fatal("failure precedence", replies.String())
			}
			g := term.Graphics(10, 20)
			if tc.removed {
				if len(g) != 0 {
					t.Fatal("out-of-range placement kept old raster")
				}
			} else if len(g) != 1 || g[0].Image != old[0].Image {
				t.Fatal("earlier validation removed old raster")
			}
		})
	}
}

func TestKittyOutOfRangeRemovesCrossScreenRecord(t *testing.T) {
	for _, virtual := range []bool{false, true} {
		term, replies := graphicTerminal(t)
		control := "a=T,f=24,s=1,v=1,i=8,p=2,C=1"
		if virtual {
			control += ",U=1"
		}
		term.Write(kittyCommand(control, []byte{0, 255, 0}))
		term.Write(kittyCommand("a=p,i=8,p=3,C=1", nil))
		term.Write([]byte("\x1b[?47h"))
		term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0}))
		replies.Reset()
		term.Write(kittyCommand("a=p,i=8,p=2,P=7,Q=1,V=-1", nil))
		if replies.String() != "\x1b_Gi=8;EINVAL: placement out of range\x1b\\" {
			t.Fatal(replies.String())
		}
		term.Write([]byte("\x1b[?47l"))
		g := term.Graphics(10, 20)
		if len(g) != 1 || g[0].PlacementID != 3 {
			t.Fatal("cross-screen removal did not preserve sibling", virtual, g)
		}
		replies.Reset()
		term.Write(kittyCommand("a=p,i=8,p=4,P=8,Q=2", nil))
		if replies.String() != "\x1b_Gi=8;ENOPARENT: parent placement not found\x1b\\" {
			t.Fatal("cross-screen old record remains", virtual, replies.String())
		}
	}
}

func TestKittyOutOfRangeTransmitKeepsNewCache(t *testing.T) {
	term, replies := graphicTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0}))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=2,C=1", []byte{0, 255, 0}))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=8,p=2,P=7,Q=1,V=100", []byte{0, 0, 255}))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("range failure kept old pixels")
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=8,p=2,C=1", nil))
	g := term.Graphics(10, 20)
	if len(g) != 1 || !bytes.Equal(g[0].Image.Pix, []byte{0, 0, 255, 255}) {
		t.Fatal("range failure rolled back cached payload", replies.String())
	}
}

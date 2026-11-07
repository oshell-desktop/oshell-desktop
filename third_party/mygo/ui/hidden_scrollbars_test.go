package ui

import "testing"

func TestHiddenScrollbarsKeepWheelAndReveal(t *testing.T) {
	var offset ScrollState
	reveal := false
	clicks := 0
	tt := NewTester(func(c *Context) {
		ScrollHorizontal(c).HideScrollbars().TrackScroll(&offset).Size(100, 40).Children(func() {
			Row(c).Children(func() {
				Box(c).Size(500, 40).Background(Hex("#ffffff")).OnClick(func() { clicks++ })
				last := Button(c, "last").Size(100, 40)
				if reveal {
					last.ScrollIntoView()
				}
			})
		})
	}, 200, 100)
	tt.Move(10, 34)
	pixel := tt.Image().RGBAAt(10, 34)
	if pixel.R != 255 || pixel.G != 255 || pixel.B != 255 {
		t.Fatalf("hidden thumb still painted: %v", pixel)
	}
	tt.ClickAt(10, 34)
	if clicks != 1 || offset.X != 0 {
		t.Fatal("hidden scrollbar intercepted content click")
	}
	tt.Move(50, 20)
	tt.rt.scroll(0, 70, Shift, false)
	tt.Frame()
	if offset.X != 70 {
		t.Fatalf("Shift wheel did not scroll horizontally: %v", offset)
	}
	tt.Scroll(50, 20, 50, 0)
	if offset.X != 120 {
		t.Fatal("native horizontal wheel stopped working", offset)
	}
	reveal = true
	tt.Frame()
	if offset.X < 490 {
		t.Fatal("requested reveal stopped working", offset)
	}
}

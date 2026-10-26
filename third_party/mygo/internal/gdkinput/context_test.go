package gdkinput

import "testing"

func TestContextRetiredSignalsCannotSelectNewTarget(t *testing.T) {
	var context Context
	retired := []uintptr{}
	retire := func(old uintptr) {
		if context.Accept(old) {
			t.Fatal("retirement callback still owns input")
		}
		retired = append(retired, old)
	}
	context.Replace(1, retire)
	context.Replace(2, retire)
	for _, old := range []uintptr{0, 1, 3} {
		if context.Accept(old) {
			t.Fatalf("stale signal %d accepted", old)
		}
	}
	if !context.Accept(2) || len(retired) != 1 || retired[0] != 1 {
		t.Fatal("new target not activated exactly once")
	}
	context.Replace(2, retire)
	context.Replace(0, retire)
	context.Replace(0, retire)
	if context.Accept(2) || len(retired) != 2 || retired[1] != 2 {
		t.Fatal("close retained identity or released it twice")
	}
}

func TestContextReplacementReentrancyDoesNotRestoreOldTarget(t *testing.T) {
	var context Context
	context.Replace(1, func(uintptr) {})
	context.Replace(2, func(old uintptr) {
		context.Replace(3, func(nested uintptr) {
			if nested != 2 {
				t.Fatal("nested retirement lost new context")
			}
		})
	})
	if context.Handle() != 3 {
		t.Fatal("outer replacement overwrote reentrant target")
	}
}

func TestContextFilterOwnsNativeLifetimeAndStopsOldKey(t *testing.T) {
	for _, mode := range []string{"keep", "switch", "close", "panic"} {
		t.Run(mode, func(t *testing.T) {
			var context Context
			refs := map[uintptr]int{1: 1, 2: 1}
			release := func(im uintptr) {
				refs[im]--
				if refs[im] < 0 {
					t.Fatal("double release")
				}
			}
			context.Replace(1, release)
			taken := false
			func() {
				defer func() {
					if got := recover(); (got != nil) != (mode == "panic") {
						t.Fatalf("unexpected panic state %v", got)
					}
				}()
				taken = context.Filter(func(im uintptr) bool {
					if im != 1 || refs[1] != 2 {
						t.Fatal("filter lacks native reference")
					}
					if mode == "switch" {
						context.Replace(2, release)
					}
					if mode == "close" || mode == "panic" {
						context.Replace(0, release)
					}
					if refs[1] == 0 {
						t.Fatal("native object freed inside its callback")
					}
					if mode == "panic" {
						panic("synthetic native failure")
					}
					return false
				}, func(im uintptr) { refs[im]++ }, release)
			}()
			if mode == "keep" {
				if taken || refs[1] != 1 {
					t.Fatal("ordinary unhandled key or reference changed")
				}
			} else if refs[1] != 0 || (mode != "panic" && !taken) {
				t.Fatal("old key escaped or old native reference leaked")
			}
		})
	}
}

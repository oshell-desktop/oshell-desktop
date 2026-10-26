package gdkinput

import (
	"github.com/egoist/mygo/internal/platform"
	"testing"
)

func TestFunctionKeys(t *testing.T) {
	for n := 1; n <= 35; n++ {
		want := platform.KeyF1 + platform.Key(n-1)
		if n > 12 {
			want = platform.KeyF13 + platform.Key(n-13)
		}
		if got := FunctionKey(0xffbd + uint32(n)); got != want {
			t.Fatalf("F%d: %d, want %d", n, got, want)
		}
	}
	for _, symbol := range []uint32{0xffbd, 0xffe1, 0xff91, 'f'} {
		if FunctionKey(symbol) != platform.KeyUnknown {
			t.Fatalf("non-function symbol %x", symbol)
		}
	}
}

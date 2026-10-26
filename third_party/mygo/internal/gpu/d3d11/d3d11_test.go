//go:build windows

package d3d11

import (
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/egoist/mygo/internal/gpu"
	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/scene"
)

// hiddenWindow creates a window that never shows, for a swap chain.
func hiddenWindow(t *testing.T, w, h int) uintptr {
	hwnd, err := newHiddenWindow(w, h)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { destroyWindow(hwnd) })
	return hwnd
}

// readBack copies the back buffer to memory: BGRA rows and their stride.
func (r *Renderer) readBack(t *testing.T) ([]byte, int) {
	pix, stride, err := r.read()
	if err != nil {
		t.Fatal(err)
	}
	return pix, stride
}

func TestDrawsAsTheCPURenderer(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// With the shaders compiled ahead of time, and with those compiled
	// from shader.hlsl when the renderer starts, as when they are older.
	for _, compile := range []bool{false, true} {
		compileShaders = compile
		s := gputest.Scene()
		r, err := New(hiddenWindow(t, s.Width, s.Height))
		compileShaders = false
		if err != nil {
			t.Skip("no Direct3D 11:", err)
		}
		// Twice: the second frame updates what the first uploaded.
		for frame := range 2 {
			if err := r.draw(s); err != nil {
				t.Fatal(err)
			}
			pix, stride := r.readBack(t)
			gputest.Compare(t, "d3d11", pix, stride, s)
			if err := r.present(1); err != nil {
				t.Fatalf("frame %d: %v", frame, err)
			}
		}
		r.Release()
	}
}

func TestFractionalImageRegionsMatchSoftware(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pixels := make([]byte, 8*6*4)
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			i := (y*8 + x) * 4
			pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = uint8(x*31), uint8(y*43), uint8((x+y)*17), 255
		}
	}
	image := scene.NewImageRGBA(8, 6, pixels)
	s := &scene.Scene{Width: 80, Height: 40, Clear: scene.Color{B: 255, A: 255}}
	for _, region := range []struct{ dst, src scene.Rect }{
		{scene.Rect{X: 2, Y: 2, W: 24, H: 16}, scene.Rect{X: 1, Y: .75, W: 6, H: 4.5}},
		{scene.Rect{X: 32, Y: 2, W: 12, H: 16}, scene.Rect{X: 1, Y: .75, W: 3, H: 4.5}},
		{scene.Rect{X: 44, Y: 2, W: 12, H: 16}, scene.Rect{X: 4, Y: .75, W: 3, H: 4.5}},
		{scene.Rect{X: 62, Y: 2, W: 5, H: 20}, scene.Rect{W: 1e-7, H: 1e-7}},
	} {
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpImage, Rect: region.dst, Src: region.src, Image: image, FullImageSampling: true})
	}
	r, err := New(hiddenWindow(t, s.Width, s.Height))
	if err != nil {
		t.Skip("no Direct3D 11:", err)
	}
	defer r.Release()
	if err := r.draw(s); err != nil {
		t.Fatal(err)
	}
	pix, stride := r.readBack(t)
	gputest.Compare(t, "fractional image regions", pix, stride, s)
}

// TestResizeSettles checks that while the window changes size the swap
// chain draws into part of larger buffers, resized only when the window
// outgrows them, and that once the settle timer asked for a frame, the
// frame gives it buffers of the window's size again. The window's swap
// chain shows that part of its buffers; one for composition, which would
// stretch the part over the buffers' size, shows them whole.
func TestResizeSettles(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func(d time.Duration) { settleDelay = d }(settleDelay)
	settleDelay = 50 * time.Millisecond
	for _, newRenderer := range []func(uintptr) (*Renderer, error){New, NewComposed} {
		r, err := newRenderer(hiddenWindow(t, 200, 150))
		if err != nil {
			t.Skip("no Direct3D 11:", err)
		}
		resizeSettles(t, r)
		r.Release()
	}
}

func resizeSettles(t *testing.T, r *Renderer) {
	t.Helper()
	kind := "window"
	if r.composed {
		kind = "composition"
	}
	frame := func(w, h int) {
		t.Helper()
		if err := r.draw(&scene.Scene{Width: w, Height: h}); err != nil {
			t.Fatal(err)
		}
		if err := r.present(0); err != nil {
			t.Fatal(err)
		}
		if r.swapChain2 == 0 {
			return
		}
		const scGetSourceSize = 30 // IDXGISwapChain2
		var sw, sh uint32
		call(r.swapChain2, scGetSourceSize, uintptr(unsafe.Pointer(&sw)), uintptr(unsafe.Pointer(&sh)))
		want := [2]int{r.w, r.h}
		if r.composed {
			want = [2]int{r.bw, r.bh}
		}
		if got := [2]int{int(sw), int(sh)}; got != want {
			t.Fatalf("%s: a %dx%d frame in %dx%d buffers shows %dx%d of them; want %dx%d", kind, w, h, r.bw, r.bh, sw, sh, want[0], want[1])
		}
	}
	frame(200, 150)
	if r.swapChain2 == 0 {
		t.Skip("no IDXGISwapChain2")
	}
	if r.bw != 200 || r.bh != 150 || r.resizing {
		t.Fatalf("%s: first frame: buffers %dx%d, resizing %v; want 200x150 and not resizing", kind, r.bw, r.bh, r.resizing)
	}
	frame(220, 160)
	bw, bh := r.bw, r.bh
	if !r.resizing || bw <= 220 || bh <= 160 {
		t.Fatalf("%s: resized: buffers %dx%d, resizing %v; want larger than 220x160 and resizing", kind, bw, bh, r.resizing)
	}
	frame(230, 155)
	if r.bw != bw || r.bh != bh || r.w != 230 || r.h != 155 {
		t.Fatalf("%s: resized within the buffers: buffers %dx%d drawing %dx%d; want %dx%d drawing 230x155", kind, r.bw, r.bh, r.w, r.h, bw, bh)
	}
	// The timer fires as messages are dispatched.
	user32 := syscall.NewLazyDLL("user32.dll")
	peek, dispatch := user32.NewProc("PeekMessageW"), user32.NewProc("DispatchMessageW")
	var msg [64]byte
	for deadline := time.Now().Add(5 * time.Second); settling[r.hwnd]; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%s: the settle timer did not fire", kind)
		}
		for {
			if ok, _, _ := peek.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1); ok == 0 {
				break
			}
			dispatch.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}
	frame(230, 155)
	if r.bw != 230 || r.bh != 155 || r.resizing {
		t.Fatalf("%s: settled: buffers %dx%d, resizing %v; want 230x155 and not resizing", kind, r.bw, r.bh, r.resizing)
	}
}

// TestShaderBytecode checks that the shaders compiled ahead of time come
// from shader.hlsl as it is, and that the compiler renderers fall back to
// compiles it.
func TestShaderBytecode(t *testing.T) {
	if gpu.SourceSum(shaderSource) != shaderSum {
		t.Fatal("shader.hlsl changed since shaders.go was generated: run go generate ./internal/gpu/d3d11 on Windows")
	}
	for _, s := range []struct{ entry, target string }{{"vs", "vs_4_0"}, {"ps", "ps_4_0"}} {
		code, err := compileShader(s.entry, s.target)
		if err != nil {
			t.Fatal(err)
		}
		// DXBC starts with its magic.
		if len(code) < 4 || string(code[:4]) != "DXBC" {
			t.Errorf("%s: not DXBC", s.entry)
		}
	}
}

// TestComposed draws into a swap chain for DirectComposition, which shows
// on the window with its alpha: frames draw as into the window's own, what
// a scene leaves transparent stays so, and once a renderer is released
// another composes the window, as after a GPU failure: a window has one
// DirectComposition target at most.
func TestComposed(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	s := gputest.Scene()
	hwnd := hiddenWindow(t, s.Width, s.Height)
	for i := range 2 {
		r, err := NewComposed(hwnd)
		if err != nil {
			t.Skip("no Direct3D 11:", err)
		}
		if err := r.draw(s); err != nil {
			r.Release()
			t.Fatalf("renderer %d: %v", i, err)
		}
		if r.target == 0 || r.visual == 0 {
			t.Fatalf("renderer %d: the swap chain shows through no DirectComposition target", i)
		}
		pix, stride := r.readBack(t)
		gputest.Compare(t, "d3d11 composed", pix, stride, s)
		if err := r.present(1); err != nil {
			t.Fatal(err)
		}
		// A frame of another size, transparent.
		if err := r.draw(&scene.Scene{Width: s.Width + 20, Height: s.Height + 10}); err != nil {
			t.Fatal(err)
		}
		if pix, _ := r.readBack(t); len(pix) < 4 || pix[0]|pix[1]|pix[2]|pix[3] != 0 {
			t.Errorf("renderer %d: a transparent frame reads %v", i, pix[:min(len(pix), 4)])
		}
		if err := r.present(0); err != nil {
			t.Fatal(err)
		}
		r.Release()
	}
}

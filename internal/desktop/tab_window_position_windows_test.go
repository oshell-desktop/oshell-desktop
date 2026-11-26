package desktop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
	"oshell-desktop/internal/core"
)

// App.Run needs the initial OS thread. Use a private Win32 desktop so the
// fixture can exercise native window stacking without moving the user's mouse.
func init() {
	if os.Getenv("OSHELL_TEST_NATIVE_TAB_RETURN") != "1" {
		return
	}
	if err := nativeTabReturnFixture(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func nativeTabReturnFixture() error {
	name, _ := syscall.UTF16PtrFromString(fmt.Sprintf("OShellTabReturn-%d", os.Getpid()))
	desktop, _, err := tabPositionUser32.NewProc("CreateDesktopW").Call(uintptr(unsafe.Pointer(name)), 0, 0, 0, 0x1ff, 0)
	if desktop == 0 {
		return fmt.Errorf("create fixture desktop: %w", err)
	}
	if ok, _, err := tabPositionUser32.NewProc("SetThreadDesktop").Call(desktop); ok == 0 {
		return fmt.Errorf("select fixture desktop: %w", err)
	}
	var failure error
	mygo.App.WhenReady(func() {
		store := &core.Store{Dir: os.Getenv("OSHELL_DATA_DIR")}
		config := core.DefaultConfig()
		config.Settings.AutomaticUpdateChecks = false
		original := newApp(store, config)
		h := &applicationHub{store: store, vault: original.vault, config: config}
		h.attach(original)
		defer mygo.App.Quit()
		original.newBlank()
		original.newBlank()
		moving := original.activeTab()
		original.detachTabAt(moving, &tabWindowPosition{400, 160})
		if len(h.windows) != 2 || h.windows[1].activeTab() != moving {
			failure = fmt.Errorf("failed to detach fixture tab")
			return
		}
		detached := h.windows[1]
		// Put the original in front. Its native caption is outside the
		// content surface registered with OLE.
		tabSetWindowPos.Call(original.win.NativeHandle(), 0, 20, 20, 0, 0, 0x11) // NOSIZE | NOACTIVATE
		captureTabWindowPosition = func() *tabWindowPosition { return &tabWindowPosition{100, 36} }
		detached.queueDetachedTab(moving)
		detached.drain()
		if len(h.windows) != 1 || h.windows[0] != original || len(original.tabs) != 2 ||
			original.activeTab() != moving || moving.active.owner.Load() != original || !detached.stopped.Load() {
			failure = fmt.Errorf("caption drop did not return the tab and close the empty detached window")
		}
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	return failure
}

func TestNativeDetachedTabReturn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "OSHELL_TEST_NATIVE_TAB_RETURN=1", "OSHELL_DATA_DIR="+t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native tab return: %v\n%s", err, out)
	}
}

func TestDetachedWindowPositionUsesExactScreenOrigin(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Exercise real Win32 positioning on our own hidden window. This includes
	// valid negative coordinates and the origin, which must not mean center.
	class, _ := syscall.UTF16PtrFromString("STATIC")
	handle, _, err := tabPositionUser32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), 0, 0x00CF0000, 40, 40, 320, 240, 0, 0, 0, 0)
	if handle == 0 {
		t.Fatal(err)
	}
	defer tabPositionUser32.NewProc("DestroyWindow").Call(handle)
	getRect := tabPositionUser32.NewProc("GetWindowRect")
	for _, position := range []tabWindowPosition{{480, 320}, {0, 0}, {-1200, 200}, {500, -600}} {
		if err := position.applyHandle(handle); err != nil {
			t.Fatal(err)
		}
		var rect struct{ left, top, right, bottom int32 }
		if ok, _, err := getRect.Call(handle, uintptr(unsafe.Pointer(&rect))); ok == 0 {
			t.Fatal(err)
		}
		if int(rect.left) != position.x || int(rect.top) != position.y {
			t.Fatalf("window origin = (%d,%d), want %+v", rect.left, rect.top, position)
		}
		if rect.right-rect.left != 320 || rect.bottom-rect.top != 240 {
			t.Fatal("positioning resized the window")
		}
	}
}

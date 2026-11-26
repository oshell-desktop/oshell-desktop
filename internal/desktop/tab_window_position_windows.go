package desktop

import (
	"fmt"
	"github.com/egoist/mygo"
	"syscall"
	"unsafe"
)

var tabPositionUser32 = syscall.NewLazyDLL("user32.dll")
var tabGetCursorPos = tabPositionUser32.NewProc("GetCursorPos")
var tabSetWindowPos = tabPositionUser32.NewProc("SetWindowPos")
var tabGetWindowRect = tabPositionUser32.NewProc("GetWindowRect")
var tabWindowFromPoint = tabPositionUser32.NewProc("WindowFromPoint")
var tabGetAncestor = tabPositionUser32.NewProc("GetAncestor")
var tabIsWindowEnabled = tabPositionUser32.NewProc("IsWindowEnabled")

func (p tabWindowPosition) windowAtRelease(workspaces []*App) *App {
	point := uint64(uint32(p.x)) | uint64(uint32(p.y))<<32
	child, _, _ := tabWindowFromPoint.Call(uintptr(point))
	const gaRoot = 2 // Do not follow owners of dialogs or other auxiliary windows.
	root, _, _ := tabGetAncestor.Call(child, gaRoot)
	if root == 0 {
		return nil
	}
	if enabled, _, _ := tabIsWindowEnabled.Call(root); enabled == 0 {
		return nil
	}
	for _, dest := range workspaces {
		if dest.win != nil && dest.win.NativeHandle() == root && dest.win.IsVisible() && !dest.win.IsMinimized() {
			return dest
		}
	}
	return nil
}

func (p tabWindowPosition) within(window *mygo.Window) bool {
	var rect struct{ left, top, right, bottom int32 }
	if ok, _, _ := tabGetWindowRect.Call(window.NativeHandle(), uintptr(unsafe.Pointer(&rect))); ok == 0 {
		return false
	}
	return p.x >= int(rect.left) && p.x < int(rect.right) && p.y >= int(rect.top) && p.y < int(rect.bottom)
}

func currentTabWindowPosition() *tabWindowPosition {
	var point struct{ x, y int32 }
	if ok, _, _ := tabGetCursorPos.Call(uintptr(unsafe.Pointer(&point))); ok == 0 {
		return nil
	}
	return &tabWindowPosition{x: int(point.x), y: int(point.y)}
}

func (p tabWindowPosition) apply(window *mygo.Window) error {
	return p.applyHandle(window.NativeHandle())
}

func (p tabWindowPosition) applyHandle(handle uintptr) error {
	// Cursor and destination must use the same physical screen coordinates.
	// The framework's DIP position uses the window's current monitor scale,
	// which may differ from the monitor where the tab was released.
	const flags = 0x0001 | 0x0004 | 0x0010 // NOSIZE | NOZORDER | NOACTIVATE
	for range 2 {
		// A cross-monitor move can synchronously process WM_DPICHANGED and its
		// suggested bounds. Reapply the origin after that DPI transition.
		if ok, _, err := tabSetWindowPos.Call(handle, 0, uintptr(p.x), uintptr(p.y), 0, 0, flags); ok == 0 {
			return fmt.Errorf("SetWindowPos: %w", err)
		}
	}
	return nil
}

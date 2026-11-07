//go:build windows && (amd64 || arm64)

package windows

import (
	"sync"
	"syscall"
)

// DWM's dark title bar setting does not affect native popup menus. UxTheme
// needs both a process preference and a window opt-in. These optional exports
// are also used by Chromium's base/win/dark_mode_support.cc; ordinal 135 has
// a different signature before Windows 10 1903, so never call it there.
var menuThemeDLL = systemDLL("uxtheme.dll")

type menuThemeAPI struct {
	setPreferredMode, allowWindow, flushMenus uintptr
}

var nativeMenuTheme = sync.OnceValue(func() menuThemeAPI {
	if windowsBuild() < 18362 || menuThemeDLL.Load() != nil {
		return menuThemeAPI{}
	}
	lookup := func(ordinal uintptr) uintptr {
		addr, _, _ := procGetProcAddress.Call(menuThemeDLL.Handle(), ordinal)
		return addr
	}
	api := menuThemeAPI{setPreferredMode: lookup(135), allowWindow: lookup(133), flushMenus: lookup(136)}
	if api.setPreferredMode == 0 || api.allowWindow == 0 || api.flushMenus == 0 {
		return menuThemeAPI{}
	}
	return api
})

func (b *Backend) applyNativeMenuTheme(owner uintptr, dark bool) {
	api := nativeMenuTheme()
	if api.setPreferredMode == 0 {
		return // Older systems keep their native menu rather than failing startup.
	}
	mode, allow := uintptr(3), uintptr(0) // ForceLight, including a light override on a dark OS.
	if dark {
		mode, allow = 2, 1 // ForceDark, including a dark override on a light OS.
	}
	if (theme{b}).Preferences().HighContrast {
		mode, allow = 0, 0 // Leave accessibility contrast colors to Windows.
	}
	syscall.SyscallN(api.setPreferredMode, mode)
	if owner != 0 {
		syscall.SyscallN(api.allowWindow, owner, allow)
	}
	// Invalidate the native menu cache when switching appearance at runtime.
	syscall.SyscallN(api.flushMenus)
}

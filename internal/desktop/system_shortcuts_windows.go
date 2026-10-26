//go:build windows

package desktop

import (
	"golang.org/x/sys/windows"
	"oshell-desktop/internal/core"
	"runtime"
	"strconv"
	"strings"
)

var hotKeyDLL = windows.NewLazySystemDLL("user32.dll")
var registerHotKey = hotKeyDLL.NewProc("RegisterHotKey")
var unregisterHotKey = hotKeyDLL.NewProc("UnregisterHotKey")

func probeSystemShortcut(key core.KeyboardShortcut) systemShortcutResult {
	mods := key.Modifiers
	if mods&1 != 0 {
		mods = mods&^1 | 4
	}
	name := core.ShortcutKeyNames[key.KeyCode]
	if mods == 2 && (name == "Tab" || name == "Escape" || name == "F4") || mods == 12 && name == "Escape" || name == "F12" {
		return systemShortcutResult{"Windows 保留组合", true}
	}
	vk := uintptr(0)
	if len(name) == 1 {
		vk = map[string]uintptr{";": 0xba, "=": 0xbb, ",": 0xbc, "-": 0xbd, ".": 0xbe, "/": 0xbf, "`": 0xc0, "[": 0xdb, "\\": 0xdc, "]": 0xdd, "'": 0xde}[name]
		if vk == 0 && (name[0] >= 'A' && name[0] <= 'Z' || name[0] >= '0' && name[0] <= '9') {
			vk = uintptr(name[0])
		}
	} else if strings.HasPrefix(name, "F") {
		n, err := strconv.Atoi(name[1:])
		if err == nil && n >= 1 && n <= 20 {
			vk = uintptr(0x70 + n - 1)
		}
	}
	if vk == 0 {
		vk = map[string]uintptr{"Tab": 9, "Enter": 13, "Space": 32, "Backspace": 8, "Escape": 27, "Left": 37, "Up": 38, "Right": 39, "Down": 40, "Home": 36, "End": 35, "PageUp": 33, "PageDown": 34}[name]
	}
	if vk == 0 {
		return systemShortcutResult{}
	}
	native := uintptr(0x4000) // MOD_NOREPEAT; registration is immediately released.
	if mods&2 != 0 {
		native |= 1
	}
	if mods&4 != 0 {
		native |= 2
	}
	if mods&8 != 0 {
		native |= 4
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const id = 0x6f53
	ok, _, err := registerHotKey.Call(0, id, native, vk)
	if ok != 0 {
		unregisterHotKey.Call(0, id)
		return systemShortcutResult{readable: true}
	}
	if err == windows.ERROR_HOTKEY_ALREADY_REGISTERED {
		return systemShortcutResult{"组合已被系统或全局热键占用", true}
	}
	return systemShortcutResult{}
}

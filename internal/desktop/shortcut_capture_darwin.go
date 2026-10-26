//go:build darwin

package desktop

import (
	"github.com/ebitengine/purego/objc"
	"oshell-desktop/internal/core"
	"strconv"
	"strings"
)

type nativeShortcutMonitor struct {
	token objc.ID
	block objc.Block
}

func extendedMacShortcut(code uint16, flags uint64) (core.KeyboardShortcut, bool) {
	name := core.ShortcutKeyNames[code]
	if !strings.HasPrefix(name, "F") {
		return core.KeyboardShortcut{}, false
	}
	number, e := strconv.Atoi(name[1:])
	if e != nil || number < 13 || number > 20 {
		return core.KeyboardShortcut{}, false
	}
	shortcut := core.KeyboardShortcut{KeyCode: code}
	if flags&(1<<20) != 0 {
		shortcut.Modifiers |= 1
	}
	if flags&(1<<19) != 0 {
		shortcut.Modifiers |= 2
	}
	if flags&(1<<18) != 0 {
		shortcut.Modifiers |= 4
	}
	if flags&(1<<17) != 0 {
		shortcut.Modifiers |= 8
	}
	return shortcut, shortcut.Valid()
}
func (h *applicationHub) initializeNativeShortcutCapture() {
	eventClass := objc.ID(objc.GetClass("NSEvent"))
	if eventClass == 0 {
		return
	}
	h.nativeKeys.block = objc.NewBlock(func(_ objc.Block, event objc.ID) objc.ID {
		code := objc.Send[uint16](event, objc.RegisterName("keyCode"))
		flags := objc.Send[uint64](event, objc.RegisterName("modifierFlags"))
		release := objc.Send[uint64](event, objc.RegisterName("type")) == 11
		if !release && h.captureFullscreenKey(event, code, flags) {
			return 0
		}
		owner := h.settingsOwner
		if owner == nil || owner.passwordLocked() || owner.features.settingsCapture == "" || owner.preferencesWindow == nil || !owner.preferencesWindow.IsFocused() {
			if h.forwardExtendedTerminalKey(event, code, flags, release) {
				return 0
			}
			return event
		}
		shortcut, ok := extendedMacShortcut(code, flags)
		if !ok {
			return event
		}
		if !release && !objc.Send[bool](event, objc.RegisterName("isARepeat")) {
			owner.acceptCapturedShortcut(shortcut)
			owner.preferencesWindow.Invalidate()
		}
		return 0 // Do not execute a saved menu binding while recording.
	})
	token := eventClass.Send(objc.RegisterName("addLocalMonitorForEventsMatchingMask:handler:"), uint64(1<<10|1<<11), h.nativeKeys.block)
	if token == 0 {
		h.nativeKeys.block.Release()
		h.nativeKeys.block = 0
		return
	}
	h.nativeKeys.token = token.Send(objc.RegisterName("retain"))
}
func (h *applicationHub) closeNativeShortcutCapture() {
	if h.nativeKeys.token != 0 {
		objc.ID(objc.GetClass("NSEvent")).Send(objc.RegisterName("removeMonitor:"), h.nativeKeys.token)
		h.nativeKeys.token.Send(objc.RegisterName("release"))
		h.nativeKeys.token = 0
	}
	if h.nativeKeys.block != 0 {
		h.nativeKeys.block.Release()
		h.nativeKeys.block = 0
	}
}

// AppKit may suppress menu key equivalents during a fullscreen transition.
// Observe only this app's configured binding and retain the latest intent.
func (h *applicationHub) captureFullscreenKey(event objc.ID, code uint16, flags uint64) bool {
	if h.settingsOwner != nil && h.settingsOwner.features.settingsCapture != "" {
		return false
	}
	modifiers := 0
	for _, flag := range []struct {
		native uint64
		core   int
	}{{1 << 20, 1}, {1 << 19, 2}, {1 << 18, 4}, {1 << 17, 8}} {
		if flags&flag.native != 0 {
			modifiers |= flag.core
		}
	}
	window := event.Send(objc.RegisterName("window"))
	for _, a := range h.windows {
		if a.win == nil || objc.ID(a.win.NativeHandle()) != window {
			continue
		}
		for _, binding := range a.config.Settings.KeyboardShortcuts.Bindings("focusFullscreen", "darwin") {
			if binding.KeyCode == code && binding.Modifiers == modifiers {
				if !objc.Send[bool](event, objc.RegisterName("isARepeat")) {
					a.toggleFocusFullscreen()
				}
				return true
			}
		}
	}
	return false
}

func (h *applicationHub) forwardExtendedTerminalKey(event objc.ID, code uint16, flags uint64, release bool) bool {
	shortcut, ok := extendedMacShortcut(code, flags)
	if !ok {
		return false
	}
	window := event.Send(objc.RegisterName("window"))
	for _, a := range h.windows {
		if a.win == nil || objc.ID(a.win.NativeHandle()) != window || a.workspaceLocked() || a.settingsOpen || a.modalActive() || a.hasOpenTerminalSearch() {
			continue
		}
		// Configured shortcuts remain owned by their native menu bindings.
		for _, action := range core.ShortcutActions {
			for _, binding := range a.config.Settings.KeyboardShortcuts.Bindings(action.ID, "darwin") {
				if binding.KeyCode == shortcut.KeyCode && binding.Modifiers == shortcut.Modifiers {
					return false
				}
			}
		}
		p := a.activePane()
		if p == nil || p.closed || p.term == nil {
			return false
		}
		number, _ := strconv.Atoi(core.ShortcutKeyNames[code][1:])
		return p.term.SendFunctionKey(number, shortcutMods(shortcut), release, objc.Send[bool](event, objc.RegisterName("isARepeat")))
	}
	return false
}

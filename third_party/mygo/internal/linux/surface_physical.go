//go:build linux && (amd64 || arm64)

package linux

import (
	"github.com/ebitengine/purego"
	"github.com/egoist/mygo/internal/gdkinput"
	"sync"
)

var (
	gdkX11DisplayType, gdkWaylandDisplayType func() uintptr
	gdkX11DisplayXDisplay                    func(ptr) ptr
	xkbGetKeyboard                           func(ptr, uint32, uint32) *gdkinput.XKBDescription
	xkbFreeKeyboard                          func(*gdkinput.XKBDescription, uint32, bool)
	xkbOnce                                  sync.Once
	cbPhysicalKeysChanged                    ptr
)

type physicalKeymap struct {
	keymap ptr
	signal uint64
	loaded bool
	keys   [256]rune
}

func initPhysicalKeys() {
	bind(libGDK, &gdkX11DisplayType, "gdk_x11_display_get_type")
	bind(libGDK, &gdkWaylandDisplayType, "gdk_wayland_display_get_type")
	bind(libGDK, &gdkX11DisplayXDisplay, "gdk_x11_display_get_xdisplay")
	cbPhysicalKeysChanged = purego.NewCallback(func(keymap, data ptr) {
		if s := theBackend.surfaceOf(data); s != nil && s.physical.keymap == keymap {
			s.physical.loaded = false
		}
	})
}

func (s *surface) clearPhysicalKeys() {
	if s.physical.keymap != 0 {
		if s.physical.signal != 0 {
			gSignalHandlerDisconnect(s.physical.keymap, s.physical.signal)
		}
		gObjectUnref(s.physical.keymap)
	}
	s.physical = physicalKeymap{}
}

func (s *surface) physicalKey(display, keymap ptr, code uint16) rune {
	if display == 0 {
		return 0
	}
	if gdkWaylandDisplayType != nil && gTypeCheckInstanceIsA(display, gdkWaylandDisplayType()) {
		return gdkinput.PhysicalWaylandKey(code)
	}
	if keymap == 0 || code >= 256 || gdkX11DisplayType == nil || gdkX11DisplayXDisplay == nil || !gTypeCheckInstanceIsA(display, gdkX11DisplayType()) {
		return 0
	}
	xkbOnce.Do(func() {
		if lib, err := open("libX11.so.6"); err == nil {
			bind(lib, &xkbGetKeyboard, "XkbGetKeyboard")
			bind(lib, &xkbFreeKeyboard, "XkbFreeKeyboard")
		}
	})
	if xkbGetKeyboard == nil || xkbFreeKeyboard == nil {
		return 0
	}
	if s.physical.keymap != keymap {
		s.clearPhysicalKeys()
		s.physical.keymap = gObjectRef(keymap)
		s.physical.signal = connect(keymap, "keys-changed", cbPhysicalKeysChanged, ptr(s.w.id))
	}
	if !s.physical.loaded {
		s.physical.loaded = true
		s.physical.keys = [256]rune{}
		if dpy := gdkX11DisplayXDisplay(display); dpy != 0 {
			// XkbNamesMask, XkbUseCoreKbd. No symbols or layout are changed.
			if desc := xkbGetKeyboard(dpy, 1<<4, 0x100); desc != nil {
				s.physical.keys = gdkinput.CopyXKBPhysicalKeys(desc)
				xkbFreeKeyboard(desc, 0, true)
			}
		}
	}
	return s.physical.keys[code]
}

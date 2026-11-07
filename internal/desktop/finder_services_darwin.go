//go:build darwin

package desktop

import (
	"github.com/ebitengine/purego/objc"
	"golang.org/x/sys/unix"
)

// The service receives file URLs or Finder's legacy filename list, never shell
// source. AppKit retains the provider for the application's lifetime.
func (h *applicationHub) initializeFinderServices() {
	sel := objc.RegisterName
	class, err := objc.RegisterClass("OShellDesktopFinderServices", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{{
		Cmd: sel("openDirectoryInOShell:userData:error:"),
		Fn: func(_ objc.ID, _ objc.SEL, board, userdata objc.ID, failure *objc.ID) {
			var values []string
			items := board.Send(sel("pasteboardItems"))
			for i, n := uint64(0), objc.Send[uint64](items, sel("count")); i < n; i++ {
				item := items.Send(sel("objectAtIndex:"), i)
				value := item.Send(sel("stringForType:"), appearanceString("public.file-url"))
				if value != 0 {
					values = append(values, unix.BytePtrToString(objc.Send[*byte](value, sel("UTF8String"))))
				}
			}
			fileURLs := len(values) > 0
			if !fileURLs {
				paths := board.Send(sel("propertyListForType:"), appearanceString("NSFilenamesPboardType"))
				if paths != 0 && objc.Send[bool](paths, sel("isKindOfClass:"), objc.ID(objc.GetClass("NSArray"))) {
					for i, n := uint64(0), objc.Send[uint64](paths, sel("count")); i < n; i++ {
						value := paths.Send(sel("objectAtIndex:"), i)
						if !objc.Send[bool](value, sel("isKindOfClass:"), objc.ID(objc.GetClass("NSString"))) {
							values = nil
							break
						}
						values = append(values, unix.BytePtrToString(objc.Send[*byte](value, sel("UTF8String"))))
					}
				}
			}
			target := h.focused(nil)
			if target == h.fileWindow {
				target = nil
				for _, a := range h.windows {
					if !a.stopped.Load() {
						target = a
					}
				}
				if target == nil {
					target = h.newWindow()
				}
			}
			if err := target.openLocalDirectories(values, fileURLs); err != nil {
				if failure != nil {
					*failure = appearanceString(err.Error())
				}
				return
			}
			if target.win != nil {
				target.win.Show()
				target.win.Focus()
			}
		},
	}})
	if err != nil {
		if len(h.windows) > 0 {
			h.windows[0].status = "Finder 目录服务未启用：" + err.Error()
		}
		return
	}
	provider := objc.ID(class).Send(sel("new"))
	objc.ID(objc.GetClass("NSApplication")).Send(sel("sharedApplication")).Send(sel("setServicesProvider:"), provider)
}

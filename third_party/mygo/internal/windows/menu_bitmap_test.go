//go:build windows && (amd64 || arm64)

package windows

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

func TestNativeMenuBitmapsFollowDPIAndOwner(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 90, A: 128})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	icon := &platform.MenuIcon{PNG: encoded.Bytes(), Width: 36, Height: 20}
	items := &platform.Menu{Items: []*platform.MenuItem{
		{Label: "folder", Visible: true, Icon: icon, Submenu: &platform.Menu{Items: []*platform.MenuItem{{Label: "host", Icon: icon, Visible: true}}}},
		{Label: "action", Visible: true},
	}}
	getItem := user32.NewProc("GetMenuItemInfoW")
	getObject := gdi32.NewProc("GetObjectW")
	b := Backend{menus: newMenuTable()}
	owner := b.menus.newOwner()
	menu := b.buildMenu(items, false, owner, 144, nil)
	sub, _, _ := procGetSubMenu.Call(menu, 0)
	bitmapAt := func(h, pos uintptr) uintptr {
		t.Helper()
		info := menuItemInfo{Mask: 0x80}
		info.Size = uint32(unsafe.Sizeof(info))
		if ok, _, err := getItem.Call(h, pos, 1, uintptr(unsafe.Pointer(&info))); ok == 0 {
			t.Fatal(err)
		}
		return info.BmpItem
	}
	defer b.menus.drop(owner)
	defer procDestroyMenu.Call(menu)
	bmp := bitmapAt(menu, 0)
	if bmp == 0 || bitmapAt(sub, 0) != bmp || bitmapAt(menu, 1) != 0 {
		t.Fatal("native folder/session icon or undecorated action incorrect")
	}
	var bitmap struct {
		Type, Width, Height, WidthBytes int32
		Planes, BitsPixel               uint16
		Bits                            unsafe.Pointer
	}
	if ok, _, err := getObject.Call(bmp, unsafe.Sizeof(bitmap), uintptr(unsafe.Pointer(&bitmap))); ok == 0 {
		t.Fatal(err)
	}
	if bitmap.Width != 54 || bitmap.Height != 30 || bitmap.BitsPixel != 32 || len(b.menus.bitmaps[owner]) != 1 {
		t.Fatalf("unexpected DPI or bitmap ownership: %+v", bitmap)
	}
	pixel := unsafe.Slice((*byte)(bitmap.Bits), 4)
	if pixel[0] != 0 || pixel[1] != 0 || pixel[2] < 89 || pixel[2] > 91 || pixel[3] != 128 {
		t.Fatalf("unexpected premultiplied BGRA: %v", pixel)
	}
}

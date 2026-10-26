//go:build windows

package e2e

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"golang.org/x/image/tiff"
	"image"
	"image/jpeg"
	"image/png"
	"syscall"
	"testing"
	"unsafe"
)

func TestP2NativeImageClipboardRepresentations(t *testing.T) {
	previous, err := mygo.Clipboard.Read()
	if err != nil {
		t.Skip("cannot preserve clipboard")
	}
	t.Cleanup(func() {
		if err := mygo.Clipboard.Write(previous); err != nil {
			t.Error(err)
		}
		_ = mygo.Clipboard.Flush()
	})
	user := syscall.NewLazyDLL("user32.dll")
	kernel := syscall.NewLazyDLL("kernel32.dll")
	write := func(id uintptr, data []byte) error {
		done := make(chan error, 1)
		mygo.RunOnMain(func() {
			ok, _, err := user.NewProc("OpenClipboard").Call(0)
			if ok == 0 {
				done <- err
				return
			}
			defer user.NewProc("CloseClipboard").Call()
			user.NewProc("EmptyClipboard").Call()
			h, _, err := kernel.NewProc("GlobalAlloc").Call(0x42, uintptr(len(data)))
			if h == 0 {
				done <- err
				return
			}
			p, _, err := kernel.NewProc("GlobalLock").Call(h)
			if p == 0 {
				kernel.NewProc("GlobalFree").Call(h)
				done <- err
				return
			}
			copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(data)), data)
			kernel.NewProc("GlobalUnlock").Call(h)
			ok, _, err = user.NewProc("SetClipboardData").Call(id, h)
			if ok == 0 {
				kernel.NewProc("GlobalFree").Call(h)
				done <- err
				return
			}
			done <- nil
		})
		return <-done
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	var p, j, ti bytes.Buffer
	png.Encode(&p, img)
	jpeg.Encode(&j, img, nil)
	tiff.Encode(&ti, img, nil)
	dib := make([]byte, 40+16)
	binary.LittleEndian.PutUint32(dib, 40)
	binary.LittleEndian.PutUint32(dib[4:], 2)
	binary.LittleEndian.PutUint32(dib[8:], 2)
	binary.LittleEndian.PutUint16(dib[12:], 1)
	binary.LittleEndian.PutUint16(dib[14:], 24)
	registered := func(name string) uintptr {
		n, _ := syscall.UTF16PtrFromString(name)
		id, _, _ := user.NewProc("RegisterClipboardFormatW").Call(uintptr(unsafe.Pointer(n)))
		return id
	}
	for _, tc := range []struct {
		name   string
		id     uintptr
		format transfer.Format
		data   []byte
	}{{"PNG", registered("PNG"), transfer.PNG, p.Bytes()}, {"TIFF", 6, "image/tiff", ti.Bytes()}, {"JPEG", registered("JFIF"), "image/jpeg", j.Bytes()}, {"DIB", 8, transfer.PNG, dib}} {
		t.Run(tc.name, func(t *testing.T) {
			if err := write(tc.id, tc.data); err != nil {
				t.Fatal(err)
			}
			data, err := mygo.Clipboard.ReadFormat(tc.format)
			if err != nil {
				t.Fatal(err)
			}
			decoded, _, err := image.Decode(bytes.NewReader(data))
			if err != nil || decoded.Bounds() != img.Bounds() {
				t.Fatal(fmt.Sprint("native image decode ", err))
			}
		})
	}
}

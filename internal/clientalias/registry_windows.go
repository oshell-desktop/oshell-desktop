// Package clientalias manages per-user Windows launcher registrations.
package clientalias

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Both client switches, their recovery information and the user PATH must
// commit together, including when another process edits the same keys.
var (
	ktm               = windows.NewLazySystemDLL("ktmw32.dll")
	createTransaction = ktm.NewProc("CreateTransaction")
	commitTransaction = ktm.NewProc("CommitTransaction")
	advapi            = windows.NewLazySystemDLL("advapi32.dll")
	createKey         = advapi.NewProc("RegCreateKeyTransactedW")
	deleteKey         = advapi.NewProc("RegDeleteKeyTransactedW")
)

type transaction struct {
	handle windows.Handle
	root   registry.Key
}

func transact(root registry.Key, edit func(transaction) error) error {
	handle, _, err := createTransaction.Call(0, 0, 0, 0, 0, 0, 0)
	if windows.Handle(handle) == windows.InvalidHandle {
		return err
	}
	defer windows.CloseHandle(windows.Handle(handle)) // Closing an uncommitted transaction rolls it back.
	if err := edit(transaction{windows.Handle(handle), root}); err != nil {
		return err
	}
	if ok, _, err := commitTransaction.Call(handle); ok == 0 {
		return err
	}
	return nil
}

func (tx transaction) key(path string) (registry.Key, bool, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, false, err
	}
	var key registry.Key
	var disposition uint32
	r, _, _ := createKey.Call(uintptr(tx.root), uintptr(unsafe.Pointer(p)), 0, 0, 0,
		registry.READ|registry.WRITE, 0, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&disposition)), uintptr(tx.handle), 0)
	if r != 0 {
		return 0, false, syscall.Errno(r)
	}
	return key, disposition == 2, nil
}

func (tx transaction) deleteEmpty(path string, key registry.Key) error {
	info, err := key.Stat()
	if err != nil || info.ValueCount != 0 || info.SubKeyCount != 0 {
		return err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	r, _, _ := deleteKey.Call(uintptr(tx.root), uintptr(unsafe.Pointer(p)), 0, 0, uintptr(tx.handle), 0)
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

type regString struct {
	Text string
	Kind uint32 // Zero denotes an absent value. Only SZ and EXPAND_SZ are accepted.
}

func readString(key registry.Key, name string) (regString, error) {
	text, kind, err := key.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return regString{}, nil
	}
	return regString{text, kind}, err
}

func (v regString) write(key registry.Key, name string) error {
	switch v.Kind {
	case registry.SZ:
		return key.SetStringValue(name, v.Text)
	case registry.EXPAND_SZ:
		return key.SetExpandStringValue(name, v.Text)
	case 0:
		err := key.DeleteValue(name)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	default:
		return registry.ErrUnexpectedType
	}
}

//go:build windows

package core

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

func rsyncStdioInput(in io.Reader) (io.Reader, func(), error) {
	if in != os.Stdin {
		return in, func() {}, nil
	}
	h, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil {
		return nil, nil, err
	}
	typ, err := windows.GetFileType(h)
	if err != nil {
		return nil, nil, err
	}
	if typ != windows.FILE_TYPE_PIPE {
		return in, func() {}, nil
	}
	// cwRsync/Cygwin hands native helpers an overlapped pipe. Go deliberately
	// skips async-handle detection for the original stdin handle (go.dev/issue/75949).
	// A private duplicate lets os.NewFile detect/register its I/O mode normally,
	// and lets cleanup cancel a pending read without closing process-wide stdin.
	var duplicate windows.Handle
	process := windows.CurrentProcess()
	if err = windows.DuplicateHandle(process, h, process, &duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(duplicate), "rsync-stdin")
	return file, func() { file.Close() }, nil
}

//go:build !windows

package core

import "io"

func rsyncStdioInput(in io.Reader) (io.Reader, func(), error) {
	return in, func() {}, nil
}

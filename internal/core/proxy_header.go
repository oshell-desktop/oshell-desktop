package core

import (
	"bufio"
	"bytes"
	"fmt"
)

const proxyHeaderLimit = 16 << 10

// Read only the response header. Bytes already buffered from the tunnel must
// remain available to the caller after CONNECT succeeds.
func readProxyHeader(r *bufio.Reader) ([]byte, error) {
	var header []byte
	continued := false
	for {
		line, err := r.ReadSlice('\n')
		if len(header)+len(line) > proxyHeaderLimit {
			return nil, fmt.Errorf("HTTP 代理响应头超过 16 KiB")
		}
		header = append(header, line...)
		if err == bufio.ErrBufferFull {
			continued = true
			continue
		}
		if err != nil {
			return nil, err
		}
		if !continued && (bytes.Equal(line, []byte("\r\n")) || bytes.Equal(line, []byte("\n"))) {
			return header, nil
		}
		continued = false
	}
}

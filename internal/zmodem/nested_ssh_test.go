package zmodem

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// Interactive OpenSSH treats a tilde after CR or LF as an escape command.
// Keep the previous byte across writes, just as its input filter does.
type sshEscapeGuard struct {
	net.Conn
	newline bool
}

func (c *sshEscapeGuard) Write(data []byte) (int, error) {
	for _, b := range data {
		if c.newline && b == '~' {
			return 0, fmt.Errorf("nested SSH escape in ZMODEM wire data")
		}
		c.newline = b == '\r' || b == '\n'
	}
	return c.Conn.Write(data)
}

func TestLrzszUploadThroughNestedSSHEscapes(t *testing.T) {
	// Repeat around subpacket boundaries, including interactive ssh's suspend,
	// disconnect, background, help, command and literal-tilde sequences.
	data := bytes.Repeat([]byte("\n~.\n~\x1a\n~&\n~?\n~C\n~~\r~.\x00\xff"), 4099)
	t.Run("synthetic", func(t *testing.T) { checkNestedSSHUpload(t, data) })
	// Exercise every control/high-bit byte over many ACK windows without
	// depending on the optional user-supplied RPM being present in CI.
	binary := make([]byte, 2<<20+17)
	for i := range binary {
		binary[i] = byte(i*37 + (i >> 3))
	}
	t.Run("binary", func(t *testing.T) { checkNestedSSHUpload(t, binary) })
	if path := os.Getenv("OSHELL_TEST_ZMODEM_FILE"); path != "" {
		t.Run("supplied-file", func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			checkNestedSSHUpload(t, data)
		})
	}
}

func checkNestedSSHUpload(t *testing.T, data []byte) {
	t.Helper()
	for _, escape := range []bool{false, true} {
		t.Run(fmt.Sprintf("receiver-escape=%v", escape), func(t *testing.T) {
			dir := t.TempDir()
			args := []string{"-b", "-y"}
			if escape {
				args = append(args, "-e")
			}
			conn, wait := lrzszPeer(t, dir, "rz", args...)
			defer conn.Close()
			guard := &sshEscapeGuard{Conn: conn, newline: true}
			err := SendFiles(guard, []SendFile{{Name: "payload.bin", Size: int64(len(data)), Reader: bytes.NewReader(data)}}, nil)
			if err != nil {
				t.Log(err)
				conn.Close()
				wait()
				t.Fatal(err)
			}
			wait()
			got, err := os.ReadFile(filepath.Join(dir, "payload.bin"))
			if err != nil || sha256.Sum256(got) != sha256.Sum256(data) {
				t.Fatalf("received content differs: %v", err)
			}
		})
	}
}

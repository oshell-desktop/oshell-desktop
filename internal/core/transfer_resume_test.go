package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pkg/sftp"
)

// A valid SFTP server may omit SSH_FILEXFER_ATTR_PERMISSIONS. A zero Go
// FileMode then looks regular, but is not proof that resuming is safe.
func TestSFTPResumeRejectsMissingFileType(t *testing.T) {
	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()
	clientPipe.SetDeadline(time.Now().Add(5 * time.Second))
	serverPipe.SetDeadline(time.Now().Add(5 * time.Second))
	done := make(chan error, 1)
	go func() {
		for _, expected := range []byte{1, 7} { // INIT, LSTAT
			var length uint32
			if err := binary.Read(serverPipe, binary.BigEndian, &length); err != nil {
				done <- err
				return
			}
			if length == 0 || length > 4096 {
				done <- errors.New("invalid fixture request length")
				return
			}
			request := make([]byte, length)
			if _, err := io.ReadFull(serverPipe, request); err != nil {
				done <- err
				return
			}
			if request[0] != expected || len(request) < 5 {
				done <- errors.New("unexpected fixture request")
				return
			}
			response := []byte{2, 0, 0, 0, 3} // VERSION 3
			if expected == 7 {
				response = append([]byte{105}, request[1:5]...)       // ATTRS, request ID
				response = binary.BigEndian.AppendUint32(response, 9) // size and times only
				response = binary.BigEndian.AppendUint64(response, 16)
				response = binary.BigEndian.AppendUint32(response, 1700000000)
				response = binary.BigEndian.AppendUint32(response, 1700000000)
			}
			packet := binary.BigEndian.AppendUint32(nil, uint32(len(response)))
			if _, err := serverPipe.Write(append(packet, response...)); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	client, err := sftp.NewClientPipe(clientPipe, clientPipe)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	targetDir := t.TempDir()
	cp := &SFTPCheckpoint{Source: "/unknown", Target: filepath.Join(targetDir, "download")}
	if err := cp.Run(context.Background(), client, nil); !errors.Is(err, ErrResumeRegularOnly) {
		t.Fatalf("unknown type accepted for resume: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(targetDir)
	if err != nil || len(entries) != 0 || cp.initialized {
		t.Fatal("created a fragment for unknown file type", entries, err)
	}
}

func resumeTestClient(t *testing.T) *sftp.Client {
	t.Helper()
	server := startSSH(t, "resume-test")
	p := testProfile(server.address)
	p.Kind = "sftp"
	conn, err := Dial(context.Background(), p, Credentials{Password: "resume-test"}, testTrust(t), acceptHost)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	c, err := sftp.NewClient(conn.Client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}
func pauseCheckpoint(t *testing.T, c *SFTPCheckpoint, client *sftp.Client) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	limiter := &TransferLimiter{}
	limiter.Set(1 << 20)
	ctx = WithTransferLimiter(ctx, limiter)
	err := c.Run(ctx, client, func(p TransferProgress) {
		if p.Bytes >= 64<<10 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pause: %v", err)
	}
	if _, err = os.Stat(c.Target); !os.IsNotExist(err) {
		t.Fatalf("published incomplete destination: %v", err)
	}
	staged, err := os.Stat(c.temporary)
	if err != nil || staged.Size() == 0 || staged.Size() >= c.size {
		t.Fatalf("not a partial transfer: %v %v", staged, err)
	}
}
func TestSFTPResumeUploadDownloadAndNoOverwrite(t *testing.T) {
	client := resumeTestClient(t)
	for _, upload := range []bool{true, false} {
		t.Run(map[bool]string{true: "upload", false: "download"}[upload], func(t *testing.T) {
			source := filepath.ToSlash(filepath.Join(t.TempDir(), "中文-source"))
			target := filepath.ToSlash(filepath.Join(t.TempDir(), "target"))
			content := bytes.Repeat([]byte("abcdefgh"), 64<<10)
			if err := os.WriteFile(source, content, 0600); err != nil {
				t.Fatal(err)
			}
			cp := &SFTPCheckpoint{Upload: upload, Source: source, Target: target}
			pauseCheckpoint(t, cp, client)
			if err := cp.Run(context.Background(), client, nil); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(got, content) {
				t.Fatal("bad resumed content", err)
			}
			blocked := &SFTPCheckpoint{Upload: upload, Source: source, Target: target}
			if err := blocked.Run(context.Background(), client, nil); err == nil {
				t.Fatal("overwrote existing target")
			}
			got, _ = os.ReadFile(target)
			if !bytes.Equal(got, content) {
				t.Fatal("existing file damaged")
			}
		})
	}
}
func TestSFTPResumeRejectsChangedSourceAndFragment(t *testing.T) {
	client := resumeTestClient(t)
	for _, mutation := range []string{"size", "mtime", "same-size-content", "fragment", "symlink"} {
		t.Run(mutation, func(t *testing.T) {
			source := filepath.ToSlash(filepath.Join(t.TempDir(), "source"))
			target := filepath.Join(t.TempDir(), "target")
			data := bytes.Repeat([]byte("a"), 512<<10)
			os.WriteFile(source, data, 0600)
			cp := &SFTPCheckpoint{Source: source, Target: target}
			pauseCheckpoint(t, cp, client)
			switch mutation {
			case "size":
				os.WriteFile(source, []byte("changed"), 0600)
			case "mtime":
				os.Chtimes(source, time.Now(), time.Now().Add(time.Hour))
			case "same-size-content":
				data[0] = 'b'
				os.WriteFile(source, data, 0600)
				os.Chtimes(source, cp.modified, cp.modified)
			case "fragment":
				f, _ := os.OpenFile(cp.temporary, os.O_WRONLY, 0600)
				f.Write([]byte("bad"))
				f.Close()
			case "symlink":
				os.Remove(cp.temporary)
				if err := os.Symlink(source, cp.temporary); err != nil {
					t.Skip("symlink unavailable", err)
				}
			}
			if err := cp.Run(context.Background(), client, nil); err == nil {
				t.Fatal("accepted changed source/fragment")
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatal("unsafe target published")
			}
		})
	}
}
func TestSFTPResumeCancellationDuringPrefixValidation(t *testing.T) {
	client := resumeTestClient(t)
	source := filepath.ToSlash(filepath.Join(t.TempDir(), "source"))
	os.WriteFile(source, bytes.Repeat([]byte("x"), 512<<10), 0600)
	cp := &SFTPCheckpoint{Source: source, Target: filepath.Join(t.TempDir(), "target")}
	pauseCheckpoint(t, cp, client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cp.Run(ctx, client, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := cp.Run(context.Background(), client, nil); err != nil {
		t.Fatal(err)
	}
}

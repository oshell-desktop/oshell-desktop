package core

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFTPFilesOutliveOpeningContext(t *testing.T) {
	p, root := ftpTestServer(t)
	if err := os.WriteFile(filepath.Join(root, "fixture.txt"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session, files, err := LoginFTP(ctx, p, "test-only")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	defer files.Close()
	if _, err := files.ReadDir("/"); err != nil {
		t.Fatal(err)
	}
	// readFiles cancels its opening/read context after publishing the first list.
	cancel()
	entries, err := files.ReadDir("/")
	if err != nil || len(entries) != 1 || entries[0].Name() != "fixture.txt" {
		t.Fatalf("refresh after opening context cancellation: %v, %v", entries, err)
	}
	target := filepath.Join(t.TempDir(), "download.txt")
	if err := files.Download(context.Background(), "/fixture.txt", target, false, nil); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "fixture" {
		t.Fatalf("download: %q, %v", data, err)
	}
}

func TestFTPOpenRejectsCanceledContext(t *testing.T) {
	p, _ := ftpTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	session, files, err := LoginFTP(ctx, p, "test-only")
	if session != nil {
		session.Close()
	}
	if files != nil {
		files.Close()
	}
	if !errors.Is(err, context.Canceled) || session != nil || files != nil {
		t.Fatalf("canceled FTP open: session=%v files=%v error=%v", session != nil, files != nil, err)
	}
}

func TestMCPBridgeCanceledRequestDoesNotWrite(t *testing.T) {
	c := &handshakeReplyConn{reader: strings.NewReader("")}
	b := &MCPBridge{conn: c}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := b.request(ctx, "sessions_list", json.RawMessage(`{}`))
	if !errors.Is(err, context.Canceled) || c.writes.Load() != 0 {
		t.Fatalf("canceled request reached transport: writes=%d error=%v", c.writes.Load(), err)
	}
}

type handshakeReplyConn struct {
	net.Conn
	reader *strings.Reader
	cancel context.CancelFunc
	closed atomic.Bool
	writes atomic.Int32
}

func (c *handshakeReplyConn) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	if c.reader.Len() == 0 && c.cancel != nil {
		c.cancel()
	}
	return n, err
}
func (c *handshakeReplyConn) Write(p []byte) (int, error) { c.writes.Add(1); return len(p), nil }
func (c *handshakeReplyConn) Close() error                { c.closed.Store(true); return nil }
func (c *handshakeReplyConn) SetDeadline(time.Time) error { return nil }

func TestProxyHandshakeCancellationAtReply(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &handshakeReplyConn{reader: strings.NewReader("HTTP/1.1 200 OK\r\n\r\n"), cancel: cancel}
	result, err := proxyHandshake(ctx, c, ProxySettings{Kind: "http"}, "", "fixture.invalid:22")
	if result != nil || !errors.Is(err, context.Canceled) || !c.closed.Load() {
		t.Fatalf("canceled handshake returned connection=%v error=%v closed=%v", result != nil, err, c.closed.Load())
	}
}

func TestProxyHandshakeDetachesSuccessfulContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &handshakeReplyConn{reader: strings.NewReader("HTTP/1.1 200 OK\r\n\r\nSSH-fixture")}
	result, err := proxyHandshake(ctx, c, ProxySettings{Kind: "http"}, "", "fixture.invalid:22")
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	cancel()
	buf := make([]byte, len("SSH-fixture"))
	if n, err := result.Read(buf); n != len(buf) || err != nil || string(buf) != "SSH-fixture" || c.closed.Load() {
		t.Fatalf("successful tunnel canceled or buffered bytes lost: %q, %v", buf[:n], err)
	}
}

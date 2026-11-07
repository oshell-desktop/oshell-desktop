package desktop

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
	"oshell-desktop/internal/zmodem/testutil"
)

// The fixture starts a real rz only logically after receiving the command:
// hold its output until the command is consumed, then pass protocol bytes.
type dropRzConn struct {
	net.Conn
	command chan struct{}
	once    sync.Once
	started atomic.Bool
}

func (c *dropRzConn) Read(b []byte) (int, error) { <-c.command; return c.Conn.Read(b) }
func (c *dropRzConn) Write(b []byte) (int, error) {
	if c.started.CompareAndSwap(false, true) {
		if string(b) != "\x15rz\r" {
			return 0, fmt.Errorf("unexpected command %q", b)
		}
		c.once.Do(func() { close(c.command) })
		return len(b), nil
	}
	return c.Conn.Write(b)
}
func (c *dropRzConn) Close() error { c.once.Do(func() { close(c.command) }); return c.Conn.Close() }

func TestTerminalDropRealRzMultiFile(t *testing.T) {
	source, dest := t.TempDir(), t.TempDir()
	names := []string{"中文 空格.bin", "empty.txt"}
	payload := bytes.Repeat([]byte{0, 0xff, 0x18, 'x'}, 20000)
	paths := []string{filepath.Join(source, names[0]), filepath.Join(source, names[1])}
	for i, path := range paths {
		data := payload
		if i == 1 {
			data = nil
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	peer, wait := testutil.LrzszShellPeer(t, dest, "rz", "-b")
	conn := &dropRzConn{Conn: peer, command: make(chan struct{})}
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p := &pane{id: core.NewID(), profile: core.Profile{Kind: "ssh", Name: "drop-target"}}
	p.owner.Store(a)
	opts := terminal.Options{Conn: conn, NoBlink: true}
	var picker atomic.Bool
	a.configureTerminalTransferWithPicker(p, &opts, false, func(context.Context, *mygo.Window, bool) ([]string, error) {
		picker.Store(true)
		return nil, fmt.Errorf("unexpected picker")
	})
	term, err := terminal.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	p.term = term
	defer term.Close()
	_, other := operatorPane(t, a, "other")
	a.uploadTerminalDrop(p, paths)
	if !p.transfer.active {
		t.Fatal(p.transfer.status)
	}
	a.uploadTerminalDrop(p, []string{paths[1]})
	if !strings.Contains(a.status, "正在传输") {
		t.Fatal("second drop not rejected")
	}
	waitOperator(t, a, func() bool { return p.transfer.visible && !p.transfer.active })
	if p.transfer.status != "ZMODEM · 传输完成" || picker.Load() {
		t.Fatal(p.transfer.status, picker.Load())
	}
	wait()
	waitOperator(t, a, func() bool { return term.Available() && strings.Contains(term.Text(), "shell-ready$") })
	for i, name := range names {
		want := payload
		if i == 1 {
			want = nil
		}
		got, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s corrupt: %v", name, err)
		}
	}
	if other.text() != "" {
		t.Fatal("drop broadcast to another terminal")
	}
}

func TestTerminalDropRejectsInvalidBatchAndBlockedWorkspace(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, conn := operatorPane(t, a, "target")
	source := filepath.Join(t.TempDir(), "valid.txt")
	if err := os.WriteFile(source, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, paths := range [][]string{{source, t.TempDir()}, {source, source + ".missing"}} {
		a.uploadTerminalDrop(p, paths)
		if p.transfer.active || !p.transfer.visible || conn.text() != "" {
			t.Fatal("invalid batch started rz")
		}
	}
	for _, mode := range []string{"closed", "modal", "local", "locked"} {
		p.transfer = transferNoticeState{}
		p.closed = mode == "closed"
		a.features.vaultOpen = mode == "modal"
		p.profile.Kind = "ssh"
		if mode == "local" {
			p.profile.Kind = "local"
		}
		if mode == "locked" {
			if err := a.vault.SetMaster("synthetic-drop-master"); err != nil {
				t.Fatal(err)
			}
			a.vault.Lock()
			a.workspaceAuthorized = false
		}
		a.uploadTerminalDrop(p, []string{source})
		if p.transfer.active || conn.text() != "" {
			t.Fatalf("%s accepted drop", mode)
		}
	}
}

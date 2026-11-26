package terminal

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"oshell-desktop/internal/core"
)

type promptOutput struct {
	sync.Mutex
	buf bytes.Buffer
}

func (o *promptOutput) Write(p []byte) (int, error) {
	o.Lock()
	defer o.Unlock()
	return o.buf.Write(p)
}
func (o *promptOutput) wait(t *testing.T, marker string) {
	t.Helper()
	until := time.Now().Add(8 * time.Second)
	for {
		o.Lock()
		s := o.buf.String()
		if i := strings.Index(s, marker); i >= 0 {
			o.buf.Next(i + len(marker))
			o.Unlock()
			return
		}
		o.Unlock()
		if time.Now().After(until) {
			t.Fatalf("missing %q in %q", marker, s)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestLocalPromptSystemAndMissingClients(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	profiles := make(chan core.Profile, 1)
	c := NewLocalConsole(func(p core.Profile) { profiles <- p })
	defer c.Close()
	c.Resize(512, 24)
	out := new(promptOutput)
	go io.Copy(out, c)
	out.wait(t, "oshell> ")
	io.WriteString(c, "cd \""+dir+"\"\r")
	out.wait(t, "["+dir+"] oshell> ")
	io.WriteString(c, "echo OSHELL_LOCAL > fixture.txt\r")
	out.wait(t, "[工具退出：0]")
	data, err := os.ReadFile(filepath.Join(dir, "fixture.txt"))
	if err != nil || strings.TrimSpace(string(data)) != "OSHELL_LOCAL" {
		t.Fatalf("shell or working directory: %q %v", data, err)
	}
	out.wait(t, "oshell> ")
	io.WriteString(c, "sftp -P 2222 somebody@localhost\r")
	select {
	case p := <-profiles:
		if p.Kind != "sftp" || p.Host != "localhost" || p.Port != 2222 || p.Username != "somebody" {
			t.Fatal(p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing client fallback")
	}
	out.wait(t, "oshell> ")
	io.WriteString(c, "telnet\r")
	out.wait(t, "telnet> ")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	fmt.Fprintf(c, "open 127.0.0.1 %d\r", port)
	listener.(*net.TCPListener).SetDeadline(time.Now().Add(3 * time.Second))
	peer, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	peer.SetDeadline(time.Now().Add(3 * time.Second))
	peer.Write([]byte{tnIAC, tnWILL, 1, tnIAC, tnDO, 99})
	reply := make([]byte, 6)
	if _, err = io.ReadFull(peer, reply); err != nil || !bytes.Equal(reply, []byte{tnIAC, tnDO, 1, tnIAC, tnWONT, 99}) {
		t.Fatalf("negotiation %x %v", reply, err)
	}
	peer.Write([]byte("PEER-READY\r\n"))
	out.wait(t, "PEER-READY")
	if !c.ConnectionActive() || c.ManagedInputAllowed() {
		t.Fatal("incorrect Telnet input ownership")
	}
	io.WriteString(c, "test\r")
	if _, err = io.ReadFull(peer, reply); err != nil || string(reply) != "test\r\n" {
		t.Fatalf("keyboard %q %v", reply, err)
	}
	c.Write([]byte{0x1d})
	out.wait(t, "telnet> ")
	io.WriteString(c, "quit\r")
	out.wait(t, "oshell> ")
	if c.ConnectionActive() || !c.ManagedInputAllowed() {
		t.Fatal("prompt did not regain input")
	}
}

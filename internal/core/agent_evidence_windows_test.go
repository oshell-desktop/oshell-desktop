package core

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/sys/windows"
)

func TestEvidenceC01WindowsAgentAuthentication(t *testing.T) {
	if os.Getenv("OSHELL_ACCEPTANCE_AGENT_PIPE") != "1" {
		t.Skip("opt-in native agent fixture; never displace a running agent")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	// ListenPipe uses FILE_CREATE: an existing real agent causes a skip, never
	// a connection to its key store or replacement of its pipe/service.
	listener, err := winio.ListenPipe(`\\.\pipe\openssh-ssh-agent`, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;" + user.User.Sid.String() + ")"})
	if err != nil {
		t.Skip("native agent pipe is occupied/unavailable; preserved existing agent", err)
	}
	defer listener.Close()
	ring := agent.NewKeyring()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _ = agent.ServeAgent(ring, c) }()
		}
	}()
	defer func() { listener.Close(); <-stopped }()
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	server := startSSH(t, "", func(c *ssh.ServerConfig) {
		c.PasswordCallback = nil
		c.PublicKeyCallback = func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if !bytes.Equal(key.Marshal(), allowed.Marshal()) {
				return nil, fmt.Errorf("synthetic rejected key")
			}
			return nil, nil
		}
	})
	t.Setenv("OSHELL_TEST_NO_AGENT", "0")
	t.Setenv("OSHELL_TEST_EXPLICIT_KEYS_ONLY", "1")
	profile := testProfile(server.address)
	for _, kind := range []string{"allowed", "wrong-key", "empty"} {
		t.Run(kind, func(t *testing.T) {
			ring.RemoveAll()
			if kind == "allowed" {
				err = ring.Add(agent.AddedKey{PrivateKey: private})
			} else if kind == "wrong-key" {
				_, wrong, e := ed25519.GenerateKey(rand.Reader)
				if e != nil {
					t.Fatal(e)
				}
				err = ring.Add(agent.AddedKey{PrivateKey: wrong})
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, err := Dial(ctx, profile, Credentials{}, testTrust(t), acceptHost)
			if conn != nil {
				conn.Close()
			}
			if (err == nil) != (kind == "allowed") {
				t.Fatal("agent authentication result", kind, err)
			}
		})
	}
}

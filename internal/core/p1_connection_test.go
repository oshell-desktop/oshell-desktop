package core

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestP1SOCKS5AuthenticationCannotDowngrade(t *testing.T) {
	for _, tc := range []struct {
		user, password string
		selected       byte
		success        bool
	}{
		{"", "", 0, true}, {"fixture", "secret", 2, true}, {"fixture", "secret", 0, false}, {"", "", 2, false},
	} {
		client, server := net.Pipe()
		client.SetDeadline(time.Now().Add(3 * time.Second))
		server.SetDeadline(time.Now().Add(3 * time.Second))
		done := make(chan error, 1)
		go func() {
			defer server.Close()
			greeting := make([]byte, 3)
			_, err := io.ReadFull(server, greeting)
			want := byte(0)
			if tc.user != "" {
				want = 2
			}
			if err == nil && !bytes.Equal(greeting, []byte{5, 1, want}) {
				t.Error("unexpected advertised methods", greeting)
			}
			if err == nil {
				err = writeAll(server, []byte{5, tc.selected})
			}
			if err == nil && tc.success {
				if tc.selected == 2 {
					auth := make([]byte, 3+len(tc.user)+len(tc.password))
					_, err = io.ReadFull(server, auth)
					if err == nil {
						err = writeAll(server, []byte{1, 0})
					}
				}
				if err == nil {
					header := make([]byte, 4)
					_, err = io.ReadFull(server, header)
					if err == nil {
						_, err = readSOCKSAddress(server, header[3])
					}
				}
				if err == nil {
					err = writeAll(server, []byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 22})
				}
			}
			done <- err
		}()
		err := connectSOCKS5(client, "fixture.invalid:22", tc.user, tc.password)
		client.Close()
		if (err == nil) != tc.success {
			t.Fatalf("selected=%d user=%q err=%v", tc.selected, tc.user, err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func TestP1HTTPProxyHeaderLimitAndTunnelBuffer(t *testing.T) {
	prefix, suffix := "HTTP/1.1 200 OK\r\nX: ", "\r\n\r\n"
	for _, size := range []int{4096 + 2, proxyHeaderLimit, proxyHeaderLimit + 1} {
		header := prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
		r := bufio.NewReaderSize(strings.NewReader(header+"SSH-tunnel"), 4096)
		got, err := readProxyHeader(r)
		if size > proxyHeaderLimit {
			if err == nil {
				t.Fatal("oversized header accepted")
			}
			continue
		}
		if err != nil || string(got) != header {
			t.Fatal("header truncated", len(got), err)
		}
		remaining, _ := io.ReadAll(r)
		if string(remaining) != "SSH-tunnel" {
			t.Fatal("tunnel bytes lost")
		}
	}
}

func TestP1LegacySSHFixedAlgorithmsAndNegotiation(t *testing.T) {
	cfg := &ssh.ClientConfig{}
	configureLegacySSH(cfg)
	if slices.Contains(cfg.Ciphers, "arcfour") || slices.Contains(cfg.Ciphers, "arcfour128") || slices.Contains(cfg.HostKeyAlgorithms, "ssh-dss") {
		t.Fatal("non-baseline insecure algorithm enabled")
	}
	for _, cipher := range []string{"aes128-cbc", "aes192-cbc", "aes256-cbc", "3des-cbc"} {
		if !slices.Contains(cfg.Ciphers, cipher) {
			t.Fatal("missing supported fixed cipher", cipher)
		}
	}
	for _, cipher := range []string{"aes128-cbc", "aes192-cbc", "aes256-cbc", "3des-cbc", "arcfour"} {
		for _, legacy := range []bool{false, true} {
			server := startSSH(t, "fixture", func(c *ssh.ServerConfig) { c.Ciphers = []string{cipher} })
			p := testProfile(server.address)
			p.LegacySSH = legacy
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			conn, err := Dial(ctx, p, Credentials{Password: "fixture"}, testTrust(t), acceptHost)
			cancel()
			if conn != nil {
				conn.Close()
			}
			if (err == nil) != (legacy && cipher != "arcfour") {
				t.Fatalf("cipher=%s legacy=%v err=%v", cipher, legacy, err)
			}
		}
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []bool{false, true} {
		signers, err := restrictRSAAuthentication([]ssh.Signer{signer}, legacy)
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(signers[0].(ssh.MultiAlgorithmSigner).Algorithms(), ssh.KeyAlgoRSA) != legacy {
			t.Fatal("RSA SHA1 not controlled by legacy switch")
		}
	}
}

func TestP1MixedInteractiveChallengeNeverUsesSavedSlot(t *testing.T) {
	var identity SSHIdentity
	server := startSSH(t, "unused", func(c *ssh.ServerConfig) {
		c.PasswordCallback = nil
		c.KeyboardInteractiveCallback = func(_ ssh.ConnMetadata, ask ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			a, err := ask("MFA", "", []string{identity.User + "@" + identity.Host + "'s password:", "OTP:"}, []bool{false, false})
			if err == nil && (len(a) != 2 || a[0] != "manual-login" || a[1] != "manual-otp") {
				t.Error("saved password disclosed to mixed challenge")
			}
			return nil, err
		}
	})
	p := testProfile(server.address)
	identity = p.Identity()
	manual := 0
	published := false
	c, err := Dial(context.Background(), p, Credentials{Password: "must-not-autofill", Challenge: func(_ context.Context, q AuthenticationPrompt) ([]string, error) {
		manual++
		if len(q.Questions) != 2 {
			t.Error("challenge was partially auto-filled")
		}
		return []string{"manual-login", "manual-otp"}, nil
	}, PasswordAuthenticated: func(string) { published = true }}, testTrust(t), acceptHost)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if manual != 1 || published || c.PasswordVerified {
		t.Fatal("mixed challenge promoted to saved password")
	}
}

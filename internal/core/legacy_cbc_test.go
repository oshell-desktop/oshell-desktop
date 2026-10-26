package core

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestC03LegacyCBCPayloadAndDefaults(t *testing.T) {
	for _, cipher := range []string{"aes192-cbc", "aes256-cbc"} {
		t.Run(cipher, func(t *testing.T) {
			if slices.Contains(ssh.SupportedAlgorithms().Ciphers, cipher) || !slices.Contains(ssh.InsecureAlgorithms().Ciphers, cipher) {
				t.Fatal("CBC must remain explicitly insecure/opt-in")
			}
			server := startSSH(t, "fixture", func(cfg *ssh.ServerConfig) { cfg.Ciphers = []string{cipher} })
			p := testProfile(server.address)
			p.LegacySSH = true
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			conn, err := Dial(ctx, p, Credentials{Password: "fixture"}, testTrust(t), acceptHost)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			payload := bytes.Repeat([]byte("CBC/中文/\x00\xff"), 9000)
			done := make(chan error, 1)
			go func() { _, err := conn.Write(payload); done <- err }()
			got := make([]byte, len(payload))
			if _, err := io.ReadFull(conn, got); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil || !bytes.Equal(got, payload) {
				t.Fatal("multi-packet CBC traffic", err)
			}
		})
	}
}

// A native OpenSSH peer prevents a matching bug in two copies of our patched
// library from being mistaken for interoperability. Only loopback is contacted.
func TestC03NativeOpenSSHCBCPeer(t *testing.T) {
	binary, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("native OpenSSH required")
	}
	for _, cipher := range []string{"aes192-cbc", "aes256-cbc"} {
		for _, mac := range []string{ssh.HMACSHA256ETM, ssh.HMACSHA512ETM, ssh.HMACSHA256, ssh.InsecureHMACSHA196} {
			t.Run(cipher+"/"+mac, func(t *testing.T) {
				server := startSSH(t, "", func(cfg *ssh.ServerConfig) {
					cfg.Ciphers = []string{cipher}
					cfg.MACs = []string{mac}
					cfg.NoClientAuth = true
					cfg.PasswordCallback = nil
				})
				dir := t.TempDir()
				config, known := filepath.Join(dir, "empty config"), filepath.Join(dir, "known hosts")
				if err := os.WriteFile(config, nil, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(known, []byte(knownhosts.Line([]string{server.address}, server.hostKey)+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
				host, port, _ := net.SplitHostPort(server.address)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, binary, "-F", config, "-T", "-c", cipher, "-m", mac, "-p", port,
					"-oBatchMode=yes", "-oStrictHostKeyChecking=yes", "-oUserKnownHostsFile="+strconv.Quote(filepath.ToSlash(known)),
					"-oIdentityFile=none", "-oIdentityAgent=none", "-oPreferredAuthentications=none",
					"-oGlobalKnownHostsFile="+os.DevNull, "-oLogLevel=ERROR", "fixture@"+host)
				prepareCommand(cmd)
				payload := bytes.Repeat([]byte("native-CBC/中文\n"), 4096)
				cmd.Stdin = bytes.NewReader(payload)
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				got, err := cmd.Output()
				if err != nil || !bytes.Equal(got, payload) {
					t.Fatalf("native OpenSSH CBC peer: %v; %s; bytes=%d", err, stderr.String(), len(got))
				}
			})
		}
	}
}

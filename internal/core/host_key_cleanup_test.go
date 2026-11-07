package core

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestHostKeyCleanupScopedAndRequiresNewApproval(t *testing.T) {
	for _, address := range []string{"example.invalid:22", "example.invalid:2222", "[::1]:2200"} {
		for _, hashed := range []bool{false, true} {
			t.Run(address+map[bool]string{false: "/plain", true: "/hashed"}[hashed], func(t *testing.T) {
				store := &TrustStore{Path: filepath.Join(t.TempDir(), "known_hosts")}
				old, next := key(t), key(t)
				host := knownhosts.Normalize(address)
				if hashed {
					host = knownhosts.HashHostname(host)
				}
				other := knownhosts.Line([]string{"other.invalid", "[example.invalid]:2223"}, old.PublicKey()) + " # keep\r\n"
				before := []byte("# header\r\n" + knownhosts.Line([]string{host}, old.PublicKey()) + "\n" + other)
				if err := os.WriteFile(store.Path, before, 0600); err != nil {
					t.Fatal(err)
				}
				remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}
				approved := 0
				cb := store.Callback(context.Background(), func(HostPrompt) bool { approved++; return true })
				err := cb(address, remote, next.PublicKey())
				var mismatch *HostKeyMismatch
				if !errors.As(err, &mismatch) || approved != 0 || mismatch.Address != address || len(mismatch.PreviousFingerprints()) != 1 {
					t.Fatal("mismatch lost identity or asked trust", err)
				}
				if err = store.RemoveHostKey(mismatch); err != nil {
					t.Fatal(err)
				}
				after, _ := os.ReadFile(store.Path)
				backup, _ := os.ReadFile(store.Path + ".before-cleanup")
				if string(after) != "# header\r\n"+other || !bytes.Equal(before, backup) {
					t.Fatal("unrelated record or backup changed")
				}
				if err = store.Callback(context.Background(), func(HostPrompt) bool { return false })(address, remote, next.PublicKey()); err == nil {
					t.Fatal("cleanup implicitly trusted new key")
				}
				if err = cb(address, remote, next.PublicKey()); err != nil || approved != 1 {
					t.Fatal("new explicit approval missing", err)
				}
				if err = cb(address, remote, next.PublicKey()); err != nil || approved != 1 {
					t.Fatal("new trust not saved", err)
				}
				if err = store.RemoveHostKey(mismatch); err == nil {
					t.Fatal("old dialog erased new trust")
				}
			})
		}
	}
}

func TestHostKeyCleanupPreservesAliasesAndRefusesSharedRules(t *testing.T) {
	for _, hosts := range []string{"other.invalid,example.invalid", "*.invalid", "!excluded.invalid,*.invalid"} {
		t.Run(hosts, func(t *testing.T) {
			store := &TrustStore{Path: filepath.Join(t.TempDir(), "known_hosts")}
			old := key(t)
			before := []byte(hosts + " " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(old.PublicKey()))) + " # comment\r\n")
			if err := os.WriteFile(store.Path, before, 0600); err != nil {
				t.Fatal(err)
			}
			err := store.Callback(context.Background(), nil)("example.invalid:22", &net.TCPAddr{Port: 22}, key(t).PublicKey())
			var mismatch *HostKeyMismatch
			if !errors.As(err, &mismatch) {
				t.Fatal(err)
			}
			err = store.RemoveHostKey(mismatch)
			after, _ := os.ReadFile(store.Path)
			if strings.Contains(hosts, "*") {
				if err == nil || !bytes.Equal(before, after) {
					t.Fatal("wildcard was erased", err)
				}
			} else {
				if err != nil || string(after) != strings.Replace(string(before), hosts, "other.invalid", 1) {
					t.Fatal("other alias changed", err)
				}
			}
		})
	}
}

func TestHostKeyCleanupStaleAndBackupFailurePreserveTrust(t *testing.T) {
	for _, scenario := range []string{"stale", "backup", "revoked", "malformed"} {
		t.Run(scenario, func(t *testing.T) {
			store := &TrustStore{Path: filepath.Join(t.TempDir(), "known_hosts")}
			old, next := key(t), key(t)
			before := []byte(knownhosts.Line([]string{"example.invalid"}, old.PublicKey()) + "\n")
			if scenario == "revoked" {
				before = []byte("@revoked " + knownhosts.Line([]string{"example.invalid"}, next.PublicKey()) + "\n")
			}
			if scenario == "malformed" {
				before = []byte("broken file\n")
			}
			if err := os.WriteFile(store.Path, before, 0600); err != nil {
				t.Fatal(err)
			}
			err := store.Callback(context.Background(), nil)("example.invalid:22", &net.TCPAddr{Port: 22}, next.PublicKey())
			var mismatch *HostKeyMismatch
			if scenario == "revoked" || scenario == "malformed" {
				if err == nil || errors.As(err, &mismatch) {
					t.Fatal("non-mismatch offered cleanup", err)
				}
				return
			}
			if !errors.As(err, &mismatch) {
				t.Fatal(err)
			}
			if scenario == "stale" {
				before = append(before, []byte("# edited\n")...)
				if err = os.WriteFile(store.Path, before, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err = os.Mkdir(store.Path+".before-cleanup", 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err = store.RemoveHostKey(mismatch); err == nil {
				t.Fatal("unsafe cleanup succeeded")
			}
			after, _ := os.ReadFile(store.Path)
			if !bytes.Equal(after, before) {
				t.Fatal("failed cleanup changed file")
			}
		})
	}
}

func TestHostKeyCleanupRealHandshake(t *testing.T) {
	var config *ssh.ServerConfig
	server := startSSH(t, "fixture", func(c *ssh.ServerConfig) { config = c })
	profile := testProfile(server.address)
	trust := testTrust(t)
	approvals := 0
	connect := func(approve bool) error {
		conn, err := Dial(context.Background(), profile, Credentials{Password: "fixture"}, trust, func(HostPrompt) bool { approvals++; return approve })
		if conn != nil {
			conn.Close()
		}
		return err
	}
	if err := connect(true); err != nil {
		t.Fatal(err)
	}
	config.AddHostKey(key(t))
	err := connect(true)
	var mismatch *HostKeyMismatch
	if !errors.As(err, &mismatch) || approvals != 1 || mismatch.Address != server.address {
		t.Fatal("handshake lost typed mismatch", err)
	}
	if err = trust.RemoveHostKey(mismatch); err != nil {
		t.Fatal(err)
	}
	if err = connect(false); err == nil || approvals != 2 {
		t.Fatal("new key did not require approval", err)
	}
	if err = connect(true); err != nil || approvals != 3 {
		t.Fatal("explicit new trust failed", err)
	}
	if err = connect(true); err != nil || approvals != 3 {
		t.Fatal("new trusted key reprompted", err)
	}
}

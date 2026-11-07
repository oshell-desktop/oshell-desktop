package core

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func hostKeyTestSigners(t *testing.T) (ssh.Signer, ssh.Signer) {
	t.Helper()
	r, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	e, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := ssh.NewSignerFromKey(r)
	if err != nil {
		t.Fatal(err)
	}
	es, err := ssh.NewSignerFromKey(e)
	if err != nil {
		t.Fatal(err)
	}
	return rs, es
}

func TestHostKeyAlgorithmsMatchRules(t *testing.T) {
	rsaKey, _ := hostKeyTestSigners(t)
	for _, tc := range []struct {
		name, address, pattern, marker string
		match                          bool
	}{
		{"plain", "host.invalid:22", "host.invalid", "", true},
		{"hashed", "host.invalid:2222", knownhosts.HashHostname("[host.invalid]:2222"), "", true},
		{"port", "host.invalid:2222", "[host.invalid]:2222", "", true},
		{"wrong-port", "host.invalid:22", "[host.invalid]:2222", "", false},
		{"ipv6", "[::1]:2200", "[::1]:2200", "", true},
		{"wildcard", "host.invalid:22", "*.invalid", "", true},
		{"negated", "host.invalid:22", "!host.invalid,*.invalid", "", false},
		{"alias", "host.invalid:22", "other.invalid,host.invalid", "", true},
		{"other", "host.invalid:22", "other.invalid", "", false},
		{"CA", "host.invalid:22", "host.invalid", "@cert-authority ", false},
		{"revoked", "host.invalid:22", "host.invalid", "@revoked ", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := testTrust(t)
			line := tc.marker + tc.pattern + " " + string(ssh.MarshalAuthorizedKey(rsaKey.PublicKey()))
			if tc.name == "revoked" {
				line += knownhosts.Line([]string{"host.invalid"}, rsaKey.PublicKey()) + "\n"
			}
			if err := os.WriteFile(store.Path, []byte(line), 0600); err != nil {
				t.Fatal(err)
			}
			base := []string{ssh.KeyAlgoECDSA256, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoED25519, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSA}
			original := slices.Clone(base)
			cfg := &ssh.ClientConfig{HostKeyAlgorithms: base}
			if err := store.PreferHostKeyAlgorithms(tc.address, cfg); err != nil {
				t.Fatal(err)
			}
			want := original
			if tc.match {
				want = []string{ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSA, ssh.KeyAlgoECDSA256, ssh.KeyAlgoED25519}
			}
			if !slices.Equal(cfg.HostKeyAlgorithms, want) || !slices.Equal(base, original) {
				t.Fatalf("order=%v original=%v", cfg.HostKeyAlgorithms, base)
			}
			after, _ := os.ReadFile(store.Path)
			if string(after) != line {
				t.Fatal("lookup modified trust file")
			}
		})
	}
}

func TestHostKeyAlgorithmsPolicyAndMissingStore(t *testing.T) {
	store := &TrustStore{Path: filepath.Join(t.TempDir(), "known_hosts")}
	cfg := &ssh.ClientConfig{}
	if err := store.PreferHostKeyAlgorithms("host.invalid:22", cfg); err != nil || cfg.HostKeyAlgorithms != nil {
		t.Fatal("unknown host changed defaults", err)
	}
	if _, err := os.Stat(store.Path); !os.IsNotExist(err) {
		t.Fatal("lookup created a trust file")
	}
	rsaKey, _ := hostKeyTestSigners(t)
	if err := os.WriteFile(store.Path, []byte(knownhosts.Line([]string{"host.invalid"}, rsaKey.PublicKey())+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []bool{false, true} {
		cfg = &ssh.ClientConfig{}
		base := ssh.DefaultHostKeyAlgorithms()
		if legacy {
			configureLegacySSH(cfg)
			base = slices.Clone(cfg.HostKeyAlgorithms)
		}
		if err := store.PreferHostKeyAlgorithms("host.invalid:22", cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.HostKeyAlgorithms[0] != ssh.KeyAlgoRSASHA256 {
			t.Fatalf("RSA SHA-2 not preferred: %v", cfg.HostKeyAlgorithms)
		}
		want, got := slices.Clone(base), slices.Clone(cfg.HostKeyAlgorithms)
		slices.Sort(want)
		slices.Sort(got)
		if !slices.Equal(want, got) {
			t.Fatal("allowed algorithm set changed")
		}
	}
	cfg = &ssh.ClientConfig{HostKeyAlgorithms: []string{ssh.KeyAlgoED25519}}
	if err := store.PreferHostKeyAlgorithms("host.invalid:22", cfg); err != nil || !slices.Equal(cfg.HostKeyAlgorithms, []string{ssh.KeyAlgoED25519}) {
		t.Fatal("added an excluded RSA algorithm", err)
	}
	if err := os.WriteFile(store.Path, []byte("malformed known hosts\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.PreferHostKeyAlgorithms("host.invalid:22", cfg); err == nil {
		t.Fatal("malformed trust file ignored")
	}
}

func TestHostKeyAlgorithmsRealHandshake(t *testing.T) {
	trusted, other := hostKeyTestSigners(t)
	replacement, _ := hostKeyTestSigners(t)
	for _, scenario := range []string{"same", "changed", "unavailable", "legacy"} {
		t.Run(scenario, func(t *testing.T) {
			server := startSSH(t, "synthetic-password", func(cfg *ssh.ServerConfig) {
				cfg.AddHostKey(other)
				switch scenario {
				case "changed":
					cfg.AddHostKey(replacement)
				case "unavailable":
				default:
					cfg.AddHostKey(trusted)
				}
			})
			trust := testTrust(t)
			before := []byte(knownhosts.Line([]string{server.address}, trusted.PublicKey()) + "\n")
			if err := os.WriteFile(trust.Path, before, 0600); err != nil {
				t.Fatal(err)
			}
			profile := testProfile(server.address)
			profile.LegacySSH = scenario == "legacy"
			approvals := 0
			conn, err := Dial(context.Background(), profile, Credentials{Password: "synthetic-password"}, trust, func(HostPrompt) bool { approvals++; return true })
			if conn != nil {
				conn.Close()
			}
			if scenario == "same" || scenario == "legacy" {
				if err != nil {
					t.Fatal(err)
				}
				if server.auth.Load() != 1 {
					t.Fatal("connection not authenticated")
				}
			} else {
				var mismatch *HostKeyMismatch
				if !errors.As(err, &mismatch) || mismatch.Address != server.address {
					t.Fatalf("changed key not blocked: %v", err)
				}
				if server.auth.Load() != 0 {
					t.Fatal("password sent before rejecting key")
				}
			}
			if approvals != 0 {
				t.Fatal("known host incorrectly requested new trust")
			}
			after, _ := os.ReadFile(trust.Path)
			if !bytes.Equal(after, before) {
				t.Fatal("saved host key changed")
			}
		})
	}
}

func TestHostKeyAlgorithmsJumpAndTarget(t *testing.T) {
	rsaKey, ecKey := hostKeyTestSigners(t)
	configure := func(cfg *ssh.ServerConfig) { cfg.AddHostKey(rsaKey); cfg.AddHostKey(ecKey) }
	target, gateway := startSSH(t, "target", configure), startSSH(t, "gateway", configure)
	trust := testTrust(t)
	// Different trusted algorithms per hop, with a hashed target address.
	before := []byte(knownhosts.Line([]string{gateway.address}, rsaKey.PublicKey()) + "\n" +
		knownhosts.HashHostname(knownhosts.Normalize(target.address)) + " " + string(ssh.MarshalAuthorizedKey(target.hostKey)))
	if err := os.WriteFile(trust.Path, before, 0600); err != nil {
		t.Fatal(err)
	}
	p, j := testProfile(target.address), testProfile(gateway.address)
	p.Proxy = ProxySettings{Kind: "jump", Host: j.Host, Port: j.Port, Username: "fixture"}
	approvals := 0
	conn, err := Dial(context.Background(), p, Credentials{Password: "target", ProxyPassword: "gateway"}, trust, func(HostPrompt) bool { approvals++; return true })
	if conn != nil {
		conn.Close()
	}
	if err != nil || approvals != 0 || gateway.auth.Load() != 1 || target.auth.Load() != 1 {
		t.Fatalf("hop-specific trust failed: %v approvals=%d", err, approvals)
	}
	after, _ := os.ReadFile(trust.Path)
	if !bytes.Equal(after, before) {
		t.Fatal("jump handshake rewrote trust")
	}
}

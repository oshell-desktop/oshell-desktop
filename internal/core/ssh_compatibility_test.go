package core

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func compatibilityKey(t *testing.T, encrypted bool) (string, ssh.Signer, ed25519.PrivateKey) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "")
	if encrypted {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(private, "", []byte("synthetic-passphrase"))
	}
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "identity")
	if err = os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	return path, signer, private
}

func TestSSHCompatibilityPassphraseHandshake(t *testing.T) {
	for _, scenario := range []string{"unencrypted-saved", "encrypted-retry", "cancel", "retry-limit"} {
		t.Run(scenario, func(t *testing.T) {
			path, signer, _ := compatibilityKey(t, scenario != "unencrypted-saved")
			server := startSSH(t, "unused", func(c *ssh.ServerConfig) {
				c.PasswordCallback = nil
				c.PublicKeyCallback = func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
					if !bytes.Equal(key.Marshal(), signer.PublicKey().Marshal()) {
						return nil, fmt.Errorf("wrong synthetic key")
					}
					return nil, nil
				}
			})
			p := testProfile(server.address)
			p.IdentityFile = path
			prompts := 0
			cred := Credentials{Method: "privateKey", KeyPassphrase: "old-saved-value", Challenge: func(ctx context.Context, q AuthenticationPrompt) ([]string, error) {
				prompts++
				if q.Identity != p.Identity() || len(q.Echo) != 1 || q.Echo[0] {
					t.Error("incorrect passphrase prompt identity/echo")
				}
				if scenario == "cancel" {
					return nil, context.Canceled
				}
				if scenario == "retry-limit" || prompts == 1 {
					return []string{"incorrect"}, nil
				}
				return []string{"synthetic-passphrase"}, nil
			}}
			conn, err := Dial(context.Background(), p, cred, testTrust(t), acceptHost)
			if conn != nil {
				conn.Close()
			}
			switch scenario {
			case "unencrypted-saved":
				if err != nil || prompts != 0 {
					t.Fatalf("valid unencrypted key rejected: %v prompts=%d", err, prompts)
				}
			case "encrypted-retry":
				if err != nil || prompts != 2 {
					t.Fatalf("passphrase correction failed: %v prompts=%d", err, prompts)
				}
			case "cancel":
				if !errors.Is(err, context.Canceled) || prompts != 1 {
					t.Fatalf("cancel lost: %v prompts=%d", err, prompts)
				}
			case "retry-limit":
				if err == nil || prompts != 3 {
					t.Fatalf("retry limit lost: %v prompts=%d", err, prompts)
				}
			}
		})
	}
}

func TestSSHCompatibilityIdentityAgentSelection(t *testing.T) {
	path, expected, private := compatibilityKey(t, true)
	_, other, otherPrivate := compatibilityKey(t, false)
	ring := agent.NewKeyring()
	for _, k := range []ed25519.PrivateKey{otherPrivate, private} {
		if err := ring.Add(agent.AddedKey{PrivateKey: k}); err != nil {
			t.Fatal(err)
		}
	}
	agents, err := ring.Signers()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"auto", "identities-only", "private-key", "public-file", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			p := NewProfile()
			p.IdentityFile = path
			p.resolvedIdentitiesOnly = mode == "identities-only"
			p.resolvedAgentDisabled = mode == "disabled"
			if mode == "public-file" {
				p.IdentityFile = filepath.Join(t.TempDir(), "identity.pub")
				if err := os.WriteFile(p.IdentityFile, ssh.MarshalAuthorizedKey(expected.PublicKey()), 0600); err != nil {
					t.Fatal(err)
				}
			}
			prompts := 0
			cred := Credentials{Challenge: func(context.Context, AuthenticationPrompt) ([]string, error) {
				prompts++
				return []string{"synthetic-passphrase"}, nil
			}}
			if mode == "private-key" {
				cred.Method = "privateKey"
			}
			got, err := loadSSHSigners(context.Background(), p, cred, agents)
			if err != nil {
				t.Fatal(err)
			}
			wantCount := 2
			if mode == "identities-only" || mode == "private-key" || mode == "disabled" {
				wantCount = 1
			}
			if len(got) != wantCount || !bytes.Equal(got[0].PublicKey().Marshal(), expected.PublicKey().Marshal()) {
				t.Fatal("configured agent identity not first / unrelated key included")
			}
			if mode == "disabled" {
				if prompts != 1 {
					t.Fatal("disabled agent used")
				}
			} else if prompts != 0 {
				t.Fatal("asked to decrypt identity already in agent")
			}
			if wantCount == 2 && !bytes.Equal(got[1].PublicKey().Marshal(), other.PublicKey().Marshal()) {
				t.Fatal("other agent identity lost")
			}
			sig, err := got[0].Sign(rand.Reader, []byte("synthetic authentication challenge"))
			if err != nil || expected.PublicKey().Verify([]byte("synthetic authentication challenge"), sig) != nil {
				t.Fatal("selected agent cannot sign", err)
			}
		})
	}
}

func TestSSHCompatibilityResolvedIdentityLimits(t *testing.T) {
	for _, identity := range []string{"none", filepath.Join(t.TempDir(), "missing-key")} {
		t.Run(filepath.Base(identity), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			path, _, _ := compatibilityKey(t, false)
			data, _ := os.ReadFile(path)
			if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519"), data, 0600); err != nil {
				t.Fatal(err)
			}
			r := ResolvedSSH{Profile: NewProfile()}
			applyResolvedSSH(&r, "hostname fixture.invalid\nidentityfile "+identity+"\nidentitiesonly yes\nidentityagent none\n")
			got, err := loadSSHSigners(context.Background(), r.Profile, Credentials{}, []ssh.Signer{key(t)})
			if err != nil || len(got) != 0 {
				t.Fatalf("disabled identity fell back to default/agent: %d %v", len(got), err)
			}
			if SSHKeyAuthenticationCandidate(context.Background(), r.Profile) {
				t.Fatal("disabled agent advertised as authentication candidate")
			}
		})
	}
}

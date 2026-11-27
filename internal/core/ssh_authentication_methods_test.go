package core

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestSharedSSHAuthenticationMethodsRoundTrip(t *testing.T) {
	fixture, err := os.ReadFile("testdata/oshell-swift-shared-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	master := []byte("interop-fixture-master-🔐")
	plain, err := OpenShared(fixture, master)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err = json.Unmarshal(plain, &raw); err != nil {
		t.Fatal(err)
	}
	methods := []string{"gssapi-with-mic", "keyboard-interactive", "publickey", "password"}
	profiles := raw["profiles"].([]any)
	profiles[0].(map[string]any)["sshAuthenticationMethods"] = methods
	profiles[1].(map[string]any)["sshAuthenticationMethods"] = []string{"publickey"}
	plain, _ = json.Marshal(raw)
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			wire, err := sealSharedVersion(plain, master, version)
			if err != nil {
				t.Fatal(err)
			}
			doc, err := DecodeSharedDocument(wire, master)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := doc.Configuration()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cfg.Profiles[0].SSHAuthenticationMethods, methods) || cfg.Profiles[2].SSHAuthenticationMethods != nil {
				t.Fatal("authentication policy lost or invented")
			}
			copy := cfg.Clone()
			copy.Profiles[0].SSHAuthenticationMethods[0] = "password"
			if cfg.Profiles[0].SSHAuthenticationMethods[0] != methods[0] {
				t.Fatal("clone shares authentication policy")
			}
			store := &Store{Dir: t.TempDir()}
			cfg.Profiles[0].Name = "Edited on Desktop"
			if err = store.Save(cfg); err != nil {
				t.Fatal(err)
			}
			loaded, err := store.Load()
			if err != nil || !reflect.DeepEqual(loaded.Profiles[0].SSHAuthenticationMethods, methods) {
				t.Fatal("local save lost policy", err)
			}
			for i := 0; i < 3; i++ {
				wire, err = doc.Encode(master)
				if err != nil {
					t.Fatal(err)
				}
				doc, err = DecodeSharedDocument(wire, master)
				if err != nil || !reflect.DeepEqual(doc.Profiles[0].Definition.SSHAuthenticationMethods, methods) {
					t.Fatal("shared round trip lost policy", err)
				}
			}
			archive := PortableArchive{Format: "OShell.sessions", Version: 4, Profiles: doc.Profiles, Proxies: doc.Proxies, Directories: doc.Directories, Links: doc.Links.Entries}
			encoded, err := archive.Encode()
			if err != nil {
				t.Fatal(err)
			}
			back, err := DecodePortableArchive(encoded)
			if err != nil || !reflect.DeepEqual(back.Profiles[0].Definition.SSHAuthenticationMethods, methods) {
				t.Fatal("archive policy", err)
			}
			for _, profile := range back.Profiles {
				secret, err := DecryptMacPassword(*profile.Password, master, profile.Definition)
				if err != nil || secret != "fixture-"+profile.Definition.Kind {
					t.Fatal("credential identity changed", err)
				}
			}
		})
	}
}

func TestSharedSSHAuthenticationMethodsValidation(t *testing.T) {
	p := NewProfile()
	p.Host = "fixture.invalid"
	encoded, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("sshAuthenticationMethods")) {
		t.Fatal("default policy must remain absent")
	}
	for _, value := range []any{[]string{}, []string{"password", "password"}, []string{"unknown"}, []string{"password", "publickey", "keyboard-interactive", "gssapi-with-mic", "password"}, "password", true, []any{1}} {
		var profile map[string]any
		json.Unmarshal(encoded, &profile)
		profile["sshAuthenticationMethods"] = value
		raw, _ := json.Marshal(map[string]any{"profiles": []any{profile}, "preferences": map[string]any{}})
		if _, err := decodeNativeDocument(raw); err == nil {
			t.Fatalf("accepted invalid method list %T", value)
		}
	}
	for _, kind := range []string{"local", "ftp"} {
		p.Kind, p.SSHAuthenticationMethods = kind, []string{"password"}
		if p.Validate() == nil {
			t.Fatal("non-SSH profile accepted policy")
		}
		if _, err := json.Marshal(p); err == nil {
			t.Fatal("non-SSH policy serialized")
		}
	}
	p.Kind, p.SSHAuthenticationMethods = "ssh", []string{}
	if _, err := json.Marshal(p); err == nil {
		t.Fatal("empty explicit policy silently omitted")
	}
}

func TestSharedSSHAuthenticationMethodsSync(t *testing.T) {
	ctx := context.Background()
	remote := t.TempDir()
	a, cfg, master := syncEngineFixture(t, remote)
	methods := []string{"gssapi-with-mic", "keyboard-interactive"}
	cfg.Profiles[0].SSHAuthenticationMethods = methods
	if err := a.Store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := a.Vault.Put(cfg.Profiles[0], "fixture-auth-password"); err != nil {
		t.Fatal(err)
	}
	plan, err := a.Prepare(ctx, cfg, master, true)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = plan.Commit(ctx, nil, master)
	if err != nil {
		t.Fatal(err)
	}
	b, other, _ := syncEngineFixture(t, remote)
	plan, err = b.Prepare(ctx, other, master, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = plan.ChooseInitial(false); err != nil {
		t.Fatal(err)
	}
	other, err = plan.Commit(ctx, nil, master)
	if err != nil || !reflect.DeepEqual(other.Profiles[0].SSHAuthenticationMethods, methods) {
		t.Fatal("download policy", err)
	}
	secret, err := b.Vault.GetFor(other.Profiles[0], other.Profiles[0].Identity())
	if err != nil || secret != "fixture-auth-password" {
		t.Fatal("download credential", err)
	}
	before, err := b.Transport.Fetch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = b.Prepare(ctx, other, master, false)
	if err != nil {
		t.Fatal(err)
	}
	other, err = plan.Commit(ctx, nil, master)
	if err != nil {
		t.Fatal(err)
	}
	after, err := b.Transport.Fetch(ctx)
	if err != nil || !bytes.Equal(before.Data, after.Data) {
		t.Fatal("unchanged sync rewrote remote", err)
	}
	// Different authentication edits remain a whole-session conflict.
	cfg.Profiles[0].SSHAuthenticationMethods = []string{"password"}
	if err = a.Store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	plan, err = a.Prepare(ctx, cfg, master, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = plan.Commit(ctx, nil, master); err != nil {
		t.Fatal(err)
	}
	other.Profiles[0].SSHAuthenticationMethods = []string{"publickey"}
	if err = b.Store.Save(other); err != nil {
		t.Fatal(err)
	}
	plan, err = b.Prepare(ctx, other, master, false)
	if err != nil || len(plan.Conflicts) != 1 {
		t.Fatal("authentication edits did not conflict", err)
	}
	if _, err = plan.Commit(ctx, nil, master); err == nil {
		t.Fatal("unresolved authentication conflict committed")
	}
}

func TestSSHAuthenticationMethodsRestrictAndOrder(t *testing.T) {
	for _, tc := range []struct {
		name            string
		methods         []string
		rejectPassword  bool
		onlyInteractive bool
		want            []string
		wantError       string
	}{
		{name: "password", methods: []string{"password"}, want: []string{"password"}},
		{name: "interactive", methods: []string{"keyboard-interactive"}, want: []string{"keyboard-interactive"}},
		{name: "order", methods: []string{"keyboard-interactive", "password"}, want: []string{"keyboard-interactive"}},
		{name: "allowed fallback", methods: []string{"password", "keyboard-interactive"}, rejectPassword: true, want: []string{"password", "keyboard-interactive"}},
		{name: "forbidden fallback", methods: []string{"password"}, onlyInteractive: true, wantError: "authenticate"},
		{name: "gssapi only", methods: []string{"gssapi-with-mic"}, wantError: "GSSAPI"},
		{name: "gssapi with allowed password", methods: []string{"gssapi-with-mic", "password"}, want: []string{"password"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempts := make(chan string, 10)
			server := startSSH(t, "fixture", func(c *ssh.ServerConfig) {
				c.PasswordCallback = func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
					attempts <- "password"
					if tc.rejectPassword || string(password) != "fixture" {
						return nil, fmt.Errorf("rejected")
					}
					return nil, nil
				}
				if tc.onlyInteractive {
					c.PasswordCallback = nil
				}
				c.KeyboardInteractiveCallback = func(_ ssh.ConnMetadata, ask ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
					attempts <- "keyboard-interactive"
					answers, err := ask("", "", []string{"Password:"}, []bool{false})
					if err != nil {
						return nil, err
					}
					if len(answers) != 1 || answers[0] != "fixture" {
						return nil, fmt.Errorf("wrong answer")
					}
					return nil, nil
				}
			})
			p := testProfile(server.address)
			p.SSHAuthenticationMethods = tc.methods
			// A forbidden key must not be loaded or prompt for its passphrase.
			p.IdentityFile = filepath.Join(t.TempDir(), "missing-private-key")
			c, err := Dial(context.Background(), p, Credentials{Password: "fixture", Challenge: func(_ context.Context, prompt AuthenticationPrompt) ([]string, error) {
				if len(prompt.Questions) != 1 {
					return nil, fmt.Errorf("unexpected fixture challenge")
				}
				return []string{"fixture"}, nil
			}}, testTrust(t), acceptHost)
			if c != nil {
				c.Close()
			}
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected %s, got %v", tc.wantError, err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			var got []string
			for len(attempts) > 0 {
				got = append(got, <-attempts)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("methods %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSSHAuthenticationMethodsPublicKeyAndJumpIsolation(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	server := startSSH(t, "fixture", func(c *ssh.ServerConfig) {
		c.PublicKeyCallback = func(_ ssh.ConnMetadata, offered ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(offered.Marshal(), accepted.Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("wrong key")
		}
	})
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(t.TempDir(), "identity")
	if err = os.WriteFile(keyPath, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	jump := startSSH(t, "jump-fixture")
	p := testProfile(server.address)
	p.IdentityFile, p.SSHAuthenticationMethods = keyPath, []string{"publickey"}
	p.JumpHost = "tester@" + jump.address
	c, err := Dial(context.Background(), p, Credentials{Password: "fixture", ProxyPassword: "jump-fixture"}, testTrust(t), acceptHost)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if server.auth.Load() != 0 || jump.auth.Load() != 1 {
		t.Fatal("target authentication policy leaked into jump or allowed password")
	}
}

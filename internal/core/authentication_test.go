package core

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"golang.org/x/crypto/ssh"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestSSHInteractiveMultipleQuestions(t *testing.T) {
	server := startSSH(t, "unused", func(c *ssh.ServerConfig) {
		c.PasswordCallback = nil
		c.KeyboardInteractiveCallback = func(_ ssh.ConnMetadata, ask ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			answers, e := ask("MFA", "Test verification", []string{"Account label:", "One-time code:"}, []bool{true, false})
			if e != nil {
				return nil, e
			}
			if !reflect.DeepEqual(answers, []string{"work", "123456"}) {
				return nil, fmt.Errorf("wrong answers")
			}
			return nil, nil
		}
	})
	p := testProfile(server.address)
	called := false
	c, e := Dial(context.Background(), p, Credentials{Challenge: func(ctx context.Context, q AuthenticationPrompt) ([]string, error) {
		called = true
		if q.Identity != p.Identity() || q.Name != "MFA" || len(q.Echo) != 2 || !q.Echo[0] || q.Echo[1] {
			t.Error("prompt metadata lost")
		}
		return []string{"work", "123456"}, nil
	}}, testTrust(t), acceptHost)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if !called {
		t.Fatal("challenge not shown")
	}
}
func TestSSHDefaultKeysTryAllCandidates(t *testing.T) {
	var accepted ssh.PublicKey
	server := startSSH(t, "unused", func(c *ssh.ServerConfig) {
		c.PasswordCallback = nil
		c.PublicKeyCallback = func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if accepted != nil && bytes.Equal(accepted.Marshal(), key.Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("wrong key")
		}
	})
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".ssh")
	os.MkdirAll(dir, 0700)
	for i, name := range []string{"id_ed25519", "id_ecdsa"} {
		public, private, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		block, e := ssh.MarshalPrivateKey(private, "")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, name), pem.EncodeToMemory(block), 0600); e != nil {
			t.Fatal(e)
		}
		if i == 1 {
			accepted, _ = ssh.NewPublicKey(public)
		}
	}
	p := testProfile(server.address)
	c, e := Dial(context.Background(), p, Credentials{}, testTrust(t), acceptHost)
	if e != nil {
		t.Fatal(e)
	}
	c.Close()
}
func TestResolvedSSHDoesNotPromoteOptionalKeyToExplicit(t *testing.T) {
	key := filepath.Join(t.TempDir(), "encrypted-key")
	os.WriteFile(key, []byte("fixture"), 0600)
	p := NewProfile()
	p.Host = "alias"
	r := ResolvedSSH{Profile: p}
	applyResolvedSSH(&r, "hostname server.example\nuser deploy\nport 2222\nidentityfile "+key+"\nproxycommand nc %h %p\n")
	if r.Profile.IdentityFile != "" || len(r.Profile.resolvedIdentityFiles) != 1 || r.Profile.Host != "server.example" || r.Profile.Port != 2222 || r.ProxyCommand != "nc %h %p" {
		t.Fatal(r)
	}
}

func TestRejectedPasswordIsNotVerifiedByPasswordlessInteractiveSuccess(t *testing.T) {
	for _, emptyChallenge := range []bool{false, true} {
		t.Run(fmt.Sprint(emptyChallenge), func(t *testing.T) {
			var attempts atomic.Int32
			server := startSSH(t, "unused", func(config *ssh.ServerConfig) {
				config.PasswordCallback = func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
					attempts.Add(1)
					return nil, fmt.Errorf("rejected")
				}
				config.KeyboardInteractiveCallback = func(_ ssh.ConnMetadata, ask ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
					if emptyChallenge {
						if _, err := ask("", "", nil, nil); err != nil {
							return nil, err
						}
					}
					return nil, nil
				}
			})
			connection, err := Dial(context.Background(), testProfile(server.address), Credentials{Password: "wrong-fixture-password", SavePassword: true}, testTrust(t), acceptHost)
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			if attempts.Load() != 1 || connection.PasswordVerified {
				t.Fatal("unrelated authentication verified a rejected password")
			}
		})
	}
}
func TestPasswordPartialSuccessRemainsVerifiedAfterOTP(t *testing.T) {
	server := startSSH(t, "unused", func(config *ssh.ServerConfig) {
		config.PasswordCallback = func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if string(password) != "fixture-password" {
				return nil, fmt.Errorf("wrong password")
			}
			return nil, &ssh.PartialSuccessError{Next: ssh.ServerAuthCallbacks{KeyboardInteractiveCallback: func(_ ssh.ConnMetadata, ask ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
				answers, err := ask("MFA", "", []string{"One-time code:"}, []bool{false})
				if err != nil {
					return nil, err
				}
				if len(answers) != 1 || answers[0] != "fixture-otp" {
					return nil, fmt.Errorf("wrong OTP")
				}
				return nil, nil
			}}}
		}
	})
	connection, err := Dial(context.Background(), testProfile(server.address), Credentials{Password: "fixture-password", SavePassword: true, Challenge: func(context.Context, AuthenticationPrompt) ([]string, error) { return []string{"fixture-otp"}, nil }}, testTrust(t), acceptHost)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if !connection.PasswordVerified {
		t.Fatal("accepted password lost its verification after successful MFA")
	}
}
func TestPublicKeySuccessDoesNotVerifyUnusedPassword(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := ssh.NewPublicKey(public)
	server := startSSH(t, "unused", func(config *ssh.ServerConfig) {
		config.PasswordCallback = nil
		config.PublicKeyCallback = func(_ ssh.ConnMetadata, offered ssh.PublicKey) (*ssh.Permissions, error) {
			if !bytes.Equal(key.Marshal(), offered.Marshal()) {
				return nil, fmt.Errorf("wrong key")
			}
			return nil, nil
		}
	})
	profile := testProfile(server.address)
	profile.IdentityFile = filepath.Join(t.TempDir(), "fixture-key")
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(profile.IdentityFile, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	connection, err := Dial(context.Background(), profile, Credentials{Password: "unused-password", SavePassword: true}, testTrust(t), acceptHost)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if connection.PasswordVerified {
		t.Fatal("key authentication verified an unused password")
	}
}

func TestPasswordPartialSuccessIsNotPublishedAfterFailedMFA(t *testing.T) {
	server := startSSH(t, "unused", func(config *ssh.ServerConfig) {
		config.PasswordCallback = func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if string(password) != "fixture-password" {
				return nil, fmt.Errorf("wrong password")
			}
			return nil, &ssh.PartialSuccessError{Next: ssh.ServerAuthCallbacks{KeyboardInteractiveCallback: func(_ ssh.ConnMetadata, ask ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
				if _, err := ask("MFA", "", []string{"One-time code:"}, []bool{false}); err != nil {
					return nil, err
				}
				return nil, fmt.Errorf("rejected OTP")
			}}}
		}
	})
	socket, err := net.Dial("tcp", server.address)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	var verified atomic.Bool
	client, err := authenticate(context.Background(), socket, testProfile(server.address), Credentials{
		Password: "fixture-password", SavePassword: true,
		Challenge: func(context.Context, AuthenticationPrompt) ([]string, error) {
			return []string{"wrong-otp"}, nil
		},
	}, testTrust(t), acceptHost, &verified)
	if client != nil {
		defer client.Close()
	}
	if err == nil || verified.Load() {
		t.Fatal("incomplete MFA published password verification")
	}
}

func TestDynamicPasswordChallengeIsVerifiedOnlyForLoginPrompts(t *testing.T) {
	for _, mode := range []string{"password", "interactive-password", "otp"} {
		t.Run(mode, func(t *testing.T) {
			server := startSSH(t, "fixture-password", func(config *ssh.ServerConfig) {
				if mode == "password" {
					return
				}
				config.PasswordCallback = nil
				config.KeyboardInteractiveCallback = func(_ ssh.ConnMetadata, ask ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
					prompt := "Password:"
					if mode == "otp" {
						prompt = "One-time code:"
					}
					answers, err := ask("", "", []string{prompt}, []bool{false})
					if err != nil {
						return nil, err
					}
					if len(answers) != 1 || answers[0] != "fixture-password" {
						return nil, fmt.Errorf("wrong answer")
					}
					return nil, nil
				}
			})
			passwordPrompts := 0
			acceptedPassword := ""
			connection, err := Dial(context.Background(), testProfile(server.address), Credentials{
				PasswordAuthenticated: func(password string) { acceptedPassword = password },
				PasswordChallenge: func(context.Context, AuthenticationPrompt) (string, error) {
					passwordPrompts++
					return "fixture-password", nil
				},
				Challenge: func(context.Context, AuthenticationPrompt) ([]string, error) {
					return []string{"fixture-password"}, nil
				},
			}, testTrust(t), acceptHost)
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			want := mode == "password" // Generic interactive Password: remains manual.
			if connection.PasswordVerified != want || (passwordPrompts == 1) != want || (acceptedPassword == "fixture-password") != want {
				t.Fatal("dynamic login password was confused with an OTP", passwordPrompts, connection.PasswordVerified)
			}
		})
	}
}

func TestRejectedSavedPasswordRetriesWithoutReplayingIt(t *testing.T) {
	for _, mode := range []string{"success", "cancel", "exhausted"} {
		t.Run(mode, func(t *testing.T) {
			var attempts atomic.Int32
			server := startSSH(t, "unused", func(config *ssh.ServerConfig) {
				config.PasswordCallback = func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
					number := attempts.Add(1)
					if number > 1 && string(password) == "stale-password" {
						t.Error("replayed stale password")
					}
					if string(password) != "replacement-password" {
						return nil, fmt.Errorf("rejected")
					}
					return nil, nil
				}
			})
			prompts, published := 0, ""
			connection, err := Dial(context.Background(), testProfile(server.address), Credentials{
				Password: "stale-password",
				PasswordChallenge: func(_ context.Context, request AuthenticationPrompt) (string, error) {
					prompts++
					if request.Instruction == "" {
						t.Error("retry lacks failure context")
					}
					if mode == "cancel" {
						return "", context.Canceled
					}
					if mode == "exhausted" {
						return "still-wrong", nil
					}
					return "replacement-password", nil
				},
				PasswordAuthenticated: func(password string) { published = password },
			}, testTrust(t), acceptHost)
			if connection != nil {
				defer connection.Close()
			}
			switch mode {
			case "success":
				if err != nil || prompts != 1 || attempts.Load() != 2 || published != "replacement-password" || !connection.PasswordVerified {
					t.Fatal("retry did not authenticate replacement", err, prompts, attempts.Load())
				}
			case "cancel":
				if err == nil || prompts != 1 || attempts.Load() != 1 || published != "" {
					t.Fatal("cancel did not stop authentication")
				}
			case "exhausted":
				if err == nil || prompts != 2 || attempts.Load() != 3 || published != "" {
					t.Fatal("password retry was not bounded", prompts, attempts.Load())
				}
			}
		})
	}
}

func TestInteractivePasswordAndOTPKeepAcceptedPassword(t *testing.T) {
	for _, mode := range []string{"password-first", "otp-first", "other-account", "echoed-password"} {
		t.Run(mode, func(t *testing.T) {
			var identity SSHIdentity
			server := startSSH(t, "unused", func(config *ssh.ServerConfig) {
				config.PasswordCallback = nil
				config.KeyboardInteractiveCallback = func(_ ssh.ConnMetadata, ask ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
					passwordPrompt := "(" + identity.User + "@" + identity.Host + ") Password:"
					if mode == "other-account" {
						passwordPrompt = "another@unrelated.test's password:"
					}
					prompts := []string{passwordPrompt, "Verification code:"}
					if mode == "otp-first" {
						prompts[0], prompts[1] = prompts[1], prompts[0]
					}
					for _, prompt := range prompts {
						answers, err := ask("MFA", "", []string{prompt}, []bool{mode == "echoed-password" && prompt == passwordPrompt})
						if err != nil {
							return nil, err
						}
						want := "fixture-otp"
						if prompt == passwordPrompt && mode != "other-account" && mode != "echoed-password" {
							want = "fixture-login-password"
						}
						if len(answers) != 1 || answers[0] != want {
							return nil, fmt.Errorf("wrong factor")
						}
					}
					return nil, nil
				}
			})
			profile := testProfile(server.address)
			identity = profile.Identity()
			published := ""
			passwordPrompts := 0
			connection, err := Dial(context.Background(), profile, Credentials{
				PasswordChallenge: func(context.Context, AuthenticationPrompt) (string, error) {
					passwordPrompts++
					return "fixture-login-password", nil
				},
				Challenge:             func(context.Context, AuthenticationPrompt) ([]string, error) { return []string{"fixture-otp"}, nil },
				PasswordAuthenticated: func(password string) { published = password },
			}, testTrust(t), acceptHost)
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			want := mode == "password-first" || mode == "otp-first"
			if connection.PasswordVerified != want || (passwordPrompts == 1) != want || (published == "fixture-login-password") != want {
				t.Fatal("MFA lost password evidence or published an unrelated answer", passwordPrompts, connection.PasswordVerified)
			}
		})
	}
}

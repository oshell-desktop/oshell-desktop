package desktop

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"io"
	"net"
	"os"
	"oshell-desktop/internal/core"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func savedConnectionFixture(t *testing.T, configure ...func(*ssh.ServerConfig, ssh.Signer)) (*App, core.Profile, *atomic.Int32) {
	return savedConnectionFixtureWithHandlers(t, nil, configure...)
}

func savedConnectionFixtureWithHandlers(t *testing.T, prepare func(sftp.Handlers) sftp.Handlers, configure ...func(*ssh.ServerConfig, ssh.Signer)) (*App, core.Profile, *atomic.Int32) {
	t.Helper()
	t.Setenv("OSHELL_TEST_NO_AGENT", "1")
	t.Setenv("OSHELL_TEST_EXPLICIT_KEYS_ONLY", "1")
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	var mu sync.Mutex
	var sockets []net.Conn
	var wg sync.WaitGroup
	t.Cleanup(func() {
		listener.Close()
		mu.Lock()
		for _, c := range sockets {
			c.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	server := &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if meta.User() != "tester" || string(password) != "saved-fixture-password" {
			return nil, fmt.Errorf("denied")
		}
		return nil, nil
	}}
	for _, apply := range configure {
		apply(server, signer)
	}
	server.AddHostKey(signer)
	fileHandlers := sftp.InMemHandler()
	fileHandlers.FileGet = isolatedSFTPReadHandles{fileHandlers.FileGet}
	fileHandlers.FilePut = isolatedSFTPWriteHandles{fileHandlers.FilePut}
	if prepare != nil {
		fileHandlers = prepare(fileHandlers)
	}
	if err := fileHandlers.FileCmd.Filecmd(sftp.NewRequest("Mkdir", "/fixture-folder")); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("OSHELL_NATIVE_AUTH_MODE") == "multiselect" {
		for _, name := range []string{"alpha.txt", "beta.txt"} {
			request := sftp.NewRequest("Put", "/"+name)
			request.Flags = 2 | 8 | 32
			writer, err := fileHandlers.FilePut.Filewrite(request)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = writer.WriteAt([]byte("native-"+name+"\n"), 0); err != nil {
				t.Fatal(err)
			}
		}
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			socket, err := listener.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			mu.Lock()
			sockets = append(sockets, socket)
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer socket.Close()
				conn, channels, requests, err := ssh.NewServerConn(socket, server)
				if err != nil {
					return
				}
				defer conn.Close()
				go ssh.DiscardRequests(requests)
				for request := range channels {
					if request.ChannelType() == "direct-tcpip" {
						var destination struct {
							Host       string
							Port       uint32
							Origin     string
							OriginPort uint32
						}
						if ssh.Unmarshal(request.ExtraData(), &destination) != nil || destination.Host != "127.0.0.1" {
							request.Reject(ssh.Prohibited, "loopback fixture only")
							continue
						}
						out, err := net.DialTimeout("tcp", net.JoinHostPort(destination.Host, strconv.Itoa(int(destination.Port))), time.Second)
						if err != nil {
							request.Reject(ssh.ConnectionFailed, "fixture unavailable")
							continue
						}
						channel, requests, err := request.Accept()
						if err != nil {
							out.Close()
							continue
						}
						go ssh.DiscardRequests(requests)
						go func() { defer out.Close(); defer channel.Close(); io.Copy(out, channel) }()
						go func() { defer out.Close(); defer channel.Close(); io.Copy(channel, out) }()
						continue
					}
					if request.ChannelType() != "session" {
						request.Reject(ssh.UnknownChannelType, "session only")
						continue
					}
					channel, requests, err := request.Accept()
					if err != nil {
						continue
					}
					go func() {
						defer channel.Close()
						for req := range requests {
							switch req.Type {
							case "subsystem":
								var request struct{ Name string }
								if ssh.Unmarshal(req.Payload, &request) != nil || request.Name != "sftp" {
									req.Reply(false, nil)
									continue
								}
								req.Reply(true, nil)
								server := sftp.NewRequestServer(channel, fileHandlers)
								server.Serve()
								server.Close()
								return
							case "pty-req", "env", "window-change":
								req.Reply(true, nil)
							case "shell":
								req.Reply(true, nil)
								io.WriteString(channel, "SAVED_CREDENTIAL_READY\r\n")
								go io.Copy(io.Discard, channel)
							default:
								req.Reply(false, nil)
							}
						}
					}()
				}
			}()
		}
	}()
	dir := t.TempDir()
	store := &core.Store{Dir: dir}
	cfg := core.DefaultConfig()
	p := core.NewProfile()
	p.Name = "Saved credential fixture"
	p.Host = "127.0.0.1"
	p.Username = "tester"
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	p.Port, _ = strconv.Atoi(port)
	// An explicit fixture key prevents discovery of the user's private keys.
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	p.IdentityFile = filepath.Join(dir, "fixture-key")
	if err = os.WriteFile(p.IdentityFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), 0600); err != nil {
		t.Fatal(err)
	}
	cfg.Profiles = []core.Profile{p}
	if err = store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	a := newApp(store, cfg)
	if err = os.WriteFile(a.trust.Path, []byte(knownhosts.Line([]string{knownhosts.Normalize(listener.Addr().String())}, signer.PublicKey())+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if a.features.authCancel != nil {
			a.features.authCancel()
		}
		for _, tab := range a.tabs {
			a.walk(tab.root, a.dispose)
		}
	})
	return a, p, &accepted
}
func TestSavedCredentialConnectsWithoutSecondAuthenticationConfirmation(t *testing.T) {
	a, p, accepted := savedConnectionFixture(t)
	if err := a.vault.PutFor(p, "saved-fixture-password", p.Identity()); err != nil {
		t.Fatal(err)
	}
	a.request(p, 0)
	waitOperator(t, a, func() bool {
		return len(a.tabs) == 1 && a.tabs[0].active.term != nil && strings.Contains(a.tabs[0].active.term.Text(), "SAVED_CREDENTIAL_READY")
	})
	if a.authOpen || a.features.authPreparing || len(a.challenges) != 0 || accepted.Load() != 1 {
		t.Fatal("saved connection required redundant authentication", a.status)
	}
	if a.password != "" || a.features.savedPassword != "" || a.features.savedIdentity != nil {
		t.Fatal("automatic connection retained dialog credentials")
	}
}
func TestSavedCredentialIdentityMismatchDoesNotAutomaticallyConnect(t *testing.T) {
	a, p, accepted := savedConnectionFixture(t)
	wrong := p.Identity()
	wrong.Host = "different.fixture.invalid"
	if err := a.vault.PutFor(p, "saved-fixture-password", wrong); err != nil {
		t.Fatal(err)
	}
	a.request(p, 0)
	waitOperator(t, a, func() bool { return !a.features.authPreparing })
	if !a.authOpen || len(a.tabs) != 0 || accepted.Load() != 0 || a.password != "" {
		t.Fatal("mismatched identity reused saved secret")
	}
}

func TestSavedCredentialBatchContinuesToUnsavedSessionWithoutLeakingPassword(t *testing.T) {
	a, p, accepted := savedConnectionFixture(t)
	if err := a.vault.PutFor(p, "saved-fixture-password", p.Identity()); err != nil {
		t.Fatal(err)
	}
	next := p
	next.ID = core.NewID()
	next.Name = "Next manual authentication"
	next.IdentityFile = ""
	a.config.Profiles = append(a.config.Profiles, next)
	a.authQueue = []core.Profile{next}
	a.request(p, 0)
	waitOperator(t, a, func() bool {
		return a.pendingProfile.ID == next.ID && !a.features.authPreparing && len(a.tabs) == 1 && a.tabs[0].active.term != nil
	})
	if !a.authOpen || len(a.authQueue) != 0 || accepted.Load() != 1 {
		t.Fatal("batch did not advance correctly")
	}
	if a.password != "" || a.features.savedPassword != "" {
		t.Fatal("previous session password leaked into next prompt")
	}
}
func TestLockedSavedCredentialWaitsForMasterBeforeAnyConnection(t *testing.T) {
	a, p, accepted := savedConnectionFixture(t)
	if err := a.vault.SetMaster("fixture-master-password"); err != nil {
		t.Fatal(err)
	}
	if err := a.vault.PutFor(p, "saved-fixture-password", p.Identity()); err != nil {
		t.Fatal(err)
	}
	a.vault.Lock()
	a.request(p, 0)
	if !a.features.vaultOpen || a.authOpen || len(a.tabs) != 0 || accepted.Load() != 0 {
		t.Fatal("locked saved credential bypassed master gate")
	}
}

// Explicitly enabled loopback-only fixture for native app acceptance.
func TestNativeSavedCredentialFixture(t *testing.T) {
	root := os.Getenv("OSHELL_NATIVE_SAVED_CREDENTIAL")
	if root == "" {
		t.Skip("explicit native fixture directory required")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("fixture directory already exists")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	var a *App
	var p core.Profile
	var accepted *atomic.Int32
	mode := os.Getenv("OSHELL_NATIVE_AUTH_MODE")
	if mode == "proxy" {
		a, p, accepted = proxyAuthenticationFixture(t)
	} else if mode == "key" {
		a, p, accepted = savedConnectionFixture(t, keyOnlyFixture)
	} else if mode == "mixed" {
		a, p, accepted = savedConnectionFixture(t, mixedMFAFixture)
	} else if mode == "mfa" {
		a, p, accepted = savedConnectionFixture(t, interactiveMFAFixture)
	} else {
		a, p, accepted = savedConnectionFixture(t)
		if mode != "dynamic" {
			password := "saved-fixture-password"
			if mode == "stale" {
				password = "stale-fixture-password"
			}
			if err := a.vault.PutFor(p, password, p.Identity()); err != nil {
				t.Fatal(err)
			}
		}
	}
	if mode == "sftp" || mode == "multiselect" {
		p.Kind = "sftp"
		p.InitialDirectory = "/"
		a.config.Profiles[0] = p
		if err := a.store.Save(a.config); err != nil {
			t.Fatal(err)
		}
		if err := a.vault.PutFor(p, "saved-fixture-password", p.Identity()); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := json.Marshal(map[string]string{"directory": a.store.Dir, "profile": p.Name})
	if err := core.AtomicWrite(filepath.Join(root, "ready.json"), data); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(5 * time.Minute)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("native fixture expired")
		case <-tick.C:
			if _, err := os.Stat(filepath.Join(root, "STOP")); err == nil {
				result, _ := json.Marshal(map[string]int32{"connections": accepted.Load()})
				if err = core.AtomicWrite(filepath.Join(root, "result.json"), result); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}
}

func TestManualAuthenticationBindsTheDisplayedResolvedIdentity(t *testing.T) {
	a, p, accepted := savedConnectionFixture(t)
	p.IdentityFile = ""
	a.request(p, 0)
	waitOperator(t, a, func() bool { return !a.features.authPreparing })
	// Model an SSH-config resolution that changes after the password prompt:
	// the displayed identity is the one the user approved, not the later result.
	shown := p.Identity()
	shown.Host = "previous-resolution.fixture.invalid"
	a.features.savedIdentity = &shown
	a.password = "saved-fixture-password"
	tt := ui.NewTester(a.view, 1000, 700)
	if !tt.HasText(shown.Host) {
		t.Fatal("prompt omitted resolved target")
	}
	if err := tt.Click("连接"); err != nil {
		t.Fatal(err)
	}
	waitOperator(t, a, func() bool { return len(a.tabs) == 1 && strings.HasPrefix(a.tabs[0].active.status, "连接失败") })
	if accepted.Load() != 0 {
		t.Fatal("password was sent to a target different from the displayed identity")
	}
}

func keyOnlyFixture(server *ssh.ServerConfig, signer ssh.Signer) {
	server.PasswordCallback = nil
	server.PublicKeyCallback = func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if meta.User() != "tester" || !bytes.Equal(key.Marshal(), signer.PublicKey().Marshal()) {
			return nil, fmt.Errorf("denied")
		}
		return nil, nil
	}
}
func TestConfiguredPrivateKeyConnectsWithoutPasswordConfirmation(t *testing.T) {
	a, p, accepted := savedConnectionFixture(t, keyOnlyFixture)
	a.request(p, 0)
	waitOperator(t, a, func() bool {
		return len(a.tabs) == 1 && a.tabs[0].active.term != nil && strings.Contains(a.tabs[0].active.term.Text(), "SAVED_CREDENTIAL_READY")
	})
	if a.authOpen || len(a.challenges) != 0 || accepted.Load() != 1 {
		t.Fatal("key authentication required redundant confirmation")
	}
	if a.vault.HasStoredCredential(p.ID) {
		t.Fatal("key authentication created password entry")
	}
}

// The upstream example shares memFile between all opens and TransferError writes
// its error onto that shared file. A cancelled reader must not poison later
// readers, as a real filesystem does not persist a handle's transport error.
type isolatedSFTPReadHandles struct{ sftp.FileReader }

func (h isolatedSFTPReadHandles) Fileread(request *sftp.Request) (io.ReaderAt, error) {
	reader, err := h.FileReader.Fileread(request)
	if err != nil {
		return nil, err
	}
	return struct{ io.ReaderAt }{reader}, nil
}

// Read/write handles need the same isolation: cancellation of an upload must
// not persist an EOF on the in-memory inode and poison a later resume.
type isolatedSFTPWriteHandles struct{ sftp.FileWriter }

func (h isolatedSFTPWriteHandles) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	writer, err := h.FileWriter.Filewrite(r)
	if err != nil {
		return nil, err
	}
	return struct{ io.WriterAt }{writer}, nil
}
func (h isolatedSFTPWriteHandles) OpenFile(r *sftp.Request) (sftp.WriterAtReaderAt, error) {
	file, err := h.FileWriter.(sftp.OpenFileWriter).OpenFile(r)
	if err != nil {
		return nil, err
	}
	return struct{ sftp.WriterAtReaderAt }{file}, nil
}

package core

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type testSSH struct {
	hostKey                          ssh.PublicKey
	terminalTypes                    chan string
	address                          string
	auth                             atomic.Int32
	keepAliveRequests                atomic.Int32
	ignoreKeepAlive, rejectKeepAlive atomic.Bool
	rejectSFTP                       atomic.Bool
}

func startSSH(t *testing.T, password string, configure ...func(*ssh.ServerConfig)) *testSSH {
	t.Helper()
	t.Setenv("SSH_AUTH_SOCK", "")
	t.Setenv("OSHELL_TEST_NO_AGENT", "1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	server := &testSSH{terminalTypes: make(chan string, 32)}
	cfg := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, b []byte) (*ssh.Permissions, error) {
		if string(b) != password {
			return nil, fmt.Errorf("denied")
		}
		server.auth.Add(1)
		return nil, nil
	}}
	for _, apply := range configure {
		apply(cfg)
	}
	signer := key(t)
	server.hostKey = signer.PublicKey()
	cfg.AddHostKey(signer)
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	server.address = l.Addr().String()
	var mu sync.Mutex
	connections := []net.Conn{}
	var wg sync.WaitGroup
	t.Cleanup(func() {
		l.Close()
		mu.Lock()
		for _, c := range connections {
			c.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			socket, e := l.Accept()
			if e != nil {
				return
			}
			mu.Lock()
			connections = append(connections, socket)
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer socket.Close()
				sc, channels, requests, e := ssh.NewServerConn(socket, cfg)
				if e != nil {
					return
				}
				defer sc.Close()
				var forwards sync.Map
				go func() {
					for req := range requests {
						switch req.Type {
						case "tcpip-forward":
							var payload struct {
								Host string
								Port uint32
							}
							ssh.Unmarshal(req.Payload, &payload)
							listener, e := net.Listen("tcp", net.JoinHostPort(payload.Host, strconv.Itoa(int(payload.Port))))
							if e != nil {
								req.Reply(false, nil)
								continue
							}
							forwards.Store(listener, true)
							req.Reply(true, nil)
							go func() {
								defer listener.Close()
								for {
									in, e := listener.Accept()
									if e != nil {
										return
									}
									go func() {
										ch, rs, e := sc.OpenChannel("forwarded-tcpip", ssh.Marshal(struct {
											Host       string
											Port       uint32
											Origin     string
											OriginPort uint32
										}{payload.Host, payload.Port, "127.0.0.1", 12345}))
										if e != nil {
											in.Close()
											return
										}
										go ssh.DiscardRequests(rs)
										relayTest(in, ch)
									}()
								}
							}()
						case "cancel-tcpip-forward":
							forwards.Range(func(k, v any) bool { k.(net.Listener).Close(); return true })
							req.Reply(true, nil)
						case "keepalive@openssh.com":
							server.keepAliveRequests.Add(1)
							if !server.ignoreKeepAlive.Load() {
								req.Reply(!server.rejectKeepAlive.Load(), nil)
							}
						default:
							req.Reply(false, nil)
						}
					}
					forwards.Range(func(k, v any) bool { k.(net.Listener).Close(); return true })
				}()
				for request := range channels {
					switch request.ChannelType() {
					case "direct-tcpip":
						var p struct {
							Host       string
							Port       uint32
							Origin     string
							OriginPort uint32
						}
						ssh.Unmarshal(request.ExtraData(), &p)
						out, e := net.DialTimeout("tcp", net.JoinHostPort(p.Host, strconv.Itoa(int(p.Port))), 3*time.Second)
						if e != nil {
							request.Reject(ssh.ConnectionFailed, e.Error())
							continue
						}
						ch, rs, e := request.Accept()
						if e != nil {
							out.Close()
							continue
						}
						go ssh.DiscardRequests(rs)
						go relayTest(out, ch)
					case "session":
						ch, rs, e := request.Accept()
						if e != nil {
							continue
						}
						go func() {
							defer ch.Close()
							for r := range rs {
								switch r.Type {
								case "pty-req":
									var payload struct {
										Term                      string
										Cols, Rows, Width, Height uint32
										Modes                     string
									}
									if ssh.Unmarshal(r.Payload, &payload) == nil {
										select {
										case server.terminalTypes <- payload.Term:
										default:
										}
									}
									r.Reply(true, nil)
								case "window-change":
									r.Reply(true, nil)
								case "shell":
									r.Reply(true, nil)
									io.Copy(ch, ch)
									ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{0}))
									return
								case "exec":
									var payload struct{ Command string }
									ssh.Unmarshal(r.Payload, &payload)
									args, ok := scpTestArgs(payload.Command)
									if !ok {
										r.Reply(false, nil)
										continue
									}
									binary, e := exec.LookPath("scp")
									if e != nil {
										r.Reply(false, nil)
										continue
									}
									r.Reply(true, nil)
									code := runSCPTestProcess(binary, args, ch)

									ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{code}))
									return
								case "subsystem":
									var payload struct{ Name string }
									ssh.Unmarshal(r.Payload, &payload)
									if payload.Name != "sftp" || server.rejectSFTP.Load() {
										r.Reply(false, nil)
										continue
									}
									r.Reply(true, nil)
									server, err := sftp.NewServer(ch)
									if err == nil {
										server.Serve()
										server.Close()
									}
									return
								default:
									r.Reply(false, nil)
								}
							}
						}()
					default:
						request.Reject(ssh.UnknownChannelType, "unsupported")
					}
				}
			}()
		}
	}()
	return server
}
func relayTest(a, b io.ReadWriteCloser) {
	done := make(chan struct{})
	go func() { io.Copy(a, b); a.Close(); close(done) }()
	io.Copy(b, a)
	b.Close()
	<-done
}
func testProfile(address string) Profile {
	p := NewProfile()
	p.Name = "test"
	p.Host, _, _ = net.SplitHostPort(address)
	_, port, _ := net.SplitHostPort(address)
	p.Port, _ = strconv.Atoi(port)
	p.Username = "tester"
	return p
}
func testTrust(t *testing.T) *TrustStore {
	return &TrustStore{Path: filepath.Join(t.TempDir(), "known_hosts")}
}
func acceptHost(HostPrompt) bool { return true }
func echoConn(t *testing.T, c io.ReadWriter, s string) {
	t.Helper()
	if n, e := c.Write([]byte(s)); e != nil || n != len(s) {
		t.Fatalf("write %d: %v", n, e)
	}
	b := make([]byte, len(s))
	done := make(chan error, 1)
	go func() { _, e := io.ReadFull(c, b); done <- e }()
	select {
	case e := <-done:
		if e != nil || string(b) != s {
			t.Fatalf("read %q: %v", b, e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("echo timeout")
	}
}
func TestJumpCloneAndLogging(t *testing.T) {
	target := startSSH(t, "target-only")
	jump := startSSH(t, "jump-only")
	p := testProfile(target.address)
	j := testProfile(jump.address)
	p.Proxy = ProxySettings{Kind: "jump", Host: j.Host, Port: j.Port, Username: "jump-user"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, e := Dial(ctx, p, Credentials{Password: "target-only", ProxyPassword: "jump-only"}, testTrust(t), acceptHost)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	clone, e := c.Clone(context.Background(), DefaultKeepAlive())
	if e != nil {
		t.Fatal(e)
	}
	defer clone.Close()
	log := filepath.Join(t.TempDir(), "session.log")
	if e = clone.StartLog(log); e != nil {
		t.Fatal(e)
	}
	c.Close()
	echoConn(t, clone, "still connected\n")
	clone.StopLog()
	data, e := os.ReadFile(log)
	if e != nil || string(data) != "still connected\n" {
		t.Fatalf("%q %v", data, e)
	}
	if target.auth.Load() != 1 || jump.auth.Load() != 1 {
		t.Fatal("clone reauthenticated")
	}
}
func unusedPort(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}
func startEcho(t *testing.T) net.Listener {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	return l
}
func TestForwarding(t *testing.T) {
	for _, kind := range []string{"local", "remote", "dynamic"} {
		t.Run(kind, func(t *testing.T) {
			target := startSSH(t, "test")
			echo := startEcho(t)
			p := testProfile(target.address)
			rule := NewTunnel()
			rule.Kind = kind
			rule.ListenPort = unusedPort(t)
			rule.DestinationPort = echo.Addr().(*net.TCPAddr).Port
			p.Tunnels = []TunnelRule{rule}
			c, e := Dial(context.Background(), p, Credentials{Password: "test"}, testTrust(t), acceptHost)
			if e != nil {
				t.Fatal(e)
			}
			endpoint := net.JoinHostPort("127.0.0.1", strconv.Itoa(rule.ListenPort))
			socket, e := net.DialTimeout("tcp", endpoint, 3*time.Second)
			if e != nil {
				c.Close()
				t.Fatal(e)
			}
			socket.SetDeadline(time.Now().Add(5 * time.Second))
			if kind == "dynamic" {
				if e = connectSOCKS5(socket, echo.Addr().String(), "", ""); e != nil {
					t.Fatal(e)
				}
			}
			echoConn(t, socket, "tunnel roundtrip")
			socket.Close()
			c.Close()
			deadline := time.Now().Add(3 * time.Second)
			for {
				v, e := net.DialTimeout("tcp", endpoint, 50*time.Millisecond)
				if e != nil {
					break
				}
				v.Close()
				if time.Now().After(deadline) {
					t.Fatal("listener leaked")
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}
func TestProxyProtocols(t *testing.T) {
	for _, kind := range []string{"http", "socks4", "socks4a", "socks5"} {
		t.Run(kind, func(t *testing.T) {
			target := startSSH(t, "test")
			l, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			defer l.Close()
			go func() {
				socket, e := l.Accept()
				if e != nil {
					return
				}
				var address string
				var incoming net.Conn = socket
				if kind == "http" {
					r := bufio.NewReader(socket)
					req, e := http.ReadRequest(r)
					if e != nil {
						socket.Close()
						return
					}
					address = req.Host
					incoming = &bufferedConn{socket, r}
					writeAll(socket, []byte("HTTP/1.1 200 Connection established\r\n\r\n"))
				} else {
					var version byte
					address, version, e = readDynamicRequest(socket)
					if e != nil {
						socket.Close()
						return
					}
					replyDynamic(socket, version, true)
				}
				out, e := net.Dial("tcp", address)
				if e != nil {
					socket.Close()
					return
				}
				relayTest(incoming, out)
			}()
			p := testProfile(target.address)
			proxy := testProfile(l.Addr().String())
			p.Proxy = ProxySettings{Kind: kind, Host: proxy.Host, Port: proxy.Port}
			c, e := Dial(context.Background(), p, Credentials{Password: "test"}, testTrust(t), acceptHost)
			if e != nil {
				t.Fatal(e)
			}
			echoConn(t, c, "proxied\n")
			c.Close()
		})
	}
}

func scpTestArgs(command string) ([]string, bool) {
	flag := ""
	for _, v := range []string{"-rt", "-rf"} {
		if strings.HasPrefix(command, "scp "+v+" -- ") {
			flag = v
			break
		}
	}
	if flag == "" {
		return nil, false
	}
	quoted := strings.TrimPrefix(command, "scp "+flag+" -- ")
	if len(quoted) < 2 || quoted[0] != '\'' || quoted[len(quoted)-1] != '\'' {
		return nil, false
	}
	value := strings.ReplaceAll(quoted[1:len(quoted)-1], `'\''`, "'")
	if shellQuote(value) != quoted {
		return nil, false
	}
	return []string{flag, "--", value}, true
}

func runSCPTestProcess(binary string, args []string, ch ssh.Channel) uint32 {
	process := exec.Command(binary, args...)
	input, e := process.StdinPipe()
	if e != nil {
		return 1
	}
	output, e := process.StdoutPipe()
	if e != nil {
		return 1
	}
	errors, e := process.StderrPipe()
	if e != nil {
		return 1
	}
	if e = process.Start(); e != nil {
		return 1
	}
	go func() { io.Copy(input, ch); input.Close() }()
	outDone, errDone := make(chan struct{}), make(chan struct{})
	go func() { io.Copy(ch, output); close(outDone) }()
	go func() { io.Copy(ch.Stderr(), errors); close(errDone) }()
	<-outDone
	<-errDone
	if process.Wait() != nil {
		return 1
	}
	return 0
}

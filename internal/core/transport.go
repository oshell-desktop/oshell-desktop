// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// A transport outlives its original shell while cloned SSH channels are open.
type transport struct {
	tunnelPolicy     string
	fileOpening      atomic.Bool
	terminalType     string
	observedPeer     *net.TCPAddr
	tunnelProfile    string
	identity         SSHIdentity
	passwordVerified bool
	client           *ssh.Client
	clients          []*ssh.Client
	mu               sync.Mutex
	refs             int
	closed           bool
	shutdownComplete bool
	done             chan struct{}
	cancel           context.CancelFunc
	closers          map[io.Closer]bool
}

func (t *transport) retain() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return fmt.Errorf("SSH 连接已关闭")
	}
	t.refs++
	return nil
}
func (t *transport) release() {
	t.mu.Lock()
	t.refs--
	last := t.refs == 0
	t.mu.Unlock()
	if last {
		t.shutdown()
	}
}
func (t *transport) track(c io.Closer) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		c.Close()
		return false
	}
	t.closers[c] = true
	return true
}
func (t *transport) untrack(c io.Closer) {
	t.mu.Lock()
	delete(t.closers, c)
	t.mu.Unlock()
	t.releaseDrainedTunnels()
}
func (t *transport) shutdown() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	close(t.done)
	t.cancel()
	cs := make([]io.Closer, 0, len(t.closers))
	for c := range t.closers {
		cs = append(cs, c)
	}
	t.mu.Unlock()
	for _, c := range cs {
		err := c.Close()
		if _, listener := c.(net.Listener); listener && (err == nil || errors.Is(err, net.ErrClosed)) {
			t.untrack(c)
		}
	}
	for i := len(t.clients) - 1; i >= 0; i-- {
		t.clients[i].Close()
	}
	t.mu.Lock()
	t.shutdownComplete = true
	t.mu.Unlock()
	t.releaseDrainedTunnels()
}

type Connection struct {
	idleGuard        atomic.Pointer[func() bool]
	idleChanges      chan KeepAliveSettings
	managedBusy      atomic.Bool
	Identity         SSHIdentity
	PasswordVerified bool
	Client           *ssh.Client
	transport        *transport
	session          *ssh.Session
	reader           *io.PipeReader
	output           *io.PipeWriter
	writer           io.WriteCloser
	writeMu          sync.Mutex
	once             sync.Once
	done             chan struct{}
	waitMu           sync.Mutex
	waitErr          error
	lastActivity     atomic.Int64
	logMu            sync.Mutex
	logFile          *os.File
	logError         error
	logPath          string
	OnOutput         func(int)
}

func (t *transport) newFiles(ctx context.Context) (*Connection, error) {
	if e := t.retain(); e != nil {
		return nil, e
	}
	c := &Connection{Client: t.client, Identity: t.identity, PasswordVerified: t.passwordVerified, transport: t, done: make(chan struct{})}
	c.reader, c.output = io.Pipe()
	c.output.Close()
	go func() {
		select {
		case <-ctx.Done():
			c.Close()
		case <-c.done:
		}
	}()
	return c, nil
}
func (c *Connection) CloneFiles(ctx context.Context) (*Connection, error) {
	return c.transport.newFiles(ctx)
}

// Lease keeps an authenticated transport alive for a file client.
func (c *Connection) Lease() (func(), error) {
	if e := c.transport.retain(); e != nil {
		return nil, e
	}
	var once sync.Once
	return func() { once.Do(c.transport.release) }, nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		if h, e := os.UserHomeDir(); e == nil {
			return filepath.Join(h, path[2:])
		}
	}
	return path
}

// The SSH reader keeps running while an operator prompt waits for input.
// Propagate a closed socket to that prompt instead of leaving an abandoned
// authentication dialog at the head of the application queue until timeout.
type authenticationConn struct {
	net.Conn
	cancel context.CancelFunc
}

func (c authenticationConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if err != nil {
		c.cancel()
	}
	return n, err
}

func (c authenticationConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if err != nil {
		c.cancel()
	}
	return n, err
}

func authenticate(ctx context.Context, c net.Conn, p Profile, cred Credentials, trust *TrustStore, approve func(HostPrompt) bool, verified *atomic.Bool) (*ssh.Client, error) {
	hctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ctx = hctx
	passwordOnly := cred.Method == "password"
	var offered, manual atomic.Bool
	attemptPassword, acceptedPassword := "", ""
	suppliedPasswordUsed := false
	authAttempts := map[string]int{}
	var challengeError error
	var auth []ssh.AuthMethod
	var authNames []string
	if cred.Method != "privateKey" && (cred.Password != "" || cred.Challenge != nil || cred.PasswordChallenge != nil) {
		authNames = append(authNames, "password")
		auth = append(auth, ssh.PasswordCallback(func() (string, error) {
			if cred.Password != "" && !suppliedPasswordUsed {
				suppliedPasswordUsed = true
				offered.Store(true)
				attemptPassword = cred.Password
				return cred.Password, nil
			}
			prompt := AuthenticationPrompt{Identity: p.Identity(), Name: "SSH 密码认证", Questions: []string{"登录密码"}, Echo: []bool{false}}
			if authAttempts["password"] > 1 {
				prompt.Instruction = "上一次密码认证未通过，请重新输入。"
			}
			if cred.PasswordChallenge != nil {
				password, err := cred.PasswordChallenge(ctx, prompt)
				challengeError = err
				offered.Store(err == nil && password != "")
				attemptPassword = password
				return password, err
			}
			if cred.Challenge == nil {
				return "", fmt.Errorf("没有可用的密码回答")
			}
			answers, e := cred.Challenge(ctx, prompt)
			if e != nil {
				challengeError = e
				return "", e
			}
			if len(answers) != 1 {
				return "", fmt.Errorf("密码回答无效")
			}
			manual.Store(true)
			return answers[0], nil
		}))
	}
	if cred.Method != "privateKey" && (cred.Password != "" || cred.Challenge != nil || cred.PasswordChallenge != nil || cred.MixedPasswordChallenge != nil) {
		authNames = append(authNames, "keyboard-interactive")
		auth = append(auth, ssh.KeyboardInteractive(func(name, instruction string, questions []string, echo []bool) ([]string, error) {
			if len(questions) > 32 || len(questions) != len(echo) {
				return nil, fmt.Errorf("认证问题数量无效")
			}
			if len(questions) == 1 && !echo[0] {
				if loginPasswordPrompt(p.Identity(), questions[0]) {
					if cred.Password != "" && !suppliedPasswordUsed {
						suppliedPasswordUsed = true
						offered.Store(true)
						attemptPassword = cred.Password
						return []string{cred.Password}, nil
					}
					if cred.PasswordChallenge != nil {
						password, err := cred.PasswordChallenge(ctx, AuthenticationPrompt{Identity: p.Identity(), Name: name, Instruction: instruction, Questions: questions, Echo: echo})
						challengeError = err
						offered.Store(err == nil && password != "")
						attemptPassword = password
						return []string{password}, err
					}
				}
			}
			if len(questions) == 0 {
				return []string{}, nil
			}
			if cred.Challenge == nil {
				return nil, fmt.Errorf("服务器要求交互式认证")
			}
			// Other factors do not replace a recognized password from this method.
			// Only a successful full handshake can publish that password.
			answers, e := cred.Challenge(ctx, AuthenticationPrompt{Identity: p.Identity(), Name: name, Instruction: instruction, Questions: questions, Echo: echo})
			challengeError = e
			if e == nil && len(answers) != len(questions) {
				return nil, fmt.Errorf("认证回答数量无效")
			}
			return answers, e
		}))
	}
	if !passwordOnly {
		identities := append([]string(nil), p.resolvedIdentityFiles...)
		if os.Getenv("OSHELL_TEST_EXPLICIT_KEYS_ONLY") == "1" {
			identities = nil
		}
		var signers []ssh.Signer
		seen := map[string]bool{}
		if p.IdentityFile != "" {
			identities = append(identities, expandHome(p.IdentityFile))
		} else if home, e := os.UserHomeDir(); e == nil && len(identities) == 0 && os.Getenv("OSHELL_TEST_EXPLICIT_KEYS_ONLY") != "1" {
			for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
				f := filepath.Join(home, ".ssh", name)
				if _, e := os.Stat(f); e == nil {
					identities = append(identities, f)
				}
			}
		}
		if cred.Method != "privateKey" {
			if a, e := dialSSHAgent(ctx); e == nil {
				defer a.Close()
				agentStop := context.AfterFunc(ctx, func() { a.Close() })
				defer agentStop()
				a.SetDeadline(time.Now().Add(15 * time.Second))
				if agentSigners, e := agent.NewClient(a).Signers(); e == nil {
					signers = append(signers, agentSigners...)
					a.SetDeadline(time.Time{})
				}
			}

		}
		for _, f := range identities {
			if seen[f] {
				continue
			}
			seen[f] = true
			data, e := ReadBounded(f)
			if e != nil {
				if f == expandHome(p.IdentityFile) {
					return nil, e
				}
				continue
			}
			var signer ssh.Signer
			if cred.KeyPassphrase != "" {
				signer, e = ssh.ParsePrivateKeyWithPassphrase(data, []byte(cred.KeyPassphrase))
			} else {
				signer, e = ssh.ParsePrivateKey(data)
			}
			if e != nil && cred.Challenge != nil {
				var missing *ssh.PassphraseMissingError
				if errors.As(e, &missing) {
					loaded := false
					if missing.PublicKey != nil {
						for _, agentSigner := range signers {
							if bytes.Equal(agentSigner.PublicKey().Marshal(), missing.PublicKey.Marshal()) {
								loaded = true
								break
							}
						}
					}
					if loaded {
						continue
					}
					answers, challengeError := cred.Challenge(ctx, AuthenticationPrompt{Identity: p.Identity(), Name: "解锁 SSH 私钥", Instruction: filepath.Base(f), Questions: []string{"私钥口令"}, Echo: []bool{false}})
					if challengeError != nil {
						return nil, challengeError
					}
					if len(answers) != 1 {
						return nil, fmt.Errorf("私钥口令回答无效")
					}
					signer, e = ssh.ParsePrivateKeyWithPassphrase(data, []byte(answers[0]))
				}
			}
			if e != nil {
				if f == expandHome(p.IdentityFile) {
					return nil, fmt.Errorf("私钥读取失败: %w", e)
				}
				continue
			}
			signers = append(signers, signer)
		}

		if len(signers) > 0 {
			var err error
			signers, err = restrictRSAAuthentication(signers, p.LegacySSH)
			if err != nil {
				return nil, err
			}
			auth = append([]ssh.AuthMethod{ssh.PublicKeys(signers...)}, auth...)
			authNames = append([]string{"publickey"}, authNames...)
		}
	}
	if len(auth) == 0 {
		return nil, fmt.Errorf("请提供密码、私钥或可用的 SSH Agent")
	}
	cfg := &ssh.ClientConfig{User: p.Username, Auth: auth, HostKeyCallback: trust.Callback(ctx, approve), Timeout: 15 * time.Second}
	// Reset evidence before each protocol-level attempt, including methods that
	// can succeed without invoking a challenge callback. Partial password success
	// is retained, but is published only after the complete handshake succeeds.
	partialCount := 0
	cfg.AuthCallback = func(state *ssh.ClientAuthContext) (ssh.AuthMethod, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if challengeError != nil {
			return nil, challengeError
		}
		if len(state.PartialSuccessMethods) > partialCount && offered.Load() && !manual.Load() {
			acceptedPassword = attemptPassword
		}
		partialCount = len(state.PartialSuccessMethods)
		offered.Store(false)
		manual.Store(false)
		attemptPassword = ""
		for i, name := range authNames {
			retry := authAttempts[name] < 3 && ((name == "password" && (cred.PasswordChallenge != nil || cred.Challenge != nil)) || (name == "keyboard-interactive" && (cred.PasswordChallenge != nil || cred.Challenge != nil)))
			if slices.Contains(state.AllowedMethods, name) && (!slices.Contains(state.TriedMethods, name) || retry) {
				authAttempts[name]++
				return auth[i], nil
			}
		}
		return nil, nil
	}

	if p.LegacySSH {
		configureLegacySSH(cfg)
	}
	stop := context.AfterFunc(hctx, func() { c.Close() })
	defer stop()
	cc, chs, req, e := ssh.NewClientConn(authenticationConn{Conn: c, cancel: cancel}, p.Address(), cfg)
	if e != nil {
		return nil, e
	}
	if hctx.Err() != nil {
		cc.Close()
		return nil, hctx.Err()
	}
	if offered.Load() && !manual.Load() {
		acceptedPassword = attemptPassword
	}
	if acceptedPassword != "" {
		verified.Store(true)
		if cred.PasswordAuthenticated != nil {
			cred.PasswordAuthenticated(acceptedPassword)
		}
	}
	return ssh.NewClient(cc, chs, req), nil
}
func Dial(ctx context.Context, p Profile, cred Credentials, trust *TrustStore, approve func(HostPrompt) bool) (*Connection, error) {
	if e := p.Validate(); e != nil {
		return nil, e
	}
	if p.ProxyID != "" && len(p.RuntimeProxyRoute) == 0 {
		return nil, fmt.Errorf("共享代理尚未解析，已阻止直连")
	}
	resolved, e := ResolveSSH(ctx, p)
	if e != nil {
		return nil, e
	}
	p = resolved.Profile
	validated := p
	validated.JumpHost = ""
	if e = validated.Validate(); e != nil {
		return nil, e
	}
	if cred.ExpectedIdentity != nil && *cred.ExpectedIdentity != p.Identity() {
		return nil, fmt.Errorf("SSH 配置身份已变化，请重新输入密码")
	}
	tctx, cancel := context.WithCancel(context.Background())
	t := &transport{terminalType: p.RuntimeTerminalType, identity: p.Identity(), done: make(chan struct{}), cancel: cancel, closers: map[io.Closer]bool{}}
	tunnels := p.Tunnels
	t.tunnelPolicy = "none"
	if hasEnabledTunnel(tunnels) {
		if t.claimTunnels(p.ID) {
			t.tunnelPolicy = "owned"
		} else {
			t.tunnelPolicy = "skipped"
			tunnels = nil
		}
	}
	ok := false
	defer func() {
		if !ok {
			t.shutdown()
		}
	}()
	if len(p.RuntimeProxyRoute) > 0 {
		if e = t.connectProxyRoute(ctx, p, cred, trust, approve); e != nil {
			return nil, e
		}
	} else {
		jumps, err := resolveJumpRoute(ctx, p, ResolveSSH)
		if err != nil {
			return nil, err
		}
		chain := append(jumps, resolved)
		for i, resolvedHop := range chain {
			hop := resolvedHop.Profile
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			var socket net.Conn
			var e error
			if i == 0 {
				proxy := p.Proxy
				if len(jumps) > 0 {
					proxy = ProxySettings{Kind: "none"}
				}
				if resolvedHop.ProxyCommand != "" {
					socket, e = dialProxyCommand(tctx, resolvedHop.ProxyCommand, hop, resolvedHop.OriginalHost)
				} else {
					socket, e = proxyConnect(ctx, proxy, cred.ProxyPassword, hop.Address(), p.KeepAlive.TCP)
				}
			} else {
				socket, e = t.clients[len(t.clients)-1].DialContext(ctx, "tcp", hop.Address())
			}
			if e != nil {
				return nil, e
			}
			if i == 0 {
				t.observeSocket(socket)
			}
			hc := cred
			if i < len(jumps) {
				hc = Credentials{Password: cred.ProxyPassword, KeyPassphrase: cred.ProxyKeyPassphrase, Challenge: authenticationTitle(cred.Challenge, "SSH 跳板机认证")}
			}
			var verified atomic.Bool
			client, e := authenticate(ctx, socket, hop, hc, trust, approve, &verified)
			if i == len(jumps) {
				t.passwordVerified = verified.Load()
			}
			if e != nil {
				socket.Close()
				return nil, fmt.Errorf("%s: %w", hop.Address(), e)
			}
			t.clients = append(t.clients, client)
		}
	}
	t.client = t.clients[len(t.clients)-1]
	go func() { _ = t.client.Wait(); t.shutdown() }()
	stop := context.AfterFunc(ctx, t.shutdown)
	defer stop()

	if e := t.startTunnels(tctx, tunnels); e != nil {
		return nil, e
	}
	var c *Connection
	if p.Kind == "sftp" {
		c, e = t.newFiles(ctx)
	} else {
		c, e = t.newSession(ctx, p.KeepAlive)
	}
	if e != nil {
		return nil, e
	}
	ok = true
	if p.KeepAlive.Enabled {
		go t.keepAlive(p.KeepAlive)
	}
	return c, nil
}
func (t *transport) newSession(ctx context.Context, keep KeepAliveSettings) (*Connection, error) {
	if e := t.retain(); e != nil {
		return nil, e
	}
	s, e := t.client.NewSession()
	if e != nil {
		t.release()
		return nil, e
	}
	c := &Connection{Client: t.client, Identity: t.identity, PasswordVerified: t.passwordVerified, transport: t, session: s, done: make(chan struct{})}
	c.reader, c.output = io.Pipe()
	s.Stdout = c.output
	s.Stderr = c.output
	c.writer, e = s.StdinPipe()
	if e == nil {
		terminalType := t.terminalType
		if terminalType == "" {
			terminalType = "xterm-256color"
		}
		e = s.RequestPty(terminalType, 24, 80, ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400})
	}
	if e == nil {
		e = s.Shell()
	}
	if e != nil {
		c.Close()
		return nil, e
	}
	c.lastActivity.Store(time.Now().UnixNano())
	c.idleChanges = make(chan KeepAliveSettings, 1)
	go func() { e := s.Wait(); c.waitMu.Lock(); c.waitErr = e; c.waitMu.Unlock(); c.Close() }()
	go func() {
		select {
		case <-ctx.Done():
			c.Close()
		case <-c.done:
		}
	}()
	go c.idleKeepAlive(keep)
	return c, nil
}
func (c *Connection) Clone(ctx context.Context, keep KeepAliveSettings) (*Connection, error) {
	if e := keep.Validate(); e != nil {
		return nil, e
	}
	return c.transport.newSession(ctx, keep)
}
func (c *Connection) Read(b []byte) (int, error) {
	n, e := c.reader.Read(b)
	if n > 0 {
		c.lastActivity.Store(time.Now().UnixNano())
		c.logMu.Lock()
		if c.logFile != nil {
			if _, err := c.logFile.Write(b[:n]); err != nil {
				c.logError = err
				c.logFile.Close()
				c.logFile = nil
			}
		}
		c.logMu.Unlock()
		if c.OnOutput != nil {
			c.OnOutput(n)
		}
	}
	return n, e
}
func (c *Connection) Write(b []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.write(b)
}
func (c *Connection) write(b []byte) (int, error) {
	if c.writer == nil {
		return 0, fmt.Errorf("文件会话没有终端输入")
	}
	if len(b) > 0 {
		c.lastActivity.Store(time.Now().UnixNano())
	}
	return c.writer.Write(b)
}
func (c *Connection) Resize(cols, rows int) error {
	if c.session == nil {
		return nil
	}
	return c.session.WindowChange(rows, cols)
}
func (c *Connection) Close() error {
	c.once.Do(func() {
		close(c.done)
		c.reader.Close()
		c.output.Close()
		if c.session != nil {
			c.session.Close()
		}
		c.StopLog()
		c.transport.release()
	})
	return nil
}
func (c *Connection) Err() error { c.waitMu.Lock(); defer c.waitMu.Unlock(); return c.waitErr }
func (c *Connection) StartLog(path string) error {
	c.logMu.Lock()
	defer c.logMu.Unlock()
	select {
	case <-c.done:
		return fmt.Errorf("连接已关闭")
	default:
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	if c.logFile != nil {
		c.logFile.Close()
	}
	c.logFile = f
	c.logError = nil
	c.logPath = path
	return nil
}
func (c *Connection) StopLog() {
	c.logMu.Lock()
	defer c.logMu.Unlock()
	if c.logFile != nil {
		if e := c.logFile.Close(); e != nil {
			c.logError = e
		}
		c.logFile = nil
	}
}
func (c *Connection) LogState() (string, bool, error) {
	c.logMu.Lock()
	defer c.logMu.Unlock()
	return c.logPath, c.logFile != nil, c.logError
}
func (c *Connection) SetIdleGuard(guard func() bool) { c.idleGuard.Store(&guard) }
func (c *Connection) SetManagedBusy(busy bool) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.managedBusy.Store(busy)
	if !busy {
		c.lastActivity.Store(time.Now().UnixNano())
	}
}
func (c *Connection) UpdateIdleKeepAlive(k KeepAliveSettings) error {
	if e := k.Validate(); e != nil {
		return e
	}
	if c.idleChanges == nil {
		return fmt.Errorf("此连接不是终端会话")
	}
	select {
	case <-c.done:
		return fmt.Errorf("连接已关闭")
	default:
	}
	// Applying live settings is activity, just like the native client. Start
	// the new interval now instead of inheriting the previous idle deadline.
	c.lastActivity.Store(time.Now().UnixNano())
	select {
	case c.idleChanges <- k:
	default:
		select {
		case <-c.idleChanges:
		default:
		}
		select {
		case c.idleChanges <- k:
		default:
		}
	}
	return nil
}
func (c *Connection) idleKeepAlive(k KeepAliveSettings) {
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		period := time.Duration(k.IdleInterval) * time.Second
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		var tick <-chan time.Time
		if k.IdleEnabled {
			remaining := period - time.Since(time.Unix(0, c.lastActivity.Load()))
			if remaining <= 0 {
				remaining = period
			}
			timer.Reset(remaining)
			tick = timer.C
		}
		select {
		case <-c.done:
			return
		case k = <-c.idleChanges:
		case <-tick:
			if e := c.sendIdleIfNeeded(k); e != nil {
				return
			}
		}
	}
}
func (c *Connection) sendIdleIfNeeded(k KeepAliveSettings) error {
	// Serialize the final idle check with normal input and transfer ownership;
	// a timer must not inject text into a newly started ZMODEM exchange.
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	select {
	case <-c.done:
		return io.ErrClosedPipe
	default:
	}
	if !k.IdleEnabled || c.managedBusy.Load() || time.Since(time.Unix(0, c.lastActivity.Load())) < time.Duration(k.IdleInterval)*time.Second {
		return nil
	}
	if guard := c.idleGuard.Load(); guard != nil && *guard != nil && !(*guard)() {
		return nil
	}
	data, err := IdleBytes(k.IdleText)
	if err != nil {
		return err
	}
	_, err = c.write(data)
	return err
}
func (t *transport) keepAlive(k KeepAliveSettings) {
	period := time.Duration(k.Interval) * time.Second
	timer := time.NewTimer(period)
	defer timer.Stop()
	pending := make(chan error, 1)
	inflight := false
	missed := 0
	for {
		select {
		case <-t.done:
			return
		case e := <-pending:
			inflight = false
			if e != nil {
				t.shutdown()
				return
			}
			missed = 0
		case <-timer.C:
			if inflight {
				missed++
				if missed >= k.MaxMissed {
					t.shutdown()
					return
				}
			} else {
				inflight = true
				go func() { _, _, e := t.client.SendRequest("keepalive@openssh.com", true, nil); pending <- e }()
			}
			timer.Reset(period)
		}
	}
}

// ObservedPeer is the actual local TCP peer. With routing this is the proxy
// or first jump, never a DNS guess for the final host.
func (c *Connection) ObservedPeer() (string, int) {
	if c == nil || c.transport == nil || c.transport.observedPeer == nil {
		return "", 0
	}
	return c.transport.observedPeer.IP.String(), c.transport.observedPeer.Port
}

// Alive reflects both the channel and shared transport, including file-only
// connections that have no shell reader to observe remote disconnection.
func (c *Connection) Alive() bool {
	if c == nil || c.transport == nil {
		return false
	}
	select {
	case <-c.done:
		return false
	case <-c.transport.done:
		return false
	default:
		return true
	}
}

// WaitClosed waits without polling for this connection or its shared transport
// to end. Cancelling the observer does not close either connection.
func (c *Connection) WaitClosed(ctx context.Context) error {
	if c == nil {
		return nil
	}
	var transportDone <-chan struct{}
	if c.transport != nil {
		transportDone = c.transport.done
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return nil
	case <-transportDone:
		return nil
	}
}

func (t *transport) observeSocket(socket net.Conn) {
	if peer, ok := socket.RemoteAddr().(*net.TCPAddr); ok {
		copy := *peer
		copy.IP = append(net.IP(nil), peer.IP...)
		t.observedPeer = &copy
	}
}

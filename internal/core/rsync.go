package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type RsyncOptions struct {
	Local, Remote     string
	Upload, Directory bool
	Rate              int64
}

const rsyncPartialDirectory = ".oshell-rsync-partial"

func RsyncArguments(options RsyncOptions, helper string) ([]string, string, error) {
	if options.Remote == "" || len(options.Remote) > 4096 || strings.ContainsAny(options.Remote, "\x00\r\n") {
		return nil, "", fmt.Errorf("rsync 远程路径无效")
	}
	local, err := filepath.Abs(options.Local)
	if err != nil || options.Local == "" {
		return nil, "", fmt.Errorf("rsync 本机路径无效")
	}
	info, err := os.Lstat(local)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || options.Directory != info.IsDir() || !info.IsDir() && !info.Mode().IsRegular() {
			return nil, "", fmt.Errorf("顶层本机路径类型不匹配，不接受符号链接或特殊文件")
		}
	} else if !os.IsNotExist(err) || options.Upload {
		return nil, "", err
	}
	remote := options.Remote
	if !strings.HasPrefix(remote, "/") && !strings.HasPrefix(remote, "./") {
		remote = "./" + remote
	}
	localArg := "./" + filepath.Base(local)
	dir := filepath.Dir(local)
	if dir == local {
		localArg = "."
	}
	if options.Directory {
		localArg += "/"
		remote = strings.TrimRight(remote, "/") + "/"
	}
	// Relative local paths avoid rsync interpreting a Windows drive colon as a
	// remote host. The child inherits the selected path's actual parent directory.
	args := []string{"-rlt", "--safe-links", "--partial-dir=" + rsyncPartialDirectory, "--progress", "--out-format=OSHELL_RSYNC_FILE:%l", "--timeout=60", "-e", shellQuote(filepath.ToSlash(helper)) + " --rsync-stdio"}
	if options.Rate > 0 {
		rate := options.Rate / 1024
		if options.Rate%1024 != 0 {
			rate++
		}
		args = append(args, fmt.Sprintf("--bwlimit=%d", rate))
	}
	endpoint := "oshell:" + shellQuote(remote)
	args = append(args, "--")
	if options.Upload {
		args = append(args, localArg, endpoint)
	} else {
		args = append(args, endpoint, localArg)
	}
	return args, dir, nil
}

type RsyncProgress struct {
	mu                 sync.Mutex
	line               []byte
	dropping           bool
	completed, current int64
	files              int
	report             func(TransferProgress)
	path               string
}

func (p *RsyncProgress) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, b := range data {
		if b == '\r' || b == '\n' {
			if !p.dropping {
				p.parse(string(p.line))
			}
			p.line = p.line[:0]
			p.dropping = false
		} else if len(p.line) < 4096 && !p.dropping {
			p.line = append(p.line, b)
		} else {
			p.line = p.line[:0]
			p.dropping = true
		}
	}
	return len(data), nil
}
func (p *RsyncProgress) parse(line string) {
	if strings.HasPrefix(line, "OSHELL_RSYNC_FILE:") {
		p.completed = min(math.MaxInt64-p.current, p.completed) + p.current
		p.current = 0
		p.files++
		return
	}
	fields := strings.Fields(line)
	if len(fields) < 2 || !strings.HasSuffix(fields[1], "%") {
		return
	}
	n, err := strconv.ParseInt(strings.ReplaceAll(fields[0], ",", ""), 10, 64)
	if err != nil || n < 0 {
		return
	}
	percent, err := strconv.ParseFloat(strings.TrimSuffix(fields[1], "%"), 64)
	if err != nil || math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 || percent > 100 {
		return
	}
	p.current = max(p.current, n)
	if p.report != nil {
		p.report(TransferProgress{Path: p.path, Bytes: min(math.MaxInt64-p.current, p.completed) + p.current, Files: p.files})
	}
}

// The private bridge opens an exec channel on the already authenticated Go SSH
// transport. rsync never starts ssh.exe, changes the proxy route or sees a password.
func (c *Connection) Rsync(ctx context.Context, options RsyncOptions, report func(TransferProgress)) error {
	if c == nil || !c.Alive() {
		return fmt.Errorf("SSH 连接不可用")
	}
	if limiter, ok := ctx.Value(transferLimitKey{}).(*TransferLimiter); ok {
		options.Rate = limiter.Rate()
	}
	binary, err := exec.LookPath("rsync")
	if err != nil {
		return fmt.Errorf("本机缺少 rsync，请先将兼容本机的 rsync 加入 PATH；程序不会自动安装")
	}
	helper, err := os.Executable()
	if err != nil {
		return err
	}
	args, dir, err := RsyncArguments(options, helper)
	if err != nil {
		return err
	}
	release, err := c.Lease()
	if err != nil {
		return err
	}
	defer release()
	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	endpoint := filepath.Join(os.TempDir(), "oshell-rsync-"+NewID())
	listener, cleanup, err := listenLaunch(endpoint)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}
	defer listener.Close()
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	clear(secret)
	served := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		listener.Close()
		if err == nil {
			err = c.serveRsync(ctx, conn, token, options)
		}
		served <- err
	}()
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = dir
	prepareCommand(command)
	command.Cancel = func() error { stopCommand(command); return nil }
	command.WaitDelay = 3 * time.Second
	for _, env := range os.Environ() {
		name, _, _ := strings.Cut(env, "=")
		if !strings.HasPrefix(strings.ToUpper(name), "RSYNC_") && !strings.HasPrefix(strings.ToUpper(name), "OSHELL_RSYNC_") {
			command.Env = append(command.Env, env)
		}
	}
	command.Env = append(command.Env, "RSYNC_OLD_ARGS=1", "RSYNC_PROTECT_ARGS=0", "OSHELL_RSYNC_ENDPOINT="+endpoint, "OSHELL_RSYNC_TOKEN="+token)
	parser := &RsyncProgress{report: report, path: options.Remote}
	stderr := &rsyncErrorTail{}
	command.Stdout = parser
	command.Stderr = io.MultiWriter(parser, stderr)
	err = command.Run()
	if err != nil {
		cancel()
	}
	listener.Close()
	bridgeErr := <-served
	if parent.Err() != nil {
		return parent.Err()
	}
	if err != nil {
		message := stderr.String()
		// A rejected handshake closes the private pipe, so rsync only reports
		// EOF/code 12. Keep the bridge's original cause instead of hiding it.
		// Ignore errors caused by our own cleanup after a child-process failure.
		if bridgeErr != nil && !errors.Is(bridgeErr, context.Canceled) && !errors.Is(bridgeErr, net.ErrClosed) {
			return fmt.Errorf("rsync 传输失败：%w；桥接：%w；%s", err, bridgeErr, message)
		}
		if strings.Contains(message, "not found") && strings.Contains(message, "rsync") {
			return fmt.Errorf("远端缺少 rsync：%s", message)
		}
		return fmt.Errorf("rsync 传输失败：%w；%s", err, message)
	}
	return bridgeErr
}

type rsyncErrorTail struct {
	mu   sync.Mutex
	data []byte
}

func (e *rsyncErrorTail) Write(data []byte) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.data = append(e.data, data...)
	if len(e.data) > 8192 {
		e.data = append([]byte(nil), e.data[len(e.data)-8192:]...)
	}
	return len(data), nil
}
func (e *rsyncErrorTail) String() string { e.mu.Lock(); defer e.mu.Unlock(); return string(e.data) }

func rsyncRemoteCommand(args []string, options RsyncOptions) (string, error) {
	if len(args) < 5 || len(args) > 64 || args[0] != "oshell" || args[1] != "rsync" || args[2] != "--server" || args[len(args)-2] != "." {
		return "", fmt.Errorf("rsync 启动参数无效")
	}
	flags := args[2 : len(args)-2]
	sender := false
	for i := 0; i < len(flags); i++ {
		arg := flags[i]
		// rsync expands --partial-dir=VALUE to two remote-shell arguments.
		// Accept only our fixed relative staging directory, not arbitrary
		// positional arguments or paths that could escape the selected target.
		if arg == "--partial-dir" {
			if i+1 >= len(flags) || flags[i+1] != rsyncPartialDirectory {
				return "", fmt.Errorf("rsync 续传目录参数无效")
			}
			i++
			continue
		}
		if strings.HasPrefix(arg, "--partial-dir=") && arg != "--partial-dir="+rsyncPartialDirectory {
			return "", fmt.Errorf("rsync 续传目录参数无效")
		}
		if arg == "--sender" {
			sender = true
		}
		if len(arg) > 4096 || strings.ContainsAny(arg, "\x00\r\n") || !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--delete") || arg == "--inplace" {
			return "", fmt.Errorf("rsync 服务端选项无效")
		}
	}
	if sender == options.Upload {
		return "", fmt.Errorf("rsync 传输方向不匹配")
	}
	remote := options.Remote
	if !strings.HasPrefix(remote, "/") && !strings.HasPrefix(remote, "./") {
		remote = "./" + remote
	}
	if options.Directory {
		remote = strings.TrimRight(remote, "/") + "/"
	}
	guardPath := strings.TrimRight(remote, "/")
	if guardPath == "" {
		guardPath = "/"
	}
	quoted := shellQuote(guardPath)
	kind := "-f"
	if options.Directory {
		kind = "-d"
	}
	guard := "if [ -L " + quoted + " ] || { [ -e " + quoted + " ] && [ ! " + kind + " " + quoted + " ]; }; then printf 'OShell: incompatible rsync path\\n' >&2; exit 1; fi; "
	if !options.Upload {
		guard += "test " + kind + " " + quoted + " || exit 1; "
	}
	command := []string{"rsync"}
	for _, arg := range flags {
		command = append(command, shellQuote(arg))
	}
	command = append(command, ".", shellQuote(remote))
	return guard + "exec " + strings.Join(command, " "), nil
}

type rsyncHello struct {
	Token     string
	Arguments []string
}

func (c *Connection) serveRsync(ctx context.Context, conn net.Conn, token string, options RsyncOptions) error {
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if err := verifyLaunchConnection(conn, false); err != nil {
		return err
	}
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var hello rsyncHello
	if err := readMCPFrame(conn, &hello); err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(hello.Token), []byte(token)) != 1 {
		return fmt.Errorf("rsync 桥接认证失败")
	}
	command, err := rsyncRemoteCommand(hello.Arguments, options)
	if err != nil {
		return err
	}
	conn.SetReadDeadline(time.Time{})
	// A channel open can outlive cancellation; close a late result without
	// disconnecting other sessions sharing this authenticated transport.
	type openedSession struct {
		session *ssh.Session
		err     error
	}
	opened := make(chan openedSession)
	go func() {
		s, e := c.Client.NewSession()
		select {
		case opened <- openedSession{s, e}:
		case <-ctx.Done():
			if s != nil {
				s.Close()
			}
		}
	}()
	var result openedSession
	select {
	case result = <-opened:
	case <-ctx.Done():
		return ctx.Err()
	}
	session, err := result.session, result.err
	if err != nil {
		return err
	}
	defer session.Close()
	stopSession := context.AfterFunc(ctx, func() { session.Close() })
	defer stopSession()
	input, err := session.StdinPipe()
	if err != nil {
		return err
	}
	var outputMu sync.Mutex
	session.Stdout = &rsyncFrameWriter{conn: conn, mu: &outputMu, kind: 0}
	session.Stderr = &rsyncFrameWriter{conn: conn, mu: &outputMu, kind: 1}
	if err = session.Start(command); err != nil {
		return err
	}
	inputDone := make(chan error, 1)
	go func() {
		defer input.Close()
		for {
			kind, data, err := readRsyncFrame(conn)
			if err != nil {
				inputDone <- err
				session.Close()
				return
			}
			if kind == 1 {
				inputDone <- nil
				return
			}
			if kind != 0 {
				inputDone <- fmt.Errorf("rsync 输入帧无效")
				session.Close()
				return
			}
			if _, err = input.Write(data); err != nil {
				inputDone <- err
				return
			}
		}
	}()
	err = session.Wait()
	code := byte(0)
	if err != nil {
		code = 1
	}
	outputMu.Lock()
	_ = writeRsyncFrame(conn, 2, []byte{code})
	outputMu.Unlock()
	conn.Close()
	session.Close()
	<-inputDone
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

type rsyncFrameWriter struct {
	conn net.Conn
	mu   *sync.Mutex
	kind byte
}

func (w *rsyncFrameWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	total := len(data)
	for len(data) > 0 {
		n := min(32768, len(data))
		if err := writeRsyncFrame(w.conn, w.kind, data[:n]); err != nil {
			return total - len(data), err
		}
		data = data[n:]
	}
	return total, nil
}
func writeRsyncFrame(w io.Writer, kind byte, data []byte) error {
	if len(data) > 65536 {
		return fmt.Errorf("rsync 帧超限")
	}
	header := make([]byte, 5, len(data)+5)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	header[4] = kind
	packet := append(header, data...)
	for len(packet) > 0 {
		n, err := w.Write(packet)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		packet = packet[n:]
	}
	return nil
}
func readRsyncFrame(r io.Reader) (byte, []byte, error) {
	var header [5]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(header[:4])
	if n > 65536 {
		return 0, nil, fmt.Errorf("rsync 帧超限")
	}
	data := make([]byte, n)
	_, err := io.ReadFull(r, data)
	return header[4], data, err
}
func RunRsyncStdio(ctx context.Context, args []string, in io.Reader, out, stderr io.Writer) error {
	endpoint, token := os.Getenv("OSHELL_RSYNC_ENDPOINT"), os.Getenv("OSHELL_RSYNC_TOKEN")
	if endpoint == "" || token == "" {
		return fmt.Errorf("rsync 桥接只能由应用内部启动")
	}
	conn, err := dialLaunch(ctx, endpoint)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err = verifyLaunchConnection(conn, true); err != nil {
		return err
	}
	if err = writeMCPFrame(conn, rsyncHello{Token: token, Arguments: args}); err != nil {
		return err
	}
	input, closeInput, err := rsyncStdioInput(in)
	if err != nil {
		return fmt.Errorf("rsync 标准输入初始化失败：%w", err)
	}
	defer closeInput()
	inputError := make(chan error, 1)
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	go func() {
		buf := make([]byte, 32768)
		for {
			n, err := input.Read(buf)
			if n > 0 {
				if writeRsyncFrame(conn, 0, buf[:n]) != nil {
					return
				}
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					inputError <- err
					conn.Close()
					return
				}
				_ = writeRsyncFrame(conn, 1, nil)
				return
			}
		}
	}()
	for {
		kind, data, err := readRsyncFrame(conn)
		if err != nil {
			select {
			case inputErr := <-inputError:
				return fmt.Errorf("rsync 标准输入读取失败：%w", inputErr)
			default:
			}
			return err
		}
		switch kind {
		case 0:
			_, err = io.Copy(out, bytes.NewReader(data))
		case 1:
			_, err = io.Copy(stderr, bytes.NewReader(data))
		case 2:
			if len(data) != 1 || data[0] != 0 {
				return fmt.Errorf("远端 rsync 退出失败")
			}
			return nil
		default:
			return fmt.Errorf("rsync 输出帧无效")
		}
		if err != nil {
			return err
		}
	}
}

type RsyncFileSystem struct {
	RemoteFileSystem
	Connection *Connection
}

func (f *RsyncFileSystem) Upload(ctx context.Context, local, remote string, report func(TransferProgress)) error {
	info, err := os.Lstat(local)
	if err != nil {
		return err
	}
	return f.Connection.Rsync(ctx, RsyncOptions{Local: local, Remote: remote, Upload: true, Directory: info.IsDir()}, report)
}
func (f *RsyncFileSystem) Download(ctx context.Context, remote, local string, replace bool, report func(TransferProgress)) error {
	info, err := f.Lstat(remote)
	if err != nil {
		return err
	}
	return f.Connection.Rsync(ctx, RsyncOptions{Local: local, Remote: remote, Directory: info.IsDir()}, report)
}

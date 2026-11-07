package core

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestRsyncArgumentsProgressAndPathRules(t *testing.T) {
	local := filepath.Join(t.TempDir(), "a 'quoted' 中文")
	if err := os.Mkdir(local, 0700); err != nil {
		t.Fatal(err)
	}
	options := RsyncOptions{Local: local, Remote: "a 'quoted' 中文", Directory: true, Upload: true, Rate: 1025}
	args, dir, err := RsyncArguments(options, "D:/Program Files/oshell.exe")
	if err != nil || dir != filepath.Dir(local) {
		t.Fatal(args, dir, err)
	}
	joined := strings.Join(args, "\n")
	if !strings.Contains(joined, "--bwlimit=2") || strings.Contains(joined, "--delete") || args[len(args)-2] != "./"+filepath.Base(local)+"/" || args[len(args)-1] != "oshell:"+shellQuote("./"+options.Remote+"/") {
		t.Fatal(args)
	}
	command, err := rsyncRemoteCommand([]string{"oshell", "rsync", "--server", "-rlt", ".", "ignored-untrusted-path"}, options)
	if err != nil || strings.Contains(command, "ignored-untrusted-path") || !strings.Contains(command, shellQuote("./"+options.Remote+"/")) {
		t.Fatal(command, err)
	}
	for _, args := range [][]string{{"oshell", "rsync", "--server", "--sender", ".", "x"}, {"oshell", "rsync", "--server", "--delete", ".", "x"}, {"bad", "rsync", "--server", "-rlt", ".", "x"}} {
		if _, err := rsyncRemoteCommand(args, options); err == nil {
			t.Fatal("invalid remote args accepted", args)
		}
	}
	options.Remote = "bad\npath"
	if _, _, err := RsyncArguments(options, "app"); err == nil {
		t.Fatal("newline path accepted")
	}
	options.Remote = "/"
	options.Upload = false
	command, err = rsyncRemoteCommand([]string{"oshell", "rsync", "--server", "--sender", "-rlt", ".", "/"}, options)
	if err != nil || !strings.Contains(command, "test -d '/'") {
		t.Fatal(command, err)
	}
	options.Directory = false
	if _, _, err := RsyncArguments(options, "app"); err == nil {
		t.Fatal("directory used as file")
	}
	var totals []int64
	parser := &RsyncProgress{report: func(p TransferProgress) { totals = append(totals, p.Bytes) }}
	for _, part := range []string{"OSHELL_RSYNC_FILE:1000\n   5", "00 50%\r1000 100%\nOSHELL_RSYNC_FILE:20\n20 100%\n", strings.Repeat("x", 6000) + "\ninvalid 50%\n12 NaN%\n"} {
		parser.Write([]byte(part))
	}
	if len(totals) != 3 || totals[0] != 500 || totals[1] != 1000 || totals[2] != 1020 || len(parser.line) != 0 {
		t.Fatal(totals)
	}
}

func TestRsyncPrivateBridgeUsesOneSSHConnection(t *testing.T) {
	server := startSSH(t, "test")
	commands := make(chan string, 4)
	handler := testSSHExecHandler(func(command string, ch ssh.Channel) {
		commands <- command
		io.WriteString(ch.Stderr(), "fixture stderr\n")
		io.Copy(ch, ch)
		ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{0}))
	})
	server.execHandler.Store(&handler)
	connection, err := Dial(context.Background(), testProfile(server.address), Credentials{Password: "test"}, testTrust(t), acceptHost)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	run := func(cancelEarly bool) {
		endpoint := filepath.Join(t.TempDir(), "rsync")
		listener, cleanup, err := listenLaunch(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		if cleanup != nil {
			defer cleanup()
		}
		defer listener.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		t.Setenv("OSHELL_RSYNC_ENDPOINT", endpoint)
		t.Setenv("OSHELL_RSYNC_TOKEN", "synthetic-private-token")
		served := make(chan error, 1)
		go func() {
			conn, err := listener.Accept()
			if err == nil {
				err = connection.serveRsync(ctx, conn, "synthetic-private-token", RsyncOptions{Remote: "dir's name", Upload: true, Directory: true})
			}
			served <- err
		}()
		data := bytes.Repeat([]byte{0, 255, 10, 13, 27, 'a'}, 12000)
		var in io.Reader = bytes.NewReader(data)
		if cancelEarly {
			r, w := io.Pipe()
			defer r.Close()
			defer w.Close()
			in = r
		}
		var out, stderr bytes.Buffer
		done := make(chan error, 1)
		go func() {
			done <- RunRsyncStdio(ctx, []string{"oshell", "rsync", "--server", "-rlt", ".", "ignored"}, in, &out, &stderr)
		}()
		select {
		case command := <-commands:
			if !strings.Contains(command, "exec rsync") || strings.Contains(command, "ignored") {
				t.Fatal(command)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		if cancelEarly {
			cancel()
		}
		select {
		case err := <-done:
			if !cancelEarly && (err != nil || !bytes.Equal(out.Bytes(), data) || stderr.String() != "fixture stderr\n") {
				t.Fatal(out.Len(), stderr.String(), err)
			}
		case <-time.After(6 * time.Second):
			t.Fatal("stdio did not stop")
		}
		select {
		case err := <-served:
			if !cancelEarly && err != nil {
				t.Fatal(err)
			}
		case <-time.After(6 * time.Second):
			t.Fatal("channel did not stop")
		}
	}
	run(false)
	run(true)
	if server.auth.Load() != 1 || !connection.Alive() {
		t.Fatal("bridge changed or closed shared transport")
	}
	// A normal SSH channel still works after rsync cancellation.
	session, err := connection.Client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	session.Close()
	t.Setenv("PATH", t.TempDir())
	if err = connection.Rsync(context.Background(), RsyncOptions{}, nil); err == nil || !strings.Contains(err.Error(), "本机缺少 rsync") {
		t.Fatal(err)
	}
}

func TestRsyncFrameRejectsOversizeAndTruncation(t *testing.T) {
	if err := writeRsyncFrame(io.Discard, 0, make([]byte, 65537)); err == nil {
		t.Fatal("oversize frame")
	}
	if _, _, err := readRsyncFrame(bytes.NewReader([]byte{0, 0, 0, 2, 0, 1})); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
}

func TestRsyncPartialDirectoryArguments(t *testing.T) {
	for _, upload := range []bool{true, false} {
		for _, partial := range [][]string{{"--partial-dir", ".oshell-rsync-partial"}, {"--partial-dir=.oshell-rsync-partial"}} {
			args := []string{"oshell", "rsync", "--server"}
			if !upload {
				args = append(args, "--sender")
			}
			args = append(args, "-ltre.iLsfxCIvu", "--log-format=X", "--timeout=60")
			args = append(args, partial...)
			args = append(args, "--safe-links", ".", "ignored")
			command, err := rsyncRemoteCommand(args, RsyncOptions{Upload: upload, Directory: true, Remote: "target 'quoted' 中文"})
			if err != nil || !strings.Contains(command, ".oshell-rsync-partial") || strings.Contains(command, "ignored") {
				t.Fatalf("%v: %s %v", args, command, err)
			}
		}
	}
	for _, flags := range [][]string{
		{"--partial-dir"}, {"--partial-dir", "--delete"}, {"--partial-dir", "../outside"},
		{"--partial-dir=/outside"}, {"--partial-dir", ".oshell-rsync-partial", "bare"},
		{"--partial-dir", ".oshell-rsync-partial", "--delete"}, {"--partial-dir", ".oshell-rsync-partial", "--inplace"},
	} {
		args := append([]string{"oshell", "rsync", "--server"}, flags...)
		args = append(args, ".", "ignored")
		if _, err := rsyncRemoteCommand(args, RsyncOptions{Upload: true, Remote: "target"}); err == nil {
			t.Fatalf("invalid options accepted: %v", flags)
		}
	}
}

type rsyncInputFailure struct{ err error }

func (r rsyncInputFailure) Read([]byte) (int, error) { return 0, r.err }

func TestRsyncStdioReportsInputFailure(t *testing.T) {
	endpoint := filepath.Join(t.TempDir(), "rsync-input")
	listener, cleanup, err := listenLaunch(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if cleanup != nil {
		defer cleanup()
	}
	t.Setenv("OSHELL_RSYNC_ENDPOINT", endpoint)
	t.Setenv("OSHELL_RSYNC_TOKEN", "fixture-token")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, err := listener.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		var hello rsyncHello
		if readMCPFrame(c, &hello) == nil {
			io.Copy(io.Discard, c)
		}
	}()
	want := errors.New("synthetic input read failure")
	err = RunRsyncStdio(ctx, nil, rsyncInputFailure{want}, io.Discard, io.Discard)
	if !errors.Is(err, want) {
		t.Fatalf("input error hidden: %v", err)
	}
	<-done
}

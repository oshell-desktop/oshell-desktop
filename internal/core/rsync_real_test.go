package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// Let the real rsync invoke this test executable exactly as it invokes the
// application's --rsync-stdio entry point. No production credentials are used.
func init() {
	if os.Getenv("OSHELL_TEST_REAL_RSYNC_HELPER") != "1" || len(os.Args) < 2 || os.Args[1] != "--rsync-stdio" {
		return
	}
	args := os.Args[2:]
	if os.Getenv("OSHELL_TEST_RSYNC_REJECT") == "1" {
		for i := range args {
			if args[i] == rsyncPartialDirectory {
				args[i] = "../outside"
			}
		}
	}
	if err := RunRsyncStdio(context.Background(), args, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

// The loopback SSH fixture has no POSIX shell. Decode only the generated exec
// argv and run a real rsync receiver/sender in its isolated directory. Shell
// guard/path quoting itself remains covered by TestRsyncArgumentsProgressAndPathRules.
func rsyncFixtureArgs(command string) ([]string, error) {
	_, tail, ok := strings.Cut(command, "exec rsync ")
	if !ok {
		return nil, fmt.Errorf("missing rsync exec")
	}
	var args []string
	var word strings.Builder
	quoted, started := false, false
	for i := 0; i < len(tail); i++ {
		switch b := tail[i]; {
		case b == '\'':
			quoted = !quoted
			started = true
		case b == '\\' && !quoted:
			i++
			if i == len(tail) {
				return nil, fmt.Errorf("unfinished escape")
			}
			word.WriteByte(tail[i])
			started = true
		case b == ' ' && !quoted:
			if started {
				args = append(args, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteByte(b)
			started = true
		}
	}
	if quoted {
		return nil, fmt.Errorf("unfinished quote")
	}
	if started {
		args = append(args, word.String())
	}
	return args, nil
}

func TestRsyncRealClientRoundTrip(t *testing.T) {
	binary, err := exec.LookPath("rsync")
	if err != nil {
		t.Skip("real rsync executable not installed")
	}
	t.Setenv("OSHELL_TEST_REAL_RSYNC_HELPER", "1")
	server := startSSH(t, "test")
	remoteRoot := t.TempDir()
	handler := testSSHExecHandler(func(command string, ch ssh.Channel) {
		args, err := rsyncFixtureArgs(command)
		code := uint32(0)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, args...)
			cmd.Dir = remoteRoot
			cmd.Stdout, cmd.Stderr = ch, ch.Stderr()
			prepareCommand(cmd)
			var input io.WriteCloser
			input, err = cmd.StdinPipe()
			if err == nil {
				err = cmd.Start()
			}
			if err == nil {
				// Wait must not wait for SSH EOF before sending exit-status.
				go func() { defer input.Close(); io.Copy(input, ch) }()
				err = cmd.Wait()
			} else if input != nil {
				input.Close()
			}
		}
		if err != nil {
			code = 1
			fmt.Fprintln(ch.Stderr(), err)
		}
		ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{code}))
	})
	server.execHandler.Store(&handler)
	c, err := Dial(context.Background(), testProfile(server.address), Credentials{Password: "test"}, testTrust(t), acceptHost)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	source := filepath.Join(t.TempDir(), "source 'quoted' 中文")
	if err = os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	name := "file 'quoted' 中文.bin"
	payload := bytes.Repeat([]byte{0, 255, 10, 13, 27, 42}, 20000)
	if err = os.WriteFile(filepath.Join(source, name), payload, 0600); err != nil {
		t.Fatal(err)
	}
	remote := "receiver 'quoted' 中文"
	options := RsyncOptions{Local: source, Remote: remote, Upload: true, Directory: true}
	run := func(o RsyncOptions) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := c.Rsync(ctx, o, nil); err != nil {
			t.Fatal(err)
		}
	}
	check := func(root string) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || !bytes.Equal(got, payload) {
			t.Fatalf("payload mismatch: %v", err)
		}
	}
	run(options)
	check(filepath.Join(remoteRoot, remote))
	if err = os.WriteFile(filepath.Join(remoteRoot, remote, "extra.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	payload = append(payload, []byte("incremental change")...)
	if err = os.WriteFile(filepath.Join(source, name), payload, 0600); err != nil {
		t.Fatal(err)
	}
	run(options)
	check(filepath.Join(remoteRoot, remote))
	if extra, err := os.ReadFile(filepath.Join(remoteRoot, remote, "extra.txt")); err != nil || string(extra) != "keep" {
		t.Fatal("extra target file removed", err)
	}
	download := RsyncOptions{Local: filepath.Join(t.TempDir(), "download 中文"), Remote: remote, Directory: true}
	run(download)
	check(download.Local)
	t.Run("absolute path and accidental leading space", func(t *testing.T) {
		target := filepath.Join(remoteRoot, "absolute 'quoted' 中文")
		absolute := filepath.ToSlash(target)
		if runtime.GOOS == "windows" {
			// The real Windows receiver is cwRsync/Cygwin; use its absolute
			// drive mount, without replacing or decoding the remote argv.
			volume := filepath.VolumeName(target)
			if len(volume) != 2 || volume[1] != ':' {
				t.Skip("absolute-path fixture requires a local Windows drive")
			}
			absolute = "/cygdrive/" + strings.ToLower(volume[:1]) + absolute[len(volume):]
		}
		bad := options
		bad.Remote = " " + absolute + "/"
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := c.Rsync(ctx, bad, nil)
		// At the filesystem boundary spaces are literal. The form must trim
		// pasted padding before both logging and starting the transfer.
		if err == nil || !strings.Contains(err.Error(), "mkdir") || !strings.Contains(err.Error(), "./ /") {
			t.Fatalf("leading-space reproduction: %v", err)
		}
		t.Logf("leading-space reproduction: %v", err)
		good := bad
		good.Remote = strings.TrimSpace(bad.Remote)
		run(good)
		check(target)
		good.Upload = false
		good.Local = filepath.Join(t.TempDir(), "absolute download")
		run(good)
		check(good.Local)
	})
	t.Run("bridge rejection is visible", func(t *testing.T) {
		t.Setenv("OSHELL_TEST_RSYNC_REJECT", "1")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := c.Rsync(ctx, options, nil)
		if err == nil || !strings.Contains(err.Error(), "rsync 续传目录参数无效") {
			t.Fatalf("bridge cause hidden: %v", err)
		}
	})
	t.Run("cancel and restart", func(t *testing.T) {
		large := bytes.Repeat([]byte("synthetic resume payload\x00\xff\r\n"), 80000)
		if err := os.WriteFile(filepath.Join(source, "large.bin"), large, 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		slow := options
		slow.Rate = 32768
		err := c.Rsync(ctx, slow, func(p TransferProgress) {
			if p.Bytes > 0 {
				cancel()
			}
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel: %v", err)
		}
		if !c.Alive() {
			t.Fatal("cancel closed shared SSH transport")
		}
		partial := filepath.Join(remoteRoot, remote, rsyncPartialDirectory, "large.bin")
		deadline := time.Now().Add(3 * time.Second)
		for {
			info, err := os.Stat(partial)
			if err == nil && info.Size() > 0 && info.Size() < int64(len(large)) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("cancel did not retain a partial file", err)
			}
			time.Sleep(20 * time.Millisecond)
		}
		run(options)
		got, err := os.ReadFile(filepath.Join(remoteRoot, remote, "large.bin"))
		if err != nil || !bytes.Equal(got, large) {
			t.Fatal("restarted transfer differs", err)
		}
	})
	if server.auth.Load() != 1 || !c.Alive() {
		t.Fatal("transfer reauthenticated or closed shared SSH")
	}
	// The existing SSH transport is still usable after success and cancellation.
	s, err := c.Client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
}

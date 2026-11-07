package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestSFTPCheckpointAfterRestart(t *testing.T) {
	const childRequest = "OSHELL_TEST_REOPEN_SFTP"
	if encoded := os.Getenv(childRequest); encoded != "" {
		var request SFTPCheckpoint
		if err := json.Unmarshal([]byte(encoded), &request); err != nil {
			t.Fatal(err)
		}
		client := resumeClientAt(t, os.Getenv("OSHELL_TEST_SFTP_SERVER"))
		start := int64(-1)
		if err := request.Run(context.Background(), client, func(p TransferProgress) {
			if start < 0 {
				start = p.Bytes
			}
		}); err != nil {
			t.Fatal(err)
		}
		if start <= 0 {
			t.Fatal("new process restarted from byte zero")
		}
		return
	}
	server := startSSH(t, "resume-test")
	client := resumeClientAt(t, server.address)
	for _, upload := range []bool{true, false} {
		t.Run(map[bool]string{true: "upload", false: "download"}[upload], func(t *testing.T) {
			source := filepath.ToSlash(filepath.Join(t.TempDir(), "source.bin"))
			target := filepath.ToSlash(filepath.Join(t.TempDir(), "target.bin"))
			content := bytes.Repeat([]byte("sftp"), 256<<10)
			if err := os.WriteFile(source, content, 0600); err != nil {
				t.Fatal(err)
			}
			request := SFTPCheckpoint{Upload: upload, Source: source, Target: target, StateDir: t.TempDir(), Identity: testProfile(server.address).Identity()}
			first := request
			pauseCheckpoint(t, &first, client)
			// Pass only a fresh request. The child must reconnect and find its
			// fragment from the on-disk checkpoint without the original handle.
			data, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSFTPCheckpointAfterRestart$")
			child.Env = append(os.Environ(), childRequest+"="+string(data), "OSHELL_TEST_SFTP_SERVER="+server.address)
			if out, err := child.CombinedOutput(); err != nil {
				t.Fatalf("restart failed: %v %s", err, out)
			}
			got, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(got, content) {
				t.Fatal("restart did not restore the verified fragment", err)
			}
			records, _ := filepath.Glob(filepath.Join(request.StateDir, "sftp-checkpoints", "*.json"))
			if len(records) != 0 {
				t.Fatal("completed checkpoint retained")
			}
			if err = os.Remove(target); err != nil {
				t.Fatal(err)
			}
			request.Fragment = filepath.ToSlash(filepath.Join(filepath.Dir(target), ".oshell-old.part"))
			if !upload {
				request.Target = filepath.Clean(target)
				request.Fragment = filepath.Clean(request.Fragment)
			}
			if err = os.WriteFile(request.Fragment, []byte("wrong"), 0600); err != nil {
				t.Fatal(err)
			}
			old := request
			if err = old.Run(context.Background(), client, nil); err == nil {
				t.Fatal("invalid old fragment accepted")
			}
			got, _ = os.ReadFile(request.Fragment)
			if string(got) != "wrong" {
				t.Fatal("old fragment modified on mismatch")
			}
			if !upload {
				alias := filepath.Join(filepath.Dir(request.Fragment), "other-file")
				if err = os.Link(request.Fragment, alias); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(alias, content[:8000], 0600); err != nil {
					t.Fatal(err)
				}
				linked := request
				if err = linked.Run(context.Background(), client, nil); err == nil {
					t.Fatal("modified a hard-linked fragment")
				}
				if err = os.Remove(alias); err != nil {
					t.Fatal(err)
				}
			} else {
				missing := request
				missing.Fragment = ""
				missing.Target = filepath.ToSlash(filepath.Join(t.TempDir(), "missing", "file"))
				if err = missing.Run(context.Background(), client, nil); err == nil {
					t.Fatal("wrote to a missing target directory")
				}
			}
			if err = os.WriteFile(request.Fragment, content[:8000], 0600); err != nil {
				t.Fatal(err)
			}
			if err = request.Run(context.Background(), client, nil); err != nil {
				t.Fatal(err)
			}
			got, err = os.ReadFile(target)
			if err != nil || !bytes.Equal(got, content) {
				t.Fatal("legacy recovery corrupted the file", err)
			}
		})
	}
}

func TestSFTPDirectoryRestartSkipsVerifiedFiles(t *testing.T) {
	client := resumeTestClient(t)
	for _, upload := range []bool{true, false} {
		source := filepath.ToSlash(t.TempDir())
		target := filepath.ToSlash(filepath.Join(t.TempDir(), "tree"))
		payload := bytes.Repeat([]byte("x"), 1<<20)
		for name, data := range map[string][]byte{"a": []byte("complete"), "b": payload} {
			if err := os.WriteFile(filepath.Join(source, name), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		tree := SFTPCheckpoint{Source: source, Target: target, Directory: true, Upload: upload, StateDir: t.TempDir(), Identity: SSHIdentity{"host", "user", 22}}
		ctx, cancel := context.WithCancel(context.Background())
		limiter := &TransferLimiter{}
		limiter.Set(1 << 20)
		first := tree
		err := first.Run(WithTransferLimiter(ctx, limiter), client, func(p TransferProgress) {
			if p.Bytes >= 64<<10 {
				cancel()
			}
		})
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatal("directory was not interrupted", err)
		}
		complete := filepath.Join(target, "a")
		old := time.Unix(946684800, 0)
		if err = os.Chtimes(complete, old, old); err != nil {
			t.Fatal(err)
		}
		tree.ResumeExisting = true
		if err = tree.Run(context.Background(), client, nil); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(complete)
		if err != nil || !info.ModTime().Equal(old) {
			t.Fatal("complete file was rewritten", err)
		}
		got, err := os.ReadFile(filepath.Join(target, "b"))
		if err != nil || !bytes.Equal(got, payload) {
			t.Fatal("directory did not recover", err)
		}
		if err = os.WriteFile(complete, []byte("changed!"), 0600); err != nil {
			t.Fatal(err)
		}
		if err = tree.Run(context.Background(), client, nil); err == nil {
			t.Fatal("accepted a same-size mismatch")
		}
	}
}

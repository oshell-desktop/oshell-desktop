package core

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/pkg/sftp"
)

func TestRetainedC08RoutingFailuresNeverUseDirectTarget(t *testing.T) {
	server := startSSH(t, "fixture")
	p := testProfile(server.address)
	p.ProxyID = NewID()
	if conn, err := Dial(context.Background(), p, Credentials{Password: "fixture"}, testTrust(t), acceptHost); err == nil {
		conn.Close()
		t.Fatal("missing shared route used direct target")
	}
	p.ProxyID = ""
	dir := t.TempDir()
	installJumpFixtureHelper(t, dir, "ssh")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OSHELL_TEST_JUMP_HELPER", "1")
	raw, _ := json.Marshal(map[string]jumpFixtureConfig{p.Host: {Host: p.Host, User: p.Username, Port: p.Port, Command: "oshell-missing-route-helper-42"}})
	t.Setenv("OSHELL_TEST_JUMP_CONFIG", string(raw))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if conn, err := Dial(ctx, p, Credentials{Password: "fixture"}, testTrust(t), acceptHost); err == nil {
		conn.Close()
		t.Fatal("failed helper fell back to direct target")
	}
	if server.auth.Load() != 0 {
		t.Fatal("rejected route reached target authentication")
	}
}

func TestRetainedC08NativeOpenSSHConfiguration(t *testing.T) {
	binary, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("native OpenSSH unavailable")
	}
	dir := t.TempDir()
	key := filepath.Join(dir, "fixture identity")
	if err = os.WriteFile(key, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	config := "Host route-alias\n HostName 127.0.0.1\n User mapped-user\n Port 2222\n IdentityFile \"" + filepath.ToSlash(key) + "\"\n ProxyJump jump-alias\nHost command-alias\n HostName 127.0.0.2\n User command-user\n ProxyCommand fixture-helper %h %p\n"
	file := filepath.Join(dir, "config")
	if err = os.WriteFile(file, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"route-alias", "command-alias"} {
		cmd := exec.Command(binary, "-G", "-F", file, "--", host)
		prepareCommand(cmd)
		output, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		p := NewProfile()
		p.Host = host
		p.Username = ""
		p.Port = 0
		resolved := ResolvedSSH{Profile: p, OriginalHost: host}
		applyResolvedSSH(&resolved, string(output))
		if host == "route-alias" {
			if resolved.Profile.Host != "127.0.0.1" || resolved.Profile.Username != "mapped-user" || resolved.Profile.Port != 2222 || resolved.Profile.JumpHost != "jump-alias" || len(resolved.Profile.resolvedIdentityFiles) != 1 {
				t.Fatal("native alias fields lost")
			}
		} else if resolved.ProxyCommand != "fixture-helper %h %p" {
			t.Fatal("native ProxyCommand lost")
		}
	}
}

func TestRetainedA02SharedAutoCursorRoundtrip(t *testing.T) {
	v, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	master := []byte("synthetic-shared-master")
	for _, automatic := range []bool{false, true} {
		cfg := DefaultConfig()
		scheme := cfg.Settings.Scheme()
		scheme.ID = NewID()
		scheme.Name = "retained cursor"
		scheme.AutoCursor = automatic
		scheme.Cursor = "#00FF00"
		cfg.Settings.CustomSchemes = []ColorScheme{scheme}
		doc, _, err := v.ExportSharedDocument(context.Background(), cfg, master, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		data, err := doc.Encode(master)
		if err != nil {
			t.Fatal(err)
		}
		back, err := DecodeSharedDocument(data, master)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := back.Configuration()
		if err != nil {
			t.Fatal(err)
		}
		if len(restored.Settings.CustomSchemes) != 1 || restored.Settings.CustomSchemes[0].AutoCursor != automatic || restored.Settings.CustomSchemes[0].Cursor != "#00FF00" {
			t.Fatal("cursor fields lost in shared roundtrip")
		}
	}
}

func TestRetainedF07SCPWithoutSFTPAndFTPFailure(t *testing.T) {
	if _, err := exec.LookPath("scp"); err != nil {
		t.Skip("synthetic SSH server requires local scp")
	}
	server := startSSH(t, "fixture")
	server.rejectSFTP.Store(true)
	conn, err := Dial(context.Background(), testProfile(server.address), Credentials{Password: "fixture"}, testTrust(t), acceptHost)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if fs, err := sftp.NewClient(conn.Client); err == nil {
		fs.Close()
		t.Fatal("SFTP was not disabled")
	}
	data := []byte(strings.Repeat("中文 payload ", 10000))
	source := filepath.Join(t.TempDir(), "a 'quoted' file.txt")
	os.WriteFile(source, data, 0600)
	remote := filepath.ToSlash(filepath.Join(t.TempDir(), "remote 'quoted' file.txt"))
	var last int64
	report := func(p TransferProgress) {
		if p.Bytes < last || p.Bytes > int64(len(data)) {
			t.Error("invalid byte progress")
		}
		last = p.Bytes
	}
	if err = conn.SCPUpload(context.Background(), source, remote, report); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "download.txt")
	last = 0
	if err = conn.SCPDownload(context.Background(), remote, dest, false, report); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, data) {
		t.Fatal("SCP content changed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = conn.SCPDownload(ctx, remote, dest, true, nil); err == nil {
		t.Fatal("cancelled download succeeded")
	}
	got, _ = os.ReadFile(dest)
	if !bytes.Equal(got, data) {
		t.Fatal("cancel changed destination")
	}
	if err = conn.SCPDownload(context.Background(), remote+"missing", dest, true, nil); err == nil {
		t.Fatal("failed remote transfer succeeded")
	}
	for _, mode := range []string{"", "fail-retr"} {
		p, root := ftpTestServer(t, mode)
		os.WriteFile(filepath.Join(root, "file.txt"), data, 0600)
		s, fs, err := LoginFTP(context.Background(), p, "test-only")
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(t.TempDir(), "ftp.txt")
		os.WriteFile(target, []byte("original"), 0600)
		last = 0
		err = fs.Download(context.Background(), "/file.txt", target, true, report)
		fs.Close()
		s.Close()
		got, _ := os.ReadFile(target)
		if mode == "" {
			if err != nil || !bytes.Equal(got, data) {
				t.Fatal("FTP normal download", err)
			}
		} else {
			if err == nil || string(got) != "original" {
				t.Fatal("FTP failure published partial file", err)
			}
			leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".oshell-ftp-*"))
			if len(leftovers) != 0 {
				t.Fatal("failure left staging data")
			}
		}
	}
}

func TestRetainedP02LocalKeyFailureAndMigration(t *testing.T) {
	for _, failure := range []string{"missing", "short", "wrong"} {
		t.Run(failure, func(t *testing.T) {
			dir := t.TempDir()
			v, err := OpenVault(dir)
			if err != nil {
				t.Fatal(err)
			}
			p := NewProfile()
			p.Host = "fixture.invalid"
			p.Username = "fixture"
			if err = v.Put(p, "synthetic-secret"); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenVault(dir)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := reopened.GetFor(p, p.Identity()); err != nil || got != "synthetic-secret" {
				t.Fatal("reopen", err)
			}
			vaultPath := filepath.Join(dir, "credentials.json")
			before, _ := os.ReadFile(vaultPath)
			keyPath := filepath.Join(dir, "credential-key")
			if failure == "missing" {
				err = os.Remove(keyPath)
			} else {
				n := 3
				if failure == "wrong" {
					n = 32
				}
				err = os.WriteFile(keyPath, bytes.Repeat([]byte{0x41}, n), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			broken, err := OpenVault(dir)
			if err == nil {
				_, err = broken.GetFor(p, p.Identity())
			}
			if err == nil {
				t.Fatal("bad key accepted")
			}
			after, _ := os.ReadFile(vaultPath)
			if !bytes.Equal(before, after) {
				t.Fatal("failed read overwrote vault")
			}
			if failure == "missing" {
				if _, err = os.Stat(keyPath); !os.IsNotExist(err) {
					t.Fatal("missing key regenerated")
				}
			}
		})
	}
	key := LocalCredentialKey{ID: NewID(), Secret: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x31}, 32))}
	dir := t.TempDir()
	raw, _ := json.Marshal(key)
	if err := os.WriteFile(filepath.Join(dir, "local-credential-key.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOShellLocalCredentialKey(dir, key.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	p := NewProfile()
	p.Host = "native.invalid"
	p.Username = "fixture"
	envelope, err := EncryptMacPassword("migration-fixture", []byte(key.Secret), p, p.Identity())
	if err != nil {
		t.Fatal(err)
	}
	envelope.LocalKeyID = key.ID
	if got, err := DecryptMacLocalPassword(envelope, loaded, p); err != nil || got != "migration-fixture" {
		t.Fatal("native migration", err)
	}
	p.Host = "other.invalid"
	if _, err := DecryptMacLocalPassword(envelope, loaded, p); err == nil {
		t.Fatal("identity binding lost")
	}
}

func TestRetainedC08MissingResolverAndAliasFields(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("ProgramData", dir)
	p := NewProfile()
	p.Host = "route-alias"
	p.Username = "fixture"
	if _, err := ResolveSSH(context.Background(), p); err == nil {
		t.Fatal("missing ssh accepted alias")
	}
	p.Host = "127.0.0.1"
	// Unix global configuration is not altered by tests. Windows is isolated here.
	if runtime.GOOS == "windows" {
		if _, err := ResolveSSH(context.Background(), p); err != nil {
			t.Fatal("literal without config", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ssh", "config"), []byte("Host *\n ProxyCommand missing-helper\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveSSH(context.Background(), p); err == nil {
		t.Fatal("literal bypassed unreadable routing configuration")
	}
	installJumpFixtureHelper(t, dir, "ssh")
	t.Setenv("OSHELL_TEST_JUMP_HELPER", "1")
	keyPath := filepath.Join(dir, "fixture-key")
	os.WriteFile(keyPath, []byte("public-test-placeholder"), 0600)
	raw, _ := json.Marshal(map[string]jumpFixtureConfig{"route-alias": {Host: "127.0.0.1", User: "mapped", Port: 2222, Identity: keyPath, Jump: "jump-alias"}})
	t.Setenv("OSHELL_TEST_JUMP_CONFIG", string(raw))
	p.Host = "route-alias"
	p.Port = 0
	p.Username = ""
	got, err := ResolveSSH(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile.Host != "127.0.0.1" || got.Profile.Username != "mapped" || got.Profile.Port != 2222 || got.Profile.JumpHost != "jump-alias" || len(got.Profile.resolvedIdentityFiles) != 1 || got.Profile.resolvedIdentityFiles[0] != keyPath {
		t.Fatal("alias fields incomplete")
	}
	p.Host = "missing-fixture"
	if _, err := ResolveSSH(context.Background(), p); err == nil {
		t.Fatal("failed ssh -G fell through")
	}
}

func TestRetainedF06ListingLimitsAndMetadata(t *testing.T) {
	for _, mode := range []string{"raw-mlsd", "raw-list", "raw-nlst"} {
		for _, extra := range []int{0, 1} {
			t.Run(mode+string(rune('0'+extra)), func(t *testing.T) {
				// 4096 fixed-size lines make an exact 4 MiB response, then one extra byte.
				line := strings.Repeat("x", 1023) + "\n"
				if mode != "raw-nlst" {
					line = "type=file;size=1; " + strings.Repeat("x", 1023-len("type=file;size=1; ")) + "\n"
				}
				data := strings.Repeat(line, 4096) + strings.Repeat("x", extra)
				p, _ := ftpTestServer(t, mode, data)
				s, fs, err := LoginFTP(context.Background(), p, "test-only")
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				defer fs.Close()
				entries, err := fs.ReadDir("/")
				if extra == 0 {
					if err != nil || len(entries) != 4096 {
						t.Fatal("boundary listing", len(entries), err)
					}
				} else if !errors.Is(err, ftp.ErrListingTooLarge) || len(entries) != 0 {
					t.Fatal("limit bypassed", len(entries), err)
				}
			})
		}
	}
	for _, tc := range []struct {
		line  string
		known bool
	}{
		{"type=file;size=1;modify=20261009000000; 文件.txt\r\n", true},
		{"-rw-r--r-- 1 user group 1 Oct 09 10:11 文件.txt\r\n", false},
		{"-rw-r--r-- 1 user group 1 Oct 09 2025 文件.txt\r\n", false},
	} {
		p, _ := ftpTestServer(t, "raw-list", tc.line)
		s, fs, err := LoginFTP(context.Background(), p, "test-only")
		if err != nil {
			t.Fatal(err)
		}
		entries, err := fs.ReadDir("/")
		fs.Close()
		s.Close()
		if err != nil || len(entries) != 1 || entries[0].Name() != "文件.txt" || entries[0].ModTime().IsZero() == tc.known {
			t.Fatal("timestamp provenance", entries, err)
		}
	}
	p, _ := ftpTestServer(t, "raw-nlst", "safe.txt\r\n../escape\r\n/absolute\r\nbad\x00name\r\n")
	s, fs, err := LoginFTP(context.Background(), p, "test-only")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	defer fs.Close()
	entries, err := fs.ReadDir("/")
	if err != nil || len(entries) != 1 || entries[0].Name() != "safe.txt" {
		t.Fatal("unsafe names", entries, err)
	}
}

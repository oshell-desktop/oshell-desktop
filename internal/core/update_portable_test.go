package core

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestReleaseSelection(t *testing.T) {
	source := UpdateSource{Repository: DefaultUpdateRepository}
	for _, target := range []string{"windows-amd64", "linux-amd64"} {
		t.Run(target, func(t *testing.T) {
			asset := releaseAsset{Name: releaseAssetName(target), Size: 100, State: "uploaded", Digest: "sha256:" + strings.Repeat("a", 64)}
			asset.URL = "https://github.com/" + source.Repository + "/releases/download/v0.3.2/" + asset.Name
			base := githubRelease{Tag: "v0.3.2", Assets: []releaseAsset{asset}}
			for _, test := range []struct {
				name   string
				change func(*githubRelease)
				bad    bool
			}{
				{"valid", func(*githubRelease) {}, false},
				{"missing digest", func(r *githubRelease) { r.Assets[0].Digest = "" }, true},
				{"foreign host", func(r *githubRelease) { r.Assets[0].URL = "https://example.com/evil" }, true},
				{"wrong repository", func(r *githubRelease) {
					r.Assets[0].URL = strings.Replace(r.Assets[0].URL, source.Repository, "other/repo", 1)
				}, true},
				{"oversize", func(r *githubRelease) { r.Assets[0].Size = UpdateArchiveLimit + 1 }, true},
				{"prerelease", func(r *githubRelease) { r.Prerelease = true }, true},
				{"draft", func(r *githubRelease) { r.Draft = true }, true},
				{"duplicate", func(r *githubRelease) { r.Assets = append(r.Assets, r.Assets[0]) }, true},
				{"missing platform", func(r *githubRelease) { r.Assets = nil }, true},
			} {
				t.Run(test.name, func(t *testing.T) {
					r := base
					r.Assets = append([]releaseAsset(nil), base.Assets...)
					test.change(&r)
					data, _ := json.Marshal(r)
					u, e := decodeRelease(data, source, target, "0.3.1")
					if (e != nil) != test.bad || !test.bad && u.Version != "0.3.2" {
						t.Fatalf("candidate=%v error=%v", u, e)
					}
				})
			}
			data, _ := json.Marshal(base)
			for _, v := range []string{"0.3.2", "0.4.0"} {
				if u, e := decodeRelease(data, source, target, v); e != nil || u != nil {
					t.Fatal("downgrade allowed", u, e)
				}
			}
		})
	}
}
func TestReleaseDownloadIntegrity(t *testing.T) {
	data := []byte("synthetic release archive")
	for _, kind := range []string{"valid", "corrupt", "truncated", "oversize", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "corrupt":
					w.Write(bytes.Repeat([]byte("x"), len(data)))
				case "truncated":
					w.Write(data[:3])
				case "oversize":
					w.Write(append(data, 'x'))
				default:
					w.Write(data)
				}
			}))
			defer srv.Close()
			u, _, _ := signedPortableFixture(t, "windows-amd64", "0.3.2", data)
			// Route this verified fixture to the local transport server only.
			u.asset.URL, u.verified.archive.URL = srv.URL, srv.URL
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "cancel" {
				cancel()
			}
			dir := t.TempDir()
			p, e := u.download(ctx, dir, nil, srv.Client())
			if kind == "valid" {
				if e != nil {
					t.Fatal(e)
				}
				b, _ := os.ReadFile(p)
				if !bytes.Equal(b, data) {
					t.Fatal("wrong data")
				}
			} else {
				if e == nil {
					t.Fatal("invalid download accepted")
				}
				files, _ := os.ReadDir(dir)
				if len(files) != 0 {
					t.Fatal("partial download retained")
				}
			}
		})
	}
}
func testPortablePlan(t *testing.T) portablePlan {
	t.Helper()
	dir := t.TempDir()
	// Match ReleaseUpdate.Prepare: Windows TEMP can use a short path or
	// different casing, while startup resolves the executable's canonical path.
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	stage := ".oshell-update-test"
	exe := "OShell Desktop.exe"
	if runtime.GOOS != "windows" {
		exe = "oshell-desktop"
	}
	if e := os.MkdirAll(filepath.Join(dir, stage, "payload"), 0700); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(dir, exe), []byte("old binary"), 0755)
	os.WriteFile(filepath.Join(dir, stage, "payload", exe), []byte("new binary"), 0755)
	h, _ := digestFile(filepath.Join(dir, stage, "payload", exe))
	return portablePlan{Executable: filepath.Join(dir, exe), Stage: stage, Token: strings.Repeat("a", 64), Version: "0.3.2", Files: []portableFile{{Name: exe, SHA256: h, Existed: true}}}
}
func TestPortableRollbackPreservesData(t *testing.T) {
	p := testPortablePlan(t)
	dir := filepath.Dir(p.Executable)
	for _, name := range []string{"config.json", "credentials.json", "credential-key", "known_hosts", "state-transaction.json"} {
		os.WriteFile(filepath.Join(dir, name), []byte("untouched "+name), 0600)
	}
	if e := applyPortableFiles(p); e != nil {
		t.Fatal(e)
	}
	if b, _ := os.ReadFile(p.Executable); string(b) != "new binary" {
		t.Fatal("not replaced")
	}
	// Recovery is idempotent, including a restart during rollback.
	for range 2 {
		if e := rollbackPortableFiles(p); e != nil {
			t.Fatal(e)
		}
	}
	if b, _ := os.ReadFile(p.Executable); string(b) != "old binary" {
		t.Fatal("not restored")
	}
	for _, name := range []string{"config.json", "credentials.json", "credential-key", "known_hosts", "state-transaction.json"} {
		if b, _ := os.ReadFile(filepath.Join(dir, name)); string(b) != "untouched "+name {
			t.Fatal("configuration changed", name)
		}
	}
}
func TestPortableTamperedPayloadDoesNotReplace(t *testing.T) {
	p := testPortablePlan(t)
	os.WriteFile(filepath.Join(filepath.Dir(p.Executable), p.Stage, "payload", p.Files[0].Name), []byte("tampered"), 0755)
	if e := applyPortableFiles(p); e == nil {
		t.Fatal("accepted altered payload")
	}
	if e := rollbackPortableFiles(p); e != nil {
		t.Fatal(e)
	}
	if b, _ := os.ReadFile(p.Executable); string(b) != "old binary" {
		t.Fatal("changed original")
	}
}

func TestPortablePartialReplacementAndRecoveryRetry(t *testing.T) {
	p := testPortablePlan(t)
	dir := filepath.Dir(p.Executable)
	stage := filepath.Join(dir, p.Stage)
	name := "libghostty-vt.so"
	os.WriteFile(filepath.Join(dir, name), []byte("old library"), 0755)
	os.WriteFile(filepath.Join(stage, "payload", name), []byte("new library"), 0755)
	digest, _ := digestFile(filepath.Join(stage, "payload", name))
	p.Files = append(p.Files, portableFile{Name: name, SHA256: digest, Existed: true})
	blocker := filepath.Join(dir, name+".oshell-new")
	os.Mkdir(blocker, 0700)
	os.WriteFile(filepath.Join(blocker, "locked"), nil, 0600)
	if e := applyPortableFiles(p); e == nil {
		t.Fatal("expected second replacement failure")
	}
	if b, _ := os.ReadFile(p.Executable); string(b) != "new binary" {
		t.Fatal("first replacement did not run")
	}
	if e := rollbackPortableFiles(p); e == nil {
		t.Fatal("expected blocked rollback")
	}
	if b, _ := os.ReadFile(filepath.Join(stage, "backup", p.Files[0].Name)); string(b) != "old binary" {
		t.Fatal("rollback discarded backup")
	}
	os.RemoveAll(blocker)
	if e := rollbackPortableFiles(p); e != nil {
		t.Fatal(e)
	}
	if b, _ := os.ReadFile(p.Executable); string(b) != "old binary" {
		t.Fatal("executable not restored")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, name)); string(b) != "old library" {
		t.Fatal("library not restored")
	}
}
func TestPortableArchiveRejectsUnsafeEntries(t *testing.T) {
	for _, name := range []string{"../config.json", "config.json", "windows-amd64-single-file/../../credential-key", "windows-amd64-single-file/config.json", "/OShell Desktop.exe", "C:/OShell Desktop.exe"} {
		t.Run(name, func(t *testing.T) {
			p := testPortablePlan(t)
			archive := filepath.Join(t.TempDir(), "bad.zip")
			f, _ := os.Create(archive)
			z := zip.NewWriter(f)
			w, _ := z.Create(name)
			w.Write([]byte("bad"))
			z.Close()
			f.Close()
			if e := (&PortableUpdate{plan: p}).extract(context.Background(), archive, "windows-amd64"); e == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeChar} {
		p := testPortablePlan(t)
		archive := filepath.Join(t.TempDir(), "bad.tar.gz")
		f, _ := os.Create(archive)
		gz := gzip.NewWriter(f)
		tr := tar.NewWriter(gz)
		tr.WriteHeader(&tar.Header{Name: "libghostty-vt.so", Linkname: "../config.json", Typeflag: kind})
		tr.Close()
		gz.Close()
		f.Close()
		if e := (&PortableUpdate{plan: p}).extract(context.Background(), archive, "linux-amd64"); e == nil {
			t.Fatal("link accepted")
		}
	}
}
func TestPortableJournalRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../config.json", "config.json", "sub/../../credential-key"} {
		p := testPortablePlan(t)
		p.Files[0].Name = name
		file := filepath.Join(filepath.Dir(p.Executable), portableJournal)
		writeUpdateJSON(file, p)
		if _, e := readPortablePlan(file); e == nil {
			t.Fatal("unsafe journal accepted", name)
		}
	}
}

func TestPortableBuiltArchives(t *testing.T) {
	root := os.Getenv("OSHELL_TEST_UPDATE_PACKAGES")
	if root == "" {
		t.Skip("set OSHELL_TEST_UPDATE_PACKAGES to the validated package staging directory")
	}
	for _, target := range []string{"windows-amd64", "linux-amd64"} {
		t.Run(target, func(t *testing.T) {
			p := testPortablePlan(t)
			// The fixture's old payload is replaced by the actual release archive.
			if e := os.Remove(filepath.Join(filepath.Dir(p.Executable), p.Stage, "payload", p.Files[0].Name)); e != nil {
				t.Fatal(e)
			}
			p.Files = nil
			u := &PortableUpdate{plan: p}
			if e := u.extract(context.Background(), filepath.Join(root, releaseAssetName(target)), target); e != nil {
				t.Fatal(e)
			}
			if len(u.plan.Files) == 0 {
				t.Fatal("empty release package")
			}
			if target == "windows-amd64" && len(u.plan.Files) != 1 {
				t.Fatal("Windows release is not single-file")
			}
		})
	}
}

func TestPortableHelperLifecycle(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("portable updater platform")
	}
	fixture := os.Getenv("OSHELL_UPDATE_FIXTURE")
	if fixture == "" {
		if os.Getenv("OSHELL_PRECOMPILED_TESTS") == "1" {
			t.Fatal("precompiled test run requires OSHELL_UPDATE_FIXTURE")
		}
		fixture = filepath.Join(t.TempDir(), "fixture"+executableSuffix())
		build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"+executableSuffix()), "build", "-o", fixture, "./testdata/update-fixture")
		if out, e := build.CombinedOutput(); e != nil {
			t.Fatalf("fixture build: %v\n%s", e, out)
		}
	}
	if info, err := os.Stat(fixture); err != nil || info.IsDir() {
		t.Fatalf("invalid update fixture: %s (%v)", fixture, err)
	}
	for _, scenario := range []string{"success", "new crashes", "crash after ready", "interrupted"} {
		t.Run(scenario, func(t *testing.T) {
			p := testPortablePlan(t)
			dir := filepath.Dir(p.Executable)
			stage := filepath.Join(dir, p.Stage)
			original, _ := os.ReadFile(fixture)
			os.WriteFile(p.Executable, original, 0755)
			incoming := append(append([]byte(nil), original...), []byte("new version marker")...)
			os.WriteFile(filepath.Join(stage, "payload", p.Files[0].Name), incoming, 0755)
			p.Files[0].SHA256, _ = digestFile(filepath.Join(stage, "payload", p.Files[0].Name))
			os.WriteFile(filepath.Join(dir, "config.json"), []byte("synthetic config"), 0600)
			if scenario == "new crashes" {
				os.WriteFile(filepath.Join(dir, "fail-new"), nil, 0600)
			}
			if scenario == "crash after ready" {
				os.WriteFile(filepath.Join(dir, "crash-after-ready"), nil, 0600)
			}
			old := exec.Command(p.Executable, "--hold")
			old.Dir = dir
			prepareUpdateCommand(old)
			if e := old.Start(); e != nil {
				t.Fatal(e)
			}
			defer func() { old.Process.Kill(); old.Wait() }()
			p.ParentPID = old.Process.Pid
			u := &PortableUpdate{plan: p}
			if scenario == "interrupted" {
				old.Process.Kill()
				old.Wait()
				if e := copyUpdateFile(fixture, filepath.Join(stage, "updater"+executableSuffix()), 0700); e != nil {
					t.Fatal(e)
				}
				writeUpdateJSON(filepath.Join(dir, portableJournal), p)
				if e := applyPortableFiles(p); e != nil {
					t.Fatal(e)
				}
				cmd := exec.Command(p.Executable)
				cmd.Dir = dir
				prepareUpdateCommand(cmd)
				if e := cmd.Start(); e != nil {
					t.Fatal(e)
				}
				if e := cmd.Wait(); e != nil {
					out, _ := os.ReadFile(filepath.Join(dir, "fixture-error"))
					t.Fatalf("recovery startup: %v; %s", e, out)
				}
			} else {
				if e := u.Start(); e != nil {
					t.Fatal(e)
				}
				if b, _ := os.ReadFile(p.Executable); !bytes.Equal(b, original) {
					t.Fatal("replaced before old process exited")
				}
				old.Process.Kill()
				old.Wait()
			}
			deadline := time.Now().Add(20 * time.Second)
			var child int
			for time.Now().Before(deadline) {
				if b, e := os.ReadFile(filepath.Join(dir, "child-started")); e == nil {
					child, _ = strconv.Atoi(string(b))
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			if child == 0 {
				out, _ := os.ReadFile(filepath.Join(dir, "fixture-error"))
				t.Fatalf("no restarted process: %s", out)
			}
			process, e := os.FindProcess(child)
			if e != nil {
				t.Fatal(e)
			}
			defer process.Release()
			defer func() { process.Kill(); process.Wait() }()
			for time.Now().Before(deadline) {
				if _, e := os.Stat(filepath.Join(dir, portableJournal)); os.IsNotExist(e) {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			if _, e := os.Stat(filepath.Join(dir, portableJournal)); !os.IsNotExist(e) {
				t.Fatal("journal not completed")
			}
			want := original
			if scenario == "success" {
				want = incoming
			}
			if b, _ := os.ReadFile(p.Executable); !bytes.Equal(b, want) {
				result, _ := os.ReadFile(filepath.Join(dir, portableResult))
				out, _ := os.ReadFile(filepath.Join(dir, "fixture-error"))
				t.Fatalf("wrong installed executable; result: %s; fixture: %s", result, out)
			}
			if b, _ := os.ReadFile(filepath.Join(dir, "config.json")); string(b) != "synthetic config" {
				t.Fatal("configuration changed")
			}
			// Wait for the detached helper to exit before TempDir cleanup on Windows.
			time.Sleep(150 * time.Millisecond)
		})
	}
}

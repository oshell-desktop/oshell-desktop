package core

import (
	"context"
	"encoding/json"
	"github.com/pkg/sftp"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestP2E03SecureCRTEmptyDirectories(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"empty", "nested/child", "parent"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "parent", "host.ini"), []byte("S:\"Protocol Name\"=SSH2\nS:\"Hostname\"=fixture.invalid\nS:\"Username\"=tester\n"), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := ReadThirdParty(context.Background(), []string{root}, "SecureCRT")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"SecureCRT/empty", "SecureCRT/nested", "SecureCRT/nested/child", "SecureCRT/parent"} {
		if !slices.Contains(report.Directories, dir) {
			t.Fatal("lost directory", dir, report.Directories)
		}
	}
	archive := report.Archive()
	if len(archive.Profiles) != 1 || len(archive.Directories) != 4 {
		t.Fatal(archive)
	}
	merged, _, err := archive.Merge(context.Background(), DefaultConfig(), "Imported", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range archive.Directories {
		if !slices.Contains(merged.Directories, "Imported/"+dir) {
			t.Fatal("merge dropped directory", dir)
		}
	}
	for _, nested := range []string{"empty", "one/two"} {
		base := t.TempDir()
		if err := os.MkdirAll(filepath.Join(base, nested), 0700); err != nil {
			t.Fatal(err)
		}
		empty, err := ReadThirdParty(context.Background(), []string{base}, "SecureCRT")
		if err != nil || len(empty.Profiles) != 0 || !slices.Contains(empty.Directories, "SecureCRT/"+nested) {
			t.Fatal("empty-only import", empty, err)
		}
	}
	data, err := json.Marshal(archive)
	if err != nil {
		t.Fatal(err)
	}
	var round PortableArchive
	if err = json.Unmarshal(data, &round); err != nil || !slices.Equal(archive.Directories, round.Directories) {
		t.Fatal("archive lost empty directories", err)
	}
}

func TestP2F04SFTPTopLevelAndChildSymlinks(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("synthetic link data"), 0600); err != nil {
		t.Fatal(err)
	}
	top := filepath.Join(root, "top-link")
	if err := os.Symlink(target, top); err != nil {
		t.Skipf("native symlink environment unavailable: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(source, "child-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "normal.txt"), []byte("normal"), 0600); err != nil {
		t.Fatal(err)
	}
	server := startSSH(t, "test")
	conn, err := Dial(context.Background(), testProfile(server.address), Credentials{Password: "test"}, testTrust(t), acceptHost)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client, err := sftp.NewClient(conn.Client)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	downloaded := filepath.Join(t.TempDir(), "download.txt")
	if err = Download(context.Background(), client, top, downloaded, false, nil); err != nil {
		t.Fatal("top STAT", err)
	}
	if data, _ := os.ReadFile(downloaded); string(data) != "synthetic link data" {
		t.Fatal("wrong linked data")
	}
	tree := filepath.Join(t.TempDir(), "tree")
	if err = Download(context.Background(), client, source, tree, false, nil); err != nil {
		t.Fatal("child link stopped batch", err)
	}
	if data, _ := os.ReadFile(filepath.Join(tree, "normal.txt")); string(data) != "normal" {
		t.Fatal("normal item missing")
	}
	if _, err = os.Lstat(filepath.Join(tree, "child-link")); !os.IsNotExist(err) {
		t.Fatal("child link followed", err)
	}
	if err = Upload(context.Background(), client, top, filepath.Join(root, "uploaded"), nil); err == nil {
		t.Fatal("local upload link followed")
	}
}
func TestP2F04TransferFiniteLimits(t *testing.T) {
	s := transferState{ctx: context.Background()}
	if err := s.check(32); err != nil {
		t.Fatal(err)
	}
	if err := s.check(33); err == nil {
		t.Fatal("depth 33 accepted")
	}
	s.visited = 9999
	if err := s.check(0); err != nil {
		t.Fatal(err)
	}
	if err := s.check(0); err == nil {
		t.Fatal("item 10001 accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.ctx = ctx
	if err := s.check(0); err == nil {
		t.Fatal("cancel ignored")
	}
}
func TestP2O07StoresMoreThanRenderingLimit(t *testing.T) {
	cfg := DefaultConfig()
	set := HighlightSet{ID: NewID(), Name: "fixture"}
	for i := 0; i < 33; i++ {
		set.Rules = append(set.Rules, HighlightRule{ID: NewID(), Pattern: "fixture", Color: "#ff0000", Enabled: i == 32})
	}
	if err := set.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.HighlightSets = []HighlightSet{set}
	cfg.Settings.HighlightSetID = set.ID
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var round Config
	if err = json.Unmarshal(data, &round); err != nil {
		t.Fatal(err)
	}
	if len(round.HighlightSets[0].Rules) != 33 || len(round.ActiveHighlightRules("fixture")) != 0 {
		t.Fatal("stored rules truncated or rule 33 rendered")
	}
}

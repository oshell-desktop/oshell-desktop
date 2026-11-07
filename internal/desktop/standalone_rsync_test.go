package desktop

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestStandaloneRsyncConfirmationAndMissingToolWithoutSFTP(t *testing.T) {
	a, p := connectedFileWorkflow(t)
	p.files.client.Close()
	p.files.lease()
	p.files.client, p.files.lease = nil, nil
	t.Setenv("PATH", t.TempDir())
	a.openStandaloneRsync(p)
	d := a.features.rsyncDraft
	if d == nil || !a.modalActive() {
		t.Fatal("directory transfer incorrectly requires SFTP")
	}
	d.local, d.remote = t.TempDir(), "/target"
	tt := ui.NewTester(a.fileWindowView, 1080, 720)
	if err := tt.Click("增量上传"); err != nil {
		t.Fatal(err)
	}
	if d.err != "请先确认增量更新规则" || p.files.busy {
		t.Fatal("overwrite confirmation bypassed")
	}
	if err := tt.Click("确认 rsync 增量更新"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("增量上传"); err != nil {
		t.Fatal(err)
	}
	waitOperator(t, a, func() bool { return !p.files.busy })
	records := p.files.records.snapshot()
	if len(records) != 1 || records[0].Protocol != "rsync" || records[0].Status != "失败" || records[0].Resume || !strings.Contains(records[0].Error, "本机缺少 rsync") {
		t.Fatal("incorrect missing-tool result", records)
	}
	if p.files.client != nil || !p.conn.Alive() {
		t.Fatal("failure changed SSH connection or opened SFTP")
	}
	a.openStandaloneRsync(p)
	tt.Frame()
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if a.features.rsyncDraft != nil || p.files.busy {
		t.Fatal("cancel retained draft")
	}
}

func TestRsyncBatchRetainsSameNamesAndDoesNotOfferSFTPResume(t *testing.T) {
	log := &transferLog{}
	client := &recordedFileSystem{RemoteFileSystem: &standaloneRsyncFileSystem{connection: &core.Connection{}}, log: log, task: &fileTransfer{}, protocol: "rsync"}
	dir := t.TempDir()
	if err := uploadUnique(context.Background(), client, []string{dir}, "/target", nil); err == nil {
		t.Fatal("disconnected upload accepted")
	}
	client.finish()
	records := log.snapshot()
	if len(records) != 1 || records[0].Target != "/target/"+filepath.Base(dir) || records[0].Status != "失败" || records[0].Resume {
		t.Fatal(records)
	}
	client.batch = nil
	if err := downloadBatch(context.Background(), client, []string{"folder"}, "/remote", dir, nil); err == nil {
		t.Fatal("disconnected download accepted")
	}
	client.finish()
	records = log.snapshot()
	if len(records) != 2 || records[1].Target != filepath.Join(dir, "folder") || records[1].Status != "失败" || records[1].Resume {
		t.Fatal(records)
	}
	if err := uploadUnique(context.Background(), client, []string{dir, dir}, "/target", nil); err == nil {
		t.Fatal("duplicate batch names accepted")
	}
	if len(log.snapshot()) != 2 {
		t.Fatal("invalid batch started a transfer")
	}
}

func TestStandaloneRsyncRemotePathInput(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		upload            bool
	}{
		{"upload pasted absolute path", " \t/data/ndpts-ctc/ \t", "/data/ndpts-ctc/", true},
		{"download pasted absolute path", " /data/ndpts-ctc/ ", "/data/ndpts-ctc/", false},
		{"preserve interior spaces and quotes", "  ./dir 'quoted' 中文/  ", "./dir 'quoted' 中文/", true},
		{"root directory", " / ", "/", true},
		{"empty input", " \t ", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, p := connectedFileWorkflow(t)
			t.Setenv("PATH", t.TempDir())
			a.openStandaloneRsync(p)
			d := a.features.rsyncDraft
			d.local, d.remote, d.confirmed = t.TempDir(), tc.input, true
			tt := ui.NewTester(a.fileWindowView, 1080, 720)
			button := "增量上传"
			if !tc.upload {
				button = "增量下载"
			}
			if err := tt.Click(button); err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if d.err != "请指定本机和远程目录" || p.files.busy || a.features.rsyncDraft != d {
					t.Fatal("empty path started a transfer")
				}
				return
			}
			waitOperator(t, a, func() bool { return !p.files.busy })
			records := p.files.records.snapshot()
			if len(records) != 1 || !strings.Contains(records[0].Error, "本机缺少 rsync") {
				t.Fatal("expected missing-tool fixture result", records)
			}
			got := records[0].Target
			if !tc.upload {
				got = records[0].Source
			}
			if got != tc.want {
				t.Fatalf("transfer received remote path %q, want %q", got, tc.want)
			}
		})
	}
}

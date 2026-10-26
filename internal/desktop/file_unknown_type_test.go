package desktop

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

type unknownListingInfo struct{ iconFileInfo }

func (unknownListingInfo) TypeKnown() bool        { return false }
func (unknownListingInfo) PermissionsKnown() bool { return false }

type unknownListingClient struct{ core.RemoteFileSystem }

func (*unknownListingClient) RealPath(p string) (string, error) {
	if p == "/folder.zip" || p == "/" {
		return p, nil
	}
	return "", fmt.Errorf("not a directory")
}
func (*unknownListingClient) ReadDir(p string) ([]os.FileInfo, error) {
	if p == "/folder.zip" {
		return nil, nil
	}
	return []os.FileInfo{unknownListingInfo{iconFileInfo{"folder.zip", 0}}, unknownListingInfo{iconFileInfo{"file.txt", 0}}}, nil
}
func (*unknownListingClient) Close() error { return nil }

func TestUnknownFileTypeDisplaysAndNavigatesWithoutInventedMetadata(t *testing.T) {
	for _, action := range []string{"enter", "double-click", "context"} {
		t.Run(action, func(t *testing.T) {
			a := parityApp(t)
			client := &unknownListingClient{}
			entries, _ := client.ReadDir("/")
			p := &pane{ftp: &core.FTPSession{}, profile: core.Profile{Kind: "ftp"}, files: &fileState{dir: "/", input: "/", client: client, entries: entries}}
			p.owner.Store(a)
			t.Cleanup(func() { p.ftp.Close() })
			view := ui.NewTester(func(c *ui.Context) { a.fileBrowser(c, p, true) }, 1050, 600)
			if !view.HasText("未知") || presentFile(entries[0]).kind != "未知" {
				t.Fatal("extension overrode unknown server type")
			}
			hint := fileAttributeHint(entries[0], "未知")
			if strings.Contains(hint, "2048 字节") || !strings.Contains(hint, "未提供") {
				t.Fatal("invented size", hint)
			}
			if err := view.Click("folder.zip"); err != nil {
				t.Fatal(err)
			}
			switch action {
			case "enter":
				view.Key(0, ui.KeyEnter)
			case "double-click":
				if err := view.Click("folder.zip"); err != nil {
					t.Fatal(err)
				}
			case "context":
				if err := view.RightClick("folder.zip"); err != nil {
					t.Fatal(err)
				}
				if err := view.ChooseMenuItem("进入目录"); err != nil {
					t.Fatal(err)
				}
			}
			waitOperator(t, a, func() bool { return !p.files.busy })
			if p.files.dir != "/folder.zip" {
				t.Fatal("unknown directory not entered", p.files.status)
			}
			a.loadFiles(p, "/")
			waitOperator(t, a, func() bool { return !p.files.busy })
			a.loadFiles(p, "/file.txt")
			waitOperator(t, a, func() bool { return !p.files.busy })
			if p.files.dir != "/" || !p.files.statusError || len(p.files.entries) != 2 {
				t.Fatal("failed probe discarded original list")
			}
		})
	}
}

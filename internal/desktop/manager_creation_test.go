package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"testing"
)

func TestManagerNewDirectoryContextCreatesSession(t *testing.T) {
	for _, kind := range []string{"SSH", "SFTP", "FTP"} {
		t.Run(kind, func(t *testing.T) {
			store := &core.Store{Dir: t.TempDir()}
			cfg := core.DefaultConfig()
			if err := store.Save(cfg); err != nil {
				t.Fatal(err)
			}
			a := newApp(store, cfg)
			main := ui.NewTester(a.view, 1280, 820)
			a.openDirectory("新建目录", "")
			main.Frame()
			a.features.directoryName = "新目录"
			a.applyDirectory()
			main.Frame()
			if a.features.directoryOpen {
				t.Fatal(a.features.directoryError)
			}
			a.navigateManager("新目录")
			tt := ui.NewTester(a.managerView, 900, 580)
			tt.RightClickAt(450, 300)
			t.Log(tt.Menu())
			if err := tt.ChooseMenuItem("新建 " + kind + " 会话…"); err != nil {
				t.Fatal(err)
			}
			if !a.editing || a.draft.Group != "新目录" {
				t.Fatalf("editor=%v group=%q", a.editing, a.draft.Group)
			}
			main.Frame()
			if !a.editing || !main.HasText("会话属性") {
				t.Fatalf("editor invisible after main frame: editing=%v texts=%v", a.editing, main.Texts())
			}
		})
	}
}

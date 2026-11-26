package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestClientAliasSettingsDraft(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.prepareSettings()
	tt := ui.NewTester(a.settingsView, 900, 700)
	if err := tt.Click("堡垒机客户端"); err != nil {
		t.Fatal(err)
	}
	before := a.clientAliases.want
	if err := tt.Click("接管 putty.exe"); err != nil {
		t.Fatal(err)
	}
	if a.clientAliases.want[0] == before[0] || a.clientAliases.want[1] != before[1] {
		t.Fatal("client switches are not independent")
	}
	if err := tt.Click("接管 xshell.exe"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	a.prepareSettings()
	if a.clientAliases.want != before {
		t.Fatal("cancel modified Windows registrations")
	}
}

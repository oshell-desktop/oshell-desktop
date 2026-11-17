package desktop

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestMCPQuickAuthorizationRemembersClientAndCancel(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		t.Run(mode, func(t *testing.T) {
			cfg := core.DefaultConfig()
			cfg.Settings.InterfaceTheme = mode
			a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
			p, _ := operatorPane(t, a, "target")
			p.agent = a.agent.Register(p.id, "target")
			p.agent.Bind(p.term)
			defer a.agent.Close()
			clients, err := core.OpenMCPClients(a.store.Dir)
			if err != nil {
				t.Fatal(err)
			}
			clients.Add("first")
			remembered, _, err := clients.Add("remembered")
			if err != nil {
				t.Fatal(err)
			}
			a.mcp.clients = clients
			a.localUI.LastMCPClient = remembered.ID
			server, err := core.StartMCPServer(a.store.Dir, clients, a.agent.Call, a.agent.Disconnected)
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			a.mcp.server = server
			a.quickAuthorizeAgent(p)
			view := ui.NewTester(a.view, 1000, 700)
			if !a.mcp.quickOpen || a.mcp.quickClient != remembered.ID || a.mcp.quickPermission != "assist" || !view.HasText("将此会话交给 Agent") {
				t.Fatal(view.Texts())
			}
			snapshot(t, view, "mcp-quick-"+mode+".png")
			if err := view.Click("取消"); err != nil {
				t.Fatal(err)
			}
			if a.mcp.quickOpen || p.agent.State().Authorized || p.agent.State().Audit != "" {
				t.Fatal("cancel granted access")
			}
		})
	}
}

func TestMCPPanelPairingLayoutsAndSharedConfigIsolation(t *testing.T) {
	for _, width := range []int{1360, 880} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
			before, _ := json.Marshal(a.config)
			a.showMCP()
			tt := ui.NewTester(a.view, width, 820)
			deadline := time.Now().Add(2 * time.Second)
			for a.mcp.busy && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
				tt.Frame()
			}
			if a.mcpServer() != nil {
				t.Fatal("service started automatically")
			}
			if !tt.HasText("MCP 控制中心") {
				t.Fatal(tt.Texts())
			}
			if a.mcp.sidebarShown != (width >= 1200) {
				t.Fatal("responsive panel mode")
			}
			if err := tt.Click("开启本机 MCP 服务"); err != nil {
				t.Fatal(err)
			}
			if a.mcpServer() == nil {
				t.Fatal(a.mcp.error)
			}
			defer a.mcpServer().Close()
			defer a.agent.Close()
			a.mcp.name = "fixture-client"
			tt.Frame()
			if err := tt.Click("添加客户端"); err != nil {
				t.Fatal(err)
			}
			if len(a.mcpClients().List()) != 1 || !strings.Contains(a.mcp.configuration, "--mcp-stdio") {
				t.Fatal("pairing/config missing", a.mcp.error)
			}
			if err := tt.Click("复制接入配置"); err != nil {
				t.Fatal(err)
			}
			if tt.Clipboard() != a.mcp.configuration {
				t.Fatal("clipboard missing config")
			}
			if err := tt.Click("已保存，隐藏凭据"); err != nil {
				t.Fatal(err)
			}
			after, _ := json.Marshal(a.config)
			if string(before) != string(after) {
				t.Fatal("MCP changed shared configuration")
			}
			if width >= 1200 {
				snapshot(t, tt, "mcp-sidebar.png")
			} else {
				snapshot(t, tt, "mcp-narrow-panel.png")
			}
			if err := tt.Click("关闭面板"); err != nil {
				t.Fatal(err)
			}
			if a.mcp.open || a.mcp.configuration != "" {
				t.Fatal("panel retained credential")
			}
		})
	}
}

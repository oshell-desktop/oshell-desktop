package desktop

import (
	"fmt"
	"os"
	"path/filepath"
)

// Drops target the pane under the pointer, independent of keyboard focus and
// broadcast scope. File-browser drops retain their existing SFTP/FTP workflow.
func (a *App) uploadTerminalDrop(p *pane, paths []string) {
	if len(paths) == 0 || p == nil || p.closed || p.owner.Load() != a || a.workspaceLocked() || a.passwordLocked() || a.workspaceMenuBlocked() || a.settingsOpen {
		return
	}
	if p.transfer.active {
		a.status = "当前终端正在传输，请完成或取消后再拖入文件"
		return
	}
	fail := func(err error) {
		p.transfer = transferNoticeState{visible: true, upload: true, status: "ZMODEM · " + err.Error()}
	}
	if p.term == nil || (p.profile.Kind != "ssh" && !(p.profile.Kind == "sftp" && p.terminalOnly)) || !p.term.ManagedInputAvailable() || p.term.InLocalToolsMode() {
		fail(fmt.Errorf("请将文件拖入已连接的 SSH 终端"))
		return
	}
	// Validate the entire batch before sending rz. Names never enter the shell
	// command; the existing ZMODEM sender opens and revalidates each file.
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			fail(fmt.Errorf("无法读取上传文件 %s", filepath.Base(path)))
			return
		}
		if !info.Mode().IsRegular() {
			fail(fmt.Errorf("rz 只支持普通文件，不能上传目录：%s", info.Name()))
			return
		}
	}
	if err := p.term.StartUpload(paths); err != nil {
		fail(err)
		return
	}
	p.transfer = transferNoticeState{active: true, visible: true, upload: true, status: "ZMODEM · 等待远端 rz 响应"}
}

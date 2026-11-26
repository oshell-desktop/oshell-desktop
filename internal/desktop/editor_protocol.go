package desktop

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"oshell-desktop/internal/core"
)

const bastionSSH = "bastion-ssh"

func (a *App) sessionType(p core.Profile) string {
	if p.Kind == "ssh" && !a.localUI.PlainSSH[strings.ToLower(p.ID)] {
		if _, ok := p.BastionTarget(); ok {
			return bastionSSH
		}
	}
	return p.Kind
}

// The editor presents a routed SSH session as a distinct type. Shared profiles
// and SSH resolution both use the same fully composed SSH username.
func (a *App) editorLogin() (string, error) {
	login := strings.TrimSpace(a.draft.Username)
	if a.features.editorChoice != bastionSSH {
		return login, nil
	}
	f := &a.features.sessionPassword
	_, embedded, ok := core.SplitBastionLogin(login)
	if login != f.lastQuickLogin {
		_, previous, _ := core.SplitBastionLogin(f.lastQuickLogin)
		if embedded != "" || previous != "" {
			f.serverAccount = embedded
		}
		f.lastQuickLogin = login
	}
	if !ok {
		return "", fmt.Errorf("快速登录字符串格式应为 堡垒机账号#编码#资产#SSH#服务器账号，最后的账号也可单独填写")
	}
	account := strings.TrimSpace(f.serverAccount)
	if strings.Contains(account, "#") || strings.ContainsFunc(account, unicode.IsControl) {
		return "", fmt.Errorf("请填写有效的服务器登录账号")
	}
	if embedded != account {
		a.draft.Username = strings.TrimSuffix(login, embedded) + account
		f.lastQuickLogin = a.draft.Username
	}
	if account == "" {
		return "", fmt.Errorf("请填写服务器登录账号")
	}
	return f.lastQuickLogin, nil
}

func editorProtocolPages(kind string) ([]string, []int) {
	if kind == "ftp" {
		return []string{"连接"}, []int{0}
	}
	if kind == "sftp" {
		return []string{"连接", "代理 / 跳板机", "保持活动"}, []int{0, 1, 3}
	}
	return []string{"连接", "代理 / 跳板机", "端口转发", "保持活动"}, []int{0, 1, 2, 3}
}
func (a *App) syncEditorProtocol() {
	f := &a.features
	previous := f.editorKind
	kind := a.draft.Kind
	if previous == "" || previous == kind {
		f.editorKind = kind
		return
	}
	if f.sessionPassword.original == nil {
		before := a.config.SessionDefaults.MakeProfile(previous, "")
		after := a.config.SessionDefaults.MakeProfile(kind, "")
		if a.port == strconv.Itoa(before.Port) {
			a.port = strconv.Itoa(after.Port)
		}
		if a.draft.Username == before.Username {
			a.draft.Username = after.Username
		}
		if a.draft.InitialDirectory == before.InitialDirectory {
			a.draft.InitialDirectory = after.InitialDirectory
		}
	} else {
		before, after := 22, 22
		if previous == "ftp" {
			before = 21
		}
		if kind == "ftp" {
			after = 21
		}
		if a.port == strconv.Itoa(before) {
			a.port = strconv.Itoa(after)
		}
		if kind == "ftp" && a.draft.InitialDirectory == "." {
			a.draft.InitialDirectory = "/"
		}
	}
	f.editorKind = kind
	f.editTab = 0
}

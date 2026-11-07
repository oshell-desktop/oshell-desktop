package core

import (
	"bytes"
	"errors"
	"io"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

var errSSOTool = errors.New("无法读取 SsoTool 的临时 SSH 会话或校验登录票据，请从堡垒机网页重新打开资产")

func ssoToolPath(args []string) string {
	if !launchArgLimit(args, 48) || !filepath.IsAbs(args[0]) {
		return ""
	}
	// SsoTool calls Process.Start(exe, path) without quoting the path.
	name := strings.Join(args, " ")
	base := strings.ToLower(filepath.Base(name))
	if strings.HasPrefix(base, "cbh_ssh_") && strings.HasSuffix(base, ".xsh") {
		return name
	}
	return ""
}

func readSSOToolSession(name string) (TerminalLaunchRequest, error) {
	fail := func() (TerminalLaunchRequest, error) { return TerminalLaunchRequest{}, errSSOTool }
	if len(filepath.VolumeName(name)) != 2 || filepath.VolumeName(name)[1] != ':' || strings.ContainsRune(name[2:], ':') {
		return fail()
	}
	f, err := openImportFile(name)
	if err != nil {
		return fail()
	}
	defer f.Close()
	const limit = 32 << 10
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return fail()
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	defer clear(data)
	if err != nil || len(data) > limit {
		return fail()
	}
	return parseSSOToolSession(data)
}

func parseSSOToolSession(data []byte) (TerminalLaunchRequest, error) {
	var zero TerminalLaunchRequest
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return zero, errSSOTool
	}
	section := ""
	fields := map[string]string{}
	for _, line := range strings.Split(strings.TrimPrefix(string(data), "\uFEFF"), "\n") {
		line = strings.TrimSpace(line)
		switch line {
		case "":
			continue
		case "[SessionInfo]", "[CONNECTION]", "[CONNECTION:AUTHENTICATION]":
			section = line
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		field := section + key
		switch field {
		case "[SessionInfo]Version", "[CONNECTION]Protocol", "[CONNECTION]Host", "[CONNECTION]Port", "[CONNECTION:AUTHENTICATION]UserName", "[CONNECTION:AUTHENTICATION]Password":
		default:
			return zero, errSSOTool
		}
		if !ok || value == "" || fields[field] != "" {
			return zero, errSSOTool
		}
		fields[field] = value
	}
	if fields["[SessionInfo]Version"] != "6.0" || fields["[CONNECTION]Protocol"] != "SSH" || fields["[CONNECTION:AUTHENTICATION]UserName"] != "#sso" {
		return zero, errSSOTool
	}
	port, err := strconv.Atoi(fields["[CONNECTION]Port"])
	host := fields["[CONNECTION]Host"]
	if err != nil || !validPort(port) || !validHost(host) {
		return zero, errSSOTool
	}
	account, err := user.Current()
	if err != nil {
		return zero, errSSOTool
	}
	login := account.Username[strings.LastIndexByte(account.Username, '\\')+1:]
	if login == "" || !strings.HasPrefix(account.Uid, "S-") {
		return zero, errSSOTool
	}
	master, err := ssoANSI([]byte(login + account.Uid))
	if err != nil {
		return zero, errSSOTool
	}
	defer clear(master)
	ticket, err := (XshellPassword{encoded: fields["[CONNECTION:AUTHENTICATION]Password"]}).decrypt(master, ssoANSI)
	if err != nil {
		return zero, errSSOTool
	}
	r := TerminalLaunchRequest{Version: 1, ID: NewID(), Host: host, Port: port, Username: "#sso", Password: &ticket, TerminalType: "xterm", BastionSSH: true}
	if _, err = r.ProfileValue(); err != nil {
		return zero, errSSOTool
	}
	return r, nil
}

// .NET Framework Encoding.Default in SsoTool uses the system ANSI code page.
// OShell's UTF-8 manifest can give GetACP a different process-local value.
var localeInfoW = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetLocaleInfoW")
var wideToMultiByte = windows.NewLazySystemDLL("kernel32.dll").NewProc("WideCharToMultiByte")

func ssoANSI(s []byte) ([]byte, error) {
	if !utf8.Valid(s) {
		return nil, errSSOTool
	}
	ascii := true
	for _, b := range s {
		if b >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return s, nil
	}
	var cp uint32
	// LOCALE_SYSTEM_DEFAULT, LOCALE_IDEFAULTANSICODEPAGE | LOCALE_RETURN_NUMBER.
	if n, _, _ := localeInfoW.Call(0x0800, 0x1004|0x20000000, uintptr(unsafe.Pointer(&cp)), 2); n == 0 {
		return nil, errSSOTool
	}
	wide := utf16.Encode([]rune(string(s)))
	defer clear(wide)
	out := make([]byte, 4*len(wide))
	n, _, _ := wideToMultiByte.Call(uintptr(cp), 0, uintptr(unsafe.Pointer(&wide[0])), uintptr(len(wide)), uintptr(unsafe.Pointer(&out[0])), uintptr(len(out)), 0, 0)
	if n == 0 {
		return nil, errSSOTool
	}
	return out[:n], nil
}

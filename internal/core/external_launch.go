// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

type TerminalLaunchRequest struct {
	Version      int     `json:"version"`
	ID           string  `json:"id"`
	Host         string  `json:"host"`
	Port         int     `json:"port"`
	Username     string  `json:"username"`
	Password     *string `json:"password,omitempty"`
	KeyFile      string  `json:"keyFile"`
	Title        string  `json:"title"`
	BastionSSH   bool    `json:"bastionSSH,omitempty"`
	TerminalType string  `json:"terminalType"`
}
type FileLaunchRequest struct {
	Version  int     `json:"version"`
	ID       string  `json:"id"`
	Profile  Profile `json:"profile"`
	Password *string `json:"password,omitempty"`
}
type ExternalLaunchRequest struct {
	Terminal *TerminalLaunchRequest `json:"terminal,omitempty"`
	File     *FileLaunchRequest     `json:"file,omitempty"`
}

func validLaunchPassword(password *string) bool {
	return password == nil || len(*password) <= 4096 && !strings.ContainsAny(*password, "\x00\r\n")
}
func (r TerminalLaunchRequest) ProfileValue() (Profile, error) {
	p := NewProfile()
	p.Name = "SSH · " + r.Host
	if r.BastionSSH && r.Title != "" {
		p.Name = r.Title
	}
	p.Group = ""
	p.Host = r.Host
	p.Port = r.Port
	p.Username = r.Username
	p.IdentityFile = r.KeyFile
	p.QuickConnect = false
	p.RuntimeTerminalType = r.TerminalType
	if r.BastionSSH {
		host := strings.ToLower(strings.Trim(r.Host, "[]"))
		if ip := net.ParseIP(host); ip != nil {
			host = ip.String()
		}
		p.RuntimeBastionEndpoint = net.JoinHostPort(host, strconv.Itoa(r.Port))
	}
	if r.Version != 1 || !uuidText.MatchString(r.ID) || len(r.Host) > 253 || len(r.Title) > 2048 || len(r.Username) > 1024 || len(r.KeyFile) > 4096 || strings.ContainsFunc(r.Title, unicode.IsControl) || strings.ContainsFunc(r.KeyFile, unicode.IsControl) || !validLaunchPassword(r.Password) || r.Password != nil && r.Username == "" {
		return p, fmt.Errorf("ZOC 连接参数无效或过长")
	}
	switch r.TerminalType {
	case "xterm", "vt100", "linux":
	default:
		return p, fmt.Errorf("不支持的终端类型")
	}
	return p, p.Validate()
}
func (r FileLaunchRequest) Validate() error {
	if r.Version != 1 || !uuidText.MatchString(r.ID) || (r.Profile.Kind != "ftp" && r.Profile.Kind != "sftp") || !validLaunchPassword(r.Password) {
		return fmt.Errorf("文件启动请求无效")
	}
	if strings.ContainsAny(r.Profile.InitialDirectory, "\x00\r\n") {
		return fmt.Errorf("远程目录无效")
	}
	return r.Profile.Validate()
}
func (r ExternalLaunchRequest) Validate() error {
	if (r.Terminal == nil) == (r.File == nil) {
		return fmt.Errorf("启动请求类型无效")
	}
	if r.Terminal != nil {
		_, e := r.Terminal.ProfileValue()
		return e
	}
	return r.File.Validate()
}
func (r ExternalLaunchRequest) ID() string {
	if r.Terminal != nil {
		return strings.ToUpper(r.Terminal.ID)
	}
	if r.File != nil {
		return strings.ToUpper(r.File.ID)
	}
	return ""
}

// Redact formatting; request values are one-shot and must not be logged/persisted.
func (r TerminalLaunchRequest) String() string { return "<one-shot terminal launch>" }
func (r FileLaunchRequest) String() string     { return "<one-shot file launch>" }
func (r ExternalLaunchRequest) String() string { return "<one-shot external launch>" }
func launchArgLimit(args []string, count int) bool {
	n := 0
	for _, arg := range args {
		n += len(arg)
	}
	return len(args) > 0 && len(args) <= count && n <= 16384
}
func ParseZOCLaunch(args []string) (TerminalLaunchRequest, error) {
	r := TerminalLaunchRequest{Version: 1, ID: NewID(), Port: 22, TerminalType: "xterm"}
	fail := func() (TerminalLaunchRequest, error) {
		return r, fmt.Errorf("ZOC 启动参数无效；只支持连接参数，不执行脚本或配置文件")
	}
	if !launchArgLimit(args, 32) {
		return fail()
	}
	supported := map[string]bool{}
	for _, key := range []string{"DEV", "CONNECT", "SSH", "SSHUSER", "SSHPASSWORD", "SSHKEY", "TITLE", "EMU", "CHARSET", "ENCODING"} {
		supported[key] = true
	}
	values := map[string]string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if len(arg) == 0 || arg[0] != '/' && arg[0] != '-' {
			return fail()
		}
		body := strings.TrimLeft(arg, "/-")
		key, value := body, ""
		if index := strings.IndexAny(body, ":="); index >= 0 {
			key, value = body[:index], body[index+1:]
		} else {
			i++
			if i >= len(args) {
				return fail()
			}
			value = args[i]
		}
		key = strings.ToUpper(key)
		if !supported[key] {
			return fail()
		}
		if _, exists := values[key]; exists {
			return fail()
		}
		values[key] = value
	}
	destination, connect := values["CONNECT"]
	ssh, sshExists := values["SSH"]
	if connect == sshExists {
		return fail()
	}
	if sshExists {
		destination = ssh
	}
	device := strings.ToUpper(values["DEV"])
	if _, exists := values["DEV"]; !exists {
		device = "SSH"
	}
	if index := strings.IndexByte(destination, '!'); index >= 0 {
		embedded := strings.ToUpper(destination[:index])
		switch embedded {
		case "SSH", "SSH2", "SECURE SHELL", "SSH (SECURE SHELL)", "TELNET", "RLOGIN":
			if _, explicit := values["DEV"]; explicit && device != embedded {
				return fail()
			}
			device = embedded
			destination = destination[index+1:]
		}
	}
	switch device {
	case "SSH", "SSH2", "SECURE SHELL", "SSH (SECURE SHELL)":
	default:
		return fail()
	}
	encoding, exists := values["CHARSET"]
	if !exists {
		encoding, exists = values["ENCODING"]
	}
	if !exists {
		encoding = "UTF-8"
	}
	if !strings.EqualFold(encoding, "UTF-8") && !strings.EqualFold(encoding, "UTF8") {
		return fail()
	}
	if index := strings.LastIndexByte(destination, '@'); index >= 0 {
		credentials := destination[:index]
		destination = destination[index+1:]
		user, password, has := strings.Cut(credentials, ":")
		r.Username = user
		if has {
			r.Password = &password
		}
	}
	if value, ok := values["SSHUSER"]; ok {
		r.Username = value
	}
	if value, ok := values["SSHPASSWORD"]; ok {
		r.Password = &value
	}
	r.Host = destination
	if strings.HasPrefix(destination, "[") {
		close := strings.IndexByte(destination, ']')
		if close < 0 {
			return fail()
		}
		r.Host = destination[1:close]
		rest := destination[close+1:]
		if rest != "" {
			if rest[0] != ':' {
				return fail()
			}
			port, e := strconv.Atoi(rest[1:])
			if e != nil {
				return fail()
			}
			r.Port = port
		}
	} else if strings.Count(destination, ":") == 1 {
		host, port, _ := strings.Cut(destination, ":")
		r.Host = host
		n, e := strconv.Atoi(port)
		if e != nil {
			return fail()
		}
		r.Port = n
	}
	r.Title = values["TITLE"]
	r.KeyFile = values["SSHKEY"]
	if v, ok := values["EMU"]; ok {
		r.TerminalType = strings.ToLower(v)
	}
	_, e := r.ProfileValue()
	return r, e
}
func ParseFileLaunch(args []string, sites func(string) ([]byte, error)) (FileLaunchRequest, error) {
	r := FileLaunchRequest{Version: 1, ID: NewID()}
	if !launchArgLimit(args, 2) {
		return r, fmt.Errorf("FileZilla 启动参数无效")
	}
	path := ""
	if len(args) == 1 && strings.HasPrefix(args[0], "--site=") {
		path = strings.TrimPrefix(args[0], "--site=")
	} else if len(args) == 2 && (args[0] == "--site" || args[0] == "-c") {
		path = args[1]
	}
	if path != "" {
		if sites == nil {
			return r, fmt.Errorf("站点读取器不可用")
		}
		if _, e := FileZillaSitePath(path); e != nil {
			return r, e
		}
		data, e := sites(path)
		if e != nil {
			return r, e
		}
		return ParseFileZillaSite(path, data)
	}
	if len(args) != 1 {
		return r, fmt.Errorf("不支持的 FileZilla 参数")
	}
	u, e := url.Parse(args[0])
	if e == nil {
		u.Scheme = strings.ToLower(u.Scheme)
	}
	if e != nil || u.Hostname() == "" || u.RawQuery != "" || u.ForceQuery || strings.Contains(args[0], "#") || (u.Scheme != "ftp" && u.Scheme != "sftp") {
		return r, fmt.Errorf("请使用 ftp://、sftp:// 或 --site；不支持 FTPS")
	}
	p := NewProfile()
	p.Name = "文件连接"
	p.Group = ""
	p.Kind = u.Scheme
	p.Host = u.Hostname()
	p.Port = 22
	if p.Kind == "ftp" {
		p.Port = 21
	}
	p.InitialDirectory = u.Path
	if p.InitialDirectory == "" {
		p.InitialDirectory = "."
	}
	p.QuickConnect = false
	if port := u.Port(); port != "" {
		p.Port, e = strconv.Atoi(port)
		if e != nil {
			return r, fmt.Errorf("端口无效")
		}
	}
	if u.User != nil {
		p.Username = u.User.Username()
		if password, ok := u.User.Password(); ok {
			r.Password = &password
		}
	} else if p.Kind == "ftp" {
		p.Username = "anonymous"
		password := "anonymous@"
		r.Password = &password
	}
	r.Profile = p
	return r, r.Validate()
}

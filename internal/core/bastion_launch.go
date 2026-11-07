package core

import (
	"errors"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var errBastionArgs = errors.New("堡垒机 SSH 启动参数无效。请选择 PuTTY 或 Xshell 的 SSH 连接方式。")

func IsSSOToolLaunch(args []string) bool { return ssoToolPath(args) != "" }

// The USM client templates pass a complete destination. Never execute a
// shell, script or saved client session to recover missing arguments.
func IsBastionSSHLaunch(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if ssoToolPath(args) != "" {
		return true
	}
	first := strings.ToLower(args[0])
	if strings.HasPrefix(first, "ssh://") || strings.HasPrefix(first, "ssh2://") {
		return true
	}
	switch first {
	case "-ssh":
		// Preserve the legacy ZOC -SSH host -SSHUSER ... form.
		if _, err := ParseZOCLaunch(args); err == nil {
			return false
		}
		return true
	case "-load", "-l", "-p", "-pw", "-url", "-newtab", "-newwin", "/ssh2", "/t", "/f", "/s", "/n", "/script", "/auth", "/l", "/password":
		return true
	}
	// PuTTY also permits the host before its connection options.
	return len(args) > 1 && !strings.HasPrefix(first, "-") && !strings.HasPrefix(first, "/") && (args[1] == "-P" || args[1] == "-l" || args[1] == "-ssh" || args[1] == "-pw")
}

type bastionArgs struct {
	r                           TerminalLaunchRequest
	host                        string
	port, user, title, password bool
	loadUSM                     bool
	rawURL                      bool
}

func (b *bastionArgs) option(key, value string) bool {
	switch key {
	case "host":
		if b.host != "" || value == "" {
			return false
		}
		b.host = value
	case "port":
		n, e := strconv.Atoi(value)
		if b.port || e != nil || !validPort(n) {
			return false
		}
		b.port = true
		b.r.Port = n
	case "user":
		if b.user || value == "" {
			return false
		}
		b.user = true
		b.r.Username = value
	case "password":
		if b.password {
			return false
		}
		b.password = true
		b.r.Password = &value
	case "title":
		if b.title {
			return false
		}
		b.title = true
		b.r.Title = value
	default:
		return false
	}
	return true
}

func (b *bastionArgs) destination() bool {
	raw := b.host
	if raw == "" {
		return false
	}
	if scheme, authority, ok := strings.Cut(raw, "://"); ok {
		if !strings.EqualFold(scheme, "ssh") && !strings.EqualFold(scheme, "ssh2") {
			return false
		}
		raw = authority
	}
	if at := strings.LastIndexByte(raw, '@'); at >= 0 {
		name, pass, has := strings.Cut(raw[:at], ":")
		// USM interpolates the ticket literally into Xshell's -url argument.
		// Standalone SSH URIs use the usual percent encoding.
		if !b.rawURL {
			var err error
			name, err = url.PathUnescape(name)
			if err != nil {
				return false
			}
			pass, err = url.PathUnescape(pass)
			if err != nil {
				return false
			}
		}
		if !b.option("user", name) {
			return false
		}
		if has && !b.option("password", pass) {
			return false
		}
		raw = raw[at+1:]
	}
	raw = strings.TrimSuffix(raw, "/")
	if strings.ContainsAny(raw, "/?#\\") {
		return false
	}
	if net.ParseIP(raw) != nil {
		b.r.Host = raw
		return true
	}
	u, err := url.Parse("ssh://" + raw)
	if err != nil || u.Hostname() == "" || strings.HasSuffix(raw, ":") || strings.ContainsAny(u.Hostname(), "[]") {
		return false
	}
	if strings.Contains(u.Hostname(), ":") && !strings.HasPrefix(raw, "[") {
		return false
	}
	if v := u.Port(); v != "" && !b.option("port", v) {
		return false
	}
	b.r.Host = u.Hostname()
	return validHost(b.r.Host)
}

var usmMobaSSH = regexp.MustCompile(`^sshpass -p ([^[:space:]]+) ssh -p ([0-9]+) '([^']+)'@([^[:space:]]+)$`)

func ParseBastionSSHLaunch(args []string) (TerminalLaunchRequest, error) {
	if !launchArgLimit(args, 48) {
		return TerminalLaunchRequest{}, errBastionArgs
	}
	if name := ssoToolPath(args); name != "" {
		return readSSOToolSession(name)
	}
	b := bastionArgs{r: TerminalLaunchRequest{Version: 1, ID: NewID(), Port: 22, TerminalType: "xterm", BastionSSH: true}}
	if strings.EqualFold(args[0], "-newtab") && len(args) == 2 {
		// Parse only USM's literal MobaXterm SSH template as data.
		m := usmMobaSSH.FindStringSubmatch(args[1])
		if m == nil || !b.option("password", m[1]) || !b.option("port", m[2]) || !b.option("user", m[3]) || !b.option("host", m[4]) {
			return TerminalLaunchRequest{}, errBastionArgs
		}
	} else if !b.parse(args) {
		return TerminalLaunchRequest{}, errBastionArgs
	}
	if !b.destination() {
		return TerminalLaunchRequest{}, errBastionArgs
	}
	if b.loadUSM {
		applyUSMPuTTYSettings(&b.r)
	}
	if _, err := b.r.ProfileValue(); err != nil {
		return TerminalLaunchRequest{}, errBastionArgs
	}
	return b.r, nil
}

func (b *bastionArgs) parse(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		flag := strings.ToLower(arg)
		switch flag {
		case "-ssh", "/ssh2", "/t", "-newwin":
			continue
		case "-url", "-load", "-l", "/l", "-pw", "/password", "-p", "/p", "-newtab", "/n", "/auth", "/f", "/s":
			i++
			if i >= len(args) {
				return false
			}
			value := args[i]
			switch flag {
			case "-url":
				b.rawURL = true
				if !b.option("host", value) {
					return false
				}
			case "-load":
				if b.loadUSM || !strings.EqualFold(value, "USMSSO Default") {
					return false
				}
				b.loadUSM = true
			case "-l", "/l":
				if !b.option("user", value) {
					return false
				}
			case "-pw", "/password":
				if !b.option("password", value) {
					return false
				}
			case "-p", "/p":
				if !b.option("port", value) {
					return false
				}
			case "-newtab", "/n":
				// USM appends the Xshell window title without quoting it.
				if flag == "-newtab" && b.rawURL {
					value = strings.Join(args[i:], " ")
					i = len(args)
				}
				if !b.option("title", value) {
					return false
				}
			case "/auth":
				if !strings.EqualFold(value, "password") {
					return false
				}
			case "/f":
				if value == "" {
					return false
				} // USM's SecureCRT settings directory
			case "/s":
				if !strings.HasPrefix(strings.ToLower(value), `usm\`) {
					return false
				}
			}
		default:
			if strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "/") || !b.option("host", arg) {
				return false
			}
		}
	}
	return true
}

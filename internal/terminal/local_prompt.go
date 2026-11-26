package terminal

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode"

	"oshell-desktop/internal/core"
)

type localPrompt struct {
	dir        string
	telnetMode bool
	connect    func(core.Profile)
}

func (p *localPrompt) prompt() string {
	if p.telnetMode {
		return "telnet> "
	}
	dir := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, p.dir)
	return "[" + dir + "] oshell> "
}

func localShellCommand(line string) core.LocalToolCommand {
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(strings.Trim(strings.Fields(line)[0], "\"'")), ".exe"))
	if runtime.GOOS == "windows" {
		path := os.Getenv("ComSpec")
		if path == "" {
			path = filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
		}
		if strings.EqualFold(line, "realcmd") {
			return core.LocalToolCommand{Name: "cmd", Path: path, Arguments: []string{"/d"}}
		}
		return core.LocalToolCommand{Name: name, Path: path, Arguments: []string{"/d", "/c", line}}
	}
	return core.LocalToolCommand{Name: name, Path: "/bin/sh", Arguments: []string{"-c", line}}
}

func shellExpression(line string) bool {
	var quote rune
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case strings.ContainsRune("|&;<>", r):
			return true
		}
	}
	return false
}

func (p *localPrompt) handle(c *ToolsConsole, line string) bool {
	if line == "" {
		return p.telnetMode
	}
	name := strings.ToLower(strings.Fields(line)[0])
	if p.telnetMode {
		switch name {
		case "exit", "quit", "close":
			p.telnetMode = false
		case "open":
			words, err := core.SplitLocalCommand(line)
			if err == nil {
				err = c.runTelnet(words[1:])
			}
			if err != nil {
				fmt.Fprint(c.writer, err, "\r\n")
			}
		default:
			io.WriteString(c.writer, "open 主机 [端口] 连接，quit 返回本地命令行。\r\n")
		}
		return true
	}
	switch strings.ToLower(line) {
	case "help", "?":
		io.WriteString(c.writer, "cd [目录] 切换目录，pwd 显示目录，tools 查看本机工具和内置客户端。\r\n"+
			"telnet [主机 [端口]]；ssh、sftp、ftp 在缺少本机客户端时支持 [用户@]主机 [端口]。\r\n"+
			"其他命令交给本机 shell，支持管道和重定向；↑/↓ 浏览历史，Ctrl+R 搜索，exit 关闭。\r\n")
		if runtime.GOOS == "windows" {
			io.WriteString(c.writer, "realcmd 进入持续的 CMD 会话，exit 返回本地提示符。\r\n")
		}
		return true
	case "clear", "cls":
		io.WriteString(c.writer, "\x1b[2J\x1b[H")
		return true
	case "tools":
		fmt.Fprint(c.writer, core.LocalPromptAvailability(), "\r\n")
		return true
	case "pwd":
		fmt.Fprint(c.writer, p.dir, "\r\n")
		return true
	}
	// Keep compound commands intact for the host shell.
	if shellExpression(line) {
		return false
	}
	if name != "cd" && name != "telnet" && name != "ssh" && name != "sftp" && name != "ftp" {
		return false
	}
	if name != "cd" {
		if _, err := exec.LookPath(name); err == nil {
			return false
		}
	}
	words, err := core.SplitLocalCommand(line)
	if err == nil {
		switch name {
		case "cd":
			err = p.chdir(words[1:])
		case "telnet":
			if len(words) == 1 {
				p.telnetMode = true
				io.WriteString(c.writer, "内置 Telnet · open 主机 [端口] 连接，quit 返回。\r\n")
			} else {
				err = c.runTelnet(words[1:])
			}
		default:
			err = p.openConnection(name, words[1:])
		}
	}
	if err != nil {
		fmt.Fprint(c.writer, err, "\r\n")
	}
	return true
}

func (p *localPrompt) chdir(args []string) error {
	if runtime.GOOS == "windows" && len(args) > 0 && strings.EqualFold(args[0], "/d") {
		args = args[1:]
	}
	if len(args) > 1 {
		return fmt.Errorf("用法：cd \"目录路径\"")
	}
	dir, err := os.UserHomeDir()
	if len(args) == 1 && strings.HasPrefix(filepath.ToSlash(args[0]), "~/") {
		dir = filepath.Join(dir, args[0][2:])
	} else if len(args) == 1 && args[0] != "~" {
		dir, err = args[0], nil
		if runtime.GOOS == "windows" && filepath.VolumeName(dir) == "" && strings.HasPrefix(filepath.ToSlash(dir), "/") {
			dir = filepath.VolumeName(p.dir) + dir
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(p.dir, dir)
		}
	}
	if err != nil {
		return err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s 不是目录", dir)
	}
	p.dir = filepath.Clean(dir)
	return nil
}

func (p *localPrompt) openConnection(kind string, args []string) error {
	profile := core.NewProfile()
	profile.Kind = kind
	if kind == "ftp" {
		profile.Port = 21
		profile.Username = "anonymous"
	}
	if len(args) == 3 && (args[0] == "-p" || args[0] == "-P") {
		args = []string{args[2], args[1]}
	}
	if len(args) < 1 || len(args) > 2 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("用法：%s [用户@]主机 [端口]", kind)
	}
	host := args[0]
	if user, h, ok := strings.Cut(host, "@"); ok {
		profile.Username = user
		host = h
	}
	profile.Host = strings.Trim(host, "[]")
	profile.Name = kind + " · " + profile.Host
	if len(args) == 2 {
		profile.Port, _ = strconv.Atoi(args[1])
	}
	if err := profile.Validate(); err != nil {
		return err
	}
	if p.connect == nil {
		return fmt.Errorf("无法打开工作区连接")
	}
	p.connect(profile)
	return nil
}

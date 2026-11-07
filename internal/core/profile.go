// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"crypto/rand"
	"encoding/json"

	"fmt"
	"net"
	"strings"
	"unicode"
)

type Profile struct {
	RuntimeTerminalType    string          `json:"-"`
	RuntimeBastionEndpoint string          `json:"-"`
	ProxyID                string          `json:"proxyID,omitempty"`
	RuntimeProxyRoute      []ProxySettings `json:"-"`
	resolvedIdentityFiles  []string
	QuickConnect           bool              `json:"quickConnect"`
	TitleMode              string            `json:"titleMode"`
	ID                     string            `json:"id"`
	Name                   string            `json:"name"`
	Group                  string            `json:"group"`
	Kind                   string            `json:"kind"`
	Host                   string            `json:"host"`
	Port                   int               `json:"port"`
	Username               string            `json:"username"`
	IdentityFile           string            `json:"identityFile"`
	JumpHost               string            `json:"jumpHost"`
	Proxy                  ProxySettings     `json:"proxy"`
	Tunnels                []TunnelRule      `json:"tunnels"`
	KeepAlive              KeepAliveSettings `json:"keepAlive"`
	LegacySSH              bool              `json:"legacySSH"`
	InitialDirectory       string            `json:"initialDirectory"`

	// Local ssh -G policy only; never serialized into shared configuration.
	resolvedIdentityFilesSet bool
	resolvedIdentitiesOnly   bool
	resolvedAgentDisabled    bool
}

func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func NewProfile() Profile {
	return Profile{ID: NewID(), Name: "新会话", Group: "服务器", Kind: "ssh", Port: 22, Proxy: ProxySettings{ID: NewID(), Kind: "none", Port: 1080}, KeepAlive: DefaultKeepAlive(), InitialDirectory: ".", QuickConnect: true, TitleMode: "shellIntegration"}
}
func (p Profile) Address() string {
	return net.JoinHostPort(strings.Trim(p.Host, "[]"), fmt.Sprint(p.Port))
}
func (p Profile) Validate() error {
	switch p.TitleMode {
	case "", "shellIntegration", "activeProbe", "passive":
	default:
		return fmt.Errorf("主机识别模式无效")
	}

	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("请填写会话名称")
	}
	if len(p.Name) > 1024 || len(p.Group) > 2048 {
		return fmt.Errorf("名称或分组过长")
	}
	for _, s := range []string{p.ID, p.Name, p.Group, p.Host, p.Username, p.IdentityFile, p.InitialDirectory} {
		if strings.ContainsFunc(s, unicode.IsControl) {
			return fmt.Errorf("字段不能包含控制字符")
		}
	}
	if p.Kind != "local" && p.InitialDirectory == "" {
		return fmt.Errorf("请填写文件初始目录，主目录可填写 .")
	}
	if p.Kind == "ftp" {
		if !validHost(p.Host) || !validPort(p.Port) {
			return fmt.Errorf("FTP 主机或端口无效")
		}
		if strings.TrimSpace(p.Username) == "" {
			return fmt.Errorf("请填写 FTP 用户名")
		}
		return nil
	}
	if p.Kind == "local" {
		return nil
	}
	if p.JumpHost != "" {
		if p.ProxyID != "" {
			return fmt.Errorf("不能同时配置共享代理和旧跳板链")
		}
		if p.Proxy.Kind != "" && p.Proxy.Kind != "none" {
			return fmt.Errorf("跳板机字段与代理不能同时设置")
		}
		jumps := strings.Split(p.JumpHost, ",")
		if len(jumps) > 8 {
			return fmt.Errorf("最多支持 8 层跳板机")
		}
		for _, j := range jumps {
			if _, e := parseJump(j, p.Username); e != nil {
				return e
			}
		}
	}
	if e := p.Proxy.Validate(); e != nil {
		return e
	}
	if e := p.KeepAlive.Validate(); e != nil {
		return e
	}
	seen := map[string]bool{}
	for _, t := range p.Tunnels {
		if !t.Enabled {
			continue
		}
		if e := t.Validate(); e != nil {
			return e
		}
		side := "local"
		if t.Kind == "remote" {
			side = "remote"
		}
		key := fmt.Sprintf("%s/%s/%d", side, t.BindHost, t.ListenPort)
		if seen[key] {
			return fmt.Errorf("隧道监听地址和端口重复")
		}
		seen[key] = true
	}
	if p.Kind != "ssh" && p.Kind != "sftp" {
		return fmt.Errorf("此版本支持 SSH、SFTP 和本地终端")
	}
	if p.Host == "" || strings.ContainsAny(p.Host, " /\\\t") || strings.HasPrefix(p.Host, "-") {
		return fmt.Errorf("请填写有效主机地址")
	}
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("端口范围为 1–65535")
	}
	if p.Username != "" && (strings.HasPrefix(p.Username, "-") || strings.ContainsFunc(p.Username, unicode.IsSpace)) {
		return fmt.Errorf("SSH 用户名不能包含空白或以 - 开头")
	}
	return nil
}

func (p *Profile) UnmarshalJSON(data []byte) error {
	type plain Profile
	v := plain(NewProfile())
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)
	if _, ok := raw["initialDirectory"]; !ok && v.Kind == "ftp" {
		v.InitialDirectory = "/"
	}
	if _, ok := raw["titleMode"]; !ok {
		var legacy struct{ ActiveHostProbe bool }
		json.Unmarshal(data, &legacy)
		if legacy.ActiveHostProbe {
			v.TitleMode = "activeProbe"
		}
	}
	if v.Proxy.Kind == "shared-reference" {
		if v.ProxyID == "" {
			return fmt.Errorf("共享代理引用缺少 ID")
		}
		v.Proxy.Kind = "none"
		v.Proxy.Port = 1080
	}
	*p = Profile(v)
	return nil
}

type QuickCommand struct {
	Group        string `json:"group"`
	AppendReturn bool   `json:"appendReturn"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	Text         string `json:"text"`
}
type SessionLink struct {
	ID        string `json:"id"`
	ProfileID string `json:"profileID"`
	Name      string `json:"name"`
	Folder    string `json:"folder"`
}

type Settings struct {
	Metal                      bool              `json:"metal"`
	MasterWarningAcknowledged  bool              `json:"masterWarningAcknowledged"`
	UpdateRepository           string            `json:"updateRepository"`
	AutomaticUpdateChecks      bool              `json:"automaticUpdateChecks"`
	PreferColorEmoji           bool              `json:"preferColorEmoji"`
	AmbiguousCharactersAreWide bool              `json:"ambiguousCharactersAreWide"`
	TerminalClockEnabled       bool              `json:"terminalClockEnabled"`
	TerminalClockFontName      string            `json:"terminalClockFontName"`
	TerminalClockFontSize      float64           `json:"terminalClockFontSize"`
	TerminalClockColor         string            `json:"terminalClockColor"`
	TerminalClockOpacity       float64           `json:"terminalClockOpacity"`
	TerminalClockPosition      string            `json:"terminalClockPosition"`
	KeyboardShortcuts          KeyboardShortcuts `json:"keyboardShortcuts"`
	LinksVisible               bool              `json:"linksVisible"`
	HighlightSetID             string            `json:"highlightSetID"`
	QuickSendScope             string            `json:"quickSendScope"`
	QuickSendVisible           bool              `json:"quickSendBarVisible"`
	AutoZmodem                 bool              `json:"autoZmodem"`
	InterfaceTheme             string            `json:"interfaceTheme"`
	ColorSchemeID              string            `json:"colorSchemeID"`
	CustomSchemes              []ColorScheme     `json:"customColorSchemes"`
	Input                      InputPreferences  `json:"input"`
	Highlights                 []HighlightRule   `json:"highlights"`
	ScrollbackLines            int               `json:"scrollbackLines"`
	FontFamily                 string            `json:"fontFamily,omitempty"`
	Dark                       bool              `json:"dark"`
	FontSize                   float64           `json:"fontSize"`
	ScrollbackMB               int               `json:"scrollbackMB"`
}
type Config struct {
	Proxies           []ProxyProfile             `json:"proxies,omitempty"`
	SharedPreferences map[string]json.RawMessage `json:"sharedPreferences,omitempty"`
	LinkRootOrder     []string                   `json:"linkRootOrder,omitempty"`
	HighlightSets     []HighlightSet             `json:"highlightSets"`
	SessionDefaults   SessionDefaults            `json:"sessionDefaults"`
	Version           int                        `json:"version"`
	Profiles          []Profile                  `json:"profiles"`
	Settings          Settings                   `json:"settings"`
	Directories       []string                   `json:"directories"`
	Links             []SessionLink              `json:"links"`
	QuickCommands     []QuickCommand             `json:"quickCommands"`
}

func DefaultConfig() Config {
	return Config{HighlightSets: []HighlightSet{StandardHighlightSet()}, Version: 3, SessionDefaults: DefaultSessionDefaults(), Profiles: []Profile{}, Settings: DefaultSettings()}
}

// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"bytes"
	"context"
	"crypto/rc4"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"golang.org/x/text/encoding/simplifiedchinese"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

const MaxVendorImport = 64 << 20

type XshellPassword struct{ encoded string }

func (p XshellPassword) String() string   { return "<Xshell encrypted password>" }
func (p XshellPassword) GoString() string { return p.String() }
func (p XshellPassword) Decrypt(master string) (string, error) {
	packet, e := base64.StdEncoding.DecodeString(p.encoded)
	if e != nil || len(master) == 0 || len(master) > 4096 || len(p.encoded) > 8192 || len(packet) <= 32 || len(packet) > 4128 {
		return "", fmt.Errorf("Xshell 密码记录无效或格式不受支持")
	}
	key := sha256.Sum256([]byte(master))
	defer clear(key[:])
	cipher, e := rc4.NewCipher(key[:])
	if e != nil {
		return "", e
	}
	plain := make([]byte, len(packet)-32)
	defer clear(plain)
	cipher.XORKeyStream(plain, packet[:len(plain)])
	cipher.Reset()
	actual := sha256.Sum256(plain)
	if subtle.ConstantTimeCompare(actual[:], packet[len(plain):]) != 1 || !utf8.Valid(plain) || bytes.ContainsAny(plain, "\x00\r\n") || len(plain) == 0 {
		return "", fmt.Errorf("Xshell 主密码不正确、文件损坏或密码格式不受支持；未保存导入结果")
	}
	return string(plain), nil
}

type ImportIssue struct {
	Source, Message string
	Skipped         bool
}
type ThirdPartyReport struct {
	Format      string
	Profiles    []Profile
	Directories []string
	Issues      []ImportIssue
	Passwords   map[string]XshellPassword
}

func (r *ThirdPartyReport) Archive() PortableArchive {
	a := PortableArchive{Format: "OShell.sessions", Version: 2, Directories: r.Directories}
	for _, p := range r.Profiles {
		a.Profiles = append(a.Profiles, PortableProfile{Definition: p})
	}
	return a
}
func (r *ThirdPartyReport) issue(source, message string, skipped bool) {
	source = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, source)
	runes := []rune(source)
	if len(runes) > 240 {
		source = string(runes[:240])
	}
	r.Issues = append(r.Issues, ImportIssue{source, message, skipped})
}
func decodeVendorText(data []byte) (string, error) {
	var words []uint16
	utf16Data := len(data) >= 2 && (data[0] == 255 && data[1] == 254 || data[0] == 254 && data[1] == 255)
	little := len(data) >= 2 && data[0] == 255
	if utf16Data {
		data = data[2:]
	} else if bytes.Contains(data[:min(len(data), 128)], []byte{0}) && len(data)%2 == 0 {
		utf16Data = true
		zeros := 0
		for i := 1; i < min(len(data), 128); i += 2 {
			if data[i] == 0 {
				zeros++
			}
		}
		little = zeros > 4
	}
	var text string
	if utf16Data {
		if len(data)%2 != 0 {
			return "", fmt.Errorf("UTF-16 文本长度无效")
		}
		for i := 0; i < len(data); i += 2 {
			if little {
				words = append(words, binary.LittleEndian.Uint16(data[i:]))
			} else {
				words = append(words, binary.BigEndian.Uint16(data[i:]))
			}
		}
		text = string(utf16.Decode(words))
	} else if utf8.Valid(data) {
		text = string(bytes.TrimPrefix(data, []byte{239, 187, 191}))
	} else {
		decoded, e := simplifiedchinese.GB18030.NewDecoder().Bytes(data)
		if e != nil {
			return "", fmt.Errorf("无法识别文本编码")
		}
		text = string(decoded)
		if strings.ContainsRune(text, utf8.RuneError) {
			return "", fmt.Errorf("无法识别文本编码")
		}
	}
	if strings.ContainsRune(text, 0) {
		return "", fmt.Errorf("文本包含空字符")
	}
	return text, nil
}
func vendorPath(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if len(name) > 2048 || strings.HasPrefix(name, "/") || strings.Contains(name, ":") || strings.ContainsFunc(name, unicode.IsControl) {
		return "", fmt.Errorf("无效的相对路径")
	}
	parts := strings.Split(name, "/")
	if len(parts) > 30 {
		return "", fmt.Errorf("目录过深")
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("目录含空层级或上级路径")
		}
	}
	return name, nil
}
func (r *ThirdPartyReport) add(pathName string, fields map[string]string) {
	isX := r.Format == "Xshell"
	get := func(secure, xshell string) string {
		key := secure
		if isX {
			key = xshell
		}
		return strings.TrimSpace(fields[key])
	}
	protocol := strings.ToUpper(get("protocol name", "connection/protocol"))
	kind := ""
	switch protocol {
	case "SSH", "SSH2":
		kind = "ssh"
	case "SFTP":
		kind = "sftp"
	case "FTP":
		kind = "ftp"
	default:
		r.issue(pathName, "协议缺失或不支持（支持 SSH2、SFTP、普通 FTP）", true)
		return
	}
	relative, e := vendorPath(pathName)
	if e != nil {
		r.issue(pathName, e.Error(), true)
		return
	}
	p := NewProfile()
	p.Kind = kind
	p.Name = path.Base(relative)
	parent := path.Dir(relative)
	p.Group = r.Format
	if parent != "." {
		p.Group += "/" + parent
	}
	p.Host = get("hostname", "connection/host")
	p.Username = get("username", "connection:authentication/username")
	if kind == "ftp" {
		p.Port = 21
		p.InitialDirectory = "/"
	}
	portKey := "port"
	if fields["[ssh2] port"] != "" {
		portKey = "[ssh2] port"
	}
	if raw := get(portKey, "connection/port"); raw != "" {
		p.Port, e = strconv.Atoi(raw)
		if e != nil {
			r.issue(relative, "端口无效", true)
			return
		}
	}
	if key := get("identity filename v2", "connection:authentication/userkey"); key != "" {
		if (filepath.IsAbs(key) || strings.HasPrefix(key, "~/")) && !strings.Contains(key, "${") && len(key) <= 4096 {
			p.IdentityFile = key
		} else {
			r.issue(relative, "私钥路径不适用于当前系统，请重新选择；不迁移私钥内容", false)
		}
	}
	keepKey := "send protocol noop"
	if fields["send protocol no-op"] != "" {
		keepKey = "send protocol no-op"
	}
	enabled := get(keepKey, "connection:keepalive/keepalive")
	if enabled == "0" || enabled == "1" {
		p.KeepAlive.Enabled = enabled == "1"
	}
	if value := get("nop interval", "connection:keepalive/keepaliveinterval"); value != "" {
		if n, e := strconv.Atoi(value); e == nil && n >= 1 && n <= 86400 {
			p.KeepAlive.Interval = n
		} else {
			r.issue(relative, "保活间隔无效，保留默认值", false)
		}
	}
	if isX {
		if v := get("", "connection:keepalive/tcpkeepalive"); v == "0" || v == "1" {
			p.KeepAlive.TCP = v == "1"
		}
	}
	p.KeepAlive.IdleEnabled = false
	if e = p.Validate(); e != nil {
		r.issue(relative, e.Error(), true)
		return
	}
	if len(r.Profiles) >= 2000 {
		r.issue(relative, "会话数量超过 2000", true)
		return
	}
	r.Profiles = append(r.Profiles, p)
	r.Directories = append(r.Directories, p.Group)
	if p.Username == "" {
		r.issue(relative, "没有用户名，连接前请核对默认用户", false)
	}
	if isX {
		if encrypted := get("", "connection:authentication/password"); encrypted != "" {
			r.Passwords[p.ID] = XshellPassword{encoded: encrypted}
		}
	}
	for key, value := range fields {
		value = strings.TrimSpace(strings.ToLower(value))
		if value != "" && value != "0" && value != "none" && value != "<none>" && (strings.Contains(key, "proxy") || strings.Contains(key, "firewall") || strings.Contains(key, "forward") || strings.Contains(key, "tunnel")) {
			r.issue(relative, "检测到代理或转发设置，需在会话属性中重新配置", false)
			break
		}
	}
}
func acceptedVendorField(key, format string) bool {
	if format == "SecureCRT" {
		switch key {
		case "is session", "hostname", "protocol name", "username", "[ssh2] port", "port", "identity filename v2", "send protocol noop", "send protocol no-op", "nop interval", "firewall name", "port forward filter", "reverse forward filter", "script filename v2", "use script file", "auto session setup":
			return true
		}
		return false
	}
	switch key {
	case "sessioninfo/version", "connection:authentication/password", "connection/host", "connection/protocol", "connection/port", "connection:authentication/username", "connection:authentication/userkey", "connection:keepalive/keepalive", "connection:keepalive/keepaliveinterval", "connection:keepalive/tcpkeepalive", "connection:keepalive/sendkeepalive", "connection:proxy/proxy", "connection:proxy/proxyname", "connection:proxy/proxyhost", "connection:proxy/proxytype", "connection:ssh/tunneling", "connection:ssh/portforwarding", "connection:ssh/forwardx11", "connection:ssh/remotecommand", "connection:login script/usescript", "connection:authentication/useexpectsend":
		return true
	}
	return strings.HasPrefix(key, "connection:ssh:tunneling/")
}
func (r *ThirdPartyReport) ini(ctx context.Context, data []byte, name string) error {
	if strings.HasPrefix(path.Base(name), "_") && strings.Contains(strings.ToLower(name), "folderdata") {
		return nil
	}
	text, e := decodeVendorText(data)
	if e != nil {
		r.issue(name, e.Error(), true)
		return nil
	}
	fields := map[string]string{}
	section := ""
	recognized := false
	for raw := range strings.SplitSeq(text, "\n") {
		if e = ctx.Err(); e != nil {
			return e
		}
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		var key, value string
		if r.Format == "Xshell" {
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				section = strings.ToLower(line[1 : len(line)-1])
				recognized = recognized || section == "connection"
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = section + "/" + strings.ToLower(strings.TrimSpace(k))
			value = v
		} else {
			if len(line) < 5 || line[1:3] != `:"` {
				continue
			}
			switch line[0] {
			case 'S', 'D', 'Z', 'B':
			default:
				continue
			}
			k, v, ok := strings.Cut(line[3:], `"=`)
			if !ok {
				continue
			}
			key = strings.ToLower(k)
			value = v
			recognized = true
			if line[0] == 'D' {
				if n, e := strconv.ParseUint(value, 16, 64); e == nil {
					value = strconv.FormatUint(n, 10)
				}
			}
		}
		if !acceptedVendorField(key, r.Format) {
			continue
		}
		if len(value) > 8192 {
			r.issue(name, "连接字段过长", true)
			return nil
		}
		if _, exists := fields[key]; exists {
			r.issue(name, "重复连接字段，未导入", true)
			return nil
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		fields[key] = value
	}
	if !recognized {
		r.issue(name, "不是所选来源的会话格式", true)
		return nil
	}
	if strings.HasPrefix(strings.ToLower(path.Base(name)), "default.") && fields["hostname"] == "" && fields["connection/host"] == "" {
		return nil
	}
	if len(r.Profiles) >= 2000 {
		return fmt.Errorf("单次最多导入 2000 个会话")
	}
	r.add(strings.TrimSuffix(name, path.Ext(name)), fields)
	return nil
}
func (r *ThirdPartyReport) xml(ctx context.Context, data []byte) error {
	text, e := decodeVendorText(data)
	if e != nil {
		return e
	}
	decoder := xml.NewDecoder(strings.NewReader(text))
	decoder.CharsetReader = func(_ string, reader io.Reader) (io.Reader, error) { return reader, nil }
	type node struct {
		name, path string
		active     bool
		fields     map[string]string
	}
	var nodes []*node
	depth, tokens := 0, 0
	rootOK, found := false, false
	field, fieldText := "", ""
	fieldDepth := 0
	for {
		if e = ctx.Err(); e != nil {
			return e
		}
		token, e := decoder.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return fmt.Errorf("SecureCRT XML 无效")
		}
		tokens++
		if tokens > 1000000 {
			return fmt.Errorf("XML 条目过多")
		}
		switch token := token.(type) {
		case xml.Directive:
			return fmt.Errorf("不允许 XML DTD 或实体定义")
		case xml.StartElement:
			depth++
			if depth > 64 {
				return fmt.Errorf("XML 层级过深")
			}
			name := strings.ToLower(token.Name.Local)
			if depth == 1 {
				rootOK = name == "vandyke"
			}
			attrs := map[string]string{}
			for _, a := range token.Attr {
				attrs[strings.ToLower(a.Name.Local)] = a.Value
			}
			if name == "key" {
				n := &node{name: attrs["name"], fields: map[string]string{}}
				if len(nodes) > 0 && nodes[len(nodes)-1].active {
					parent := nodes[len(nodes)-1]
					nested := false
					for _, ancestor := range nodes {
						nested = nested || ancestor.fields["is session"] == "1"
					}
					if !nested {
						n.active = true
						n.path = n.name
						if parent.path != "" {
							n.path = parent.path + "/" + n.name
						}
					}
				} else if strings.EqualFold(n.name, "sessions") {
					n.active = true
					n.path = ""
					found = true
				}
				nodes = append(nodes, n)
			} else if (name == "string" || name == "dword") && len(nodes) > 0 && nodes[len(nodes)-1].active && acceptedVendorField(strings.ToLower(attrs["name"]), r.Format) {
				if field != "" {
					return fmt.Errorf("XML 字段嵌套无效")
				}
				field = strings.ToLower(attrs["name"])
				fieldText = ""
				fieldDepth = depth
			}
		case xml.CharData:
			if field != "" {
				fieldText += string(token)
				if len(fieldText) > 8192 {
					return fmt.Errorf("XML 字段过长")
				}
			}
		case xml.EndElement:
			if field != "" && depth == fieldDepth {
				n := nodes[len(nodes)-1]
				if _, exists := n.fields[field]; exists {
					return fmt.Errorf("XML 存在重复连接字段")
				}
				n.fields[field] = fieldText
				field = ""
				fieldText = ""
			}
			if strings.EqualFold(token.Name.Local, "key") && len(nodes) > 0 {
				n := nodes[len(nodes)-1]
				nodes = nodes[:len(nodes)-1]
				if n.active && n.path != "" {
					if n.fields["is session"] == "1" || n.fields["protocol name"] != "" || n.fields["hostname"] != "" {
						if !strings.EqualFold(n.name, "default") || n.fields["hostname"] != "" {
							if len(r.Profiles) >= 2000 {
								return fmt.Errorf("单次最多导入 2000 个会话")
							}
							r.add(n.path, n.fields)
						}
					} else if p, e := vendorPath(n.path); e == nil {
						r.Directories = append(r.Directories, r.Format+"/"+p)
					} else {
						return e
					}
				}
			}
			depth--
		}
	}
	if !rootOK || !found {
		return fmt.Errorf("不是包含 Sessions 节点的 SecureCRT XML")
	}
	return nil
}
func ReadThirdParty(ctx context.Context, paths []string, format string) (ThirdPartyReport, error) {
	r := ThirdPartyReport{Format: format, Passwords: map[string]XshellPassword{}}
	if format != "Xshell" && format != "SecureCRT" {
		return r, fmt.Errorf("未知导入来源")
	}
	total, visited := 0, 0
	seen := map[string]bool{}
	var read func(string, string) error
	read = func(file, name string) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		visited++
		if visited > vendorDirectoryEntries {
			return fmt.Errorf("导入项目超过 8000")
		}
		if seen[file] {
			return nil
		}
		seen[file] = true
		info, e := os.Lstat(file)
		if e != nil {
			return e
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			r.issue(name, "不读取符号链接或特殊文件", true)
			return nil
		}
		ext := strings.ToLower(filepath.Ext(file))
		if format == "SecureCRT" && ext != ".ini" && ext != ".xml" || format == "Xshell" && ext != ".xsh" && ext != ".xts" && ext != ".zip" {
			return nil
		}
		if info.Size() > int64(MaxVendorImport-total) {
			return fmt.Errorf("导入总大小超过 64 MiB")
		}
		f, e := openImportFile(file)
		if e != nil {
			return e
		}
		opened, e := f.Stat()
		if e != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
			f.Close()
			return fmt.Errorf("导入文件类型或身份在读取期间变化")
		}
		data, e := io.ReadAll(io.LimitReader(f, int64(MaxVendorImport-total)+1))
		f.Close()
		if e != nil {
			return e
		}
		total += len(data)
		if total > MaxVendorImport || int64(len(data)) != info.Size() {
			return fmt.Errorf("导入超过大小限制，或文件在读取期间变化")
		}
		if ext == ".xml" {
			return r.xml(ctx, data)
		}
		if ext == ".zip" || ext == ".xts" {
			reader, e := readVendorZIP(data)
			if e != nil {
				return e
			}
			names := map[string]bool{}
			for _, entry := range reader.File {
				if e = ctx.Err(); e != nil {
					return e
				}
				relative, e := vendorPath(strings.TrimSuffix(entry.Name, "/"))
				if e != nil {
					return e
				}
				key := strings.ToLower(relative)
				if names[key] {
					return fmt.Errorf("ZIP 包含重复路径")
				}
				names[key] = true
				if entry.Flags&1 != 0 || entry.Mode()&os.ModeSymlink != 0 {
					return fmt.Errorf("ZIP 不支持加密容器或符号链接")
				}
				var decoded []byte
				if entry.FileInfo().IsDir() || strings.EqualFold(path.Ext(relative), ".xsh") {
					decoded, e = readVendorZIPEntry(entry)
					if e != nil {
						return e
					}
					total += len(decoded)
					if total > MaxVendorImport {
						return fmt.Errorf("ZIP 展开大小超过限制")
					}
				}
				parts := strings.Split(relative, "/")
				if strings.EqualFold(parts[0], "sessions") {
					parts = parts[1:]
				} else if len(parts) > 1 && strings.EqualFold(parts[0], "xshell") && strings.EqualFold(parts[1], "sessions") {
					parts = parts[2:]
				}
				if len(parts) == 0 {
					continue
				}
				relative = strings.Join(parts, "/")
				if entry.FileInfo().IsDir() {
					r.Directories = append(r.Directories, format+"/"+relative)
					continue
				}
				if !strings.EqualFold(path.Ext(relative), ".xsh") {
					continue
				}
				if e = r.ini(ctx, decoded, relative); e != nil {
					return e
				}
			}
			return nil
		}
		return r.ini(ctx, data, filepath.ToSlash(name))
	}
	for _, root := range paths {
		info, e := os.Lstat(root)
		if e != nil {
			return r, e
		}
		if info.Mode()&os.ModeSymlink != 0 {
			r.issue(filepath.Base(root), "不读取符号链接", true)
			continue
		}
		if info.IsDir() {
			e = filepath.WalkDir(root, func(file string, d fs.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if e = ctx.Err(); e != nil {
					return e
				}
				if d.IsDir() {
					if file != root {
						visited++
						if visited > vendorDirectoryEntries {
							return fmt.Errorf("导入项目超过 8000")
						}
						if format == "SecureCRT" {
							relative, err := filepath.Rel(root, file)
							if err != nil {
								return err
							}
							directory, err := vendorPath(filepath.ToSlash(relative))
							if err != nil {
								return err
							}
							r.Directories = append(r.Directories, format+"/"+directory)
						}
					}
					return nil
				}
				relative, e := filepath.Rel(root, file)
				if e != nil {
					return e
				}
				return read(file, relative)
			})
			if e != nil {
				return r, e
			}
		} else if e = read(root, filepath.Base(root)); e != nil {
			return r, e
		}
	}
	dirs := map[string]bool{}
	for _, d := range r.Directories {
		dirs[d] = true
	}
	r.Directories = nil
	for d := range dirs {
		r.Directories = append(r.Directories, d)
	}
	sort.Strings(r.Directories)
	if len(r.Directories) > 4000 {
		return r, fmt.Errorf("目录数量超过 4000")
	}
	return r, r.Archive().Validate()
}

type VendorImportStats struct{ Sessions, Passwords, Preserved, Unmatched int }

func (r *ThirdPartyReport) Merge(ctx context.Context, cfg Config, vault *Vault, directory string, includePasswords, fillMissing bool, master string) (Config, []CredentialUpdate, VendorImportStats, error) {
	stats := VendorImportStats{}
	next := cfg.Clone()
	var updates []CredentialUpdate
	var e error
	if !fillMissing {
		next, _, e = r.Archive().Merge(ctx, cfg, directory, false, nil)
		if e != nil {
			return cfg, nil, stats, e
		}
		stats.Sessions = len(r.Profiles)
	}
	if !includePasswords {
		return next, nil, stats, nil
	}
	used := map[string]bool{}
	for i, source := range r.Profiles {
		if e = ctx.Err(); e != nil {
			return cfg, nil, stats, e
		}
		encrypted, exists := r.Passwords[source.ID]
		if !exists {
			continue
		}
		target := Profile{}
		if fillMissing {
			var matches, pathMatches []Profile
			for _, p := range cfg.Profiles {
				if p.Name == source.Name && p.Kind == source.Kind && p.Host == source.Host && p.Port == source.Port && p.Username == source.Username && (directory == "" || Within(p.Group, directory)) {
					matches = append(matches, p)
					if p.Group == source.Group || strings.HasSuffix(p.Group, "/"+source.Group) {
						pathMatches = append(pathMatches, p)
					}
				}
			}
			if len(pathMatches) > 0 {
				matches = pathMatches
			}
			if len(matches) > 1 {
				return cfg, nil, stats, fmt.Errorf("匹配到多个相同会话，请选择更具体的目录")
			}
			if len(matches) == 0 {
				stats.Unmatched++
				continue
			}
			target = matches[0]
			if used[target.ID] {
				return cfg, nil, stats, fmt.Errorf("文件包含重复的匹配会话")
			}
			used[target.ID] = true
			secret, _, e := vault.exportCredential(target)
			if e != nil {
				return cfg, nil, stats, e
			}
			if secret != "" {
				stats.Preserved++
				continue
			}
		} else {
			target = next.Profiles[len(cfg.Profiles)+i]
		}
		password, e := encrypted.Decrypt(master)
		if e != nil {
			return cfg, nil, stats, e
		}
		updates = append(updates, CredentialUpdate{Profile: target, Password: password, Identity: target.Identity()})
		stats.Passwords++
	}
	return next, updates, stats, nil
}

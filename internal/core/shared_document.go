// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/pbkdf2"
	"reflect"
	"sort"
	"strings"
)

const MaxSharedVault = 48 << 20
const sharedAAD = "OShell.shared-vault.v1"

type sharedEnvelope struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	Iterations int    `json:"iterations"`
	Salt       []byte `json:"salt"`
	Ciphertext []byte `json:"ciphertext"`
}

func SealShared(data, master []byte) ([]byte, error) { return sealSharedVersion(data, master, 1) }
func sealSharedVersion(data, master []byte, version int) ([]byte, error) {
	if version != 1 && version != 2 {
		return nil, fmt.Errorf("共享协议版本不支持")
	}
	if len(data) > 32<<20 || len(master) == 0 || len(master) > 4096 {
		return nil, fmt.Errorf("共享数据或密码大小无效")
	}
	e := sharedEnvelope{Format: "OShell.shared-vault", Version: version, Iterations: 600000, Salt: make([]byte, 16)}
	if _, err := rand.Read(e.Salt); err != nil {
		return nil, err
	}
	key := pbkdf2.Key(master, e.Salt, e.Iterations, 32, sha256.New)
	defer clear(key)
	var err error
	e.Ciphertext, err = seal(key, data, sharedEnvelopeAAD(version))
	if err != nil {
		return nil, err
	}
	return json.Marshal(e)
}
func OpenShared(data, master []byte) ([]byte, error) {
	if len(data) > MaxSharedVault {
		return nil, fmt.Errorf("共享数据超过大小限制")
	}
	var e sharedEnvelope
	if json.Unmarshal(data, &e) != nil || e.Format != "OShell.shared-vault" || (e.Version != 1 && e.Version != 2) || e.Iterations < 600000 || e.Iterations > 2000000 || len(e.Salt) != 16 || len(e.Ciphertext) < 28 || len(e.Ciphertext) > 32<<20+28 || len(master) == 0 || len(master) > 4096 {
		return nil, fmt.Errorf("共享数据格式无效")
	}
	key := pbkdf2.Key(master, e.Salt, e.Iterations, 32, sha256.New)
	defer clear(key)
	plain, err := unseal(key, e.Ciphertext, sharedEnvelopeAAD(e.Version))
	if err != nil {
		return nil, fmt.Errorf("共享主密码不正确，或共享数据已损坏")
	}
	return plain, nil
}
func sharedEnvelopeAAD(version int) string {
	if version == 2 {
		return "OShell.shared-vault.v2"
	}
	return sharedAAD
}
func IsSharedVault(data []byte) bool {
	var header struct{ Format string }
	return len(data) <= MaxSharedVault && json.Unmarshal(data, &header) == nil && header.Format == "OShell.shared-vault"
}

type SharedLinks struct {
	Entries   []SessionLink `json:"entries"`
	Folders   []string      `json:"folders"`
	Visible   bool          `json:"visible"`
	RootOrder []string      `json:"rootOrder"`
}
type SharedDocument struct {
	Proxies       []PortableProxy            `json:"proxies"`
	Profiles      []PortableProfile          `json:"profiles"`
	Preferences   map[string]json.RawMessage `json:"preferences"`
	Directories   []string                   `json:"directories"`
	QuickCommands []QuickCommand             `json:"quickCommands"`
	HighlightSets []HighlightSet             `json:"highlightSets"`
	Links         SharedLinks                `json:"sessionLinks"`
	Defaults      SessionDefaults            `json:"sessionDefaults"`
	Verifier      *LegacyPassword            `json:"masterPasswordVerifier"`
}

func masterVerifierProfile() Profile {
	p := NewProfile()
	p.ID = "F7FCF275-EB85-488F-AF21-A0424101259A"
	p.Host = "oshell-vault"
	p.Username = "vault"
	p.Port = 22
	return p
}

const verifierMarker = "OShell master password verifier v1"

func (d SharedDocument) Validate(master []byte) error {
	if d.Profiles == nil || d.Preferences == nil || d.Verifier == nil || d.Verifier.LocalKeyID != "" {
		return fmt.Errorf("共享配置缺少主密码保护")
	}
	p := masterVerifierProfile()
	text, e := DecryptMacPassword(*d.Verifier, master, p)
	if e != nil || subtle.ConstantTimeCompare([]byte(text), []byte(verifierMarker)) != 1 {
		return fmt.Errorf("共享主密码校验失败")
	}
	archive := PortableArchive{Format: "OShell.sessions", Version: 4, Proxies: d.Proxies, Profiles: d.Profiles, Directories: d.Directories, Links: d.Links.Entries}
	if e = archive.Validate(); e != nil {
		return e
	}
	for _, p := range d.Profiles {
		for _, envelope := range []*LegacyPassword{p.Password, p.ProxyPassword} {
			if envelope != nil {
				if envelope.LocalKeyID != "" {
					return fmt.Errorf("共享配置包含本机密钥引用")
				}
				if e = validatePasswordEnvelope(*envelope); e != nil {
					return e
				}
			}
		}
	}
	for _, proxy := range d.Proxies {
		if proxy.Password != nil {
			if proxy.Password.LocalKeyID != "" {
				return fmt.Errorf("共享代理包含本机密钥引用")
			}
			if e = validatePasswordEnvelope(*proxy.Password); e != nil {
				return e
			}
		}
	}
	cfg, e := d.Configuration()
	if e != nil {
		return e
	}
	return cfg.Validate()
}
func (d SharedDocument) Encode(master []byte) ([]byte, error) {
	normalized, err := d.canonicalProtocol()
	if err != nil {
		return nil, err
	}
	d = normalized
	if e := d.Validate(master); e != nil {
		return nil, e
	}
	data, e := json.Marshal(d)
	if e != nil {
		return nil, e
	}
	canonical, err := canonicalConfigurationJSON(data)
	if err != nil {
		return nil, err
	}
	return sealSharedVersion(canonical, master, 2)
}
func DecodeSharedDocument(data, master []byte) (SharedDocument, error) {
	plain, e := OpenShared(data, master)
	if e != nil {
		return SharedDocument{}, e
	}
	defer clear(plain)
	d, err := decodeNativeDocument(plain)
	if err != nil {
		return d, err
	}
	return d, d.Validate(master)
}
func decodeNativeDocument(plain []byte) (SharedDocument, error) {
	canonical, err := canonicalConfigurationJSON(plain)
	if err != nil {
		return SharedDocument{}, err
	}
	plain = canonical
	var e error
	d := SharedDocument{Defaults: DefaultSessionDefaults(), Links: SharedLinks{Visible: true}, HighlightSets: []HighlightSet{StandardHighlightSet()}}
	if e = json.Unmarshal(plain, &d); e != nil {
		return d, e
	}
	var legacy struct {
		FTPProfiles []struct {
			ID, Name, Host, Username, InitialDirectory string
			Port                                       int
			EncryptedPassword                          *LegacyPassword
		}
	}
	if e = json.Unmarshal(plain, &legacy); e != nil {
		return d, e
	}
	for _, ftp := range legacy.FTPProfiles {
		p := NewProfile()
		p.ID = strings.ToUpper(ftp.ID)
		p.Proxy.ID = absentProxyID(p.ID)
		p.Name = ftp.Name
		p.Kind = "ftp"
		p.Host = ftp.Host
		p.Port = ftp.Port
		p.Username = ftp.Username
		p.InitialDirectory = ftp.InitialDirectory
		p.Group = "FTP"
		duplicate := false
		for _, existing := range d.Profiles {
			if strings.EqualFold(existing.Definition.ID, p.ID) {
				// Match Swift's FTPProfile equality: an ID collision must not
				// silently discard a name, initial path or saved password.
				if existing.Definition.Host != p.Host || existing.Definition.Port != p.Port || existing.Definition.Username != p.Username || existing.Definition.Kind != "ftp" || existing.Definition.Name != p.Name || existing.Definition.InitialDirectory != p.InitialDirectory || !reflect.DeepEqual(existing.Password, ftp.EncryptedPassword) {
					return d, fmt.Errorf("旧 FTP 会话 ID 与现有会话冲突")
				}
				duplicate = true
				break
			}
		}
		if !duplicate {
			d.Profiles = append(d.Profiles, PortableProfile{Definition: p, Password: ftp.EncryptedPassword})
		}
	}
	return d, nil
}
func preferencesForShared(cfg Config) map[string]json.RawMessage {
	prefs := map[string]json.RawMessage{}
	for key, value := range cfg.SharedPreferences {
		prefs[key] = append([]byte(nil), value...)
	}
	put := func(key string, value any) { prefs[key], _ = json.Marshal(value) }
	s := cfg.Settings
	font := s.FontFamily
	if font == "DejaVu Sans Mono" {
		font = "DejaVuSansMono"
	}
	put("masterWarningAcknowledged", s.MasterWarningAcknowledged)
	put("updateRepository", s.UpdateRepository)
	put("automaticUpdateChecks", s.AutomaticUpdateChecks)
	put("preferColorEmoji", s.PreferColorEmoji)
	put("ambiguousCharactersAreWide", s.AmbiguousCharactersAreWide)
	put("terminalClockEnabled", s.TerminalClockEnabled)
	put("terminalClockPosition", s.TerminalClockPosition)
	put("terminalClockFontName", s.TerminalClockFontName)
	put("terminalClockFontSize", s.TerminalClockFontSize)
	put("terminalClockColor", s.TerminalClockColor)
	put("terminalClockOpacity", s.TerminalClockOpacity)
	put("fontName", font)
	put("fontSize", s.FontSize)
	if s.TerminalAppearance != nil {
		put("terminalAppearance", s.TerminalAppearance)
	} else {
		delete(prefs, "terminalAppearance")
	}
	put("scrollback", s.ScrollbackLines)
	put("darkTheme", sharedSchemeIsDark(s))
	put("metal", s.Metal)
	put("autoZmodem", s.AutoZmodem)
	put("interfaceTheme", s.InterfaceTheme)
	put("colorSchemeID", s.ColorSchemeID)
	put("customColorSchemes", append([]ColorScheme{}, s.CustomSchemes...))
	keys := s.KeyboardShortcuts.Clone()
	if keys.Overrides == nil {
		keys.Overrides = map[string]ShortcutOverride{}
	}
	put("keyboardShortcuts", keys)
	put("copyOnSelect", s.Input.CopyOnSelect)
	put("rightClickPaste", s.Input.RightClickPaste)
	put("copyTrimLeadingWhitespace", s.Input.TrimLeading)
	put("copyTrimTrailingWhitespace", s.Input.TrimTrailing)
	put("confirmMultilinePaste", s.Input.ConfirmMultiline)
	put("quickSendBarVisible", s.QuickSendVisible)
	put("quickSendScope", sharedQuickSendScope(s.QuickSendScope))
	if s.HighlightSetID == "" {
		put("highlightSetID", nil)
	} else {
		put("highlightSetID", s.HighlightSetID)
	}
	delete(prefs, "oshellDesktop")
	return prefs
}
func (d SharedDocument) Configuration() (Config, error) {
	cfg := DefaultConfig()
	s := &cfg.Settings
	if raw := d.Preferences["oshellDesktop"]; raw != nil {
		if e := json.Unmarshal(raw, s); e != nil {
			return cfg, e
		}
	}
	values := map[string]any{"metal": &s.Metal, "masterWarningAcknowledged": &s.MasterWarningAcknowledged, "updateRepository": &s.UpdateRepository, "automaticUpdateChecks": &s.AutomaticUpdateChecks, "preferColorEmoji": &s.PreferColorEmoji, "ambiguousCharactersAreWide": &s.AmbiguousCharactersAreWide, "terminalClockEnabled": &s.TerminalClockEnabled, "terminalClockPosition": &s.TerminalClockPosition, "terminalClockFontName": &s.TerminalClockFontName, "terminalClockFontSize": &s.TerminalClockFontSize, "terminalClockColor": &s.TerminalClockColor, "terminalClockOpacity": &s.TerminalClockOpacity, "fontName": &s.FontFamily, "fontSize": &s.FontSize, "scrollback": &s.ScrollbackLines, "darkTheme": &s.Dark, "autoZmodem": &s.AutoZmodem, "interfaceTheme": &s.InterfaceTheme, "colorSchemeID": &s.ColorSchemeID, "customColorSchemes": &s.CustomSchemes, "keyboardShortcuts": &s.KeyboardShortcuts, "copyOnSelect": &s.Input.CopyOnSelect, "rightClickPaste": &s.Input.RightClickPaste, "copyTrimLeadingWhitespace": &s.Input.TrimLeading, "copyTrimTrailingWhitespace": &s.Input.TrimTrailing, "confirmMultilinePaste": &s.Input.ConfirmMultiline, "quickSendBarVisible": &s.QuickSendVisible, "highlightSetID": &s.HighlightSetID}
	values["terminalAppearance"] = &s.TerminalAppearance
	if raw, ok := d.Preferences["quickSendScope"]; ok {
		scope, err := decodeSharedQuickSendScope(raw)
		if err != nil {
			return cfg, err
		}
		s.QuickSendScope = scope
	}
	for key, target := range values {
		if raw, ok := d.Preferences[key]; ok {
			if key == "highlightSetID" && string(raw) == "null" {
				s.HighlightSetID = ""
				continue
			}
			if e := json.Unmarshal(raw, target); e != nil {
				return cfg, fmt.Errorf("共享设置字段无效：%s", key)
			}
		}
	}
	s.ClampClockStyle()
	if s.FontFamily == "DejaVuSansMono" {
		s.FontFamily = "DejaVu Sans Mono"
	}
	if uuidText.MatchString(s.HighlightSetID) {
		s.HighlightSetID = strings.ToUpper(s.HighlightSetID)
	}
	for _, p := range d.Profiles {
		p.Definition.ID = strings.ToUpper(p.Definition.ID)
		p.Definition.Proxy.ID = strings.ToUpper(p.Definition.Proxy.ID)
		cfg.Profiles = append(cfg.Profiles, p.Definition)
	}
	cfg.Directories = append([]string{}, d.Directories...)
	for _, folder := range d.Links.Folders {
		if !canonicalDirectory(folder) {
			return cfg, fmt.Errorf("共享快捷目录无效")
		}
		cfg.Directories = append(cfg.Directories, "Links/"+folder)
	}
	cfg.Links = append([]SessionLink(nil), d.Links.Entries...)
	for i := range cfg.Links {
		cfg.Links[i].ID = strings.ToUpper(cfg.Links[i].ID)
		cfg.Links[i].ProfileID = strings.ToUpper(cfg.Links[i].ProfileID)
	}
	cfg.LinkRootOrder = append([]string(nil), d.Links.RootOrder...)
	for i, key := range cfg.LinkRootOrder {
		prefix, id, ok := strings.Cut(key, ":")
		if ok && uuidText.MatchString(id) {
			cfg.LinkRootOrder[i] = prefix + ":" + strings.ToUpper(id)
		}
	}
	cfg.Settings.LinksVisible = d.Links.Visible
	cfg.QuickCommands = append([]QuickCommand(nil), d.QuickCommands...)
	cfg.HighlightSets = append([]HighlightSet(nil), d.HighlightSets...)
	for i := range cfg.HighlightSets {
		if uuidText.MatchString(cfg.HighlightSets[i].ID) {
			cfg.HighlightSets[i].ID = strings.ToUpper(cfg.HighlightSets[i].ID)
		}
	}
	cfg.SessionDefaults = d.Defaults
	cfg.SharedPreferences = d.Preferences
	for _, proxy := range d.Proxies {
		cfg.Proxies = append(cfg.Proxies, proxy.Definition)
	}
	if e := cfg.MigrateProxyCatalog(); e != nil {
		return cfg, e
	}
	return cfg, nil
}

type credentialSnapshot struct {
	localKey []byte
	key      []byte
	entries  map[string]vaultEntry
}

func (v *Vault) snapshotCredentials() (credentialSnapshot, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.data.Master && len(v.key) == 0 {
		return credentialSnapshot{}, ErrVaultLocked
	}
	snapshot := credentialSnapshot{key: append([]byte(nil), v.key...), entries: map[string]vaultEntry{}}
	for id, entry := range v.data.Entries {
		if entry.Local && len(snapshot.localKey) == 0 {
			var err error
			snapshot.localKey, err = loadLocalVaultKey(v.dir)
			if err != nil {
				clear(snapshot.key)
				return credentialSnapshot{}, err
			}
		}
		snapshot.entries[id] = entry
	}
	return snapshot, nil
}
func (s credentialSnapshot) credential(p Profile) (string, SSHIdentity, string, error) {
	identity := p.Identity()
	entry, ok := s.entries[p.ID]
	if !ok || entry.Identity != credentialIdentity(p) {
		return "", identity, "", nil
	}
	key := s.key
	if entry.Local {
		key = s.localKey
	}
	if len(key) == 0 {
		return "", identity, "", ErrVaultLocked
	}
	plain, e := unseal(key, entry.Data, entryAAD(p.ID, entry))
	if e != nil {
		return "", identity, "", e
	}
	defer clear(plain)
	raw, _ := json.Marshal(entry)
	digest := sha256.Sum256(raw)
	if entry.Resolved != nil {
		identity = *entry.Resolved
	}
	return string(plain), identity, hex.EncodeToString(digest[:]), nil
}
func sameRawIdentity(a, b Profile) bool {
	return strings.EqualFold(a.ID, b.ID) && a.Host == b.Host && a.Port == b.Port && a.Username == b.Username && a.Kind == b.Kind
}
func (v *Vault) ExportSharedDocument(ctx context.Context, cfg Config, master []byte, cached *SharedDocument, oldDigests map[string]string) (SharedDocument, map[string]string, error) {
	document := SharedDocument{Proxies: []PortableProxy{}, Profiles: []PortableProfile{}, Preferences: preferencesForShared(cfg), Directories: []string{}, QuickCommands: append([]QuickCommand{}, cfg.QuickCommands...), HighlightSets: cfg.Clone().HighlightSets, Defaults: cfg.SessionDefaults, Links: SharedLinks{Entries: append([]SessionLink{}, cfg.Links...), Visible: cfg.Settings.LinksVisible, RootOrder: append([]string{}, cfg.LinkRootOrder...), Folders: []string{}}}
	snapshot, e := v.snapshotCredentials()
	if e != nil {
		return document, nil, e
	}
	defer clear(snapshot.key)
	defer clear(snapshot.localKey)
	digests := map[string]string{}
	previous := map[string]PortableProfile{}
	if cached != nil {
		for _, p := range cached.Profiles {
			previous[strings.ToUpper(p.Definition.ID)] = p
		}
		document.Verifier = cached.Verifier
	}
	if document.Verifier == nil {
		p := masterVerifierProfile()
		envelope, e := EncryptMacPassword(verifierMarker, master, p, p.Identity())
		if e != nil {
			return document, nil, e
		}
		document.Verifier = &envelope
	}
	folders := map[string]bool{}
	for _, dir := range cfg.AllDirectories() {
		if dir == "Links" {
			continue
		}
		if strings.HasPrefix(dir, "Links/") {
			folders[strings.TrimPrefix(dir, "Links/")] = true
		} else {
			document.Directories = append(document.Directories, dir)
		}
	}
	for folder := range folders {
		document.Links.Folders = append(document.Links.Folders, folder)
	}
	sort.Strings(document.Links.Folders)
	for i := range document.HighlightSets {
		document.HighlightSets[i].Rules = append([]HighlightRule{}, document.HighlightSets[i].Rules...)
		for j := range document.HighlightSets[i].Rules {
			r := &document.HighlightSets[i].Rules[j]
			if r.Color == "" {
				r.Color = r.TextColor()
			}
		}
	}
	for _, p := range cfg.Profiles {
		if e = ctx.Err(); e != nil {
			return document, nil, e
		}
		value := PortableProfile{Definition: p}
		previous := previous[strings.ToUpper(p.ID)]
		for index, profile := range []Profile{p, ProxyCredentialProfile(p.Proxy)} {
			if index == 1 && p.ProxyID != "" {
				continue
			}
			secret, identity, digest, e := snapshot.credential(profile)
			if e != nil {
				return document, nil, e
			}
			key := strings.ToUpper(profile.ID)
			digests[key] = digest
			if secret == "" {
				continue
			}
			oldProfile, oldEnvelope := previous.Definition, previous.Password
			if index == 1 {
				oldProfile = ProxyCredentialProfile(previous.Definition.Proxy)
				oldEnvelope = previous.ProxyPassword
			}
			envelope := oldEnvelope
			if !reusableSharedCredential(envelope, oldProfile, profile, identity, secret, master, oldDigests[key] == digest) {
				newEnvelope, e := EncryptMacPassword(secret, master, profile, identity)
				if e != nil {
					return document, nil, e
				}
				envelope = &newEnvelope
			}
			if index == 0 {
				value.Password = envelope
			} else {
				value.ProxyPassword = envelope
			}
		}
		document.Profiles = append(document.Profiles, value)
	}
	previousProxies := map[string]PortableProxy{}
	if cached != nil {
		for _, proxy := range cached.Proxies {
			previousProxies[strings.ToUpper(proxy.Definition.ID())] = proxy
		}
	}
	for _, proxy := range cfg.Proxies {
		if e = ctx.Err(); e != nil {
			return document, nil, e
		}
		profile := ProxyCredentialProfile(proxy.Settings)
		value := PortableProxy{Definition: proxy}
		secret, identity, digest, e := snapshot.credential(profile)
		if e != nil {
			return document, nil, e
		}
		key := strings.ToUpper(profile.ID)
		digests[key] = digest
		if secret != "" {
			old := previousProxies[key]
			value.Password = old.Password
			if !reusableSharedCredential(value.Password, ProxyCredentialProfile(old.Definition.Settings), profile, identity, secret, master, oldDigests[key] == digest) {
				envelope, e := EncryptMacPassword(secret, master, profile, identity)
				if e != nil {
					return document, nil, e
				}
				value.Password = &envelope
			}
		}
		document.Proxies = append(document.Proxies, value)
	}
	return document, digests, document.Validate(master)
}
func (v *Vault) MasterSecret() ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.data.Master {
		return nil, fmt.Errorf("请先设置本机主密码")
	}
	if len(v.master) == 0 {
		return nil, ErrVaultLocked
	}
	return append([]byte(nil), v.master...), nil
}

// Local re-encryption (for example after UUID normalization) must not create a
// new shared password revision when the secret and its target are unchanged.
func reusableSharedCredential(envelope *LegacyPassword, old, current Profile, identity SSHIdentity, secret string, master []byte, sameDigest bool) bool {
	if envelope == nil || !sameRawIdentity(old, current) || envelope.Identity != identity {
		return false
	}
	if sameDigest {
		return true
	}
	previous, e := DecryptMacPassword(*envelope, master, current)
	return e == nil && subtle.ConstantTimeCompare([]byte(previous), []byte(secret)) == 1
}

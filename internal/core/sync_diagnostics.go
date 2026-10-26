package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type SyncEvent string

const (
	SyncConnectionTest SyncEvent = "connectionTest"
	SyncQueued         SyncEvent = "queued"
	SyncLocalChanged   SyncEvent = "localChanged"
	SyncConfigured     SyncEvent = "configured"
	SyncDisabled       SyncEvent = "disabled"
	SyncStarted        SyncEvent = "started"
	SyncFetched        SyncEvent = "fetched"
	SyncUploading      SyncEvent = "uploading"
	SyncApplying       SyncEvent = "applying"
	SyncCompleted      SyncEvent = "completed"
	SyncUnchanged      SyncEvent = "unchanged"
	SyncConflict       SyncEvent = "conflict"
	SyncPostponed      SyncEvent = "postponed"
	SyncFailed         SyncEvent = "failed"
	SyncCancelled      SyncEvent = "cancelled"
)

var syncEventTitles = map[SyncEvent]string{SyncConnectionTest: "WebDAV 连接测试", SyncQueued: "同步已排队", SyncLocalChanged: "本地配置已修改", SyncConfigured: "同步配置已更新", SyncDisabled: "同步已停用", SyncStarted: "开始读取共享数据", SyncFetched: "共享数据读取完成", SyncUploading: "加密并上传", SyncApplying: "应用共享数据到本地", SyncCompleted: "同步完成", SyncUnchanged: "检查完成，内容无变化", SyncConflict: "检测到冲突，等待选择", SyncPostponed: "用户稍后处理", SyncFailed: "同步失败，本地数据保留", SyncCancelled: "本次同步已取消"}

func (e SyncEvent) Title() string { return syncEventTitles[e] }

type SyncSafeFailure struct {
	Kind string `json:"kind"`
	Code int    `json:"code,omitempty"`
}

var syncFailureTitles = map[string]string{"tls": "TLS 证书校验失败", "authentication": "认证失败", "remoteChanged": "共享版本已变化", "network": "网络请求失败或超时", "http": "HTTP 响应异常", "invalidData": "配置、密码或加密数据校验失败", "storage": "本地或同步目录读写失败", "cancelled": "请求已取消", "directoryUnavailable": "同步目录暂不可用", "cloudConflict": "存在文件级冲突", "remoteDeleted": "共享同步文件已删除", "encryption": "主密码或加密数据校验失败"}

func (f SyncSafeFailure) Title() string { return syncFailureTitles[f.Kind] }
func safeDiagnosticFailure(err error) *SyncSafeFailure {
	if err == nil {
		return nil
	}
	failure := &SyncSafeFailure{Kind: "invalidData"}
	var transport *SyncFailure
	var path *os.PathError
	var network net.Error
	switch {
	case errors.Is(err, context.Canceled):
		failure.Kind = "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		failure.Kind = "network"
	case errors.As(err, &transport):
		failure.Code = transport.Status
		switch transport.Kind {
		case "tls", "tls-changed", "tls-validity":
			failure.Kind = "tls"
		case "authentication":
			failure.Kind = "authentication"
		case "conflict":
			failure.Kind = "remoteChanged"
		case "network":
			failure.Kind = "network"
		case "directory", "webdav-directory":
			failure.Kind = "directoryUnavailable"
		case "file-conflict":
			failure.Kind = "cloudConflict"
		case "remote-deleted":
			failure.Kind = "remoteDeleted"
		case "encryption":
			failure.Kind = "encryption"
		default:
			if transport.Status != 0 {
				failure.Kind = "http"
			}
		}
	case errors.As(err, &path):
		failure.Kind = "storage"
	case errors.As(err, &network):
		failure.Kind = "network"
	}
	return failure
}

type SyncDiagnosticEntry struct {
	RunID      string           `json:"run,omitempty"`
	Date       time.Time        `json:"date"`
	Backend    string           `json:"backend"`
	Event      SyncEvent        `json:"event"`
	Manual     bool             `json:"manual"`
	DurationMS int64            `json:"durationMS,omitempty"`
	Failure    *SyncSafeFailure `json:"failure,omitempty"`
}
type SyncSummary struct {
	Scope            string           `json:"scope"`
	LastAttempt      *time.Time       `json:"lastAttempt,omitempty"`
	LastSuccess      *time.Time       `json:"lastSuccess,omitempty"`
	Outcome          SyncEvent        `json:"outcome,omitempty"`
	LocalFingerprint string           `json:"localFingerprint,omitempty"`
	Failure          *SyncSafeFailure `json:"failure,omitempty"`
}
type SyncDiagnosticDocument struct {
	Version int                   `json:"version"`
	Entries []SyncDiagnosticEntry `json:"entries"`
	Summary SyncSummary           `json:"summary"`
}
type SyncDiagnostics struct{ Directory, Target, Backend string }

var syncDiagnosticsMu sync.Mutex

func (d SyncDiagnostics) scope() string {
	hash := sha256.Sum256([]byte(d.Target))
	return hex.EncodeToString(hash[:])
}
func (d SyncDiagnostics) path() string { return filepath.Join(d.Directory, "logs", "sync.json") }
func safeDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}
func (d SyncDiagnostics) read() (SyncDiagnosticDocument, error) {
	doc := SyncDiagnosticDocument{Version: 1, Entries: []SyncDiagnosticEntry{}}
	file, err := openImportFile(d.path())
	if os.IsNotExist(err) {
		return doc, nil
	}
	if err != nil {
		return doc, errors.New("无法读取同步诊断日志")
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return doc, errors.New("同步诊断日志超过限制")
	}
	if err = json.Unmarshal(raw, &doc); err != nil || doc.Version != 1 {
		return SyncDiagnosticDocument{}, errors.New("同步诊断日志格式无效")
	}
	if doc.Summary.Scope != "" && !safeDigest(doc.Summary.Scope) || doc.Summary.LocalFingerprint != "" && !safeDigest(doc.Summary.LocalFingerprint) || doc.Summary.Outcome != "" && doc.Summary.Outcome.Title() == "" {
		return SyncDiagnosticDocument{}, errors.New("同步诊断摘要无效")
	}
	if doc.Summary.Failure != nil && doc.Summary.Failure.Title() == "" {
		return SyncDiagnosticDocument{}, errors.New("同步诊断摘要无效")
	}
	for _, entry := range doc.Entries {
		if !validDiagnosticRun(entry.RunID) || entry.Event.Title() == "" || entry.Backend != "directory" && entry.Backend != "webdav" && entry.Backend != "none" || entry.Failure != nil && entry.Failure.Title() == "" {
			return SyncDiagnosticDocument{}, errors.New("同步诊断事件无效")
		}
	}
	return pruneSyncDiagnostic(doc), nil
}
func (d SyncDiagnostics) Snapshot() (SyncDiagnosticDocument, error) {
	syncDiagnosticsMu.Lock()
	defer syncDiagnosticsMu.Unlock()
	if pending, ok := syncDiagnosticUnsaved[d.path()]; ok {
		pending = pruneSyncDiagnostic(pending)
		syncDiagnosticUnsaved[d.path()] = pending
		return cloneSyncDiagnostic(pending), nil
	}
	doc, err := d.read()
	if err == nil {
		rememberSyncDiagnostic(d.path(), doc)
	}
	return doc, err
}

// This API accepts no arbitrary message, endpoint, profile, username or password.
func (d SyncDiagnostics) Record(event SyncEvent, manual bool, duration time.Duration, failure error, fingerprint string) error {
	return d.record(event, manual, duration, failure, fingerprint, "")
}
func validDiagnosticRun(run string) bool {
	if run == "" {
		return true
	}
	if len(run) != 36 || run[8] != '-' || run[13] != '-' || run[18] != '-' || run[23] != '-' {
		return false
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(run, "-", ""))
	return err == nil && len(raw) == 16
}
func (d SyncDiagnostics) record(event SyncEvent, manual bool, duration time.Duration, failure error, fingerprint, run string) error {
	if !validDiagnosticRun(run) || event.Title() == "" || d.Backend != "directory" && d.Backend != "webdav" && d.Backend != "none" || fingerprint != "" && !safeDigest(fingerprint) {
		return errors.New("不允许写入非枚举同步诊断内容")
	}
	syncDiagnosticsMu.Lock()
	defer syncDiagnosticsMu.Unlock()
	var doc SyncDiagnosticDocument
	var err error
	if pending, ok := syncDiagnosticUnsaved[d.path()]; ok {
		doc = cloneSyncDiagnostic(pending)
	} else {
		doc, err = d.read()
		if err != nil {
			if previous, ok := syncDiagnosticClean[d.path()]; ok {
				doc = cloneSyncDiagnostic(previous.document)
			} else {
				doc = SyncDiagnosticDocument{Version: 1, Entries: []SyncDiagnosticEntry{}}
			}
		}
	}
	doc = pruneSyncDiagnostic(doc)
	now := time.Now().UTC()
	entry := SyncDiagnosticEntry{RunID: run, Date: now, Backend: d.Backend, Event: event, Manual: manual, DurationMS: max(int64(0), duration.Milliseconds()), Failure: safeDiagnosticFailure(failure)}
	doc.Entries = append(doc.Entries, entry)
	if len(doc.Entries) > 2000 {
		doc.Entries = doc.Entries[len(doc.Entries)-2000:]
	}
	if doc.Summary.Scope != d.scope() {
		doc.Summary = SyncSummary{Scope: d.scope()}
	}
	doc.Summary.Outcome = event
	if event == SyncFailed {
		doc.Summary.Failure = entry.Failure
	}
	if event == SyncStarted {
		doc.Summary.LastAttempt = &now
	}
	if event == SyncCompleted || event == SyncUnchanged {
		doc.Summary.Failure = nil
		doc.Summary.LastSuccess = &now
		doc.Summary.LocalFingerprint = fingerprint
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	for len(raw) > 1<<20 && len(doc.Entries) > 1 {
		doc.Entries = doc.Entries[len(doc.Entries)/4+1:]
		raw, err = json.Marshal(doc)
		if err != nil {
			return err
		}
	}
	if err = os.MkdirAll(filepath.Dir(d.path()), 0700); err == nil {
		err = AtomicWrite(d.path(), raw)
	}
	if err != nil {
		syncDiagnosticUnsaved[d.path()] = cloneSyncDiagnostic(doc)
		delete(syncDiagnosticClean, d.path())
		return errors.New("无法写入同步诊断日志，当前运行的记录已保留在内存")
	}
	delete(syncDiagnosticUnsaved, d.path())
	rememberSyncDiagnostic(d.path(), doc)
	return nil
}
func (d SyncDiagnostics) Export() ([]byte, error) {
	doc, err := d.Snapshot()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(struct {
		Format  string                `json:"format"`
		Entries []SyncDiagnosticEntry `json:"entries"`
	}{"OShellDesktop.sync-diagnostics.v1", doc.Entries}, "", "  ")
}
func (e *SharedSyncEngine) Diagnostics() SyncDiagnostics {
	backend := "webdav"
	switch e.Transport.(type) {
	case DirectoryTransport, *DirectoryTransport:
		backend = "directory"
	}
	return SyncDiagnostics{Directory: e.Store.Dir, Target: e.Replica.Target, Backend: backend}
}
func (e *SharedSyncEngine) diagnosticFingerprint() string {
	if e.Selection != nil {
		return SyncLocalFingerprint(e.Store, e.Vault, *e.Selection)
	}
	return SyncLocalFingerprint(e.Store, e.Vault)
}

func (e *SharedSyncEngine) recordDiagnostic(event SyncEvent, start time.Time, err error, run string) {
	fingerprint := ""
	if event == SyncCompleted || event == SyncUnchanged {
		fingerprint = e.diagnosticFingerprint()
	}
	issue := e.Diagnostics().record(event, e.Manual, time.Since(start), err, fingerprint, run)
	if e.Progress != nil {
		e.Progress(event, issue)
	}
}

func (d SyncDiagnostics) SummaryForTarget(doc SyncDiagnosticDocument) SyncSummary {
	if doc.Summary.Scope != d.scope() {
		return SyncSummary{}
	}
	return doc.Summary
}
func SyncLocalFingerprint(store *Store, vault *Vault, selection ...SyncContentSelection) string {
	scope := syncSelection(selection)
	if store == nil || vault == nil {
		return ""
	}
	// Read without accepting a new Store revision: status rendering must never
	// acknowledge another process's changes on behalf of a future save.
	store.mu.Lock()
	data, err := readConfigurationFile(filepath.Join(store.Dir, "config.json"))
	var cfg Config
	if err == nil {
		if store.baseline != nil && store.observedRevision == configurationDigest(data) && (!store.protected || len(store.master) > 0) {
			cfg = store.baseline.Clone()
		} else {
			cfg, _, err = DecodeConfiguration(data, store.master)
		}
	}
	clear(data)
	store.mu.Unlock()
	if err != nil {
		return ""
	}
	vault.mu.Lock()
	if vault.blocked || vault.data.Master && len(vault.key) == 0 {
		vault.mu.Unlock()
		return ""
	}
	ids := map[string]bool{}
	for _, p := range cfg.Profiles {
		ids[strings.ToUpper(p.ID)] = true
		if p.Proxy.ID != "" {
			ids[strings.ToUpper(ProxyCredentialProfile(p.Proxy).ID)] = true
		}
	}
	for _, p := range cfg.Proxies {
		ids[strings.ToUpper(ProxyCredentialProfile(p.Settings).ID)] = true
	}
	entries := map[string]vaultEntry{}
	for id, entry := range vault.data.Entries {
		if ids[strings.ToUpper(id)] {
			entries[id] = entry
		}
	}
	if !scope.Sessions {
		cfg.Profiles, cfg.Proxies, cfg.Directories, cfg.Links, cfg.LinkRootOrder = nil, nil, nil, nil, nil
		entries = nil
	}
	// Hash selected categories, their encrypted credentials and master protection.
	// WebDAV credentials and unselected/device-only settings never make it dirty.
	body, err := json.Marshal(struct {
		Profiles    []Profile
		Proxies     []ProxyProfile
		Directories []string
		Links       []SessionLink
		RootOrder   []string
		Credentials map[string]vaultEntry
		Master      bool
		Salt        []byte
	}{cfg.Profiles, cfg.Proxies, cfg.Directories, cfg.Links, cfg.LinkRootOrder, entries, vault.data.Master, vault.data.Salt})
	vault.mu.Unlock()
	if err != nil {
		return ""
	}
	if scope.ClientSettings || scope.QuickCommands || scope.SessionDefaults {
		extra, err := json.Marshal(sessionSyncDocument(SharedDocument{Preferences: preferencesForShared(cfg), HighlightSets: cfg.HighlightSets, QuickCommands: cfg.QuickCommands, Defaults: cfg.SessionDefaults, Links: SharedLinks{Visible: cfg.Settings.LinksVisible}}, SyncContentSelection{ClientSettings: scope.ClientSettings, QuickCommands: scope.QuickCommands, SessionDefaults: scope.SessionDefaults}))
		if err != nil {
			return ""
		}
		body = append(body, extra...)
	}
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

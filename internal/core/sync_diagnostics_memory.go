package core

import "time"

// Protected by syncDiagnosticsMu. Only failed writes retain unsaved history.
// A small clean cache lets a newly inaccessible log retain its last success;
// ordinary successful writes do not grow process memory with old log paths.
type syncDiagnosticCached struct {
	document SyncDiagnosticDocument
	use      uint64
}

var syncDiagnosticUnsaved = map[string]SyncDiagnosticDocument{}
var syncDiagnosticClean = map[string]syncDiagnosticCached{}
var syncDiagnosticUse uint64

func cloneSyncDiagnostic(doc SyncDiagnosticDocument) SyncDiagnosticDocument {
	out := doc
	if doc.Summary.Failure != nil {
		failure := *doc.Summary.Failure
		out.Summary.Failure = &failure
	}
	out.Entries = append([]SyncDiagnosticEntry(nil), doc.Entries...)
	for i, entry := range out.Entries {
		if entry.Failure != nil {
			failure := *entry.Failure
			out.Entries[i].Failure = &failure
		}
	}
	if doc.Summary.LastAttempt != nil {
		value := *doc.Summary.LastAttempt
		out.Summary.LastAttempt = &value
	}
	if doc.Summary.LastSuccess != nil {
		value := *doc.Summary.LastSuccess
		out.Summary.LastSuccess = &value
	}
	return out
}
func pruneSyncDiagnostic(doc SyncDiagnosticDocument) SyncDiagnosticDocument {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	entries := make([]SyncDiagnosticEntry, 0, min(len(doc.Entries), 2000))
	for _, entry := range doc.Entries {
		if !entry.Date.Before(cutoff) {
			entries = append(entries, entry)
		}
	}
	if len(entries) > 2000 {
		entries = entries[len(entries)-2000:]
	}
	doc.Entries = entries
	return doc
}
func rememberSyncDiagnostic(path string, doc SyncDiagnosticDocument) {
	syncDiagnosticUse++
	syncDiagnosticClean[path] = syncDiagnosticCached{cloneSyncDiagnostic(doc), syncDiagnosticUse}
	if len(syncDiagnosticClean) > 32 {
		oldest := ""
		age := ^uint64(0)
		for key, value := range syncDiagnosticClean {
			if value.use < age {
				oldest = key
				age = value.use
			}
		}
		delete(syncDiagnosticClean, oldest)
	}
}
func (d SyncDiagnostics) WriteIssue() string {
	syncDiagnosticsMu.Lock()
	defer syncDiagnosticsMu.Unlock()
	if _, ok := syncDiagnosticUnsaved[d.path()]; ok {
		return "诊断日志未能写入磁盘；当前运行仍可查看和导出内存记录"
	}
	return ""
}

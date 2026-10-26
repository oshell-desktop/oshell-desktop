// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var ErrSyncDeferred = errors.New("共享端已接收本次修改，本机有新变化，等待下一轮同步合并")

type SharedSyncEngine struct {
	Selection *SyncContentSelection
	Manual    bool
	Progress  func(SyncEvent, error)
	Store     *Store
	Vault     *Vault
	Transport SyncTransport
	Replica   SharedReplicaStore
}
type SharedSyncPlan struct {
	selection                        SyncContentSelection
	fullLocal                        SharedDocument
	protocolUpgrade                  bool
	runID                            string
	started                          time.Time
	originalLocal                    SharedDocument
	Local, Remote                    SharedDocument
	HasRemote, InitialChoiceRequired bool
	Conflicts                        []SharedMergeConflict
	configuration                    Config
	revision, vaultRevision, etag    string
	digests                          map[string]string
	baseline                         *SharedDocument
	compatibility                    *SharedDocument
	merge                            *SharedMerge
	engine                           *SharedSyncEngine
}

func sameShared(a, b SharedDocument) bool {
	left, err := a.canonicalProtocol()
	if err != nil {
		return false
	}
	right, err := b.canonicalProtocol()
	if err != nil {
		return false
	}
	x, _ := json.Marshal(left)
	y, _ := json.Marshal(right)
	return equalSharedJSON(x, y)
}
func (e *SharedSyncEngine) Prepare(ctx context.Context, cfg Config, master []byte, allowCreate bool) (_ *SharedSyncPlan, resultErr error) {
	selection := DefaultSyncPolicy().Content
	if e.Selection != nil {
		selection = *e.Selection
	}
	if selection == (SyncContentSelection{}) {
		return nil, fmt.Errorf("请至少选择一类同步内容")
	}
	started := time.Now()
	runID := NewID()
	e.recordDiagnostic(SyncStarted, started, nil, runID)
	defer func() {
		if resultErr != nil {
			event := SyncFailed
			if errors.Is(resultErr, context.Canceled) {
				event = SyncCancelled
			}
			e.recordDiagnostic(event, started, resultErr, runID)
		}
	}()
	protected, locked := e.Vault.State()
	if !protected || locked {
		return nil, fmt.Errorf("同步需要已解锁的主密码保护")
	}
	revision, err := e.Store.Revision()
	if err != nil {
		return nil, err
	}
	vaultRevision := e.Vault.Revision()
	replica, err := e.Replica.Load(master)
	if err != nil {
		return nil, err
	}
	var cached, baseline *SharedDocument
	var digests map[string]string
	if replica != nil {
		cached = replica.CachedLocal
		baseline = replica.Baseline
		digests = replica.CredentialDigests
	}
	exportConfig := cfg.Clone()
	if !selection.Sessions {
		exportConfig.Profiles, exportConfig.Proxies, exportConfig.Directories, exportConfig.Links, exportConfig.LinkRootOrder = nil, nil, nil, nil, nil
	}
	local, digests, err := e.Vault.ExportSharedDocument(ctx, exportConfig, master, cached, digests)
	if err != nil {
		return nil, err
	}
	fullLocal := local
	draft, err := e.ConflictStore().Load(master)
	if err != nil {
		return nil, err
	}
	if draft != nil {
		if draft.Revision != revision || draft.VaultRevision != vaultRevision {
			return nil, fmt.Errorf("本机配置或密码库在冲突后已变化，原草稿已保留；请先确认并放弃旧草稿，再重新同步")
		}
		local = draft.Local
		baseline = draft.Base
	}
	// Validate full documents before projecting them. Unknown or invalid legacy
	// settings must still fail closed rather than being silently discarded.
	local = sessionSyncDocument(local, selection)
	if baseline != nil {
		projected := sessionSyncDocument(*baseline, selection)
		baseline = &projected
	}
	object, err := e.Transport.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	e.recordDiagnostic(SyncFetched, started, nil, runID)
	plan := &SharedSyncPlan{runID: runID, started: started, Local: local, originalLocal: local, configuration: cfg.Clone(), revision: revision, vaultRevision: vaultRevision, digests: digests, baseline: baseline, engine: e}
	plan.selection, plan.fullLocal = selection, fullLocal
	if object == nil {
		if baseline != nil || draft != nil || !allowCreate {
			return nil, safeSyncFailure("remote-deleted")
		}
		return plan, nil
	}
	remote, err := DecodeSharedDocument(object.Data, master)
	if err != nil {
		if errors.Is(err, ErrSharedProtocol) {
			return nil, err
		}
		return nil, safeSyncFailure("encryption")
	}
	// Both verifiers were authenticated with the same shared password. Random
	// envelope nonces do not mean that password protection itself changed.
	remote.Verifier = local.Verifier
	compatibility := remote
	plan.compatibility = &compatibility
	plan.Remote = sessionSyncDocument(remote, selection)
	var header struct{ Version int }
	json.Unmarshal(object.Data, &header)
	plan.protocolUpgrade = remote.needsProtocolUpgrade() || header.Version != 2
	remote = plan.Remote
	plan.HasRemote = true
	plan.etag = object.ETag
	if sameShared(local, remote) {
		return plan, nil
	}
	if baseline == nil {
		plan.InitialChoiceRequired = true
		e.recordDiagnostic(SyncConflict, started, nil, runID)
		if err = plan.saveConflictDraft(master); err != nil {
			return nil, err
		}
		return plan, nil
	}
	base := *baseline
	base.Verifier = local.Verifier
	plan.merge, err = NewSharedMerge(base, local, remote)
	if err != nil {
		return nil, err
	}
	plan.Conflicts = plan.merge.Conflicts
	if len(plan.Conflicts) > 0 {
		e.recordDiagnostic(SyncConflict, started, nil, runID)
		if err = plan.saveConflictDraft(master); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

// ChooseInitial selects shared sessions or merges both session collections and
// their dependencies. Client settings remain local in either case.
func (p *SharedSyncPlan) ChooseInitial(mergeLocal bool) error {
	if !p.InitialChoiceRequired {
		return fmt.Errorf("无需首次同步选择")
	}
	p.InitialChoiceRequired = false
	if !mergeLocal {
		p.Local = p.Remote
		return nil
	}
	merge, err := newInitialSessionMerge(p.Local, p.Remote, p.selection)
	if err != nil {
		return err
	}
	p.merge = merge
	p.Conflicts = merge.Conflicts
	return nil
}
func (p *SharedSyncPlan) Commit(ctx context.Context, choices map[string]SharedMergeChoice, master []byte) (_ Config, resultErr error) {
	e := p.engine
	started := p.started
	if started.IsZero() {
		started = time.Now()
	}
	runID := p.runID
	defer func() {
		if resultErr != nil {
			event := SyncFailed
			issue := resultErr
			if errors.Is(resultErr, ErrSyncDeferred) {
				event = SyncQueued
				issue = nil
			}
			if errors.Is(resultErr, context.Canceled) {
				event = SyncCancelled
				issue = resultErr
			}
			e.recordDiagnostic(event, started, issue, runID)
		}
	}()
	if p.InitialChoiceRequired {
		return p.configuration, fmt.Errorf("请先选择如何处理已有共享数据")
	}
	document := p.Local
	if p.merge != nil {
		var err error
		document, err = p.merge.Resolve(choices, master)
		if err != nil {
			return p.configuration, err
		}
	}
	revision, err := e.Store.Revision()
	if err != nil {
		return p.configuration, err
	}
	if revision != p.revision || e.Vault.Revision() != p.vaultRevision {
		return p.configuration, fmt.Errorf("选择期间本机配置或密码库发生变化，请重新同步")
	}

	if p.HasRemote && !p.protocolUpgrade && sameShared(p.originalLocal, p.Remote) && sameShared(document, p.Remote) {
		if p.baseline == nil || !sameShared(*p.baseline, document) {
			replica := SharedReplica{Version: 1, Target: e.Replica.Target, Baseline: &document, CachedLocal: &p.Local, CredentialDigests: p.digests}
			if err = e.Replica.Save(replica, master); err != nil {
				return p.configuration, err
			}
		}
		if err = e.ConflictStore().Clear(); err != nil {
			return p.configuration, fmt.Errorf("同步完成，但冲突草稿清理失败：%w", err)
		}
		e.recordDiagnostic(SyncUnchanged, started, nil, runID)
		return p.configuration, nil
	}
	replacement := sessionSyncWire(document, &p.fullLocal, p.selection)
	var next Config
	var updates []CredentialUpdate
	if p.selection.Sessions {
		next, updates, err = replacement.PrepareLocalReplacement(ctx, p.configuration, master)
	} else {
		next, err = replacement.Configuration()
	}
	if err != nil {
		return p.configuration, err
	}
	next = applySessionSync(next, p.configuration, p.selection)
	wire := sessionSyncWire(document, p.compatibility, p.selection)
	encrypted, err := wire.Encode(master)
	if err != nil {
		return p.configuration, err
	}
	if p.baseline == nil && p.HasRemote {
		backup, err := p.configurationBackup(master)
		if err != nil {
			return p.configuration, err
		}
		dir := filepath.Join(e.Store.Dir, "storage-backups")
		if err = os.MkdirAll(dir, 0700); err != nil {
			return p.configuration, err
		}
		if err = AtomicWrite(filepath.Join(dir, NewID()+".json"), backup); err != nil {
			return p.configuration, err
		}
	}
	uploaded := !p.HasRemote || p.protocolUpgrade || !sameShared(document, p.Remote)
	if uploaded {
		e.recordDiagnostic(SyncUploading, started, nil, runID)
		if err = e.Transport.Put(ctx, encrypted, p.etag); err != nil {
			return p.configuration, err
		}
	} else {
		// Even a download-only merge must not apply a remote revision that changed
		// while the conflict dialog was open.
		latest, err := e.Transport.Fetch(ctx)
		if err != nil {
			return p.configuration, err
		}
		if latest == nil || latest.ETag != p.etag {
			return p.configuration, safeSyncFailure("conflict")
		}
	}
	e.recordDiagnostic(SyncApplying, started, nil, runID)
	if err = e.Store.commitCredentials(ctx, next, e.Vault, updates, p.revision, p.vaultRevision); err != nil {
		if uploaded && (errors.Is(err, ErrConfigurationChanged) || errors.Is(err, ErrVaultChanged) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			if checkpointErr := p.saveUploadContinuation(master); checkpointErr != nil {
				return p.configuration, errors.Join(err, fmt.Errorf("上传已完成，但无法保存后续同步基准：%w", checkpointErr))
			}
			return p.configuration, errors.Join(ErrSyncDeferred, err)
		}
		return p.configuration, err
	}
	// The cached export uses local ciphertext digests after the transaction.
	exportConfig := next.Clone()
	if !p.selection.Sessions {
		exportConfig.Profiles, exportConfig.Proxies, exportConfig.Directories, exportConfig.Links, exportConfig.LinkRootOrder = nil, nil, nil, nil, nil
	}
	cached, digests, err := e.Vault.ExportSharedDocument(ctx, exportConfig, master, &document, p.digests)
	if err != nil {
		return next, fmt.Errorf("配置已同步，但无法更新同步基准：%w", err)
	}
	replica := SharedReplica{Version: 1, Target: e.Replica.Target, Baseline: &document, CachedLocal: &cached, CredentialDigests: digests}
	if err = e.Replica.Save(replica, master); err != nil {
		return next, fmt.Errorf("配置已同步，但无法保存同步基准：%w", err)
	}
	if err = e.ConflictStore().Clear(); err != nil {
		return next, fmt.Errorf("同步完成，但冲突草稿清理失败：%w", err)
	}
	e.recordDiagnostic(SyncCompleted, started, nil, runID)
	return next, nil
}

// A successful PUT published the captured local changes, even if a later local
// edit prevents applying the merge. Use the pre-merge LOCAL snapshot as the next
// baseline: using the merged document would treat unapplied remote additions as
// local deletions. Never advance this baseline on failed PUTs or download-only
// attempts, and never save it with a stale or locked master password.
func (p *SharedSyncPlan) saveUploadContinuation(master []byte) error {
	e := p.engine
	e.Vault.mu.Lock()
	defer e.Vault.mu.Unlock()
	if e.Vault.blocked {
		return ErrPendingTransaction
	}
	if len(e.Vault.master) == 0 || subtle.ConstantTimeCompare(e.Vault.master, master) != 1 {
		return ErrVaultLocked
	}
	release, err := lockStateDirectory(context.Background(), e.Store.Dir, false)
	if err != nil {
		return err
	}
	defer release()
	if err = e.Vault.checkDiskRevisionLocked(); err != nil {
		return err
	}
	replica := SharedReplica{Version: 1, Target: e.Replica.Target, Baseline: &p.originalLocal, CachedLocal: &p.originalLocal, CredentialDigests: p.digests}
	if err = e.Replica.Save(replica, master); err != nil {
		return err
	}
	// The choices in this draft have already been published remotely. Keeping
	// its old local revision would block the next merge with a stale-draft error.
	return e.ConflictStore().Clear()
}
func (p *SharedSyncPlan) configurationBackup(master []byte) ([]byte, error) {
	// ChooseInitial may replace Local with Remote; export the captured original
	// configuration again so the first-sync backup never contains the replacement.
	document, _, err := p.engine.Vault.ExportSharedDocument(context.Background(), p.configuration, master, nil, nil)
	if err != nil {
		return nil, err
	}
	return document.Encode(master)
}

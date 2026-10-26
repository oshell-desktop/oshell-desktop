// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
)

type pane struct {
	fileDisplayIP          string
	search                 terminalSearchState
	externalLaunch         bool
	terminalOnly           bool
	disposal               sync.WaitGroup
	hostProbe              hostProbeState
	promptScheduled        atomic.Bool
	lastHostPrompt         string
	hostObservation        core.HostObservation
	localNames             core.LocalHostname
	visible, pendingUnread atomic.Bool
	hostname               string
	titleMode              string // Runtime choice for unsaved local sessions; moves with the pane.
	transfer               transferNoticeState
	ftp                    *core.FTPSession
	owner                  atomic.Pointer[App]
	id                     string
	profile                core.Profile
	term                   *terminal.Terminal
	conn                   *core.Connection
	fileSource             *core.Connection // SSH session that opened this file pane, including while connecting.
	cancel                 context.CancelFunc
	status                 string
	closed                 bool
	focus                  ui.Handle
	files                  *fileState
}
type node struct {
	id                                    string
	pane                                  *pane
	first, second                         *node
	vertical                              bool
	size                                  float32
	splitRatio, splitExtent, splitApplied float32
	bounds                                ui.Rect
}
type tab struct {
	hover    tabHoverState
	zoomed   *pane
	group    string
	unread   bool
	id, name string
	root     *node
	active   *pane
}
type App struct {
	linkMenuOpen                                    bool
	linkMenuAnchor, linkMenuFolder                  string
	linkScroll                                      ui.ScrollState
	workspaceAuthorized                             bool
	idleMemory                                      idleMemoryReclaimer
	imagePaste                                      *imagePasteState
	closeAfterTabMove                               bool
	tabNativeDrag                                   bool
	localUIOriginal                                 localPreferences
	localUI, localUIDraft                           localPreferences
	titleOverwrite                                  bool
	draftTitleMode                                  string
	syncMonitorOpen                                 bool
	syncMonitor                                     *mygo.Window
	sidebar                                         sidebarState
	storageDirectoryDraft, storageDirectoryOriginal string
	storagePickerGeneration                         uint64
	quickSendScope                                  string
	settingsPreviewApplied, settingsPreviewDark     bool
	focusFullscreen                                 focusFullscreenState
	lastInputFocus                                  ui.Handle
	clockPending                                    atomic.Bool
	clockText                                       string
	clockSizeDraft                                  string
	clockFollowsText                                bool
	configBusy                                      bool
	groups                                          groupState
	challenges                                      []*authenticationChallenge
	preferencesWindow                               *mygo.Window
	hub                                             *applicationHub
	store                                           *core.Store
	config                                          core.Config
	trust                                           *core.TrustStore
	win                                             *mygo.Window
	events                                          []func()
	eventMu                                         sync.Mutex
	stopped                                         atomic.Bool
	tabs                                            []*tab
	current                                         int
	manager                                         *mygo.Window
	managerGroup                                    string
	managerProtocol, managerLastProtocol            string
	managerAll                                      bool
	managerSearchScope                              string
	managerCache                                    *managerRowsCache
	managerParentClick                              bool
	managerRow                                      int
	managerRename                                   *managerRenameState
	managerClipboard                                *sessionConfigurationClipboard
	authPasswordRevealed                            bool
	managerList                                     *ui.ListState
	history                                         []string
	query, selected, status, quick                  string
	editing, authOpen, confirmSend                  bool
	draft                                           core.Profile
	port, formError, password, keyPassword          string
	connectAfter                                    bool
	pendingProfile                                  core.Profile
	pendingSplit                                    int
	settingsOpen                                    bool
	filesVisible                                    bool
	features                                        featureState
	vault                                           *core.Vault
	authQueue                                       []core.Profile
	batchTerminalOpened                             bool
	batchTerminalOwner                              *App
	managerSelection                                ui.Selection[string]
}

func Run() error { return runDesktop() }
func newApp(s *core.Store, c core.Config) *App {
	v, err := core.OpenVault(s.Dir)
	a := &App{store: s, config: c, trust: &core.TrustStore{Path: filepath.Join(s.Dir, "known_hosts")}, managerRow: -1, managerList: &ui.ListState{}, status: "就绪", vault: v}
	a.localUI = loadLocalPreferences(s.Dir)
	if err != nil {
		a.status = "密码库读取失败：" + err.Error()
	}
	_, lockedConfig := s.ConfigurationProtection()
	if !lockedConfig {
		a.quickSendScope = c.Settings.QuickSendScope
	}
	a.workspaceAuthorized = !a.passwordLocked()
	return a
}
func (a *App) post(fn func()) bool {
	a.eventMu.Lock()
	if a.stopped.Load() {
		a.eventMu.Unlock()
		return false
	}
	a.events = append(a.events, fn)
	a.eventMu.Unlock()
	if a.win != nil {
		a.win.Invalidate()
	}
	return true
}
func (a *App) drain() {
	a.eventMu.Lock()
	pending := a.events
	a.events = nil
	a.eventMu.Unlock()
	for _, fn := range pending {
		fn()
	}
	a.finishTabMoveClose()
	a.resumeDeferredUpdate()
	if len(pending) > 0 && a.preferencesWindow != nil {
		a.preferencesWindow.Invalidate()
	}
	if len(pending) > 0 && a.manager != nil {
		a.manager.Invalidate()
	}
}
func (a *App) save() bool {
	if a.passwordLocked() {
		a.status = "错误：请先解锁再修改配置"
		return false
	}
	if a.configurationBusy() {
		a.status = "错误：配置事务正在进行，请稍后重试"
		return false
	}

	previous := a.config
	candidate := previous.Clone()
	candidate.Version = 3
	if e := candidate.MigrateProxyCatalog(); e != nil {
		a.status = "保存失败：" + e.Error()
		return false
	}
	a.config = candidate
	if a.hub != nil {
		if !a.hub.save(a) {
			a.config = previous
			return false
		}
		return true
	}
	if err := a.store.Save(a.config); err != nil {
		a.status = "保存失败：" + err.Error()
		if localConflictError(err) {
			a.beginLocalConflict(candidate, nil)
		}
		a.config = previous
		return false
	}
	a.markSharedSyncChanged()
	return true
}
func (a *App) activeTab() *tab {
	if a.current < 0 || a.current >= len(a.tabs) {
		return nil
	}
	return a.tabs[a.current]
}
func (a *App) activePane() *pane {
	if t := a.activeTab(); t != nil {
		return t.active
	}
	return nil
}
func (a *App) walk(n *node, fn func(*pane)) {
	if n == nil {
		return
	}
	if n.pane != nil {
		fn(n.pane)
		return
	}
	a.walk(n.first, fn)
	a.walk(n.second, fn)
}
func (a *App) dispose(p *pane) {
	if p.closed {
		return
	}
	p.closed = true
	p.search.matches = nil
	p.search.generation++
	if s := a.imagePaste; s != nil && s.pane == p {
		if s.cancel != nil {
			s.cancel()
		}
		a.imagePaste = nil
	}
	for _, q := range append([]*authenticationChallenge(nil), a.challenges...) {
		if q.pane == p {
			a.finishChallenge(q, true)
		}
	}
	a.cancelHostProbe(p)
	delete(a.features.syncTargets, p.id)
	a.pruneSyncTargets()
	if p.cancel != nil {
		p.cancel()
	}
	if p.ftp != nil {
		p.disposal.Add(1)
		go func() { defer p.disposal.Done(); p.ftp.Close() }()
	}
	if p.files != nil {
		if p.files.readCancel != nil {
			p.files.readCancel()
		}
		if p.files.cancel != nil {
			p.files.cancel()
		}
	}
	if p.files != nil && p.files.client != nil {
		files := p.files
		p.disposal.Add(1)
		go func() {
			defer p.disposal.Done()
			files.client.Close()
			if files.lease != nil {
				files.lease()
			}
		}()
	}
	if p.term != nil {
		p.disposal.Add(1)
		go func() { defer p.disposal.Done(); p.term.Close() }()
	} else if p.conn != nil {
		p.disposal.Add(1)
		go func() { defer p.disposal.Done(); p.conn.Close() }()
	}
}
func (a *App) closeTab(t *tab) {
	active := a.activeTab()
	g := a.groupByID(t.group)
	a.walk(t.root, a.dispose)
	for i, v := range a.tabs {
		if v == t {
			a.tabs = slices.Delete(a.tabs, i, i+1)
			break
		}
	}
	a.current = -1
	if active != nil && active != t {
		for i, v := range a.tabs {
			if v == active {
				a.current = i
				break
			}
		}
	}
	if a.groups.root != nil {
		a.pruneGroups()
		if active == t && g != nil && a.groupByID(g.id) != nil && !g.hidden {
			a.activateGroup(g)
		}
	} else {
		a.current = min(a.current, len(a.tabs)-1)
	}
	a.scheduleIdleMemoryReclaim()
}

// Ended-session exit/quit must not steal focus from an explicit command editor.
func (a *App) closeEndedPane(t *tab, p *pane) {
	a.withCommandEditorFocus(func() { a.closePane(t, p) })
}
func (a *App) withCommandEditorFocus(close func()) {
	restore := a.lastInputFocus
	o := &a.features.operators
	keep := restore != (ui.Handle{}) && ((a.config.Settings.QuickSendVisible && restore == o.quickFocus) || (o.composerOpen && restore == o.composerFocus))
	close()
	if keep {
		// Group pruning may have queued focus for a newly revealed terminal.
		// Cancel that request before restoring the command editor.
		for _, remaining := range a.allPanes() {
			remaining.focus.CancelFocus()
		}
		restore.Focus()
	}
}

func (a *App) closePane(t *tab, p *pane) {
	if t.zoomed == p {
		t.zoomed = nil
	}
	if t.root.pane == p {
		a.closeTab(t)
		return
	}
	var remove func(*node) *node
	remove = func(n *node) *node {
		if n == nil {
			return nil
		}
		if n.pane == p {
			return nil
		}
		if n.pane != nil {
			return n
		}
		n.first = remove(n.first)
		n.second = remove(n.second)
		if n.first == nil {
			return n.second
		}
		if n.second == nil {
			return n.first
		}
		return n
	}
	t.root = remove(t.root)
	a.dispose(p)
	if t.active == p {
		t.active = nil
		a.walk(t.root, func(next *pane) {
			if t.active == nil {
				t.active = next
			}
		})
		if t.active != nil {
			t.active.focus.Focus()
		}
	}
}
func (a *App) local() {
	p := core.NewProfile()
	p.Name = "本地终端"
	p.Kind = "local"
	a.open(p, core.Credentials{}, 0)
}
func (a *App) request(p core.Profile, split int, terminalMode ...bool) {
	asTerminal := len(terminalMode) > 0 && terminalMode[0] && p.Kind == "sftp"
	if a.passwordLocked() {
		a.features.startupUnlock = true
		a.showVault()
		return
	}
	if a.stopped.Load() || a.hub != nil && a.hub.quitting {
		return
	}
	if a.hub != nil {
		target := a
		if p.Kind == "ftp" || p.Kind == "sftp" && !asTerminal {
			target = a.hub.externalFileWindow()
		} else if a == a.hub.fileWindow && p.Kind != "ssh" {
			target = a.hub.focused(nil)
		}
		if target != a {
			a.closeManager()
			target.authQueue = append(target.authQueue, a.authQueue...)
			a.authQueue = nil
			target.batchTerminalOpened = a.batchTerminalOpened
			target.batchTerminalOwner = a.batchTerminalOwner
			target.request(p, split, asTerminal)
			if target.win != nil {
				target.win.Focus()
			}
			return
		}
	}
	resolvedProfile, e := a.config.ConnectionProfile(p)
	if e != nil {
		a.status = "错误：" + e.Error()
		return
	}
	p = resolvedProfile
	a.closeManager()
	if p.Kind == "local" || p.Kind == "blank" {
		a.open(p, core.Credentials{}, split)
		return
	}
	a.features.pendingTerminal = asTerminal
	a.pendingProfile = p
	a.pendingSplit = split
	a.password = ""
	a.authPasswordRevealed = false
	a.keyPassword = ""
	a.features.proxyPassword = ""
	a.features.proxyKeyPassword = ""
	a.features.savePassword = false
	a.features.passwordProtection = "local"
	if a.vault != nil {
		a.features.passwordProtection = a.vault.CredentialProtection(p.ID)
	}
	if a.features.authCancel != nil {
		a.features.authCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.features.authCancel = cancel
	a.features.authGeneration++
	generation := a.features.authGeneration
	a.features.authPreparing = true
	a.features.savedPassword = ""
	a.features.savedIdentity = nil
	a.authOpen = true
	probeKeys := a.vault == nil || !a.vault.HasStoredCredential(p.ID)
	go func() {
		resolved, e := core.ResolveSSH(ctx, p)
		keyCandidate := false
		if e == nil && probeKeys {
			keyCandidate = core.SSHKeyAuthenticationCandidate(ctx, resolved.Profile)
		}
		a.post(func() {
			if generation != a.features.authGeneration || !a.authOpen || ctx.Err() != nil || a.stopped.Load() {
				return
			}
			a.features.authPreparing = false
			if e != nil {
				a.status = "SSH 配置解析失败：" + e.Error()
				return
			}
			identity := resolved.Profile.Identity()
			a.features.savedIdentity = &identity
			if a.vault != nil {
				saved, e := a.vault.GetFor(p, identity)
				if e == core.ErrVaultLocked {
					a.authOpen = false
					a.features.vaultOpen = true
					a.features.vaultAction = "unlock"
					a.features.resumeAuth = true
					return
				}
				if e != nil {
					a.status = "密码读取失败：" + e.Error()
					return
				}
				if saved != "" {
					a.authOpen = false
					a.password = ""
					a.authPasswordRevealed = false
					a.keyPassword = ""
					a.features.savedPassword = ""
					a.features.savedIdentity = nil
					a.open(p, core.Credentials{Password: saved, ExpectedIdentity: &identity, OpenTerminal: asTerminal}, split)
					a.nextAuthentication()
					return
				}
				a.password = saved
				a.features.savedPassword = saved
				a.features.savedIdentity = &identity
			}
			if keyCandidate && (a.vault == nil || !a.vault.HasStoredCredential(p.ID)) {
				a.authOpen = false
				a.password = ""
				a.authPasswordRevealed = false
				a.keyPassword = ""
				a.features.savedPassword = ""
				a.features.savedIdentity = nil
				a.open(p, core.Credentials{ExpectedIdentity: &identity, OpenTerminal: asTerminal}, split)
				a.nextAuthentication()
			}
		})
	}()

	a.authOpen = true
}
func (a *App) edit(p core.Profile) {
	if a.passwordLocked() || a.configurationBusy() || a.features.sessionPassword.busy {
		return
	}
	a.features.managerReturn = nil
	a.closeManager()
	p.Tunnels = append([]core.TunnelRule(nil), p.Tunnels...)
	a.draft = p
	a.initConnectionEditor()
	a.port = fmt.Sprint(p.Port)
	a.formError = ""
	a.editing = true
	if a.win != nil {
		a.win.Show()
		a.win.Focus()
		a.win.Invalidate()
	}
}

func (a *App) open(profile core.Profile, cred core.Credentials, split int) {
	a.openConnection(profile, cred, split, nil)
}
func (a *App) openConnection(profile core.Profile, cred core.Credentials, split int, source *core.Connection, ftpSources ...*core.FTPSession) {
	if a.passwordLocked() || a.stopped.Load() || a.hub != nil && a.hub.quitting {
		return
	}
	a.cancelIdleMemoryReclaim()
	// Retain the source before replacing/disposal of a pane that owns its last lease.
	sourceRelease := func() {}
	if source != nil {
		var err error
		sourceRelease, err = source.Lease()
		if err != nil {
			a.status = "无法复用 SSH 连接：" + err.Error()
			return
		}
	}
	var ftpClone *core.FTPSession
	if profile.Kind == "ftp" && len(ftpSources) > 0 && ftpSources[0] != nil {
		var err error
		ftpClone, err = ftpSources[0].Clone()
		if err != nil {
			sourceRelease()
			a.status = "FTP 复制失败：" + err.Error()
			return
		}
	}
	a.groupForNewTab()
	filesOnly := a.opensAsFileSession(profile) && !cred.OpenTerminal
	ctx, cancel := context.WithCancel(context.Background())
	p := &pane{externalLaunch: cred.ExternalLaunch, terminalOnly: cred.OpenTerminal, hostname: profile.Host, id: core.NewID(), profile: profile, cancel: cancel, status: "正在连接…"}
	if filesOnly {
		p.fileSource = source
	}
	p.owner.Store(a)
	a.initializeHostname(p)
	n := &node{id: core.NewID(), pane: p}
	t := a.activeTab()
	forceNew := split == -2
	if forceNew {
		split = 0
	}
	if !forceNew && split == 0 && !filesOnly && profile.Kind != "blank" && t != nil && reusableIdlePane(t.active) {
		a.features.reconnectTarget = t.active
		split = -1
	}
	if split == -1 {
		old := a.features.reconnectTarget
		a.features.reconnectTarget = nil
		if old == nil || old.closed || a.ownerTab(old) == nil {
			sourceRelease()
			if ftpClone != nil {
				ftpClone.Close()
			}
			cancel()
			return
		}
		t = a.ownerTab(old)
		var replace func(*node)
		replace = func(v *node) {
			if v == nil {
				return
			}
			if v.pane == old {
				v.pane = p
				return
			}
			replace(v.first)
			replace(v.second)
		}
		wasZoomed := t.zoomed == old
		if old.profile.ID == profile.ID && old.profile.Identity() == profile.Identity() {
			p.files = reconnectFileState(old.files)
		}
		replace(t.root)
		a.dispose(old)
		t.active = p
		t.name = profile.Name
		if wasZoomed {
			t.zoomed = p
		}
		a.selectTab(t)
	} else if split != 0 && t != nil {
		var replace func(*node)
		replace = func(v *node) {
			if v.pane == t.active {
				old := *v
				*v = node{id: core.NewID(), first: &old, second: n, vertical: split == 2, splitRatio: .5}
				return
			}
			if v.pane == nil {
				replace(v.first)
				replace(v.second)
			}
		}
		replace(t.root)
		t.active = p
	} else {
		t = &tab{group: a.groups.active, id: core.NewID(), name: profile.Name, root: n, active: p}
		a.tabs = append(a.tabs, t)
		a.setCurrentTab(t, true)
	}
	post := func(fn func()) bool {
		owner := p.owner.Load()
		if owner == nil {
			return false
		}
		return owner.post(fn)
	}
	settings := a.config.Settings
	credentialVault := a.vault
	vaultRevision := ""
	if credentialVault != nil {
		vaultRevision = credentialVault.Revision()
	}
	requireExistingProfile := false
	allowPasswordSave := canSaveSessionPassword(profile, cred, credentialVault)
	// External launcher credentials and subsequent challenges stay one-shot.
	if !allowPasswordSave {
		cred.SavePassword = false
	}
	for _, stored := range a.config.Profiles {
		if stored.ID == profile.ID && credentialVault != nil {
			requireExistingProfile = true
			break
		}
	}
	passwordConsent := map[string]bool{cred.Password: cred.SavePassword}
	passwordModes := map[string]string{cred.Password: cred.PasswordProtection}
	cred.PasswordAuthenticated = func(password string) {
		cred.Password = password
		cred.SavePassword = passwordConsent[password]
		cred.PasswordProtection = passwordModes[password]
	}
	cred.PasswordChallenge = func(ctx context.Context, request core.AuthenticationPrompt) (string, error) {
		result := askAuthenticationWithSaving(ctx, p, request, allowPasswordSave)
		if result.err != nil {
			return "", result.err
		}
		if len(result.values) != 1 {
			return "", fmt.Errorf("密码回答无效")
		}
		passwordConsent[result.values[0]] = result.remember
		passwordModes[result.values[0]] = result.protection
		return result.values[0], nil
	}
	cred.MixedPasswordChallenge = func(ctx context.Context, request core.AuthenticationPrompt, index int) ([]string, error) {
		result := askAuthenticationWithSaving(ctx, p, request, allowPasswordSave)
		if result.err != nil {
			return nil, result.err
		}
		if len(result.values) != len(request.Questions) || index < 0 || index >= len(result.values) {
			return nil, fmt.Errorf("密码回答无效")
		}
		passwordConsent[result.values[index]] = result.remember
		passwordModes[result.values[index]] = result.protection
		return result.values, nil
	}
	cred.Challenge = func(ctx context.Context, request core.AuthenticationPrompt) ([]string, error) {
		return askAuthentication(ctx, p, request)
	}
	cred.ProxyCredentials = func(ctx context.Context, settings core.ProxySettings, identity core.SSHIdentity) (core.Credentials, error) {
		result := core.Credentials{Challenge: cred.Challenge, Method: settings.SSHAuthentication}
		if a.vault != nil {
			saved, e := a.vault.GetFor(core.ProxyCredentialProfile(settings), identity)
			if e != nil {
				return result, e
			}
			result.Password = saved
			if saved != "" {
				result.ExpectedIdentity = &identity
			}
		}
		return result, nil
	}
	go func() {
		defer sourceRelease()
		if profile.Kind == "ftp" {
			var session *core.FTPSession
			var files core.RemoteFileSystem
			var e error
			if ftpClone != nil {
				session = ftpClone
				files, e = session.OpenFiles(ctx)
				if e != nil {
					session.Close()
				}
			} else {
				session, files, e = core.LoginFTP(ctx, profile, cred.Password)
			}
			if e != nil {
				post(func() {
					if !p.closed {
						p.status = "FTP 连接失败：" + e.Error()
					}
				})
				return
			}
			if ctx.Err() != nil {
				files.Close()
				session.Close()
				return
			}
			if !post(func() {
				owner := p.owner.Load()
				if owner == nil || p.closed {
					files.Close()
					session.Close()
					return
				}
				p.ftp = session
				owner.resolveFileSessionAddress(ctx, p)
				if p.files == nil {
					p.files = &fileState{dir: "/", input: "/"}
				}
				p.files.client = files
				p.files.lease = func() {}
				p.status = "已连接"
				if allowPasswordSave && !p.externalLaunch && cred.SavePassword && cred.Password != "" && owner.vault != nil {
					if saved := owner.saveAuthenticatedSessionPassword(profile, cred.Password, profile.Identity(), credentialVault, vaultRevision, requireExistingProfile, cred.PasswordProtection); saved != nil {
						p.profile.Username = saved.Username
					}
				}
				dir := profile.InitialDirectory
				if dir == "" {
					dir = "/"
				}
				owner.loadFiles(p, dir)
			}) {
				files.Close()
				session.Close()
			}
			return
		}
		var conn *core.Connection
		var err error
		if source != nil {
			if filesOnly {
				conn, err = source.CloneFiles(ctx)
			} else {
				conn, err = source.Clone(ctx, profile.KeepAlive)
			}
		} else if profile.Kind != "local" && profile.Kind != "blank" {
			dialProfile := profile
			if cred.OpenTerminal && profile.Kind == "sftp" {
				dialProfile.Kind = "ssh"
			}
			if filesOnly {
				dialProfile.Kind = "sftp"
			}
			conn, err = core.Dial(ctx, dialProfile, cred, a.trust, func(h core.HostPrompt) bool {
				return askHostAuthentication(ctx, p, h)
			})
		}
		if err != nil {
			post(func() {
				a := p.owner.Load()
				if a == nil {
					return
				}
				if !p.closed {
					p.status = "连接失败：" + err.Error()
				}
			})
			return
		}
		if ctx.Err() != nil {
			if conn != nil {
				conn.Close()
			}
			return
		}
		if allowPasswordSave && !p.externalLaunch && cred.SavePassword && cred.Password != "" && a.vault != nil && conn != nil && conn.PasswordVerified {
			post(func() {
				a := p.owner.Load()
				if a == nil {
					return
				}
				if p.closed {
					return
				}
				if saved := a.saveAuthenticatedSessionPassword(profile, cred.Password, conn.Identity, credentialVault, vaultRevision, requireExistingProfile, cred.PasswordProtection); saved != nil {
					p.profile.Username = saved.Username
				}
			})
		}
		if filesOnly {
			if !post(func() {
				a := p.owner.Load()
				if a == nil {
					return
				}
				if p.closed {
					go conn.Close()
					return
				}
				p.conn = conn
				p.status = "已连接"
				a.resolveFileSessionAddress(ctx, p)
				observeFileConnection(ctx, p, conn)
				dir := profile.InitialDirectory
				if dir == "" {
					dir = "."
				}
				a.loadFiles(p, dir)
			}) {
				conn.Close()
			}
			return
		}
		opts := terminal.Options{PreferColorEmoji: settings.PreferColorEmoji, ManagedClipboardBindings: true, AmbiguousCharactersAreWide: settings.AmbiguousCharactersAreWide, Font: terminal.Font{Family: terminalFamily(settings.FontFamily), Size: float32(settings.FontSize)}, Scrollback: settings.ScrollbackMB << 20, Theme: terminalTheme(settings.Scheme()), NoBlink: true}

		if conn != nil {
			opts.Conn = conn
		} else if profile.Kind == "blank" {
			console := terminal.NewToolsConsole()
			opts.Conn = console
			opts.ManagedInputAllowed = console.ManagedInputAllowed
		}
		a.configureTerminalTransfer(p, &opts, settings.AutoZmodem)
		transferHandler := opts.TransferHandler
		opts.TransferHandler = func(ctx context.Context, upload bool, stream net.Conn) error {
			if conn != nil {
				conn.SetManagedBusy(true)
				defer conn.SetManagedBusy(false)
			}
			return transferHandler(ctx, upload, stream)
		}
		opts.CopyOnSelect = settings.Input.CopyOnSelect
		opts.CustomWordSelection, opts.WordSeparators, opts.ShiftWhitespace = true, a.localUI.WordSeparators, a.localUI.ShiftWhitespace
		opts.KeyAliases = terminalKeyAliases(settings.KeyboardShortcuts)
		opts.RightClickPaste = settings.Input.RightClickPaste
		opts.TrimCopyLeading = settings.Input.TrimLeading
		opts.TrimCopyTrailing = settings.Input.TrimTrailing
		opts.OnUserInput = func(ev ui.InputEvent) {
			if owner := p.owner.Load(); owner != nil {
				owner.routeInput(p, ev)
			}
		}
		opts.BeforeClipboardPaste = func() bool {
			if owner := p.owner.Load(); owner != nil {
				return owner.requestClipboardImage(p)
			}
			return false
		}
		opts.BeforePaste = func(text string) bool {
			if owner := p.owner.Load(); owner != nil {
				owner.requestPaste(p, text)
			}
			return false // Workspace validates the complete destination set before sending.
		}
		opts.OnOutput = func(_ []byte) { scheduleHostnameScan(p) }
		a.configureTerminalStatus(p, &opts)
		opts.OnActivity = func() {
			if !p.visible.Load() && p.pendingUnread.CompareAndSwap(false, true) {
				post(func() {
					if owner := p.owner.Load(); owner != nil {
						owner.refreshPaneVisibility()
					}
				})
			}
		}

		opts.OnShellIdentity = func(identity core.HostIdentity) {
			post(func() {
				if owner := p.owner.Load(); owner != nil {
					owner.receiveHostIdentity(p, identity)
				}
			})
		}
		opts.OnDirectory = func(directory string) {
			post(func() {
				if owner := p.owner.Load(); owner != nil {
					owner.receiveHostnameHint(p, core.HostnameFromDirectory(directory))
				}
			})
		}
		opts.OnTitle = func(title string) {
			post(func() {
				if owner := p.owner.Load(); owner != nil {
					owner.receiveHostTitle(p, title)
				}
			})
		}
		opts.LocalToolsOnExit = profile.Kind == "ssh" || profile.Kind == "local" || cred.OpenTerminal
		opts.OnProcessState = func() { post(func() {}) }
		opts.OnSessionEnd = func() {
			post(func() {
				if owner := p.owner.Load(); owner != nil && !p.closed {
					owner.cancelHostProbe(p)
					p.status = "已断开 · 本机工具模式"
					owner.useLocalHostIdentity(p)
				}
			})
		}
		opts.OnExit = func(code int) {
			post(func() {
				a := p.owner.Load()
				if a == nil {
					return
				}
				if !p.closed {
					if profile.Kind == "blank" || opts.LocalToolsOnExit {
						if owner := a.ownerTab(p); owner != nil {
							a.closeEndedPane(owner, p)
						}
						return
					}
					p.status = fmt.Sprintf("已断开 · 退出码 %d", code)
					if conn != nil && conn.Err() != nil {
						p.status = "已断开：" + conn.Err().Error()
					}
				}
			})
		}
		term, e := terminal.New(opts)
		if e != nil {
			if opts.Conn != nil {
				opts.Conn.Close()
			}
			if conn != nil {
				conn.Close()
			}
			post(func() {
				a := p.owner.Load()
				if a == nil {
					return
				}
				if !p.closed {
					p.status = "终端初始化失败：" + e.Error()
				}
			})
			return
		}
		if conn != nil {
			conn.SetIdleGuard(term.IdleInputSafe)
		}
		if ctx.Err() != nil {
			term.Close()
			return
		}
		if !post(func() {
			a := p.owner.Load()
			if a == nil {
				return
			}
			if p.closed {
				go term.Close()
				return
			}
			p.term = term
			a.showHostname(p)
			p.conn = conn
			p.status = "已连接"
			p.focus.Focus()
		}) {
			term.Close()
		}
	}()
}

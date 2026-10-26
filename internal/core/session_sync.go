package core

import (
	"encoding/json"
	"sort"
	"strings"
)

// The v2 wire contract remains complete. Unselected categories are projected to
// defaults for comparisons and preserved verbatim when composing the wire data.
func sessionSyncDocument(d SharedDocument, selection ...SyncContentSelection) SharedDocument {
	s := syncSelection(selection)
	defaults := DefaultConfig()
	if !s.Sessions {
		d.Profiles = []PortableProfile{}
		d.Proxies = []PortableProxy{}
		d.Directories = []string{}
		d.Links.Entries = []SessionLink{}
		d.Links.Folders = []string{}
		d.Links.RootOrder = []string{}
	}
	if !s.ClientSettings {
		d.Preferences = preferencesForShared(defaults)
		d.HighlightSets = defaults.HighlightSets
		d.Links.Visible = defaults.Settings.LinksVisible
	} else {
		prefs := preferencesForShared(defaults)
		copy := make(map[string]json.RawMessage, len(d.Preferences))
		for k, v := range d.Preferences {
			copy[k] = v
		}
		for _, key := range []string{"updateRepository", "automaticUpdateChecks", "masterWarningAcknowledged"} {
			copy[key] = prefs[key]
		}
		d.Preferences = copy
	}
	if !s.QuickCommands {
		d.QuickCommands = []QuickCommand{}
	}
	if !s.SessionDefaults {
		d.Defaults = defaults.SessionDefaults
	}
	return d
}

func sessionSyncWire(sessions SharedDocument, remote *SharedDocument, selection ...SyncContentSelection) SharedDocument {
	s := syncSelection(selection)
	wire := sessionSyncDocument(sessions, s)
	if remote != nil {
		if !s.Sessions {
			wire.Profiles, wire.Proxies, wire.Directories = remote.Profiles, remote.Proxies, remote.Directories
			wire.Links.Entries, wire.Links.Folders, wire.Links.RootOrder = remote.Links.Entries, remote.Links.Folders, remote.Links.RootOrder
		}
		if !s.ClientSettings {
			wire.Preferences, wire.HighlightSets, wire.Links.Visible = remote.Preferences, remote.HighlightSets, remote.Links.Visible
		} else {
			for _, key := range []string{"updateRepository", "automaticUpdateChecks", "masterWarningAcknowledged"} {
				wire.Preferences[key] = remote.Preferences[key]
			}
		}
		if !s.QuickCommands {
			wire.QuickCommands = remote.QuickCommands
		}
		if !s.SessionDefaults {
			wire.Defaults = remote.Defaults
		}
	}
	return wire
}

func applySessionSync(shared, local Config, selection ...SyncContentSelection) Config {
	s := syncSelection(selection)
	result := local.Clone()
	if s.Sessions {
		result.Profiles = shared.Profiles
		result.Proxies = shared.Proxies
		result.Directories = shared.Directories
		result.Links = shared.Links
		result.LinkRootOrder = shared.LinkRootOrder
	}
	if s.ClientSettings {
		result.Settings = shared.Settings
		result.Settings.ScrollbackMB = local.Settings.ScrollbackMB
		result.Settings.UpdateRepository = local.Settings.UpdateRepository
		result.Settings.AutomaticUpdateChecks = local.Settings.AutomaticUpdateChecks
		result.Settings.MasterWarningAcknowledged = local.Settings.MasterWarningAcknowledged
		result.SharedPreferences = shared.SharedPreferences
		result.HighlightSets = shared.HighlightSets
	}
	if s.QuickCommands {
		result.QuickCommands = shared.QuickCommands
	}
	if s.SessionDefaults {
		result.SessionDefaults = shared.SessionDefaults
	}
	return result
}

// First-use merge must retain dependencies belonging to disjoint local and
// remote sessions. Conflicting definitions still require an explicit choice.
func newInitialSessionMerge(local, remote SharedDocument, selection ...SyncContentSelection) (*SharedMerge, error) {
	s := syncSelection(selection)
	local, remote = sessionSyncDocument(local, s), sessionSyncDocument(remote, s)
	remote.Verifier = local.Verifier
	local.Proxies, remote.Proxies = sessionUnion(local.Proxies, remote.Proxies, func(p PortableProxy) string { return strings.ToUpper(p.Definition.ID()) })
	local.Links.Entries, remote.Links.Entries = sessionUnion(local.Links.Entries, remote.Links.Entries, func(l SessionLink) string { return strings.ToUpper(l.ID) })
	directories := unionSessionStrings(remote.Directories, local.Directories)
	sort.Strings(directories)
	local.Directories, remote.Directories = directories, directories
	folders := unionSessionStrings(remote.Links.Folders, local.Links.Folders)
	sort.Strings(folders)
	local.Links.Folders, remote.Links.Folders = folders, folders
	order := unionSessionStrings(remote.Links.RootOrder, local.Links.RootOrder)
	local.Links.RootOrder, remote.Links.RootOrder = order, order
	base := remote
	base.Profiles = []PortableProfile{}
	base.Proxies = []PortableProxy{}
	base.Links.Entries = []SessionLink{}
	defaults := sessionSyncDocument(SharedDocument{})
	if s.ClientSettings {
		base.Preferences, base.HighlightSets, base.Links.Visible = defaults.Preferences, defaults.HighlightSets, defaults.Links.Visible
	}
	if s.QuickCommands {
		base.QuickCommands = defaults.QuickCommands
	}
	if s.SessionDefaults {
		base.Defaults = defaults.Defaults
	}
	return NewSharedMerge(base, local, remote)
}

func sessionUnion[T any](local, remote []T, key func(T) string) ([]T, []T) {
	own, other := map[string]T{}, map[string]T{}
	order := []string{}
	for _, item := range remote {
		id := key(item)
		order = append(order, id)
		other[id] = item
	}
	for _, item := range local {
		id := key(item)
		own[id] = item
		if _, exists := other[id]; !exists {
			order = append(order, id)
		}
	}
	l, r := make([]T, 0, len(order)), make([]T, 0, len(order))
	for _, id := range order {
		lv, lok := own[id]
		rv, rok := other[id]
		if !lok {
			lv = rv
		}
		if !rok {
			rv = lv
		}
		l, r = append(l, lv), append(r, rv)
	}
	return l, r
}

func unionSessionStrings(remote, local []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, values := range [][]string{remote, local} {
		for _, value := range values {
			// Root-order UUIDs are case insensitive; directory names are not.
			key := value
			if prefix, id, ok := strings.Cut(value, ":"); ok && uuidText.MatchString(id) {
				key = prefix + ":" + strings.ToUpper(id)
			}
			if !seen[key] {
				seen[key] = true
				out = append(out, value)
			}
		}
	}
	return out
}

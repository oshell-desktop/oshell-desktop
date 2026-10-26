package desktop

import (
	"oshell-desktop/internal/core"
	"reflect"
	"strings"
)

// Hide only untouched factory shortcuts. Stored/shared data is never deleted.
func (a *App) presentationConfig() core.Config {
	cfg := a.config.Clone()
	refs := map[string]bool{}
	for _, l := range cfg.Links {
		refs[strings.ToLower(l.ProfileID)] = true
	}
	profiles := cfg.Profiles[:0]
	hidden := false
	for _, p := range cfg.Profiles {
		if p.Kind == "local" && (p.Group == "" || p.Group == "本机") && !refs[strings.ToLower(p.ID)] && (a.vault == nil || !a.vault.HasCredential(p)) {
			factory := core.NewProfile()
			factory.ID = p.ID
			factory.Proxy.ID = p.Proxy.ID
			factory.Name = "本地终端"
			factory.Group = p.Group
			factory.Kind = "local"
			// Nil and empty tunnel lists have the same factory meaning.
			if len(p.Tunnels) == 0 {
				factory.Tunnels = p.Tunnels
			}
			if reflect.DeepEqual(factory, p) {
				hidden = true
				continue
			}
		}
		profiles = append(profiles, p)
	}
	cfg.Profiles = profiles
	if hidden {
		used := false
		for _, p := range cfg.Profiles {
			used = used || p.Group == "本机" || strings.HasPrefix(p.Group, "本机/")
		}
		for _, d := range cfg.Directories {
			used = used || strings.HasPrefix(d, "本机/")
		}
		if !used {
			dirs := cfg.Directories[:0]
			for _, d := range cfg.Directories {
				if d != "本机" {
					dirs = append(dirs, d)
				}
			}
			cfg.Directories = dirs
		}
	}
	return cfg
}

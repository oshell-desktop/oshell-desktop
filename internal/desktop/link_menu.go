package desktop

import (
	"oshell-desktop/internal/core"
	"strings"
)

func linkDropDestination(source string, target core.LinkRootItem, index int, items []core.LinkRootItem, fraction float32) (before, folder string) {
	if !strings.HasPrefix(source, "folder:") && target.Kind == "folder" && fraction > 0.2 && fraction < 0.8 {
		return "", target.Folder
	}
	if fraction < 0.5 {
		return target.Key(), ""
	}
	if index+1 < len(items) {
		return items[index+1].Key(), ""
	}
	return "", ""
}
func (a *App) currentLinkProfile(id string) (core.Profile, bool) {
	for _, link := range a.config.Links {
		if link.ID == id {
			return a.profileByID(link.ProfileID)
		}
	}
	return core.Profile{}, false
}

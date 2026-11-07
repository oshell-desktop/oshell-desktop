package desktop

import (
	"github.com/egoist/mygo/ui"
	"path"
	"sort"
	"strings"
)

type directoryPickerState struct {
	shown bool
	open  map[string]*bool
}

func (a *App) directoryTree(c *ui.Context, value *string, state *directoryPickerState) bool {
	chosen := false
	selected := strings.Trim(*value, "/")
	children := map[string][]string{}
	seen := map[string]bool{}
	for _, directory := range append(a.config.AllDirectories(), selected) {
		directory = strings.Trim(directory, "/")
		for directory != "" && directory != "." {
			parent := path.Dir(directory)
			if parent == "." {
				parent = ""
			}
			if !seen[directory] {
				children[parent] = append(children[parent], directory)
				seen[directory] = true
			}
			directory = parent
		}
	}
	for parent := range children {
		sort.Strings(children[parent])
	}
	if state.open == nil {
		state.open = map[string]*bool{}
	}
	ui.Scroll(c).Height(165).Children(func() {
		ui.Tree(c, func() {
			var row func(string)
			row = func(directory string) {
				label := path.Base(directory)
				if directory == "" {
					label = "/（根目录）"
				}
				var expanded *bool
				if len(children[directory]) > 0 {
					expanded = state.open[directory]
					if expanded == nil {
						v := directory == "" || selected == directory || strings.HasPrefix(selected, directory+"/")
						expanded = &v
						state.open[directory] = expanded
					}
				}
				item := ui.TreeItem(c.Key(directory), label, expanded, func() {
					for _, child := range children[directory] {
						row(child)
					}
				}).Height(detailRowHeight).Selected(selected == directory).Label("会话目录 /" + directory)
				if item.Clicked() {
					*value = directory
					chosen = true
				}
			}
			row("")
		})
	})
	return chosen
}
func (a *App) sessionDirectoryField(c *ui.Context) {
	state := &a.features.directoryPicker
	ui.Field(c, "目录", func() {
		anchor := ui.Button(c, "/"+strings.Trim(a.draft.Group, "/")).Justify(ui.Start).TextAlign(ui.Start).Label("选择会话目录").OnClick(func() { state.shown = !state.shown })
		ui.Popover(c, anchor, &state.shown, func() {
			if a.directoryTree(c, &a.draft.Group, state) {
				state.shown = false
			}
		}).Width(max(220, anchor.Bounds().W))
	})
}

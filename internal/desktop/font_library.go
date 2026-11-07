package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

type fontLibraryState struct {
	open, busy      bool
	selected, error string
	draft           core.FontLibraryDraft
	cancel          context.CancelFunc
	generation      uint64
}

// One private collection is shared by every window in this process.
var userFontData [][]byte

func activateUserFonts(fonts []core.LibraryFont) error {
	data := make([][]byte, len(fonts))
	for i, f := range fonts {
		data[i] = f.Data
	}
	if err := ui.ReplaceUserFonts(data); err != nil {
		return err
	}
	userFontData = data
	return nil
}

func (a *App) loadUserFonts() error {
	draft, err := core.NewFontLibrary(a.store.Dir).Load()
	if err != nil {
		return err
	}
	if err = activateUserFonts(draft.Fonts); err != nil {
		return err
	}
	a.features.fontLibrary.draft = draft
	return nil
}

func (a *App) showFontLibrary() {
	f := &a.features.fontLibrary
	f.open = true
	f.error = ""
	f.selected = ""
	if err := a.loadUserFonts(); err != nil {
		f.error = err.Error()
	}
}

func (a *App) closeFontLibrary() {
	f := &a.features.fontLibrary
	f.open = false
	f.generation++
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil
	}
	f.busy = false
}

func (a *App) commitUserFonts(draft core.FontLibraryDraft) error {
	before := draft.Before
	previous := userFontData
	if err := activateUserFonts(draft.Fonts); err != nil {
		return err
	}
	if err := core.NewFontLibrary(a.store.Dir).Commit(draft); err != nil {
		rollback := ui.ReplaceUserFonts(previous)
		userFontData = previous
		return errors.Join(err, rollback)
	}
	// Reuse the validated bytes; selection in the settings draft stays untouched.
	draft.Before.Fonts = []core.FontAsset{}
	for _, f := range draft.Fonts {
		draft.Before.Fonts = append(draft.Before.Fonts, f.Asset)
	}
	a.features.fontLibrary.draft = draft
	if !reflect.DeepEqual(before, draft.Before) {
		if options, err := a.loadSyncOptions(); err == nil && options.Enabled && options.Policy.Effective().Content.FontFiles {
			a.markSharedSyncChanged()
			if a.hub != nil {
				a.hub.scheduleSharedSync(2 * time.Second)
			}
		}
	}
	windows := []*App{a}
	if a.hub != nil {
		windows = a.hub.configurationWindows()
	}
	for _, w := range windows {
		if w.win != nil {
			w.win.Invalidate()
		}
	}
	return nil
}

func (a *App) importUserFonts(paths []string) {
	f := &a.features.fontLibrary
	if f.busy {
		return
	}
	f.busy = true
	f.error = ""
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	generation := f.generation
	library := core.NewFontLibrary(a.store.Dir)
	go func() {
		defer cancel()
		draft, err := library.PrepareImport(ctx, paths)
		a.post(func() {
			if generation != f.generation || !f.open {
				return
			}
			f.busy = false
			f.cancel = nil
			if err == nil {
				err = a.commitUserFonts(draft)
			}
			if err != nil {
				f.error = err.Error()
			}
		})
	}()
}

func (a *App) chooseUserFonts(directory bool) {
	f := &a.features.fontLibrary
	if f.busy {
		return
	}
	f.busy = true
	generation := f.generation
	options := mygo.OpenDialogOptions{Parent: a.preferencesWindow, Title: "导入本机字体", Directory: directory, Multiple: true}
	if !directory {
		options.Filters = []mygo.FileFilter{{Name: "字体文件", Extensions: []string{"ttf", "otf", "ttc", "otc"}}}
	}
	go func() {
		paths, err := mygo.Dialog.Open(options)
		a.post(func() {
			if !f.open || generation != f.generation {
				return
			}
			f.busy = false
			if err != nil {
				f.error = err.Error()
				return
			}
			if len(paths) > 0 {
				a.importUserFonts(paths)
			}
		})
	}()
}

func (a *App) removeUserFont() {
	f := &a.features.fontLibrary
	if f.busy || f.selected == "" {
		return
	}
	draft := f.draft
	draft.Fonts = nil
	for _, font := range f.draft.Fonts {
		if font.Asset.Name() != f.selected {
			draft.Fonts = append(draft.Fonts, font)
		}
	}
	if err := a.commitUserFonts(draft); err != nil {
		f.error = err.Error()
		return
	}
	f.selected = ""
	f.error = ""
}

func (a *App) fontLibraryDialog(c *ui.Context) {
	f := &a.features.fontLibrary
	wasOpen := f.open
	desktopModal(c, &f.open, func() {
		dialogBody(c, 620).Gap(12).Children(func() {
			windowHeading(c, "本机字体库")
			ui.Text(c, "导入和移除立即保存；字体选择需在设置中点击“应用”。仅本机使用，不同步、不附带归档。").Wrap().FontSize(12).TextColor(c.Theme().TextMuted)
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "导入字体文件…").Disabled(f.busy).OnClick(func() { a.chooseUserFonts(false) })
				ui.Button(c, "导入字体目录…").Disabled(f.busy).OnClick(func() { a.chooseUserFonts(true) })
				ui.Button(c, "打开字体目录").OnClick(func() {
					l := core.NewFontLibrary(a.store.Dir)
					err := l.CheckDirectory()
					if err == nil {
						err = os.MkdirAll(l.Directory, 0700)
					}
					if err == nil {
						err = mygo.Shell.OpenPath(l.Directory)
					}
					if err != nil {
						f.error = err.Error()
					}
				})
			})
			total := 0
			for _, font := range f.draft.Fonts {
				total += font.Asset.Size
			}
			ui.Text(c, fmt.Sprintf("活动字体：%d / 64 文件 · %.1f / 64 MiB", len(f.draft.Fonts), float64(total)/(1<<20))).FontSize(12)
			ui.Scroll(c).Height(220).Gap(4).Border(1, c.Theme().Border).Padding(6).Children(func() {
				if len(f.draft.Fonts) == 0 {
					ui.Text(c, "尚未导入字体；系统字体和内置字体仍可使用。").Wrap()
				}
				for _, font := range f.draft.Fonts {
					names := []string{}
					for _, face := range font.Faces {
						names = append(names, face.Name)
					}
					row := ui.Button(c, strings.Join(names, " / ")+fmt.Sprintf(" · %s · %.1f KiB", strings.ToUpper(font.Asset.Extension), float64(font.Asset.Size)/1024)).FillWidth().Label("字体 " + font.Asset.Name()).OnClick(func() { f.selected = font.Asset.Name() })
					if f.selected == font.Asset.Name() {
						row.Background(c.Theme().Selection)
					}
				}
			})
			for _, font := range f.draft.Fonts {
				if font.Asset.Name() == f.selected {
					ui.Text(c, "Aa Bb 0123456789  |  中文终端  |  ~/server $ ls -lah").Font(terminalFamily(font.Faces[0].Name)).FontSize(16).Wrap().Label("本机字体预览")
					if !font.Faces[0].Monospaced {
						ui.Text(c, "非等宽字体：终端仍按字符网格排列。").FontSize(12).TextColor(c.Theme().Warning)
					}
				}
			}
			ui.Text(c, "单文件最多 20 MiB；目录扫描最多 10,000 项，跳过隐藏项、符号链接及应用包。移除仅退出活动库，保留历史文件并自动回退显示。").FontSize(11).TextColor(c.Theme().TextMuted).Wrap()
			if f.busy {
				ui.Text(c, "正在读取字体…")
			}
			if f.error != "" {
				ui.Text(c, f.error).Wrap().TextColor(c.Theme().Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "移除所选字体").Disabled(f.busy || f.selected == "").OnClick(a.removeUserFont)
				ui.Button(c, "完成").OnClick(a.closeFontLibrary)
			})
		})
	})
	if wasOpen && !f.open {
		a.closeFontLibrary()
	}
}

package desktop

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/goregular"
	"oshell-desktop/internal/core"
)

func TestUserFontLibraryUIAndPersistence(t *testing.T) {
	defer func() { _ = activateUserFonts(nil) }()
	store := &core.Store{Dir: t.TempDir()}
	a := newApp(store, core.DefaultConfig())
	a.prepareSettings()
	before := a.config.Clone()
	draftFamily := a.features.settingsDraft.FontFamily
	tt := ui.NewTester(a.settingsView, 1080, 760)
	if err := tt.Click("管理本机字体…"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("本机字体库") {
		t.Fatal("manager missing")
	}
	source := filepath.Join(t.TempDir(), "fixture.ttf")
	if err := os.WriteFile(source, gomono.TTF, 0600); err != nil {
		t.Fatal(err)
	}
	a.importUserFonts([]string{source})
	waitOperator(t, a, func() bool { return !a.features.fontLibrary.busy })
	f := &a.features.fontLibrary
	if f.error != "" || len(f.draft.Fonts) != 1 {
		t.Fatal(f.error, len(f.draft.Fonts))
	}
	asset := f.draft.Fonts[0].Asset
	tt.Frame()
	if err := tt.Click("字体 " + asset.Name()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.config, before) || a.features.settingsDraft.FontFamily != draftFamily {
		t.Fatal("import applied settings")
	}
	for _, dark := range []bool{false, true} {
		a.features.settingsDraft.Dark = dark
		a.features.settingsDraft.InterfaceTheme = map[bool]string{true: "dark", false: "light"}[dark]
		tt.Frame()
		snapshot(t, tt, map[bool]string{true: "font-library-dark.png", false: "font-library-light.png"}[dark])
	}
	small := ui.NewTester(a.settingsView, 700, 560)
	for _, name := range []string{"导入字体文件…", "导入字体目录…", "打开字体目录", "移除所选字体", "完成"} {
		r, ok := small.Find(name)
		if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > 700 || r.Y+r.H > 560 {
			t.Fatalf("small font manager clipped %s: %+v", name, r)
		}
	}
	snapshot(t, small, "font-library-small.png")
	a.closeSettings()
	loaded := newApp(store, core.DefaultConfig())
	if err := loaded.loadUserFonts(); err != nil {
		t.Fatal(err)
	}
	if len(loaded.features.fontLibrary.draft.Fonts) != 1 {
		t.Fatal("restart did not load active fonts")
	}
	a.prepareSettings()
	a.showFontLibrary()
	a.features.fontLibrary.selected = asset.Name()
	tt.Frame()
	if err := tt.Click("移除所选字体"); err != nil {
		t.Fatal(err)
	}
	if a.features.fontLibrary.error != "" || len(a.features.fontLibrary.draft.Fonts) != 0 {
		t.Fatal(a.features.fontLibrary.error)
	}
	data, err := os.ReadFile(filepath.Join(store.Dir, "fonts", asset.Name()))
	if err != nil || !bytes.Equal(data, gomono.TTF) {
		t.Fatal("history not retained", err)
	}
	loaded2, err := core.NewFontLibrary(store.Dir).Load()
	if err != nil || len(loaded2.Fonts) != 0 {
		t.Fatal("removal not durable", err)
	}
}

func TestUserFontNativeOpenTypeAndCollections(t *testing.T) {
	defer func() { _ = ui.ReplaceUserFonts(nil) }()
	cff, err := os.ReadFile("../core/testdata/fonts/CFFTest.otf")
	if err != nil {
		t.Fatal(err)
	}
	for _, font := range [][]byte{cff, gomono.TTF} {
		if err = ui.ReplaceUserFonts([][]byte{font}); err != nil {
			t.Fatal("native font", err)
		}
		// Standard single-face collection wrapping an existing real SFNT fixture.
		collection := make([]byte, 16)
		copy(collection, "ttcf")
		binary.BigEndian.PutUint32(collection[4:], 0x10000)
		binary.BigEndian.PutUint32(collection[8:], 1)
		binary.BigEndian.PutUint32(collection[12:], 16)
		collection = append(collection, font...)
		for i := 0; i < int(binary.BigEndian.Uint16(font[4:])); i++ {
			offset := 16 + 12 + 16*i + 8
			binary.BigEndian.PutUint32(collection[offset:], binary.BigEndian.Uint32(collection[offset:])+16)
		}
		if _, err = core.FontFaces(collection); err != nil {
			t.Fatal(err)
		}
		if err = ui.ReplaceUserFonts([][]byte{collection}); err != nil {
			t.Fatal("native collection", err)
		}
	}
}

func TestUserFontFacesRemainDistinctWithinFamily(t *testing.T) {
	defer func() { _ = activateUserFonts(nil) }()
	regular, err := core.FontFaces(gomono.TTF)
	if err != nil {
		t.Fatal(err)
	}
	bold, err := core.FontFaces(gomonobold.TTF)
	if err != nil {
		t.Fatal(err)
	}
	if regular[0].Name == bold[0].Name {
		t.Fatal("distinct faces collapsed to family")
	}
	if err = ui.ReplaceUserFonts([][]byte{gomono.TTF, gomonobold.TTF}); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(renderedFont(regular[0].Name), renderedFont(bold[0].Name)) {
		t.Fatal("selected bold face rendered as regular")
	}
	regularPixels := renderedFont(regular[0].Name)
	if err = ui.ReplaceUserFonts([][]byte{gomono.TTF}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(regularPixels, renderedFont(regular[0].Name)) {
		t.Fatal("removing sibling face changed regular")
	}
	if !bytes.Equal(renderedFont(bold[0].Name), renderedFont("")) {
		t.Fatal("removed face did not fall back")
	}
}

func TestUserFontRegistrationFallbackAndRollback(t *testing.T) {
	defer func() { _ = activateUserFonts(nil) }()
	_ = activateUserFonts(nil)
	fallback := renderedFont("")
	builtin := map[string][]byte{}
	for _, name := range []string{"DejaVu Sans Mono", "Cascadia Code", "Symbols Nerd Font Mono"} {
		builtin[name] = renderedFont(name)
	}
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p := filepath.Join(t.TempDir(), "go.ttf")
	if err := os.WriteFile(p, goregular.TTF, 0600); err != nil {
		t.Fatal(err)
	}
	draft, err := core.NewFontLibrary(a.store.Dir).PrepareImport(context.Background(), []string{p})
	if err != nil {
		t.Fatal(err)
	}
	if err = a.commitUserFonts(draft); err != nil {
		t.Fatal(err)
	}
	selectedName := draft.Fonts[0].Faces[0].Name
	active := renderedFont(selectedName)
	if reflect.DeepEqual(active, fallback) {
		t.Fatal("imported face not used")
	}
	if err = ui.ReplaceUserFonts([][]byte{[]byte("invalid font")}); err == nil {
		t.Fatal("native stack accepted invalid font")
	}
	if got := renderedFont(selectedName); !reflect.DeepEqual(active, got) {
		t.Fatal("native registration failure lost prior font")
	}
	// A stale library snapshot must restore the prior private collection.
	draft.Fonts = nil
	if err = a.commitUserFonts(draft); err == nil {
		t.Fatal("accepted stale manifest")
	}
	if got := renderedFont(selectedName); !reflect.DeepEqual(active, got) {
		t.Fatal("failed persistence lost prior font")
	}
	d, err := core.NewFontLibrary(a.store.Dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	d.Fonts = nil
	if err = a.commitUserFonts(d); err != nil {
		t.Fatal(err)
	}
	if got := renderedFont(selectedName); !reflect.DeepEqual(got, fallback) {
		t.Fatal("removed family did not fall back")
	}
	for name, want := range builtin {
		if got := renderedFont(name); !reflect.DeepEqual(got, want) {
			t.Fatal("builtin changed", name)
		}
	}
}

func TestUserFontImportCancelledWithManager(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.showFontLibrary()
	p := filepath.Join(t.TempDir(), "go.ttf")
	if err := os.WriteFile(p, gomono.TTF, 0600); err != nil {
		t.Fatal(err)
	}
	a.importUserFonts([]string{p})
	a.closeFontLibrary()
	// Process the completion after cancellation; it must not publish a hidden import.
	a.showFontLibrary()
	deadline := time.Now().Add(3 * time.Second)
	for {
		a.eventMu.Lock()
		ready := len(a.events) > 0
		a.eventMu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("import worker did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	a.drain()
	if len(a.features.fontLibrary.draft.Fonts) != 0 {
		t.Fatal("cancelled import committed")
	}
}

func renderedFont(name string) []byte {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Background(ui.Hex("#FFFFFF")).Children(func() {
			ui.Text(c, "MWil09\ue0a0").Font(terminalFamily(name)).FontSize(18).TextColor(ui.Hex("#000000"))
		})
	}, 360, 50)
	return bytes.Clone(tt.Image().Pix)
}

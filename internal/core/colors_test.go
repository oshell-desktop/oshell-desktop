package core

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestXshellLibraryAndSharedAppearance(t *testing.T) {
	schemes := XshellSchemes()
	if len(schemes) != 20 {
		t.Fatal(len(schemes))
	}
	file, err := os.ReadFile("../../resources/color-schemes/IR_Black.json")
	if err != nil {
		t.Fatal(err)
	}
	var ir ColorScheme
	if err := json.Unmarshal(file, &ir); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, scheme := range schemes {
		if scheme.Validate() != nil || seen[scheme.ID] {
			t.Fatalf("invalid template %s", scheme.Name)
		}
		seen[scheme.ID] = true
		if scheme.Name == "IR_Black" && !reflect.DeepEqual(scheme, ir) {
			t.Fatal("Mac import differs from the built-in template")
		}
	}
	if !seen[ir.ID] || ir.Cursor != "#00FF00" || ir.ANSI[9] != "#FA6C60" {
		t.Fatal("IR_Black missing or inaccurate")
	}
	cfg := DefaultConfig()
	if _, exists := preferencesForShared(cfg)["terminalAppearance"]; exists {
		t.Fatal("legacy document gained extended preferences")
	}
	var defaults TerminalAppearance
	canonical, err := canonicalSharedJSON([]byte("{}"), "terminalAppearance")
	if err != nil || json.Unmarshal(canonical, &defaults) != nil || defaults != DefaultTerminalAppearance() {
		t.Fatal("contract defaults differ", err)
	}
	defaults.Left, defaults.Right, defaults.CharacterSpacing, defaults.CursorShape = 17, 4, -3, "bar"
	cfg.Settings.TerminalAppearance = &defaults
	cfg.Settings.ColorSchemeID, cfg.Settings.CustomSchemes = ir.ID, schemes
	prefs := preferencesForShared(cfg)
	raw, _ := json.Marshal(map[string]any{"profiles": []any{}, "preferences": prefs})
	doc, err := decodeNativeDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	back, err := doc.Configuration()
	if err != nil || back.Settings.TerminalAppearance == nil || *back.Settings.TerminalAppearance != defaults || !reflect.DeepEqual(back.Settings.Scheme(), ir) {
		t.Fatal("appearance lost", err)
	}
	if again := preferencesForShared(back); !bytes.Equal(again["terminalAppearance"], prefs["terminalAppearance"]) {
		t.Fatal("no-op round trip changed preferences")
	}
	back.Settings.TerminalAppearance = nil
	if _, ok := preferencesForShared(back)["terminalAppearance"]; ok {
		t.Fatal("disabled appearance resurrected from previous shared document")
	}
	for _, invalid := range []string{`{"other":1}`, `{"lineSpacing":-6}`, `{"cursorShape":"bad"}`, `{"blinkInterval":0}`, `{"asianSize":6}`} {
		if _, err := canonicalSharedJSON([]byte(invalid), "terminalAppearance"); err == nil {
			t.Fatal(invalid)
		}
	}
}

func TestBaselineColorPresetsAndImport(t *testing.T) {
	presets := ColorPresets()
	if len(presets) != 9 {
		t.Fatal(len(presets))
	}
	for _, p := range presets {
		if e := p.Validate(); e != nil {
			t.Fatal(p.Name, e)
		}
	}
	if presets[0].Background != "#11151A" || presets[0].Cursor != "#55E5CC" {
		t.Fatal("baseline colors changed")
	}
	data, _ := json.Marshal(presets[0])
	imported, e := ImportColors(data)
	if e != nil || len(imported) != 1 || imported[0].ID == presets[0].ID {
		t.Fatal(imported, e)
	}
	text := "[Example]\nbackground=ffffff\ntext=000000\n"
	for _, c := range []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"} {
		text += c + "=112233\n" + c + "(bold)=aabbcc\n"
	}
	text += "[Names]\ncount=1\nname0=Example\n"
	imported, e = ImportColors([]byte(text))
	if e != nil || len(imported) != 1 || imported[0].ANSI[8] != "#AABBCC" {
		t.Fatal(imported, e)
	}
	if _, e = ImportColors([]byte(strings.ReplaceAll(text, "text=000000\n", ""))); e == nil {
		t.Fatal("accepted incomplete Xshell scheme")
	}
}

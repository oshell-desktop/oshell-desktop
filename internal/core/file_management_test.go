package core

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestFileManagementSettingsLocalRoundTripAndRejectUnknown(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadFileManagementSettings(dir)
	if err != nil || s != DefaultFileManagementSettings() {
		t.Fatal(s, err)
	}
	for _, position := range []string{"left", "right", "bottom", "window"} {
		s.Placement = position
		s.Mode = "terminal"
		s.Command = "yazi --chooser-file /tmp/choice"
		if err = SaveFileManagementSettings(dir, s); err != nil {
			t.Fatal(err)
		}
		got, err := LoadFileManagementSettings(dir)
		if err != nil || got != s {
			t.Fatal(got, err)
		}
	}
	for _, command := range []string{"", "yazi\nranger", "echo\x1bsecret"} {
		s.Command = command
		if err = s.Validate(); err == nil {
			t.Fatal("bad command accepted")
		}
	}
	path := filepath.Join(dir, "file-management.json")
	for _, data := range [][]byte{[]byte(`{"version":99,"mode":"builtIn","placement":"window","command":"yazi"}`), []byte(`{"version":1,"mode":"builtIn","placement":"window","command":"yazi","future":true}`)} {
		os.WriteFile(path, data, 0600)
		if _, err = LoadFileManagementSettings(dir); err == nil {
			t.Fatal("unknown setting accepted")
		}
		if err = SaveFileManagementSettings(dir, DefaultFileManagementSettings()); err == nil {
			t.Fatal("unknown setting overwritten")
		}
		got, _ := os.ReadFile(path)
		if !bytes.Equal(data, got) {
			t.Fatal("file changed")
		}
	}
}

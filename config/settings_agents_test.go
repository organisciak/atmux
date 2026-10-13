package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAgentsColumnsRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	if err := SaveAgentsColumns(3); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s.AgentsColumns != 3 || s.DefaultAction != "landing" {
		t.Fatalf("settings = %+v", s)
	}
}

func TestSaveAgentsColumnsLeavesUnreadableSettingsAlone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	path, err := SettingsPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SaveAgentsColumns(2); err == nil {
		t.Fatal("expected an error for unreadable settings")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "{not json" {
		t.Fatalf("unreadable settings were overwritten: %q", data)
	}
}

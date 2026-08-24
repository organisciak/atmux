package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parseDirective(t *testing.T, contents string) *Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), DefaultConfigName)
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return cfg
}

func TestSessionTitleDirective_Values(t *testing.T) {
	for _, on := range []string{"session_title:on", "session_title:true", "session_title:yes", "session_title:1"} {
		if !parseDirective(t, on+"\n").SessionTitle {
			t.Errorf("%q should enable session title", on)
		}
	}
	for _, off := range []string{"session_title:off", "session_title:false", "session_title:no", "session_title:0"} {
		if parseDirective(t, off+"\n").SessionTitle {
			t.Errorf("%q should disable session title", off)
		}
	}
}

func TestSessionTitleDirective_BareMeansOn(t *testing.T) {
	// A directive written with no value reads as intent to turn it on.
	if !parseDirective(t, "session_title:\n").SessionTitle {
		t.Fatal("a bare session_title directive should enable it")
	}
}

func TestSessionTitleDirective_DefaultsOff(t *testing.T) {
	// Overwriting a user's status-right is invasive, so it must be opt-in.
	if parseDirective(t, "color:#ff0000\n").SessionTitle {
		t.Fatal("session title must default to off")
	}
}

func TestSessionTitleDirective_RejectsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultConfigName)
	if err := os.WriteFile(path, []byte("session_title:maybe\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Parse(path); err == nil {
		t.Fatal("expected an error for an unparseable value")
	}
}

func TestSessionTitle_LocalEnablesOverGlobal(t *testing.T) {
	merged := mergeConfigs(&Config{SessionTitle: false}, &Config{SessionTitle: true})
	if !merged.SessionTitle {
		t.Fatal("a project opting in should win over a global default")
	}

	inherited := mergeConfigs(&Config{SessionTitle: true}, &Config{})
	if !inherited.SessionTitle {
		t.Fatal("a global opt-in should carry into projects")
	}
}

func TestRemoteControlDirective(t *testing.T) {
	if !parseDirective(t, "remote_control:on\n").RemoteControl {
		t.Error("remote_control:on should enable it")
	}
	if parseDirective(t, "remote_control:off\n").RemoteControl {
		t.Error("remote_control:off should disable it")
	}
	// Opening remote access must never be an accident of omission.
	if parseDirective(t, "color:#ff0000\n").RemoteControl {
		t.Error("remote control must default to off")
	}
}

func TestRemoteControl_LocalEnablesOverGlobal(t *testing.T) {
	if !mergeConfigs(&Config{}, &Config{RemoteControl: true}).RemoteControl {
		t.Error("a project opting in should win")
	}
	if !mergeConfigs(&Config{RemoteControl: true}, &Config{}).RemoteControl {
		t.Error("a global opt-in should carry into projects")
	}
}

func TestGlobalTemplate_UsesAutoPermissions(t *testing.T) {
	tpl := GlobalTemplate()
	if !strings.Contains(tpl, "--permission-mode auto") {
		t.Error("global template should default to auto permissions")
	}
	if strings.Contains(tpl, "agent:claude --dangerously-skip-permissions") {
		t.Error("global template should no longer default to bypassing permissions")
	}
}

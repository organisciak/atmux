package cmd

import "testing"

func TestResolveKeybindDefaultsKeyPerCommand(t *testing.T) {
	for command, want := range map[string]string{"browse": "S", "sessions": "s", "agents": "a"} {
		keybindCommand, keybindKey = command, "S"
		keybindCmd.Flags().Lookup("key").Changed = false
		if _, err := resolveKeybind(keybindCmd); err != nil {
			t.Fatal(err)
		}
		if keybindKey != want {
			t.Errorf("%s: key = %q, want %q", command, keybindKey, want)
		}
	}
}

func TestResolveKeybindKeepsExplicitKey(t *testing.T) {
	keybindCommand, keybindKey = "agents", "C-a"
	keybindCmd.Flags().Lookup("key").Changed = true
	defer func() { keybindCmd.Flags().Lookup("key").Changed = false }()
	if _, err := resolveKeybind(keybindCmd); err != nil {
		t.Fatal(err)
	}
	if keybindKey != "C-a" {
		t.Fatalf("explicit --key overridden: %q", keybindKey)
	}
}

func TestResolveKeybindRejectsUnknownCommand(t *testing.T) {
	keybindCommand = "kill"
	if _, err := resolveKeybind(keybindCmd); err == nil {
		t.Fatal("unknown command accepted")
	}
}

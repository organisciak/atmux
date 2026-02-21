package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOnboardSpaceTogglesAgentEnabled(t *testing.T) {
	m := newOnboardModel()
	m.step = 1
	m.cursor = 0

	wasEnabled := m.agents[0].enabled

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	next, ok := updated.(onboardModel)
	if !ok {
		t.Fatalf("expected onboardModel, got %T", updated)
	}

	if next.agents[0].enabled == wasEnabled {
		t.Fatalf("expected agent enabled to toggle on space key")
	}
}

func TestOnboardEditCommandsEnterEditing(t *testing.T) {
	m := newOnboardModel()
	m.step = 3
	m.cursor = 0 // "Edit Commands" button

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, ok := updated.(onboardModel)
	if !ok {
		t.Fatalf("expected onboardModel, got %T", updated)
	}

	if !next.editingCommands {
		t.Fatalf("expected editingCommands to be true after pressing Enter on Edit Commands")
	}
	if len(next.commandInputs) == 0 {
		t.Fatalf("expected commandInputs to be initialized")
	}
}

func TestOnboardEditCommandsApply(t *testing.T) {
	m := newOnboardModel()
	m.agents = []agentChoice{
		{name: "Claude", command: "claude", enabled: true, yolo: true},
		{name: "Codex", command: "codex", enabled: true, yolo: false},
	}
	m.step = 3
	m.cursor = 0

	// Enter edit mode
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := updated.(onboardModel)

	if len(next.commandInputs) != 2 {
		t.Fatalf("expected 2 command inputs, got %d", len(next.commandInputs))
	}

	// Verify the first input has the generated command
	val := next.commandInputs[0].Value()
	if val != "claude --dangerously-skip-permissions" {
		t.Fatalf("expected first input to be 'claude --dangerously-skip-permissions', got '%s'", val)
	}

	// Modify the first input
	next.commandInputs[0].SetValue("claude --model opus")

	// Confirm edits (Enter)
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	final := updated.(onboardModel)

	if final.editingCommands {
		t.Fatalf("expected editingCommands to be false after confirming")
	}
	if final.agents[0].command != "claude --model opus" {
		t.Fatalf("expected first agent command to be 'claude --model opus', got '%s'", final.agents[0].command)
	}
	if final.agents[0].yolo {
		t.Fatalf("expected yolo to be cleared after editing")
	}
}

func TestOnboardEditCommandsCancel(t *testing.T) {
	m := newOnboardModel()
	m.step = 3
	m.cursor = 0

	// Enter edit mode
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := updated.(onboardModel)

	if !next.editingCommands {
		t.Fatalf("expected editingCommands to be true")
	}

	// Modify command
	next.commandInputs[0].SetValue("modified-command")

	// Cancel with Esc
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyEscape})
	final := updated.(onboardModel)

	if final.editingCommands {
		t.Fatalf("expected editingCommands to be false after Esc")
	}
	// Original command should be unchanged
	if final.agents[0].command != "claude" {
		t.Fatalf("expected agent command to be unchanged after cancel, got '%s'", final.agents[0].command)
	}
}

func TestOnboardSpaceTogglesYoloForSelectedEnabledAgent(t *testing.T) {
	m := newOnboardModel()
	m.step = 2
	m.cursor = 1
	m.agents = []agentChoice{
		{name: "Agent A", command: "a", enabled: true, yolo: false},
		{name: "Agent B", command: "b", enabled: false, yolo: false},
		{name: "Agent C", command: "c", enabled: true, yolo: false},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	next, ok := updated.(onboardModel)
	if !ok {
		t.Fatalf("expected onboardModel, got %T", updated)
	}

	if next.agents[0].yolo {
		t.Fatalf("expected first enabled agent to remain unchanged")
	}
	if !next.agents[2].yolo {
		t.Fatalf("expected selected enabled agent yolo flag to toggle")
	}
}

func TestParseTmuxConfBindings(t *testing.T) {
	// Create a temporary tmux.conf
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, ".tmux.conf")

	content := `# tmux configuration
set -g mouse on

bind-key S choose-tree
bind s choose-tree -s
bind-key -r C-a send-prefix
bind T run-shell "some-tool"
`
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// We can't override home dir for parseTmuxConfBindings, so test the
	// regex pattern matching directly via findDuplicateKeybinding
	found, match := findDuplicateKeybinding(content, "S")
	if !found {
		t.Fatal("expected to find binding for S")
	}
	if !strings.Contains(match, "choose-tree") {
		t.Fatalf("expected match to contain 'choose-tree', got '%s'", match)
	}

	found, match = findDuplicateKeybinding(content, "s")
	if !found {
		t.Fatal("expected to find binding for s")
	}
	if !strings.Contains(match, "choose-tree") {
		t.Fatalf("expected match to contain 'choose-tree', got '%s'", match)
	}

	found, _ = findDuplicateKeybinding(content, "X")
	if found {
		t.Fatal("expected no binding for X")
	}

	found, match = findDuplicateKeybinding(content, "T")
	if !found {
		t.Fatal("expected to find binding for T")
	}
	if !strings.Contains(match, "some-tool") {
		t.Fatalf("expected match to contain 'some-tool', got '%s'", match)
	}
}

func TestOnboardKeybindToggle(t *testing.T) {
	m := newOnboardModel()
	m.step = 4
	m.cursor = 0

	// Both should start enabled
	if !m.keybindOptions[0].enabled {
		t.Fatal("expected browse binding to be enabled by default")
	}
	if !m.keybindOptions[1].enabled {
		t.Fatal("expected sessions binding to be enabled by default")
	}

	// Toggle first off with space
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	next := updated.(onboardModel)
	if next.keybindOptions[0].enabled {
		t.Fatal("expected browse binding to be toggled off")
	}

	// Move to second and toggle off
	next.cursor = 1
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeySpace})
	next = updated.(onboardModel)
	if next.keybindOptions[1].enabled {
		t.Fatal("expected sessions binding to be toggled off")
	}

	// Toggle first back on with Enter
	next.cursor = 0
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next = updated.(onboardModel)
	if !next.keybindOptions[0].enabled {
		t.Fatal("expected browse binding to be toggled on via Enter")
	}
}

func TestOnboardKeybindInitializesOptions(t *testing.T) {
	m := newOnboardModel()
	if len(m.keybindOptions) != 2 {
		t.Fatalf("expected 2 keybind options, got %d", len(m.keybindOptions))
	}
	if m.keybindOptions[0].key != "S" {
		t.Fatalf("expected first option key to be 'S', got '%s'", m.keybindOptions[0].key)
	}
	if m.keybindOptions[1].key != "s" {
		t.Fatalf("expected second option key to be 's', got '%s'", m.keybindOptions[1].key)
	}
	if m.keybindOptions[0].command != "atmux browse --popup" {
		t.Fatalf("expected first command to be 'atmux browse --popup', got '%s'", m.keybindOptions[0].command)
	}
	if m.keybindOptions[1].command != "atmux sessions -p" {
		t.Fatalf("expected second command to be 'atmux sessions -p', got '%s'", m.keybindOptions[1].command)
	}
	if !m.keybindOptions[1].isDefault {
		t.Fatal("expected sessions binding to be flagged as default tmux binding")
	}
}

func TestOnboardKeybindMaxCursor(t *testing.T) {
	m := newOnboardModel()
	m.step = 4
	// 2 keybind options + "Add selected" + "Skip" = max cursor of 3
	expected := len(m.keybindOptions) + 1
	if max := m.maxCursor(); max != expected {
		t.Fatalf("expected maxCursor to be %d for keybind step, got %d", expected, max)
	}
}

func TestOnboardKeybindAddWritesToFile(t *testing.T) {
	// Create temp dir and override home for this test
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, ".tmux.conf")
	os.WriteFile(confPath, []byte("# existing config\n"), 0644)

	m := newOnboardModel()
	m.step = 4

	// Override the addKeybindings to use a custom path by testing directly
	// Since addKeybindings uses os.UserHomeDir, we test the file writing
	// by calling it after setting up the temp path
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Enable both bindings
	m.keybindOptions[0].enabled = true
	m.keybindOptions[1].enabled = true

	if err := m.addKeybindings(); err != nil {
		t.Fatalf("addKeybindings failed: %v", err)
	}

	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)

	if !strings.Contains(s, `bind-key S run-shell "atmux browse --popup"`) {
		t.Fatal("expected browse binding in output")
	}
	if !strings.Contains(s, `bind-key s run-shell "atmux sessions -p"`) {
		t.Fatal("expected sessions binding in output")
	}
	if !m.browseBindAdded {
		t.Fatal("expected browseBindAdded to be true")
	}
	if !m.sessionsBindAdded {
		t.Fatal("expected sessionsBindAdded to be true")
	}
}

func TestOnboardKeybindSkipDoesNotWrite(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, ".tmux.conf")
	os.WriteFile(confPath, []byte("# existing config\n"), 0644)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	m := newOnboardModel()
	m.step = 4

	// Disable both bindings and call addKeybindings
	m.keybindOptions[0].enabled = false
	m.keybindOptions[1].enabled = false

	if err := m.addKeybindings(); err != nil {
		t.Fatalf("addKeybindings failed: %v", err)
	}

	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if strings.Contains(s, "atmux") {
		t.Fatal("expected no atmux bindings when both disabled")
	}
}

func TestOnboardKeybindIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, ".tmux.conf")
	// Pre-populate with existing binding
	existing := "# existing config\n" + `bind-key S run-shell "atmux browse --popup"` + "\n"
	os.WriteFile(confPath, []byte(existing), 0644)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	m := newOnboardModel()
	m.keybindOptions[0].enabled = true
	m.keybindOptions[1].enabled = false

	if err := m.addKeybindings(); err != nil {
		t.Fatalf("addKeybindings failed: %v", err)
	}

	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	// Should not duplicate the existing binding
	count := strings.Count(s, `bind-key S run-shell "atmux browse --popup"`)
	if count != 1 {
		t.Fatalf("expected exactly 1 browse binding, got %d", count)
	}
	if !m.browseBindAdded {
		t.Fatal("expected browseBindAdded to be true even for existing binding")
	}
}

package tui

import (
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

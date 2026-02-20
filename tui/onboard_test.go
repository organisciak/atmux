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

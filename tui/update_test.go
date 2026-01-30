package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/porganisciak/agent-tmux/tmux"
)

func TestAttachKeySetsSession(t *testing.T) {
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	m.calculateLayout()
	m.tree = &tmux.Tree{
		Sessions: []tmux.TmuxSession{
			{
				Name:     "sess",
				Attached: true,
				Windows: []tmux.Window{
					{
						Index:  0,
						Name:   "win",
						Active: true,
						Panes: []tmux.Pane{
							{
								Index:  0,
								Title:  "pane",
								Active: true,
								Target: "sess:0.0",
							},
						},
					},
				},
			},
		},
	}
	m.rebuildFlatNodes()
	m.selectedIndex = 2

	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	updated, _ := m.handleTreeKeys(key)
	updatedModel := updated.(Model)

	if updatedModel.attachSession != "sess" {
		t.Fatalf("expected attach session sess, got %q", updatedModel.attachSession)
	}
}

func TestMouseResizeUpdatesTreeWidth(t *testing.T) {
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	m.calculateLayout()

	startWidth := m.treeWidth
	dividerX := startWidth - 1

	updated, _ := m.handleMouseMsg(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      dividerX,
		Y:      inputHeight + 2,
	})
	resizingModel := updated.(Model)
	if !resizingModel.resizing {
		t.Fatalf("expected resizing to start on divider press")
	}

	targetX := startWidth + 9
	updated, _ = resizingModel.handleMouseMsg(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonLeft,
		X:      targetX,
		Y:      inputHeight + 2,
	})
	motionModel := updated.(Model)
	expectedWidth := targetX + 1
	if motionModel.treeWidth != expectedWidth {
		t.Fatalf("expected tree width %d, got %d", expectedWidth, motionModel.treeWidth)
	}

	updated, _ = motionModel.handleMouseMsg(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      targetX,
		Y:      inputHeight + 2,
	})
	releasedModel := updated.(Model)
	if releasedModel.resizing {
		t.Fatalf("expected resizing to stop on release")
	}
}

func TestToggleMouseCapture(t *testing.T) {
	t.Setenv("TMUX", "")
	m := NewModel(Options{})
	m.focused = FocusTree
	if !m.mouseEnabled {
		t.Fatalf("expected mouse enabled by default")
	}
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}}
	updated, _ := m.handleKeyMsg(key)
	updatedModel := updated.(Model)
	if updatedModel.mouseEnabled {
		t.Fatalf("expected mouse to be disabled after toggle")
	}
}

func TestInputHistoryCapturesClearedDraft(t *testing.T) {
	m := NewModel(Options{})
	m.focused = FocusInput
	m.commandInput.Focus()

	for _, r := range []rune{'h', 'i'} {
		key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		updated, _ := m.handleInputKeys(key)
		m = updated.(Model)
	}

	for i := 0; i < 2; i++ {
		key := tea.KeyMsg{Type: tea.KeyBackspace}
		updated, _ := m.handleInputKeys(key)
		m = updated.(Model)
	}

	if len(m.inputHistory) != 1 || m.inputHistory[0] != "hi" {
		t.Fatalf("expected history to include cleared draft, got %v", m.inputHistory)
	}
}

func TestToggleExpandCollapsesChildren(t *testing.T) {
	m := NewModel(Options{})
	m.tree = &tmux.Tree{
		Sessions: []tmux.TmuxSession{
			{
				Name:     "sess",
				Attached: true,
				Windows: []tmux.Window{
					{
						Index:  0,
						Name:   "win",
						Active: true,
						Panes: []tmux.Pane{
							{
								Index:  0,
								Title:  "pane",
								Active: true,
								Target: "sess:0.0",
							},
						},
					},
				},
			},
		},
	}
	m.rebuildFlatNodes()
	if len(m.flatNodes) != 3 {
		t.Fatalf("expected 3 nodes when expanded, got %d", len(m.flatNodes))
	}

	m.selectedIndex = 0
	m.toggleExpand()
	if len(m.flatNodes) != 1 {
		t.Fatalf("expected 1 node after collapse, got %d", len(m.flatNodes))
	}
}

func TestMouseClickIconTogglesExpand(t *testing.T) {
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	m.tree = &tmux.Tree{
		Sessions: []tmux.TmuxSession{
			{
				Name:     "sess",
				Attached: true,
				Windows: []tmux.Window{
					{
						Index:  0,
						Name:   "win",
						Active: true,
						Panes: []tmux.Pane{
							{
								Index:  0,
								Title:  "pane",
								Active: true,
								Target: "sess:0.0",
							},
						},
					},
				},
			},
		},
	}
	m.rebuildFlatNodes()
	if len(m.flatNodes) != 3 {
		t.Fatalf("expected 3 nodes when expanded, got %d", len(m.flatNodes))
	}

	y := inputHeight + 2
	updated, _ := m.handleLeftClick(0, y)
	m = updated.(Model)

	if len(m.flatNodes) != 1 {
		t.Fatalf("expected 1 node after icon click collapse, got %d", len(m.flatNodes))
	}
}

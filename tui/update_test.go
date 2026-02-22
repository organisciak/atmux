package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/porganisciak/agent-tmux/history"
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

func TestRecentEnterSetsAttachSessionAndReviveDir(t *testing.T) {
	m := NewModel(Options{})
	m.focusRecent = true
	m.recentSelectedIndex = 0
	m.recentSessions = []history.Entry{
		{SessionName: "agent-proj", WorkingDirectory: "/tmp/proj"},
	}

	updated, _ := m.handleRecentKeys(tea.KeyMsg{Type: tea.KeyEnter})
	updatedModel := updated.(Model)

	if updatedModel.attachSession != "agent-proj" {
		t.Fatalf("expected attach session agent-proj, got %q", updatedModel.attachSession)
	}
	if updatedModel.reviveDir != "/tmp/proj" {
		t.Fatalf("expected revive dir /tmp/proj, got %q", updatedModel.reviveDir)
	}
}

func TestRecentAttachKeySetsAttachSessionAndReviveDir(t *testing.T) {
	m := NewModel(Options{})
	m.focusRecent = true
	m.recentSelectedIndex = 0
	m.recentSessions = []history.Entry{
		{SessionName: "agent-proj", WorkingDirectory: "/tmp/proj"},
	}

	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	updated, _ := m.handleRecentKeys(key)
	updatedModel := updated.(Model)

	if updatedModel.attachSession != "agent-proj" {
		t.Fatalf("expected attach session agent-proj, got %q", updatedModel.attachSession)
	}
	if updatedModel.reviveDir != "/tmp/proj" {
		t.Fatalf("expected revive dir /tmp/proj, got %q", updatedModel.reviveDir)
	}
}

func TestRecentDoubleClickSetsAttachSessionAndReviveDir(t *testing.T) {
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	m.calculateLayout()
	m.recentSessions = []history.Entry{
		{SessionName: "agent-proj", WorkingDirectory: "/tmp/proj"},
	}

	clickX := 1
	clickY := inputHeight + 4 // first visible recent line when tree has no nodes

	updated, _ := m.handleLeftClick(clickX, clickY)
	m = updated.(Model)
	updated, _ = m.handleLeftClick(clickX, clickY)
	updatedModel := updated.(Model)

	if updatedModel.attachSession != "agent-proj" {
		t.Fatalf("expected attach session agent-proj, got %q", updatedModel.attachSession)
	}
	if updatedModel.reviveDir != "/tmp/proj" {
		t.Fatalf("expected revive dir /tmp/proj, got %q", updatedModel.reviveDir)
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

func TestMultiHostFlatNodes(t *testing.T) {
	m := NewModel(Options{})
	m.hostTrees = []tmux.HostTree{
		{
			Host: "",
			Tree: &tmux.Tree{
				Sessions: []tmux.TmuxSession{
					{
						Name: "local-sess",
						Windows: []tmux.Window{
							{Index: 0, Name: "bash", Active: true,
								Panes: []tmux.Pane{{Index: 0, Target: "local-sess:0.0", Active: true, Title: "bash"}}},
						},
					},
				},
			},
		},
		{
			Host: "devbox",
			Tree: &tmux.Tree{
				Sessions: []tmux.TmuxSession{
					{
						Name: "remote-sess",
						Windows: []tmux.Window{
							{Index: 0, Name: "zsh", Active: true,
								Panes: []tmux.Pane{{Index: 0, Target: "remote-sess:0.0", Active: true, Title: "zsh"}}},
						},
					},
				},
			},
		},
	}
	m.tree = &tmux.Tree{} // merged tree placeholder
	m.rebuildFlatNodes()

	// Expected: [local] host, local-sess, bash, bash pane, [devbox] host, remote-sess, zsh, zsh pane = 8
	if len(m.flatNodes) != 8 {
		names := make([]string, 0, len(m.flatNodes))
		for _, n := range m.flatNodes {
			names = append(names, n.Type+":"+n.Name)
		}
		t.Fatalf("expected 8 flat nodes, got %d: %v", len(m.flatNodes), names)
	}

	// First node should be host header for "local"
	if m.flatNodes[0].Type != "host" || m.flatNodes[0].Name != "local" {
		t.Fatalf("expected first node to be host:local, got %s:%s", m.flatNodes[0].Type, m.flatNodes[0].Name)
	}

	// Fourth node should be a pane with empty host
	if m.flatNodes[3].Type != "pane" || m.flatNodes[3].Host != "" {
		t.Fatalf("expected local pane, got %s (host=%q)", m.flatNodes[3].Type, m.flatNodes[3].Host)
	}

	// Fifth node should be host header for "devbox"
	if m.flatNodes[4].Type != "host" || m.flatNodes[4].Name != "devbox" {
		t.Fatalf("expected host:devbox, got %s:%s", m.flatNodes[4].Type, m.flatNodes[4].Name)
	}

	// Last pane should have host "devbox"
	if m.flatNodes[7].Type != "pane" || m.flatNodes[7].Host != "devbox" {
		t.Fatalf("expected devbox pane, got %s (host=%q)", m.flatNodes[7].Type, m.flatNodes[7].Host)
	}
}

func TestMultiHostFlatNodes_RemoteError(t *testing.T) {
	m := NewModel(Options{})
	m.hostTrees = []tmux.HostTree{
		{
			Host: "",
			Tree: &tmux.Tree{
				Sessions: []tmux.TmuxSession{
					{Name: "ok-sess", Windows: []tmux.Window{{Index: 0, Name: "bash"}}},
				},
			},
		},
		{
			Host: "broken",
			Err:  errors.New("connection refused"),
		},
	}
	m.tree = &tmux.Tree{}
	m.rebuildFlatNodes()

	// Expected: [local] host, ok-sess session, bash window, [broken] host, error node = 5
	if len(m.flatNodes) != 5 {
		names := make([]string, 0, len(m.flatNodes))
		for _, n := range m.flatNodes {
			names = append(names, n.Type+":"+n.Name)
		}
		t.Fatalf("expected 5 flat nodes, got %d: %v", len(m.flatNodes), names)
	}

	// Error node should describe the problem
	errNode := m.flatNodes[4]
	if errNode.Host != "broken" {
		t.Fatalf("expected error node host=broken, got %q", errNode.Host)
	}
	if errNode.Name == "" {
		t.Fatal("expected error node to have a descriptive name")
	}
}

func TestMultiHostToggleExpand(t *testing.T) {
	m := NewModel(Options{})
	m.hostTrees = []tmux.HostTree{
		{
			Host: "",
			Tree: &tmux.Tree{
				Sessions: []tmux.TmuxSession{
					{Name: "s1", Windows: []tmux.Window{{Index: 0, Name: "w1"}}},
				},
			},
		},
		{
			Host: "devbox",
			Tree: &tmux.Tree{
				Sessions: []tmux.TmuxSession{
					{Name: "s2", Windows: []tmux.Window{{Index: 0, Name: "w2"}}},
				},
			},
		},
	}
	m.tree = &tmux.Tree{}
	m.rebuildFlatNodes()

	initialCount := len(m.flatNodes)

	// Collapse the first host node
	m.selectedIndex = 0
	m.toggleExpand()

	if len(m.flatNodes) >= initialCount {
		t.Fatalf("expected fewer nodes after collapsing host, got %d (was %d)", len(m.flatNodes), initialCount)
	}

	// The second host should still be visible
	found := false
	for _, n := range m.flatNodes {
		if n.Type == "host" && n.Name == "devbox" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected devbox host to still be visible after collapsing local host")
	}
}

func TestExecutorForHost(t *testing.T) {
	localExec := tmux.NewLocalExecutor()
	m := NewModel(Options{})
	m.hostTrees = []tmux.HostTree{
		{Host: "", Executor: localExec},
		{Host: "devbox", Executor: nil}, // no executor for broken host
	}

	if got := m.executorForHost(""); got != localExec {
		t.Fatalf("expected local executor, got %v", got)
	}
	if got := m.executorForHost("devbox"); got != nil {
		t.Fatalf("expected nil for devbox, got %v", got)
	}
	if got := m.executorForHost("nonexistent"); got != nil {
		t.Fatalf("expected nil for nonexistent, got %v", got)
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

	// Tree content starts at inputHeight + 2 (input bar + tree border + padding)
	y := inputHeight + 2
	updated, _ := m.handleLeftClick(0, y)
	m = updated.(Model)

	if len(m.flatNodes) != 1 {
		t.Fatalf("expected 1 node after icon click collapse, got %d", len(m.flatNodes))
	}
}

func TestMouseClickIconTogglesHostExpand(t *testing.T) {
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	m.calculateLayout()
	m.hostTrees = []tmux.HostTree{
		{
			Host: "",
			Tree: &tmux.Tree{
				Sessions: []tmux.TmuxSession{
					{
						Name: "local-sess",
						Windows: []tmux.Window{
							{Index: 0, Name: "local-win"},
						},
					},
				},
			},
		},
		{
			Host: "devbox",
			Tree: &tmux.Tree{
				Sessions: []tmux.TmuxSession{
					{
						Name: "remote-sess",
						Windows: []tmux.Window{
							{Index: 0, Name: "remote-win"},
						},
					},
				},
			},
		},
	}
	m.tree = &tmux.Tree{}
	m.rebuildFlatNodes()

	initialCount := len(m.flatNodes)
	if initialCount < 6 {
		t.Fatalf("expected expanded multi-host tree, got %d nodes", initialCount)
	}

	// Click the first host's `[-]` icon.
	y := inputHeight + 2
	updated, _ := m.handleLeftClick(2, y)
	m = updated.(Model)

	if len(m.flatNodes) >= initialCount {
		t.Fatalf("expected fewer nodes after collapsing host, got %d (was %d)", len(m.flatNodes), initialCount)
	}
	if len(m.flatNodes) == 0 || m.flatNodes[0].Type != "host" || m.flatNodes[0].Expanded {
		t.Fatal("expected first host node to remain visible and be collapsed")
	}

	foundSecondHost := false
	for _, node := range m.flatNodes {
		if node.Type == "host" && node.Name == "devbox" {
			foundSecondHost = true
			break
		}
	}
	if !foundSecondHost {
		t.Fatal("expected devbox host to remain visible after collapsing local host")
	}
}

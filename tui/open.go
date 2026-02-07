package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/history"
	"github.com/porganisciak/agent-tmux/tmux"
)

// OpenResult contains the outcome of the open TUI interaction.
type OpenResult struct {
	SessionName   string
	WorkingDir    string
	IsFromHistory bool
}

// RunOpen runs the quick open TUI.
func RunOpen() (*OpenResult, error) {
	m := newOpenModel()
	p := tea.NewProgram(m, tea.WithMouseCellMotion())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	if model, ok := finalModel.(openModel); ok {
		return &OpenResult{
			SessionName:   model.selectedSession,
			WorkingDir:    model.selectedDir,
			IsFromHistory: model.isHistory,
		}, nil
	}
	return &OpenResult{}, nil
}

const (
	tabActive = 0
	tabRecent = 1
)

type openModel struct {
	activeSessions  []tmux.SessionLine
	historyEntries  []history.Entry
	width           int
	height          int
	selectedIndex   int
	activeTab       int // 0=active, 1=recent
	selectedSession string
	selectedDir     string
	isHistory       bool
	loadError       error
	lineJump        lineJumpState
}

func newOpenModel() openModel {
	return openModel{activeTab: tabActive}
}

func (m openModel) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			lines, err := tmux.ListSessionsRaw()
			return openSessionsMsg{lines: lines, err: err}
		},
		func() tea.Msg {
			store, err := history.Open()
			if err != nil {
				return openHistoryMsg{err: err}
			}
			defer store.Close()
			entries, err := store.LoadHistory()
			return openHistoryMsg{entries: entries, err: err}
		},
	)
}

type openSessionsMsg struct {
	lines []tmux.SessionLine
	err   error
}

type openHistoryMsg struct {
	entries []history.Entry
	err     error
}

func (m openModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case openSessionsMsg:
		m.activeSessions = msg.lines
		if msg.err != nil {
			m.loadError = msg.err
		}
		m.clampSelection()
		return m, nil

	case openHistoryMsg:
		m.historyEntries = m.filterHistory(msg.entries)
		if msg.err != nil && m.loadError == nil {
			m.loadError = msg.err
		}
		m.clampSelection()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if idx, ok := m.lineJump.consumeKey(msg, m.currentTabLen()); ok {
			m.selectedIndex = idx
			return m, nil
		}
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.activeTab = (m.activeTab + 1) % 2
			m.selectedIndex = 0
			m.clampSelection()
			return m, nil
		case "up", "k":
			if m.selectedIndex > 0 {
				m.selectedIndex--
			}
			return m, nil
		case "down", "j":
			max := m.currentTabLen() - 1
			if m.selectedIndex < max {
				m.selectedIndex++
			}
			return m, nil
		case "enter":
			return m.selectCurrent()
		}

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// Simple click handling - items start at line 4
			itemStart := 4
			if msg.Y >= itemStart && msg.Y < itemStart+m.currentTabLen() {
				idx := msg.Y - itemStart
				if idx >= 0 && idx < m.currentTabLen() {
					m.selectedIndex = idx
					return m.selectCurrent()
				}
			}
		}
	}
	return m, nil
}

func (m openModel) currentTabLen() int {
	if m.activeTab == tabActive {
		return len(m.activeSessions)
	}
	return len(m.historyEntries)
}

func (m *openModel) clampSelection() {
	max := m.currentTabLen()
	if m.selectedIndex >= max {
		m.selectedIndex = max - 1
	}
	if m.selectedIndex < 0 {
		m.selectedIndex = 0
	}
}

func (m openModel) filterHistory(entries []history.Entry) []history.Entry {
	activeNames := make(map[string]bool)
	for _, s := range m.activeSessions {
		activeNames[s.Name] = true
	}
	var filtered []history.Entry
	for _, e := range entries {
		if !activeNames[e.SessionName] {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

func (m openModel) selectCurrent() (tea.Model, tea.Cmd) {
	if m.activeTab == tabActive {
		if m.selectedIndex < len(m.activeSessions) {
			m.selectedSession = m.activeSessions[m.selectedIndex].Name
			m.isHistory = false
		}
	} else {
		if m.selectedIndex < len(m.historyEntries) {
			entry := m.historyEntries[m.selectedIndex]
			m.selectedSession = entry.SessionName
			m.selectedDir = entry.WorkingDirectory
			m.isHistory = true
		}
	}
	return m, tea.Quit
}

func (m openModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Styles
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(primaryColor)
	tabStyle := lipgloss.NewStyle().Foreground(dimColor)
	activeTabStyle := lipgloss.NewStyle().Bold(true).Foreground(primaryColor).Underline(true)
	numStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	hintStyle := lipgloss.NewStyle().Foreground(dimColor)
	numberWidth := len(fmt.Sprintf("%d", max(1, m.currentTabLen())))

	// Title
	title := titleStyle.Render("Open Session")

	// Tabs
	var tabs string
	if m.activeTab == tabActive {
		tabs = activeTabStyle.Render("Active") + "  " + tabStyle.Render("Recent")
	} else {
		tabs = tabStyle.Render("Active") + "  " + activeTabStyle.Render("Recent")
	}
	tabs += hintStyle.Render("  (Tab to switch)")

	// Items
	var items []string
	if m.activeTab == tabActive {
		if len(m.activeSessions) == 0 {
			items = append(items, lipgloss.NewStyle().Foreground(dimColor).Render("  No active sessions"))
		} else {
			for i, s := range m.activeSessions {
				numText := fmt.Sprintf("%*d.", numberWidth, i+1)
				var row string
				if i == m.selectedIndex {
					row = selectedStyle.Render("> ") +
						selectedStyle.Render(numText) +
						" " +
						formatSessionName(s.Name, selectedStyle)
				} else {
					row = "  " +
						numStyle.Render(numText) +
						" " +
						formatSessionName(s.Name, lipgloss.NewStyle())
				}
				items = append(items, row)
			}
		}
	} else {
		if len(m.historyEntries) == 0 {
			items = append(items, lipgloss.NewStyle().Foreground(dimColor).Render("  No recent sessions"))
		} else {
			for i, e := range m.historyEntries {
				numText := fmt.Sprintf("%*d.", numberWidth, i+1)
				ago := openTimeAgo(e.LastUsedAt)
				var row string
				if i == m.selectedIndex {
					row = selectedStyle.Render("> ") +
						selectedStyle.Render(numText) +
						" " +
						formatSessionName(e.Name, selectedStyle) +
						"  " +
						selectedStyle.Render("("+ago+")")
				} else {
					row = "  " +
						numStyle.Render(numText) +
						" " +
						formatSessionName(e.Name, lipgloss.NewStyle()) +
						"  " +
						hintStyle.Render("("+ago+")")
				}
				items = append(items, row)
			}
		}
	}

	// Hints
	hints := hintStyle.Render("digits jump • ↑↓ navigate • Enter select • q quit")

	// Combine
	sections := []string{title, tabs, ""}
	sections = append(sections, items...)
	sections = append(sections, "", hints)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func openTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

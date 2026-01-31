package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/history"
	"github.com/porganisciak/agent-tmux/tmux"
)

type SessionsOptions struct {
	AltScreen bool
}

// SessionsResult contains the outcome of the sessions list interaction.
type SessionsResult struct {
	SessionName  string // Session selected for attach, empty if quit
	WorkingDir   string // Working directory for revival (if from history)
	IsFromHistory bool   // True if reviving from history rather than attaching
}

// RunSessionsList runs a simple session list UI and returns the selected session.
func RunSessionsList(opts SessionsOptions) (*SessionsResult, error) {
	m := newSessionsModel()
	programOptions := []tea.ProgramOption{
		tea.WithMouseCellMotion(),
	}
	if opts.AltScreen {
		programOptions = append(programOptions, tea.WithAltScreen())
	}
	p := tea.NewProgram(m, programOptions...)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	if model, ok := finalModel.(sessionsModel); ok {
		return &SessionsResult{
			SessionName:   model.attachSession,
			WorkingDir:    model.reviveDir,
			IsFromHistory: model.isHistorySelection,
		}, nil
	}
	return &SessionsResult{}, nil
}

type sessionsModel struct {
	lines              []tmux.SessionLine
	historyEntries     []history.Entry
	width              int
	height             int
	selectedIndex      int
	attachSession      string
	reviveDir          string
	isHistorySelection bool
	lastError          error
	historyError       error
}

func newSessionsModel() sessionsModel {
	return sessionsModel{selectedIndex: 0}
}

func (m sessionsModel) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			lines, err := tmux.ListSessionsRaw()
			return sessionsLoadedMsg{lines: lines, err: err}
		},
		func() tea.Msg {
			store, err := history.Open()
			if err != nil {
				return historyLoadedMsg{err: err}
			}
			defer store.Close()
			entries, err := store.LoadHistory()
			return historyLoadedMsg{entries: entries, err: err}
		},
	)
}

type sessionsLoadedMsg struct {
	lines []tmux.SessionLine
	err   error
}

type historyLoadedMsg struct {
	entries []history.Entry
	err     error
}

func (m sessionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionsLoadedMsg:
		m.lines = msg.lines
		m.lastError = msg.err
		m.clampSelection()
		return m, nil
	case historyLoadedMsg:
		m.historyEntries = m.filterHistory(msg.entries)
		m.historyError = msg.err
		m.clampSelection()
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.selectedIndex > 0 {
				m.selectedIndex--
			}
			return m, nil
		case "down", "j":
			total := m.totalItems()
			if m.selectedIndex < total-1 {
				m.selectedIndex++
			}
			return m, nil
		case "enter":
			return m.selectCurrent()
		}
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			headerHeight := 2
			total := m.totalItems()
			// Active sessions area
			if msg.Y >= headerHeight && msg.Y < headerHeight+len(m.lines) {
				clicked := msg.Y - headerHeight
				if clicked >= 0 && clicked < len(m.lines) {
					m.selectedIndex = clicked
					return m.selectCurrent()
				}
			}
			// Recent history area (after sessions + header)
			recentHeaderY := headerHeight + len(m.lines) + 2 // +2 for spacing and header
			if msg.Y >= recentHeaderY && msg.Y < recentHeaderY+len(m.historyEntries) {
				clicked := msg.Y - recentHeaderY + len(m.lines)
				if clicked >= len(m.lines) && clicked < total {
					m.selectedIndex = clicked
					return m.selectCurrent()
				}
			}
		}
	}
	return m, nil
}

// totalItems returns the total number of selectable items.
func (m sessionsModel) totalItems() int {
	return len(m.lines) + len(m.historyEntries)
}

// clampSelection ensures selectedIndex is within bounds.
func (m *sessionsModel) clampSelection() {
	total := m.totalItems()
	if m.selectedIndex >= total {
		m.selectedIndex = total - 1
	}
	if m.selectedIndex < 0 {
		m.selectedIndex = 0
	}
}

// filterHistory removes history entries that have active sessions.
func (m sessionsModel) filterHistory(entries []history.Entry) []history.Entry {
	activeNames := make(map[string]bool)
	for _, line := range m.lines {
		activeNames[line.Name] = true
	}
	var filtered []history.Entry
	for _, e := range entries {
		if !activeNames[e.SessionName] {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// selectCurrent handles selection of the current item.
func (m sessionsModel) selectCurrent() (tea.Model, tea.Cmd) {
	if m.selectedIndex < len(m.lines) {
		// Active session
		m.attachSession = m.lines[m.selectedIndex].Name
		m.isHistorySelection = false
	} else {
		// History entry
		histIdx := m.selectedIndex - len(m.lines)
		if histIdx >= 0 && histIdx < len(m.historyEntries) {
			entry := m.historyEntries[histIdx]
			m.attachSession = entry.SessionName
			m.reviveDir = entry.WorkingDirectory
			m.isHistorySelection = true
		}
	}
	return m, tea.Quit
}

func (m sessionsModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	title := lipgloss.NewStyle().Bold(true).Render("Sessions")
	subtitle := lipgloss.NewStyle().Foreground(dimColor).Render("↑↓ select, Enter attach, q quit")

	var sections []string
	sections = append(sections, title, subtitle, "")

	// Error display
	if m.lastError != nil {
		err := lipgloss.NewStyle().Foreground(errorColor).Render("Error: " + m.lastError.Error())
		sections = append(sections, err)
	}

	// Active sessions section
	sectionHeader := lipgloss.NewStyle().Bold(true).Foreground(secondaryColor)
	if len(m.lines) > 0 {
		sections = append(sections, sectionHeader.Render("Active"))
		for i, line := range m.lines {
			row := "  " + line.Line
			if i == m.selectedIndex {
				row = selectedStyle.Render("> " + line.Line)
			}
			sections = append(sections, row)
		}
	} else {
		sections = append(sections, sectionHeader.Render("Active"))
		sections = append(sections, lipgloss.NewStyle().Foreground(dimColor).Render("  No active sessions"))
	}

	// Recent history section
	if len(m.historyEntries) > 0 {
		sections = append(sections, "") // spacing
		sections = append(sections, sectionHeader.Render("Recent"))
		for i, entry := range m.historyEntries {
			globalIdx := len(m.lines) + i
			ago := sessionsTimeAgo(entry.LastUsedAt)
			row := fmt.Sprintf("  %s  %s", entry.Name, lipgloss.NewStyle().Foreground(dimColor).Render("("+ago+")"))
			if globalIdx == m.selectedIndex {
				row = selectedStyle.Render(fmt.Sprintf("> %s  (%s)", entry.Name, ago))
			}
			sections = append(sections, row)
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func sessionsTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

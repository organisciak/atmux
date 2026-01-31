package tui

import (
	"fmt"
	"strings"
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
	SessionName   string // Session selected for attach, empty if quit
	WorkingDir    string // Working directory for revival (if from history)
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
	memoryBySession    map[string]tmux.SessionMemory
	width              int
	height             int
	selectedIndex      int
	attachSession      string
	reviveDir          string
	isHistorySelection bool
	lastError          error
	historyError       error
	memoryError        error
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
			memory, err := tmux.FetchSessionMemory()
			return memoryLoadedMsg{memory: memory, err: err}
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

type memoryLoadedMsg struct {
	memory map[string]tmux.SessionMemory
	err    error
}

func (m sessionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionsLoadedMsg:
		m.lines = msg.lines
		m.lastError = msg.err
		m.clampSelection()
		return m, nil
	case memoryLoadedMsg:
		m.memoryBySession = msg.memory
		m.memoryError = msg.err
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
			memSummary := m.memorySummary(line.Name)
			if memSummary != "" {
				row += "  " + lipgloss.NewStyle().Foreground(dimColor).Render(memSummary)
			}
			if i == m.selectedIndex {
				row = selectedStyle.Render("> " + line.Line)
				if memSummary != "" {
					row += "  " + lipgloss.NewStyle().Foreground(dimColor).Render(memSummary)
				}
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

func (m sessionsModel) memorySummary(sessionName string) string {
	if m.memoryBySession == nil {
		return ""
	}
	mem, ok := m.memoryBySession[sessionName]
	if !ok {
		return ""
	}
	return formatSessionMemory(mem)
}

func formatSessionMemory(mem tmux.SessionMemory) string {
	var windows []string
	for _, win := range mem.Windows {
		if len(win.Panes) == 0 {
			continue
		}
		label := win.Name
		if label == "" {
			label = fmt.Sprintf("win%d", win.Index)
		}
		var panes []string
		for _, pane := range win.Panes {
			if pane.RSSBytes <= 0 {
				continue
			}
			panes = append(panes, fmt.Sprintf("%d:%s", pane.Index, formatMemoryBytes(pane.RSSBytes)))
		}
		if len(panes) == 0 {
			continue
		}
		windows = append(windows, fmt.Sprintf("%s[%s]", label, strings.Join(panes, " ")))
	}
	if len(windows) == 0 {
		return ""
	}
	return strings.Join(windows, " ")
}

func formatMemoryBytes(b int64) string {
	const kb = int64(1024)
	const mb = 1024 * kb
	const gb = 1024 * mb

	switch {
	case b >= gb:
		return fmt.Sprintf("%.1fG", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%dM", (b+mb/2)/mb)
	case b >= kb:
		return fmt.Sprintf("%dK", (b+kb/2)/kb)
	default:
		return fmt.Sprintf("%dB", b)
	}
}

package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/history"
)

// RecentsOptions configures the recents TUI.
type RecentsOptions struct {
	AltScreen bool
	Limit     int // Maximum entries to show (0 = all)
}

// RecentsResult contains the outcome of the recents interaction.
type RecentsResult struct {
	SessionName string // Selected session name (empty if quit)
	WorkingDir  string // Working directory to revive in
}

// RunRecents runs the recents TUI and returns the selected session.
func RunRecents(opts RecentsOptions) (*RecentsResult, error) {
	m := newRecentsModel(opts)
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
	if model, ok := finalModel.(recentsModel); ok {
		return &RecentsResult{
			SessionName: model.selectedSession,
			WorkingDir:  model.selectedDir,
		}, nil
	}
	return &RecentsResult{}, nil
}

// recentsModel is the Bubble Tea model for the recents view.
type recentsModel struct {
	entries         []history.Entry
	filteredEntries []history.Entry
	width           int
	height          int
	selectedIndex   int
	selectedSession string
	selectedDir     string
	filterText      string
	filterMode      bool
	lastError       error
	limit           int
}

func newRecentsModel(opts RecentsOptions) recentsModel {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	return recentsModel{
		selectedIndex: 0,
		limit:         limit,
	}
}

func (m recentsModel) Init() tea.Cmd {
	return func() tea.Msg {
		store, err := history.Open()
		if err != nil {
			return recentsLoadedMsg{err: err}
		}
		defer store.Close()
		entries, err := store.LoadHistory()
		return recentsLoadedMsg{entries: entries, err: err}
	}
}

type recentsLoadedMsg struct {
	entries []history.Entry
	err     error
}

type recentsDeletedMsg struct {
	id  int64
	err error
}

func (m recentsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case recentsLoadedMsg:
		m.entries = msg.entries
		m.lastError = msg.err
		m.applyFilter()
		m.clampSelection()
		return m, nil

	case recentsDeletedMsg:
		if msg.err != nil {
			m.lastError = msg.err
			return m, nil
		}
		m.entries = removeEntry(m.entries, msg.id)
		m.applyFilter()
		m.clampSelection()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.filterMode {
			return m.handleFilterKey(msg)
		}
		return m.handleNormalKey(msg)

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			return m.handleMouseClick(msg)
		}
	}
	return m, nil
}

func (m recentsModel) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.selectedIndex > 0 {
			m.selectedIndex--
		}
		return m, nil
	case "down", "j":
		if m.selectedIndex < len(m.filteredEntries)-1 {
			m.selectedIndex++
		}
		return m, nil
	case "enter":
		return m.selectCurrent()
	case "/":
		m.filterMode = true
		return m, nil
	case "x", "delete", "backspace":
		if cmd := m.deleteSelected(); cmd != nil {
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}

func (m recentsModel) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filterMode = false
		m.filterText = ""
		m.applyFilter()
		m.clampSelection()
		return m, nil
	case "enter":
		m.filterMode = false
		return m, nil
	case "backspace":
		if len(m.filterText) > 0 {
			m.filterText = m.filterText[:len(m.filterText)-1]
			m.applyFilter()
			m.clampSelection()
		}
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	default:
		// Add character to filter if printable
		if len(msg.String()) == 1 {
			m.filterText += msg.String()
			m.applyFilter()
			m.clampSelection()
		}
		return m, nil
	}
}

func (m recentsModel) handleMouseClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Header is 4 lines (title, subtitle, empty, section header)
	headerHeight := 4
	clickedIdx := msg.Y - headerHeight
	if clickedIdx >= 0 && clickedIdx < len(m.filteredEntries) {
		m.selectedIndex = clickedIdx
		return m.selectCurrent()
	}
	return m, nil
}

func (m recentsModel) selectCurrent() (tea.Model, tea.Cmd) {
	if m.selectedIndex >= 0 && m.selectedIndex < len(m.filteredEntries) {
		entry := m.filteredEntries[m.selectedIndex]
		m.selectedSession = entry.SessionName
		m.selectedDir = entry.WorkingDirectory
	}
	return m, tea.Quit
}

func (m *recentsModel) applyFilter() {
	if m.filterText == "" {
		// Apply limit
		if m.limit > 0 && len(m.entries) > m.limit {
			m.filteredEntries = m.entries[:m.limit]
		} else {
			m.filteredEntries = m.entries
		}
		return
	}

	filter := strings.ToLower(m.filterText)
	var filtered []history.Entry
	for _, e := range m.entries {
		// Search in name and working directory
		if strings.Contains(strings.ToLower(e.Name), filter) ||
			strings.Contains(strings.ToLower(e.WorkingDirectory), filter) {
			filtered = append(filtered, e)
		}
	}
	// Apply limit
	if m.limit > 0 && len(filtered) > m.limit {
		m.filteredEntries = filtered[:m.limit]
	} else {
		m.filteredEntries = filtered
	}
}

func (m *recentsModel) clampSelection() {
	if m.selectedIndex >= len(m.filteredEntries) {
		m.selectedIndex = len(m.filteredEntries) - 1
	}
	if m.selectedIndex < 0 {
		m.selectedIndex = 0
	}
}

func (m recentsModel) deleteSelected() tea.Cmd {
	if m.selectedIndex < 0 || m.selectedIndex >= len(m.filteredEntries) {
		return nil
	}
	entry := m.filteredEntries[m.selectedIndex]
	return func() tea.Msg {
		store, err := history.Open()
		if err != nil {
			return recentsDeletedMsg{id: entry.ID, err: err}
		}
		defer store.Close()
		return recentsDeletedMsg{id: entry.ID, err: store.DeleteEntry(entry.ID)}
	}
}

func removeEntry(entries []history.Entry, id int64) []history.Entry {
	for i, entry := range entries {
		if entry.ID == id {
			return append(entries[:i], entries[i+1:]...)
		}
	}
	return entries
}

func (m recentsModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	title := lipgloss.NewStyle().Bold(true).Render("Recent Sessions")
	subtitle := lipgloss.NewStyle().Foreground(dimColor).Render("Enter: revive  /: filter  x: remove  q: quit")

	var sections []string
	sections = append(sections, title, subtitle, "")

	// Error display
	if m.lastError != nil {
		err := lipgloss.NewStyle().Foreground(errorColor).Render("Error: " + m.lastError.Error())
		sections = append(sections, err)
	}

	// Filter display
	if m.filterMode || m.filterText != "" {
		filterLabel := "Filter: "
		if m.filterMode {
			filterLabel = lipgloss.NewStyle().Foreground(primaryColor).Render("Filter: ")
		}
		filterDisplay := filterLabel + m.filterText
		if m.filterMode {
			filterDisplay += lipgloss.NewStyle().Foreground(primaryColor).Render("_")
		}
		sections = append(sections, filterDisplay, "")
	}

	// Session list
	if len(m.filteredEntries) == 0 {
		if m.filterText != "" {
			sections = append(sections, lipgloss.NewStyle().Foreground(dimColor).Render("  No matching sessions"))
		} else {
			sections = append(sections, lipgloss.NewStyle().Foreground(dimColor).Render("  No recent sessions"))
		}
	} else {
		for i, entry := range m.filteredEntries {
			row := m.renderEntry(entry, i == m.selectedIndex)
			sections = append(sections, row)
		}
	}

	// Add tip at the bottom
	sections = append(sections, "", RenderTip())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m recentsModel) renderEntry(entry history.Entry, selected bool) string {
	// Format time ago
	ago := recentsTimeAgo(entry.LastUsedAt)
	agoStyle := lipgloss.NewStyle().Foreground(dimColor)

	// Shorten working directory
	displayPath := shortenHomePath(entry.WorkingDirectory)
	pathStyle := lipgloss.NewStyle().Foreground(dimColor)

	// Format name with dimmed prefix
	var nameStr string
	if selected {
		nameStr = formatSessionName(entry.Name, selectedStyle)
	} else {
		nameStr = formatSessionName(entry.Name, lipgloss.NewStyle())
	}

	// Build row
	var prefix string
	if selected {
		prefix = selectedStyle.Render("> ")
	} else {
		prefix = "  "
	}

	// Layout: prefix + name + padding + path + time
	// For now, simple layout
	return fmt.Sprintf("%s%-20s  %s  %s",
		prefix,
		nameStr,
		pathStyle.Render(displayPath),
		agoStyle.Render("("+ago+")"))
}

func recentsTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		m := int(d.Minutes())
		if m <= 1 {
			return "just now"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

func shortenHomePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

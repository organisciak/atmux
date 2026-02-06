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
	Executors []tmux.TmuxExecutor // Executors for local + remote hosts
}

// SessionsResult contains the outcome of the sessions list interaction.
type SessionsResult struct {
	SessionName   string           // Session selected for attach, empty if quit
	WorkingDir    string           // Working directory for revival (if from history)
	IsFromHistory bool             // True if reviving from history rather than attaching
	Host          string           // Host label for remote sessions ("" for local)
	Executor      tmux.TmuxExecutor // The executor for the selected session
}

// RunSessionsList runs a simple session list UI and returns the selected session.
func RunSessionsList(opts SessionsOptions) (*SessionsResult, error) {
	executors := opts.Executors
	if len(executors) == 0 {
		executors = []tmux.TmuxExecutor{tmux.NewLocalExecutor()}
	}
	m := newSessionsModel(executors)
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
		var exec tmux.TmuxExecutor
		if model.executorMap != nil {
			if e, ok := model.executorMap[model.selectedHost]; ok {
				exec = e
			}
		}
		if exec == nil && len(executors) > 0 {
			exec = executors[0]
		}
		return &SessionsResult{
			SessionName:   model.attachSession,
			WorkingDir:    model.reviveDir,
			IsFromHistory: model.isHistorySelection,
			Host:          model.selectedHost,
			Executor:      exec,
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
	selectedHost       string
	lastError          error
	historyError       error
	memoryError        error
	executors          []tmux.TmuxExecutor
	executorMap        map[string]tmux.TmuxExecutor
	confirmKill        bool
	killSessionName    string
}

func newSessionsModel(executors []tmux.TmuxExecutor) sessionsModel {
	executorMap := make(map[string]tmux.TmuxExecutor, len(executors))
	for _, exec := range executors {
		executorMap[exec.HostLabel()] = exec
	}
	return sessionsModel{
		selectedIndex: 0,
		executors:     executors,
		executorMap:   executorMap,
	}
}

func (m sessionsModel) Init() tea.Cmd {
	return tea.Batch(
		m.fetchAllSessions(),
		func() tea.Msg {
			// Only fetch memory for local sessions
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

// fetchAllSessions fetches sessions from all executors.
func (m sessionsModel) fetchAllSessions() tea.Cmd {
	return func() tea.Msg {
		var allLines []tmux.SessionLine
		for _, exec := range m.executors {
			lines, err := tmux.ListSessionsRawWithExecutor(exec)
			if err != nil {
				continue // Skip unreachable hosts
			}
			allLines = append(allLines, lines...)
		}
		return sessionsLoadedMsg{lines: allLines, err: nil}
	}
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

type killSessionMsg struct {
	sessionName string
	err         error
}

func (m sessionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle kill confirmation if active
	if m.confirmKill {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "y", "Y":
				m.confirmKill = false
				return m, m.killSession(m.killSessionName)
			case "n", "N", "esc":
				m.confirmKill = false
				return m, nil
			}
			return m, nil // Ignore other keys while confirmation is shown
		}
	}

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
	case historyDeletedMsg:
		if msg.err != nil {
			m.historyError = msg.err
			return m, nil
		}
		m.historyEntries = removeHistoryEntry(m.historyEntries, msg.id)
		m.clampSelection()
		return m, nil
	case killSessionMsg:
		if msg.err != nil {
			m.lastError = msg.err
			return m, nil
		}
		// Refresh sessions and history after killing
		m.killSessionName = ""
		return m, tea.Batch(
			m.fetchAllSessions(),
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
		case "x", "delete", "backspace":
			if m.selectedIndex < len(m.lines) {
				// Active session: prompt to kill
				line := m.lines[m.selectedIndex]
				m.confirmKill = true
				m.killSessionName = line.Name
				return m, nil
			}
			// History entry: delete from history
			if cmd := m.deleteSelectedHistoryEntry(); cmd != nil {
				return m, cmd
			}
			return m, nil
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
		line := m.lines[m.selectedIndex]
		m.attachSession = line.Name
		m.selectedHost = line.Host
		m.isHistorySelection = false
	} else {
		// History entry
		histIdx := m.selectedIndex - len(m.lines)
		if histIdx >= 0 && histIdx < len(m.historyEntries) {
			entry := m.historyEntries[histIdx]
			m.attachSession = entry.SessionName
			m.reviveDir = entry.WorkingDirectory
			m.isHistorySelection = true
			m.selectedHost = "" // History is always local
		}
	}
	return m, tea.Quit
}

func (m sessionsModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	title := lipgloss.NewStyle().Bold(true).Render("Sessions")
	xHint := "x remove"
	if m.selectedIndex < len(m.lines) {
		xHint = "x kill"
	}
	subtitle := lipgloss.NewStyle().Foreground(dimColor).Render("↑↓ select, Enter attach, " + xHint + ", q quit")

	var sections []string

	// Show kill confirmation if active
	if m.confirmKill {
		sections = append(sections, title, subtitle, "")
		warning := lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true).
			Render(fmt.Sprintf("Kill session '%s'? (y/n)", m.killSessionName))
		// Check if this is the currently attached session
		for _, line := range m.lines {
			if line.Name == m.killSessionName && strings.Contains(line.Line, "(attached)") {
				warning += "\n" + lipgloss.NewStyle().
					Foreground(errorColor).
					Render("WARNING: This is the currently attached session!")
				break
			}
		}
		sections = append(sections, warning)
		return lipgloss.JoinVertical(lipgloss.Left, sections...)
	}

	sections = append(sections, title, subtitle, "")

	// Error display
	if m.lastError != nil {
		err := lipgloss.NewStyle().Foreground(errorColor).Render("Error: " + m.lastError.Error())
		sections = append(sections, err)
	}
	if m.historyError != nil {
		err := lipgloss.NewStyle().Foreground(errorColor).Render("History error: " + m.historyError.Error())
		sections = append(sections, err)
	}

	// Active sessions section - group by host if remotes exist
	sectionHeader := lipgloss.NewStyle().Bold(true).Foreground(secondaryColor)

	hasRemote := false
	for _, line := range m.lines {
		if line.Host != "" {
			hasRemote = true
			break
		}
	}

	if len(m.lines) > 0 {
		if hasRemote {
			// Group by host
			type hostGroup struct {
				host  string
				lines []tmux.SessionLine
				start int // global index start
			}
			var groups []hostGroup
			groupMap := make(map[string]*hostGroup)
			idx := 0
			for _, line := range m.lines {
				h := line.Host
				if g, ok := groupMap[h]; ok {
					g.lines = append(g.lines, line)
				} else {
					g := &hostGroup{host: h, start: idx}
					g.lines = append(g.lines, line)
					groupMap[h] = g
					groups = append(groups, *g)
				}
				idx++
			}
			// Render each group
			// Rebuild groups since we need to recalculate after map mutation
			currentIdx := 0
			for _, g := range groups {
				hostLabel := "Active (local)"
				if g.host != "" {
					hostLabel = "Active @ " + g.host
				}
				sections = append(sections, sectionHeader.Render(hostLabel))
				for _, line := range groupMap[g.host].lines {
					var row string
					memSummary := m.memorySummary(line.Name)
					if currentIdx == m.selectedIndex {
						row = selectedStyle.Render("> ") + formatSessionLine(line.Line, selectedStyle)
					} else {
						row = "  " + formatSessionLine(line.Line, lipgloss.NewStyle())
					}
					if memSummary != "" {
						row += "  " + lipgloss.NewStyle().Foreground(dimColor).Render(memSummary)
					}
					sections = append(sections, row)
					currentIdx++
				}
			}
		} else {
			sections = append(sections, sectionHeader.Render("Active"))
			for i, line := range m.lines {
				var row string
				memSummary := m.memorySummary(line.Name)
				if i == m.selectedIndex {
					row = selectedStyle.Render("> ") + formatSessionLine(line.Line, selectedStyle)
				} else {
					row = "  " + formatSessionLine(line.Line, lipgloss.NewStyle())
				}
				if memSummary != "" {
					row += "  " + lipgloss.NewStyle().Foreground(dimColor).Render(memSummary)
				}
				sections = append(sections, row)
			}
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
			meta := lipgloss.NewStyle().Foreground(dimColor).Render("(" + ago + ")")
			dir := lipgloss.NewStyle().Foreground(dimColor).Render(entry.WorkingDirectory)
			var row string
			if globalIdx == m.selectedIndex {
				formattedName := formatSessionName(entry.Name, selectedStyle)
				row = selectedStyle.Render("> ") + formattedName + "  " + meta + "  " + dir
			} else {
				formattedName := formatSessionName(entry.Name, lipgloss.NewStyle())
				row = "  " + formattedName + "  " + meta + "  " + dir
			}
			sections = append(sections, row)
		}
	}

	// Add tip at the bottom
	sections = append(sections, "", RenderTipForContext(TipSessions))

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

type historyDeletedMsg struct {
	id  int64
	err error
}

func (m sessionsModel) selectedHistoryEntry() (history.Entry, bool) {
	if m.selectedIndex < len(m.lines) {
		return history.Entry{}, false
	}
	idx := m.selectedIndex - len(m.lines)
	if idx < 0 || idx >= len(m.historyEntries) {
		return history.Entry{}, false
	}
	return m.historyEntries[idx], true
}

func (m sessionsModel) deleteSelectedHistoryEntry() tea.Cmd {
	entry, ok := m.selectedHistoryEntry()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		store, err := history.Open()
		if err != nil {
			return historyDeletedMsg{id: entry.ID, err: err}
		}
		defer store.Close()
		return historyDeletedMsg{id: entry.ID, err: store.DeleteEntry(entry.ID)}
	}
}

func (m sessionsModel) killSession(name string) tea.Cmd {
	return func() tea.Msg {
		err := tmux.KillSession(name)
		return killSessionMsg{sessionName: name, err: err}
	}
}

func removeHistoryEntry(entries []history.Entry, id int64) []history.Entry {
	for i, entry := range entries {
		if entry.ID == id {
			return append(entries[:i], entries[i+1:]...)
		}
	}
	return entries
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

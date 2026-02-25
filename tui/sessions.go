package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/history"
	"github.com/porganisciak/agent-tmux/tmux"
)

// stalenessTier classifies session freshness.
type stalenessTier int

const (
	tierFresh        stalenessTier = iota
	tierGettingStale               // between fresh and stale thresholds
	tierStale                      // beyond stale threshold
)

type SessionsOptions struct {
	AltScreen        bool
	Executors        []tmux.TmuxExecutor // Executors for local + remote hosts
	ShowBeads        bool                // Show beads issue counts per session
	DisableStaleness bool                // Disable staleness indicators
}

// SessionsResult contains the outcome of the sessions list interaction.
type SessionsResult struct {
	SessionName   string            // Session selected for attach, empty if quit
	WorkingDir    string            // Working directory for revival (if from history)
	IsFromHistory bool              // True if reviving from history rather than attaching
	Host          string            // Host label for remote sessions ("" for local)
	Executor      tmux.TmuxExecutor // The executor for the selected session
}

// RunSessionsList runs a simple session list UI and returns the selected session.
func RunSessionsList(opts SessionsOptions) (*SessionsResult, error) {
	executors := opts.Executors
	if len(executors) == 0 {
		executors = []tmux.TmuxExecutor{tmux.NewLocalExecutor()}
	}
	m := newSessionsModel(executors, opts.ShowBeads, opts.DisableStaleness)
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
	beadsCounts        map[string]*int // nil value = not loaded yet; *int distinguishes "not loaded" from "0 open"
	showBeads          bool
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
	rawHistoryEntries  []history.Entry   // Unfiltered history (for re-filtering)
	pendingExecutors   int               // Executors still loading
	confirmKill        bool
	killSessionName    string
	lineJump           lineJumpState

	// Staleness
	stalenessDisabled    bool
	freshThreshold       time.Duration
	staleThreshold       time.Duration
	suggestionThreshold  int
	confirmKillStale     bool
	staleSessionNames    []string
}

func newSessionsModel(executors []tmux.TmuxExecutor, showBeads bool, disableStaleness bool) sessionsModel {
	executorMap := make(map[string]tmux.TmuxExecutor, len(executors))
	for _, exec := range executors {
		executorMap[exec.HostLabel()] = exec
	}

	// Load staleness config
	var stalenessDisabled bool
	var freshThreshold, staleThreshold time.Duration
	var suggestionThreshold int

	settings, err := config.LoadSettings()
	if err == nil && settings.Staleness != nil {
		stalenessDisabled = settings.Staleness.Disabled
		freshThreshold, staleThreshold = settings.Staleness.ParsedStalenessThresholds()
		suggestionThreshold = settings.Staleness.EffectiveSuggestionThreshold()
	} else {
		freshThreshold, staleThreshold = (&config.StalenessConfig{}).ParsedStalenessThresholds()
		suggestionThreshold = (&config.StalenessConfig{}).EffectiveSuggestionThreshold()
	}
	if disableStaleness {
		stalenessDisabled = true
	}

	return sessionsModel{
		selectedIndex:       0,
		executors:           executors,
		executorMap:         executorMap,
		showBeads:           showBeads,
		pendingExecutors:    len(executors),
		stalenessDisabled:   stalenessDisabled,
		freshThreshold:      freshThreshold,
		staleThreshold:      staleThreshold,
		suggestionThreshold: suggestionThreshold,
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

// fetchAllSessions launches one async command per executor so that local
// sessions appear immediately and remote hosts pop in when ready.
func (m sessionsModel) fetchAllSessions() tea.Cmd {
	var cmds []tea.Cmd
	for _, exec := range m.executors {
		executor := exec // capture for closure
		cmds = append(cmds, func() tea.Msg {
			lines, err := tmux.ListSessionsRawWithExecutor(executor)
			return executorSessionsMsg{lines: lines, err: err}
		})
	}
	return tea.Batch(cmds...)
}

// groupSessionsByHost reorders sessions so local sessions come first, then
// each remote host group, preserving activity order within each group.
// This keeps m.lines indices consistent with the display order when the
// View groups by host.
func groupSessionsByHost(lines []tmux.SessionLine) []tmux.SessionLine {
	var local []tmux.SessionLine
	remoteGroups := make(map[string][]tmux.SessionLine)
	var remoteOrder []string
	for _, line := range lines {
		if line.Host == "" {
			local = append(local, line)
		} else {
			if _, seen := remoteGroups[line.Host]; !seen {
				remoteOrder = append(remoteOrder, line.Host)
			}
			remoteGroups[line.Host] = append(remoteGroups[line.Host], line)
		}
	}
	result := make([]tmux.SessionLine, 0, len(lines))
	result = append(result, local...)
	for _, host := range remoteOrder {
		result = append(result, remoteGroups[host]...)
	}
	return result
}

// executorSessionsMsg is sent when a single executor finishes loading sessions.
type executorSessionsMsg struct {
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

type beadsCountMsg struct {
	sessionName string
	count       int
	hasBeads    bool
	err         error
}

func fetchBeadsCount(sessionName string) tea.Cmd {
	return func() tea.Msg {
		path := tmux.GetSessionPath(sessionName)
		if path == "" {
			return beadsCountMsg{sessionName: sessionName, hasBeads: false}
		}
		if _, err := os.Stat(filepath.Join(path, ".beads")); err != nil {
			return beadsCountMsg{sessionName: sessionName, hasBeads: false}
		}
		cmd := exec.Command("bd", "count", "--status=open", "--json")
		cmd.Dir = path
		output, err := cmd.Output()
		if err != nil {
			return beadsCountMsg{sessionName: sessionName, hasBeads: true, err: err}
		}
		var result struct {
			Count int `json:"count"`
		}
		json.Unmarshal(output, &result)
		return beadsCountMsg{sessionName: sessionName, count: result.Count, hasBeads: true}
	}
}

func (m sessionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle kill confirmation if active
	if m.confirmKill {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "enter":
				m.confirmKill = false
				return m, m.killSession(m.killSessionName)
			case "esc", "n", "N":
				m.confirmKill = false
				return m, nil
			}
			return m, nil // Ignore other keys while confirmation is shown
		}
	}

	// Handle kill-stale confirmation if active
	if m.confirmKillStale {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "enter":
				m.confirmKillStale = false
				names := m.staleSessionNames
				m.staleSessionNames = nil
				return m, m.killMultipleSessions(names)
			case "esc", "n", "N":
				m.confirmKillStale = false
				m.staleSessionNames = nil
				return m, nil
			}
			return m, nil
		}
	}

	switch msg := msg.(type) {
	case executorSessionsMsg:
		m.pendingExecutors--
		if msg.err == nil && len(msg.lines) > 0 {
			m.lines = append(m.lines, msg.lines...)
			sort.SliceStable(m.lines, func(i, j int) bool {
				return m.lines[i].Activity > m.lines[j].Activity
			})
			m.lines = groupSessionsByHost(m.lines)
			// Re-filter history against updated session list
			if m.rawHistoryEntries != nil {
				m.historyEntries = m.filterHistory(m.rawHistoryEntries)
			}
			m.clampSelection()
			// Trigger beads loading for newly arrived local sessions
			if m.showBeads {
				var cmds []tea.Cmd
				for _, line := range msg.lines {
					if line.Host == "" {
						cmds = append(cmds, fetchBeadsCount(line.Name))
					}
				}
				if len(cmds) > 0 {
					return m, tea.Batch(cmds...)
				}
			}
		}
		return m, nil
	case beadsCountMsg:
		if !msg.hasBeads {
			return m, nil
		}
		if m.beadsCounts == nil {
			m.beadsCounts = make(map[string]*int)
		}
		if msg.err != nil {
			return m, nil
		}
		count := msg.count
		m.beadsCounts[msg.sessionName] = &count
		return m, nil
	case memoryLoadedMsg:
		m.memoryBySession = msg.memory
		m.memoryError = msg.err
		return m, nil
	case historyLoadedMsg:
		m.rawHistoryEntries = msg.entries
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
		m.lines = nil
		m.pendingExecutors = len(m.executors)
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
	case killMultipleSessionsMsg:
		if msg.err != nil {
			m.lastError = msg.err
			return m, nil
		}
		m.lines = nil
		m.pendingExecutors = len(m.executors)
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
		if idx, ok := m.lineJump.consumeKey(msg, len(m.lines)); ok {
			m.selectedIndex = idx
			return m, nil
		}
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
		case "S":
			if !m.stalenessDisabled {
				stale := m.staleSessions()
				if len(stale) > 0 {
					m.confirmKillStale = true
					m.staleSessionNames = stale
				}
			}
			return m, nil
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
			// Build a Y-position → item-index mapping that accounts for
			// all non-selectable rows (title, subtitle, headers, banners).
			y := 0
			y += 3 // title + subtitle + blank line

			// Staleness suggestion banner
			if !m.stalenessDisabled && len(m.lines) >= m.suggestionThreshold && m.staleSessionCount() > 0 {
				y += 2 // banner + blank
			}

			// Error lines
			if m.lastError != nil {
				y++
			}
			if m.historyError != nil {
				y++
			}

			// Active sessions with host group headers
			total := m.totalItems()
			lastHost := "\x00"
			hasRemote := false
			for _, line := range m.lines {
				if line.Host != "" {
					hasRemote = true
					break
				}
			}
			activeStartY := y
			for i, line := range m.lines {
				if hasRemote && line.Host != lastHost {
					y++ // host group header row
					lastHost = line.Host
				} else if !hasRemote && i == 0 {
					y++ // "Active" header
				}
				if msg.Y == y {
					m.selectedIndex = i
					return m.selectCurrent()
				}
				y++
			}

			// Recent history area: blank line + "Recent" header
			if len(m.historyEntries) > 0 {
				y += 2 // spacing + "Recent" header
				for i := range m.historyEntries {
					globalIdx := len(m.lines) + i
					if msg.Y == y && globalIdx < total {
						m.selectedIndex = globalIdx
						return m.selectCurrent()
					}
					y++
				}
			}
			_ = activeStartY
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
	subtitleParts := "↑↓ select, digits jump, Enter attach, " + xHint
	if !m.stalenessDisabled {
		subtitleParts += ", S kill-stale"
	}
	subtitleParts += ", q quit"
	subtitle := lipgloss.NewStyle().Foreground(dimColor).Render(subtitleParts)
	numberWidth := len(fmt.Sprintf("%d", max(1, len(m.lines))))

	var sections []string

	// Show kill confirmation if active
	if m.confirmKill {
		sections = append(sections, title, subtitle, "")
		warning := lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true).
			Render(fmt.Sprintf("Kill session '%s'? (Enter/Esc)", m.killSessionName))
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

	// Show kill-stale confirmation if active
	if m.confirmKillStale {
		sections = append(sections, title, subtitle, "")
		header := lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true).
			Render(fmt.Sprintf("Kill %d stale session(s)? (Enter/Esc)", len(m.staleSessionNames)))
		sections = append(sections, header)
		for _, name := range m.staleSessionNames {
			sections = append(sections, lipgloss.NewStyle().Foreground(errorColor).Render("  - "+name))
		}
		return lipgloss.JoinVertical(lipgloss.Left, sections...)
	}

	sections = append(sections, title, subtitle, "")

	// Suggestion banner when many sessions and some are stale
	if !m.stalenessDisabled && len(m.lines) >= m.suggestionThreshold {
		staleCount := m.staleSessionCount()
		if staleCount > 0 {
			banner := lipgloss.NewStyle().Foreground(gettingStaleColor).Render(
				fmt.Sprintf("%d stale session(s) — press S to kill stale", staleCount))
			sections = append(sections, banner, "")
		}
	}

	// Error display
	if m.lastError != nil {
		err := lipgloss.NewStyle().Foreground(errorColor).Render("Error: " + m.lastError.Error())
		sections = append(sections, err)
	}
	if m.historyError != nil {
		err := lipgloss.NewStyle().Foreground(errorColor).Render("History error: " + m.historyError.Error())
		sections = append(sections, err)
	}

	// Active sessions section — iterate m.lines in order (already grouped
	// by host via groupSessionsByHost) and insert a header when the host changes.
	sectionHeader := lipgloss.NewStyle().Bold(true).Foreground(secondaryColor)

	if len(m.lines) > 0 {
		lastHost := "\x00" // sentinel so the first line always triggers a header
		hasRemote := false
		for _, line := range m.lines {
			if line.Host != "" {
				hasRemote = true
				break
			}
		}
		for i, line := range m.lines {
			if hasRemote && line.Host != lastHost {
				hostLabel := "Active (local)"
				if line.Host != "" {
					hostLabel = "Active @ " + line.Host
				}
				sections = append(sections, sectionHeader.Render(hostLabel))
				lastHost = line.Host
			} else if !hasRemote && i == 0 {
				sections = append(sections, sectionHeader.Render("Active"))
			}
			row := m.renderActiveSessionRow(i, line, numberWidth)
			sections = append(sections, row)
		}
	} else if m.pendingExecutors > 0 {
		sections = append(sections, sectionHeader.Render("Active"))
		sections = append(sections, lipgloss.NewStyle().Foreground(dimColor).Render("  Loading..."))
	} else {
		sections = append(sections, sectionHeader.Render("Active"))
		sections = append(sections, lipgloss.NewStyle().Foreground(dimColor).Render("  No active sessions"))
	}

	// Show loading indicator for remote hosts still connecting
	if m.pendingExecutors > 0 && len(m.lines) > 0 {
		sections = append(sections, lipgloss.NewStyle().Foreground(dimColor).Render("  Loading remote hosts..."))
	}

	// Recent history section
	if len(m.historyEntries) > 0 {
		sections = append(sections, "") // spacing
		sections = append(sections, sectionHeader.Render("Recent"))
		for i, entry := range m.historyEntries {
			globalIdx := len(m.lines) + i
			ago := sessionsTimeAgo(entry.LastUsedAt)

			// Color the time-ago text by staleness
			var metaColor lipgloss.Color
			if m.stalenessDisabled {
				metaColor = dimColor
			} else {
				metaColor = stalenessColor(m.historyStalenessTier(entry.LastUsedAt))
			}
			meta := lipgloss.NewStyle().Foreground(metaColor).Render("(" + ago + ")")
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

	result := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return truncateToHeight(result, m.height)
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

// classifyStalenessTier returns the staleness tier for a given age.
func classifyStalenessTier(age time.Duration, freshThreshold, staleThreshold time.Duration) stalenessTier {
	if age <= freshThreshold {
		return tierFresh
	}
	if age <= staleThreshold {
		return tierGettingStale
	}
	return tierStale
}

// sessionStalenessTier classifies a session's staleness based on its activity timestamp.
func (m sessionsModel) sessionStalenessTier(activity int64) stalenessTier {
	if m.stalenessDisabled || activity == 0 {
		return tierFresh
	}
	return classifyStalenessTier(time.Since(time.Unix(activity, 0)), m.freshThreshold, m.staleThreshold)
}

// historyStalenessTier classifies a history entry's staleness based on its last-used time.
func (m sessionsModel) historyStalenessTier(lastUsed time.Time) stalenessTier {
	if m.stalenessDisabled || lastUsed.IsZero() {
		return tierFresh
	}
	return classifyStalenessTier(time.Since(lastUsed), m.freshThreshold, m.staleThreshold)
}

// stalenessColor returns the color for a given staleness tier.
func stalenessColor(tier stalenessTier) lipgloss.Color {
	switch tier {
	case tierGettingStale:
		return gettingStaleColor
	case tierStale:
		return staleColor
	default:
		return freshColor
	}
}

// staleSessions returns the names of active sessions classified as stale.
func (m sessionsModel) staleSessions() []string {
	var names []string
	for _, line := range m.lines {
		if m.sessionStalenessTier(line.Activity) == tierStale {
			names = append(names, line.Name)
		}
	}
	return names
}

// staleSessionCount returns the number of stale active sessions.
func (m sessionsModel) staleSessionCount() int {
	count := 0
	for _, line := range m.lines {
		if m.sessionStalenessTier(line.Activity) == tierStale {
			count++
		}
	}
	return count
}

// truncateToHeight trims rendered output to at most maxHeight lines,
// ensuring the top (most important) content is always visible.
func truncateToHeight(s string, maxHeight int) string {
	if maxHeight <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= maxHeight {
		return s
	}
	return strings.Join(lines[:maxHeight], "\n")
}

type killMultipleSessionsMsg struct {
	killed []string
	err    error
}

func (m sessionsModel) killMultipleSessions(names []string) tea.Cmd {
	return func() tea.Msg {
		for _, name := range names {
			if err := tmux.KillSession(name); err != nil {
				return killMultipleSessionsMsg{killed: names, err: err}
			}
		}
		return killMultipleSessionsMsg{killed: names}
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

func (m sessionsModel) beadsLabel(sessionName string) string {
	if !m.showBeads {
		return ""
	}
	count, ok := m.beadsCounts[sessionName]
	if !ok || count == nil {
		return ""
	}
	label := fmt.Sprintf("bd:%d", *count)
	if *count > 0 {
		return beadsCountStyle.Render(label)
	}
	return lipgloss.NewStyle().Foreground(dimColor).Render(label)
}

func (m sessionsModel) renderActiveSessionRow(index int, line tmux.SessionLine, numberWidth int) string {
	number := fmt.Sprintf("%*d.", numberWidth, index+1)
	memSummary := m.memorySummary(line.Name)
	bdLabel := m.beadsLabel(line.Name)

	// Determine number color based on staleness
	tier := m.sessionStalenessTier(line.Activity)
	var numberColor lipgloss.Color
	if m.stalenessDisabled {
		numberColor = dimColor
	} else {
		numberColor = stalenessColor(tier)
	}

	if index == m.selectedIndex {
		row := selectedStyle.Render("> ") +
			lipgloss.NewStyle().Foreground(numberColor).Bold(true).Render(number) +
			" " +
			formatSessionLine(line.Line, selectedStyle)
		if bdLabel != "" {
			row += "  " + bdLabel
		}
		if memSummary != "" {
			row += "  " + lipgloss.NewStyle().Foreground(dimColor).Render(memSummary)
		}
		return row
	}

	row := "  " +
		lipgloss.NewStyle().Foreground(numberColor).Render(number) +
		" " +
		formatSessionLine(line.Line, lipgloss.NewStyle())
	if bdLabel != "" {
		row += "  " + bdLabel
	}
	if memSummary != "" {
		row += "  " + lipgloss.NewStyle().Foreground(dimColor).Render(memSummary)
	}
	return row
}

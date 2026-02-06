package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/history"
	"github.com/porganisciak/agent-tmux/tmux"
)

// LandingResult contains the outcome of the landing page interaction
type LandingResult struct {
	Action     string // "resume", "attach", "revive", or "" (quit)
	Target     string // Session name for attach
	WorkingDir string // Working directory for revive
	Changed    bool   // Whether settings were changed
}

// LandingOptions configures the landing page behavior
type LandingOptions struct {
	SessionName string // Session name derived from current directory
	AltScreen   bool   // Whether to use alternate screen
}

// RunLanding runs the landing page TUI and returns the user's selection
func RunLanding(opts LandingOptions) (*LandingResult, error) {
	m := newLandingModel(opts.SessionName)
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
	if model, ok := finalModel.(landingModel); ok {
		return &LandingResult{
			Action:     model.action,
			Target:     model.attachSession,
			WorkingDir: model.reviveDir,
			Changed:    model.settingsChanged,
		}, nil
	}
	return &LandingResult{}, nil
}

// Section indices
const (
	sectionResume   = 0
	sectionSessions = 1
	sectionRecent   = 2
	sectionOptions  = 3
)

// Number of recent sessions to show initially (before "Show more")
const recentSessionsCollapsed = 3

// Option indices
const (
	optionResume   = 0
	optionSessions = 1
	optionLanding  = 2
)

// clickZone represents a clickable area
type clickZone struct {
	y1, y2  int // Y range (inclusive start, exclusive end)
	section int // Which section this zone belongs to
	index   int // Index within section (-1 for section header/single item)
}

type landingModel struct {
	sessionName     string             // Session name for current directory
	sessions        []tmux.SessionLine // All existing sessions
	recentSessions  []history.Entry    // Recent sessions from history
	recentExpanded  bool               // Whether recent section is expanded
	selectedIndex   int                // Selection within current section
	focusedSection  int                // 0=resume, 1=sessions, 2=recent, 3=options
	options         [3]bool            // Checkbox states [resume, sessions, landing]
	width           int
	height          int
	attachSession   string // Session to attach on quit
	reviveDir       string // Working directory for revive
	action          string // "resume", "attach", "revive", or ""
	lastError       error
	historyError    error
	settingsChanged bool
	clickZones      []clickZone // Clickable areas calculated during render
	confirmKill     bool        // Whether kill confirmation is active
	killSessionName string      // Session name pending kill confirmation
}

// landingKillMsg is returned after attempting to kill a session.
type landingKillMsg struct {
	name string
	err  error
}

// landingHistoryDeletedMsg is returned after attempting to delete a history entry.
type landingHistoryDeletedMsg struct {
	id  int64
	err error
}

func newLandingModel(sessionName string) landingModel {
	// Load current settings to set checkbox state
	settings, _ := config.LoadSettings()
	var options [3]bool
	switch settings.DefaultAction {
	case "resume":
		options[optionResume] = true
	case "sessions":
		options[optionSessions] = true
	default: // "landing"
		options[optionLanding] = true
	}

	return landingModel{
		sessionName:    sessionName,
		focusedSection: sectionResume,
		options:        options,
	}
}

func (m landingModel) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			lines, err := tmux.ListSessionsRaw()
			return sessionsLoadedMsg{lines: lines, err: err}
		},
		func() tea.Msg {
			store, err := history.Open()
			if err != nil {
				return landingHistoryLoadedMsg{err: err}
			}
			defer store.Close()
			entries, err := store.LoadHistory()
			return landingHistoryLoadedMsg{entries: entries, err: err}
		},
	)
}

// landingHistoryLoadedMsg is used by landing page to receive history
type landingHistoryLoadedMsg struct {
	entries []history.Entry
	err     error
}

func (m landingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle kill confirmation if active
	if m.confirmKill {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "y", "Y":
				m.confirmKill = false
				return m, m.killSelectedSession()
			case "n", "N", "esc":
				m.confirmKill = false
				return m, nil
			}
			return m, nil // Ignore other keys while confirmation is shown
		}
	}

	switch msg := msg.(type) {
	case sessionsLoadedMsg:
		m.sessions = msg.lines
		m.lastError = msg.err
		m.filterRecentSessions()
		m.calculateClickZones()
		return m, nil

	case landingHistoryLoadedMsg:
		m.historyError = msg.err
		if msg.err == nil {
			m.recentSessions = msg.entries
			m.filterRecentSessions()
		}
		m.calculateClickZones()
		return m, nil

	case landingKillMsg:
		if msg.err != nil {
			m.lastError = msg.err
			return m, nil
		}
		// Refresh sessions list after killing
		return m, func() tea.Msg {
			lines, err := tmux.ListSessionsRaw()
			return sessionsLoadedMsg{lines: lines, err: err}
		}

	case landingHistoryDeletedMsg:
		if msg.err != nil {
			m.historyError = msg.err
			return m, nil
		}
		m.recentSessions = removeHistoryEntry(m.recentSessions, msg.id)
		// Adjust selection index if needed
		if m.selectedIndex >= m.visibleRecentCount() {
			m.selectedIndex = m.visibleRecentCount() - 1
			if m.selectedIndex < 0 {
				m.selectedIndex = 0
			}
		}
		m.calculateClickZones()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.calculateClickZones()
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.MouseMsg:
		return m.handleMouseMsg(msg)
	}
	return m, nil
}

// filterRecentSessions removes history entries that have active sessions.
func (m *landingModel) filterRecentSessions() {
	if m.recentSessions == nil {
		return
	}
	activeNames := make(map[string]bool)
	for _, line := range m.sessions {
		activeNames[line.Name] = true
	}
	var filtered []history.Entry
	for _, e := range m.recentSessions {
		if !activeNames[e.SessionName] {
			filtered = append(filtered, e)
		}
	}
	m.recentSessions = filtered
}

// visibleRecentCount returns the number of recent sessions currently visible.
func (m landingModel) visibleRecentCount() int {
	if m.recentExpanded {
		return len(m.recentSessions)
	}
	if len(m.recentSessions) <= recentSessionsCollapsed {
		return len(m.recentSessions)
	}
	return recentSessionsCollapsed
}

// hasRecentFooter returns true if "show more/less" should be shown.
func (m landingModel) hasRecentFooter() bool {
	return len(m.recentSessions) > recentSessionsCollapsed
}

func (m landingModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit

	case "tab":
		// Move to next section (skip recent if empty)
		m.focusedSection = (m.focusedSection + 1) % 4
		if m.focusedSection == sectionRecent && len(m.recentSessions) == 0 {
			m.focusedSection = sectionOptions
		}
		m.selectedIndex = 0
		return m, nil

	case "shift+tab":
		// Move to previous section (skip recent if empty)
		m.focusedSection = (m.focusedSection + 3) % 4
		if m.focusedSection == sectionRecent && len(m.recentSessions) == 0 {
			m.focusedSection = sectionSessions
		}
		m.selectedIndex = 0
		return m, nil

	case "up", "k":
		return m.moveUp()

	case "down", "j":
		return m.moveDown()

	case "enter":
		return m.handleEnter()

	case " ":
		if m.focusedSection == sectionOptions {
			return m.toggleOption(m.selectedIndex)
		}
		return m, nil

	case "x", "delete":
		switch m.focusedSection {
		case sectionSessions:
			if m.selectedIndex >= 0 && m.selectedIndex < len(m.sessions) {
				m.confirmKill = true
				m.killSessionName = m.sessions[m.selectedIndex].Name
			}
			return m, nil
		case sectionRecent:
			// Only delete if a session entry is selected (not the footer)
			if m.selectedIndex >= 0 && m.selectedIndex < m.visibleRecentCount() {
				return m, m.deleteSelectedRecentEntry()
			}
			return m, nil
		}
		return m, nil
	}
	return m, nil
}

// killSelectedSession sends a command to kill the session stored in killSessionName.
func (m landingModel) killSelectedSession() tea.Cmd {
	name := m.killSessionName
	return func() tea.Msg {
		err := tmux.KillSession(name)
		return landingKillMsg{name: name, err: err}
	}
}

// deleteSelectedRecentEntry deletes the currently selected recent history entry.
func (m landingModel) deleteSelectedRecentEntry() tea.Cmd {
	if m.selectedIndex < 0 || m.selectedIndex >= len(m.recentSessions) {
		return nil
	}
	entry := m.recentSessions[m.selectedIndex]
	return func() tea.Msg {
		store, err := history.Open()
		if err != nil {
			return landingHistoryDeletedMsg{id: entry.ID, err: err}
		}
		defer store.Close()
		return landingHistoryDeletedMsg{id: entry.ID, err: store.DeleteEntry(entry.ID)}
	}
}

func (m landingModel) moveUp() (tea.Model, tea.Cmd) {
	switch m.focusedSection {
	case sectionResume:
		// Already at top, wrap to options section bottom
		m.focusedSection = sectionOptions
		m.selectedIndex = 2
	case sectionSessions:
		if m.selectedIndex > 0 {
			m.selectedIndex--
		} else {
			// Move to resume section
			m.focusedSection = sectionResume
			m.selectedIndex = 0
		}
	case sectionRecent:
		if m.selectedIndex > 0 {
			m.selectedIndex--
		} else {
			// Move to sessions section (bottom)
			m.focusedSection = sectionSessions
			if len(m.sessions) > 0 {
				m.selectedIndex = len(m.sessions) - 1
			} else {
				// No sessions, go to resume
				m.focusedSection = sectionResume
				m.selectedIndex = 0
			}
		}
	case sectionOptions:
		if m.selectedIndex > 0 {
			m.selectedIndex--
		} else {
			// Move to recent section (bottom) or sessions if no recent
			if len(m.recentSessions) > 0 {
				m.focusedSection = sectionRecent
				// Position at the last selectable item (either footer or last visible session)
				m.selectedIndex = m.visibleRecentCount()
				if m.hasRecentFooter() {
					// Footer is selectable
				} else {
					m.selectedIndex = m.visibleRecentCount() - 1
				}
			} else if len(m.sessions) > 0 {
				m.focusedSection = sectionSessions
				m.selectedIndex = len(m.sessions) - 1
			} else {
				m.focusedSection = sectionResume
				m.selectedIndex = 0
			}
		}
	}
	return m, nil
}

func (m landingModel) moveDown() (tea.Model, tea.Cmd) {
	switch m.focusedSection {
	case sectionResume:
		// Move to sessions section
		m.focusedSection = sectionSessions
		m.selectedIndex = 0
		// If no sessions, skip to recent or options
		if len(m.sessions) == 0 {
			if len(m.recentSessions) > 0 {
				m.focusedSection = sectionRecent
			} else {
				m.focusedSection = sectionOptions
			}
		}
	case sectionSessions:
		if m.selectedIndex < len(m.sessions)-1 {
			m.selectedIndex++
		} else {
			// Move to recent section or options
			if len(m.recentSessions) > 0 {
				m.focusedSection = sectionRecent
				m.selectedIndex = 0
			} else {
				m.focusedSection = sectionOptions
				m.selectedIndex = 0
			}
		}
	case sectionRecent:
		// Total selectable items: visible sessions + footer (if any)
		maxIdx := m.visibleRecentCount() - 1
		if m.hasRecentFooter() {
			maxIdx = m.visibleRecentCount() // footer is at index visibleRecentCount
		}
		if m.selectedIndex < maxIdx {
			m.selectedIndex++
		} else {
			// Move to options section
			m.focusedSection = sectionOptions
			m.selectedIndex = 0
		}
	case sectionOptions:
		if m.selectedIndex < 2 {
			m.selectedIndex++
		} else {
			// Wrap to resume section
			m.focusedSection = sectionResume
			m.selectedIndex = 0
		}
	}
	return m, nil
}

func (m landingModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.focusedSection {
	case sectionResume:
		m.action = "resume"
		m.attachSession = m.sessionName
		return m, tea.Quit

	case sectionSessions:
		if m.selectedIndex >= 0 && m.selectedIndex < len(m.sessions) {
			m.action = "attach"
			m.attachSession = m.sessions[m.selectedIndex].Name
			return m, tea.Quit
		}

	case sectionRecent:
		// Check if footer is selected
		if m.hasRecentFooter() && m.selectedIndex >= m.visibleRecentCount() {
			// Toggle expanded state
			m.recentExpanded = !m.recentExpanded
			// Clamp selection if needed
			maxIdx := m.visibleRecentCount()
			if m.hasRecentFooter() {
				maxIdx++ // include footer
			}
			if m.selectedIndex >= maxIdx {
				m.selectedIndex = maxIdx - 1
			}
			m.calculateClickZones()
			return m, nil
		}
		// Select a recent session to revive
		if m.selectedIndex >= 0 && m.selectedIndex < m.visibleRecentCount() {
			entry := m.recentSessions[m.selectedIndex]
			m.action = "revive"
			m.attachSession = entry.SessionName
			m.reviveDir = entry.WorkingDirectory
			return m, tea.Quit
		}

	case sectionOptions:
		return m.toggleOption(m.selectedIndex)
	}
	return m, nil
}

func (m landingModel) toggleOption(index int) (tea.Model, tea.Cmd) {
	// Options are mutually exclusive
	for i := range m.options {
		m.options[i] = false
	}
	m.options[index] = true
	m.settingsChanged = true

	// Save settings
	settings := &config.Settings{}
	switch index {
	case optionResume:
		settings.DefaultAction = "resume"
	case optionSessions:
		settings.DefaultAction = "sessions"
	case optionLanding:
		settings.DefaultAction = "landing"
	}
	settings.Save()

	return m, nil
}

func (m landingModel) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	y := msg.Y

	// Check each click zone
	for _, zone := range m.clickZones {
		if y >= zone.y1 && y < zone.y2 {
			m.focusedSection = zone.section

			switch zone.section {
			case sectionResume:
				m.selectedIndex = 0
				m.action = "resume"
				m.attachSession = m.sessionName
				return m, tea.Quit

			case sectionSessions:
				if zone.index >= 0 && zone.index < len(m.sessions) {
					m.selectedIndex = zone.index
					m.action = "attach"
					m.attachSession = m.sessions[zone.index].Name
					return m, tea.Quit
				}

			case sectionRecent:
				// Check if footer is clicked
				if zone.index == -2 { // special index for footer
					m.recentExpanded = !m.recentExpanded
					m.selectedIndex = m.visibleRecentCount() // select footer
					m.calculateClickZones()
					return m, nil
				}
				if zone.index >= 0 && zone.index < len(m.recentSessions) {
					m.selectedIndex = zone.index
					entry := m.recentSessions[zone.index]
					m.action = "revive"
					m.attachSession = entry.SessionName
					m.reviveDir = entry.WorkingDirectory
					return m, tea.Quit
				}

			case sectionOptions:
				if zone.index >= 0 && zone.index < 3 {
					m.selectedIndex = zone.index
					return m.toggleOption(zone.index)
				}
			}
		}
	}

	return m, nil
}

func (m landingModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	var sections []string

	// Title
	title := m.renderTitle()
	sections = append(sections, title)

	// Resume section
	resumeSection := m.renderResumeSection()
	sections = append(sections, resumeSection)

	// Sessions section
	sessionsSection := m.renderSessionsSection()
	sections = append(sections, sessionsSection)

	// Recent sessions section (only if there are entries)
	if len(m.recentSessions) > 0 {
		recentSection := m.renderRecentSection()
		sections = append(sections, recentSection)
	}

	// Options section
	optionsSection := m.renderOptionsSection()
	sections = append(sections, optionsSection)

	// Status bar (or kill confirmation)
	if m.confirmKill {
		confirmStyle := lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true).
			Width(m.width).
			Align(lipgloss.Center).
			Padding(1, 0)
		sections = append(sections, confirmStyle.Render(
			fmt.Sprintf("Kill session '%s'? (y/n)", m.killSessionName)))
	} else {
		statusBar := m.renderStatusBar()
		sections = append(sections, statusBar)
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// calculateClickZones updates the click zones based on current layout
func (m *landingModel) calculateClickZones() {
	m.clickZones = nil
	currentY := 0

	// Title: "atmux" with padding (1 line top, 1 line content, 1 line bottom)
	currentY += 3

	// Resume section: border(1) + content(1) + border(1) = 3 lines
	m.clickZones = append(m.clickZones, clickZone{
		y1:      currentY,
		y2:      currentY + 3,
		section: sectionResume,
		index:   -1,
	})
	currentY += 3

	// Sessions section: border(1) + header(1) + divider(1) + items + border(1)
	sessionItems := len(m.sessions)
	if sessionItems == 0 {
		sessionItems = 1 // "No active sessions" line
	}
	sessionListStart := currentY + 3 // border + header + divider
	for i := 0; i < len(m.sessions); i++ {
		m.clickZones = append(m.clickZones, clickZone{
			y1:      sessionListStart + i,
			y2:      sessionListStart + i + 1,
			section: sectionSessions,
			index:   i,
		})
	}
	currentY += 3 + sessionItems + 1 // header area + items + bottom border

	// Recent sessions section (if entries exist): border(1) + header(1) + divider(1) + items + footer? + border(1)
	if len(m.recentSessions) > 0 {
		visibleRecent := m.visibleRecentCount()
		recentListStart := currentY + 3 // border + header + divider
		for i := 0; i < visibleRecent; i++ {
			m.clickZones = append(m.clickZones, clickZone{
				y1:      recentListStart + i,
				y2:      recentListStart + i + 1,
				section: sectionRecent,
				index:   i,
			})
		}
		// Footer click zone (show more/less)
		if m.hasRecentFooter() {
			footerY := recentListStart + visibleRecent
			m.clickZones = append(m.clickZones, clickZone{
				y1:      footerY,
				y2:      footerY + 1,
				section: sectionRecent,
				index:   -2, // special index for footer
			})
			currentY += 3 + visibleRecent + 1 + 1 // header area + items + footer + bottom border
		} else {
			currentY += 3 + visibleRecent + 1 // header area + items + bottom border
		}
	}

	// Options section: border(1) + header(1) + divider(1) + description(1) + 3 options + border(1)
	optionListStart := currentY + 4 // border + header + divider + description
	for i := 0; i < 3; i++ {
		m.clickZones = append(m.clickZones, clickZone{
			y1:      optionListStart + i,
			y2:      optionListStart + i + 1,
			section: sectionOptions,
			index:   i,
		})
	}
}

func (m landingModel) renderTitle() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(primaryColor).
		Width(m.width).
		Align(lipgloss.Center).
		Padding(1, 0)

	return titleStyle.Render("atmux")
}

func (m landingModel) renderResumeSection() string {
	var content string

	prefix := "  "
	if m.focusedSection == sectionResume {
		prefix = "> "
	}

	sessionExists := m.sessionExists()
	var actionLabel string
	if sessionExists {
		actionLabel = "Resume session: "
	} else {
		actionLabel = "Start session here: "
	}

	// Build style for the label part
	labelStyle := lipgloss.NewStyle().Foreground(primaryColor)
	nameStyle := lipgloss.NewStyle().Foreground(primaryColor)
	if m.focusedSection == sectionResume {
		labelStyle = labelStyle.Bold(true).Inherit(selectedStyle)
		nameStyle = nameStyle.Bold(true).Inherit(selectedStyle)
	}

	// Format session name with dimmed prefix, styled rest
	formattedName := formatSessionName(m.sessionName, nameStyle)
	content = labelStyle.Render(prefix+actionLabel) + formattedName

	boxStyle := borderStyle.Width(m.width-4).Padding(0, 1)
	if m.focusedSection == sectionResume {
		boxStyle = activeBorderStyle.Width(m.width-4).Padding(0, 1)
	}

	return boxStyle.Render(content)
}

func (m landingModel) renderSessionsSection() string {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(secondaryColor)

	header := headerStyle.Render("Attach Active Session")
	divider := lipgloss.NewStyle().Foreground(dimColor).Render(strings.Repeat("─", 12))

	var rows []string
	rows = append(rows, header)
	rows = append(rows, divider)

	if m.lastError != nil {
		errStyle := lipgloss.NewStyle().Foreground(errorColor)
		rows = append(rows, errStyle.Render("  Error: "+m.lastError.Error()))
	} else if len(m.sessions) == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(dimColor)
		rows = append(rows, emptyStyle.Render("  No active sessions"))
	} else {
		for i, session := range m.sessions {
			prefix := "    "
			prefixStyle := lipgloss.NewStyle()
			lineStyle := lipgloss.NewStyle()

			if m.focusedSection == sectionSessions && i == m.selectedIndex {
				prefix = "  > "
				prefixStyle = prefixStyle.Bold(true).Inherit(selectedStyle)
				lineStyle = lineStyle.Bold(true).Inherit(selectedStyle)
			}

			// Format session info with dimmed agent-/atmux- prefix
			if strings.Contains(session.Line, "(attached)") {
				lineStyle = lineStyle.Foreground(activeColor)
			}

			formattedLine := formatSessionLine(session.Line, lineStyle)
			rows = append(rows, prefixStyle.Render(prefix)+formattedLine)
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	boxStyle := borderStyle.Width(m.width-4).Padding(0, 1)
	if m.focusedSection == sectionSessions {
		boxStyle = activeBorderStyle.Width(m.width-4).Padding(0, 1)
	}

	return boxStyle.Render(content)
}

func (m landingModel) renderRecentSection() string {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(secondaryColor)

	header := headerStyle.Render("Recent Sessions")
	divider := lipgloss.NewStyle().Foreground(dimColor).Render(strings.Repeat("─", 12))

	var rows []string
	rows = append(rows, header)
	rows = append(rows, divider)

	if m.historyError != nil {
		errStyle := lipgloss.NewStyle().Foreground(errorColor)
		rows = append(rows, errStyle.Render("  Error: "+m.historyError.Error()))
	} else {
		visibleCount := m.visibleRecentCount()
		for i := 0; i < visibleCount && i < len(m.recentSessions); i++ {
			entry := m.recentSessions[i]
			prefix := "    "
			prefixStyle := lipgloss.NewStyle()
			nameStyle := lipgloss.NewStyle()

			if m.focusedSection == sectionRecent && i == m.selectedIndex {
				prefix = "  > "
				prefixStyle = prefixStyle.Bold(true).Inherit(selectedStyle)
				nameStyle = nameStyle.Bold(true).Inherit(selectedStyle)
			}

			// Format: session name (time ago) directory
			formattedName := formatSessionName(entry.Name, nameStyle)
			ago := landingTimeAgo(entry.LastUsedAt)
			meta := lipgloss.NewStyle().Foreground(dimColor).Render(" (" + ago + ")")
			dir := lipgloss.NewStyle().Foreground(dimColor).Render("  " + entry.WorkingDirectory)

			rows = append(rows, prefixStyle.Render(prefix)+formattedName+meta+dir)
		}

		// Show more/less footer
		if m.hasRecentFooter() {
			footerSelected := m.focusedSection == sectionRecent && m.selectedIndex >= visibleCount
			var footerText string
			var icon string

			if m.recentExpanded {
				icon = "\u25b2" // Up arrow
				footerText = "Show less"
			} else {
				icon = "\u25bc" // Down arrow
				hidden := len(m.recentSessions) - visibleCount
				footerText = fmt.Sprintf("Show more (%d)", hidden)
			}

			iconStyled := lipgloss.NewStyle().Foreground(primaryColor).Render(icon)
			style := lipgloss.NewStyle().Foreground(dimColor)
			if footerSelected {
				style = style.Bold(true).Background(lipgloss.Color("236"))
			}

			rows = append(rows, "  "+iconStyled+" "+style.Render(footerText))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	boxStyle := borderStyle.Width(m.width-4).Padding(0, 1)
	if m.focusedSection == sectionRecent {
		boxStyle = activeBorderStyle.Width(m.width-4).Padding(0, 1)
	}

	return boxStyle.Render(content)
}

// landingTimeAgo formats a time as a relative string (e.g., "5m ago", "2h ago")
func landingTimeAgo(t time.Time) string {
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

func (m landingModel) renderOptionsSection() string {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(secondaryColor)

	header := headerStyle.Render("Defaults")
	divider := lipgloss.NewStyle().Foreground(dimColor).Render(strings.Repeat("─", 8))

	descStyle := lipgloss.NewStyle().Foreground(dimColor)
	description := descStyle.Render("When starting with `atmux`, I always want to:")

	optionLabels := []string{
		"Start/Resume session directly (skip this page)",
		"List sessions (`atmux sessions`)",
		"Show this landing page",
	}

	var rows []string
	rows = append(rows, header)
	rows = append(rows, divider)
	rows = append(rows, description)

	for i, label := range optionLabels {
		checkbox := "[ ]"
		if m.options[i] {
			checkbox = "[x]"
		}

		prefix := "  "
		style := lipgloss.NewStyle()

		if m.focusedSection == sectionOptions && i == m.selectedIndex {
			prefix = "> "
			style = style.Bold(true).Inherit(selectedStyle)
		}

		rows = append(rows, style.Render(prefix+checkbox+" "+label))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	boxStyle := borderStyle.Width(m.width-4).Padding(0, 1)
	if m.focusedSection == sectionOptions {
		boxStyle = activeBorderStyle.Width(m.width-4).Padding(0, 1)
	}

	return boxStyle.Render(content)
}

func (m landingModel) renderStatusBar() string {
	hints := []string{
		"↑↓ navigate",
		"Tab section",
		"Enter select",
		"Space toggle",
		"q quit",
	}

	// Add context-specific hints
	switch m.focusedSection {
	case sectionSessions:
		if len(m.sessions) > 0 {
			hints = append(hints, "x kill")
		}
	case sectionRecent:
		if len(m.recentSessions) > 0 {
			hints = append(hints, "x remove")
		}
	}

	hintStyle := lipgloss.NewStyle().Foreground(dimColor)
	separator := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(" │ ")

	var styledHints []string
	for _, hint := range hints {
		styledHints = append(styledHints, hintStyle.Render(hint))
	}

	hintsLine := strings.Join(styledHints, separator)

	tip := RenderTipForContext(TipLanding)

	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Padding(1, 0).
		Render(lipgloss.JoinVertical(lipgloss.Center, hintsLine, tip))
}

func (m landingModel) sessionExists() bool {
	for _, s := range m.sessions {
		if s.Name == m.sessionName {
			return true
		}
	}
	return false
}

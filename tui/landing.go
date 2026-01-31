package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/tmux"
)

// LandingResult contains the outcome of the landing page interaction
type LandingResult struct {
	Action  string // "resume", "attach", or "" (quit)
	Target  string // Session name for attach
	Changed bool   // Whether settings were changed
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
			Action:  model.action,
			Target:  model.attachSession,
			Changed: model.settingsChanged,
		}, nil
	}
	return &LandingResult{}, nil
}

// Section indices
const (
	sectionResume   = 0
	sectionSessions = 1
	sectionOptions  = 2
)

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
	selectedIndex   int                // Selection within current section
	focusedSection  int                // 0=resume, 1=sessions, 2=options
	options         [3]bool            // Checkbox states [resume, sessions, landing]
	width           int
	height          int
	attachSession   string // Session to attach on quit
	action          string // "resume", "attach", or ""
	lastError       error
	settingsChanged bool
	clickZones      []clickZone // Clickable areas calculated during render
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
	return func() tea.Msg {
		lines, err := tmux.ListSessionsRaw()
		return sessionsLoadedMsg{lines: lines, err: err}
	}
}

func (m landingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionsLoadedMsg:
		m.sessions = msg.lines
		m.lastError = msg.err
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

func (m landingModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit

	case "tab":
		// Move to next section
		m.focusedSection = (m.focusedSection + 1) % 3
		m.selectedIndex = 0
		return m, nil

	case "shift+tab":
		// Move to previous section
		m.focusedSection = (m.focusedSection + 2) % 3
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
	}
	return m, nil
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
	case sectionOptions:
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
	}
	return m, nil
}

func (m landingModel) moveDown() (tea.Model, tea.Cmd) {
	switch m.focusedSection {
	case sectionResume:
		// Move to sessions section
		m.focusedSection = sectionSessions
		m.selectedIndex = 0
		// If no sessions, skip to options
		if len(m.sessions) == 0 {
			m.focusedSection = sectionOptions
		}
	case sectionSessions:
		if m.selectedIndex < len(m.sessions)-1 {
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

	// Options section
	optionsSection := m.renderOptionsSection()
	sections = append(sections, optionsSection)

	// Status bar
	statusBar := m.renderStatusBar()
	sections = append(sections, statusBar)

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
	var actionText string
	if sessionExists {
		actionText = fmt.Sprintf("Resume session: %s", m.sessionName)
	} else {
		actionText = fmt.Sprintf("Start new session: %s", m.sessionName)
	}

	style := lipgloss.NewStyle().Foreground(primaryColor)
	if m.focusedSection == sectionResume {
		style = style.Bold(true).Inherit(selectedStyle)
	}

	content = style.Render(prefix + actionText)

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

	header := headerStyle.Render("Load Session")
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
			style := lipgloss.NewStyle()

			if m.focusedSection == sectionSessions && i == m.selectedIndex {
				prefix = "  > "
				style = style.Bold(true).Inherit(selectedStyle)
			}

			// Format session info
			info := session.Line
			if strings.Contains(info, "(attached)") {
				style = style.Foreground(activeColor)
			}

			rows = append(rows, style.Render(prefix+info))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	boxStyle := borderStyle.Width(m.width-4).Padding(0, 1)
	if m.focusedSection == sectionSessions {
		boxStyle = activeBorderStyle.Width(m.width-4).Padding(0, 1)
	}

	return boxStyle.Render(content)
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

	hintStyle := lipgloss.NewStyle().Foreground(dimColor)
	separator := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(" │ ")

	var styledHints []string
	for _, hint := range hints {
		styledHints = append(styledHints, hintStyle.Render(hint))
	}

	hintsLine := strings.Join(styledHints, separator)

	onboardHint := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("New to atmux? Run `atmux onboard` for a quick guide.")

	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Padding(1, 0).
		Render(lipgloss.JoinVertical(lipgloss.Center, hintsLine, onboardHint))
}

func (m landingModel) sessionExists() bool {
	for _, s := range m.sessions {
		if s.Name == m.sessionName {
			return true
		}
	}
	return false
}

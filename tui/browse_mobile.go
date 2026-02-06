package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/tmux"
)

// Mobile layout styles
var (
	mobileHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(primaryColor).
				Padding(0, 1)

	mobileSessionStyle = lipgloss.NewStyle().
				Padding(0, 1)

	mobileSessionSelectedStyle = lipgloss.NewStyle().
					Background(lipgloss.Color("236")).
					Foreground(lipgloss.Color("255")).
					Bold(true).
					Padding(0, 1)

	mobileSessionAttachedStyle = lipgloss.NewStyle().
					Foreground(activeColor).
					Padding(0, 1)

	mobileSectionStyle = lipgloss.NewStyle().
				Foreground(dimColor).
				Padding(0, 1)

	mobileButtonStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(buttonColor).
				Bold(true).
				Padding(0, 2).
				Margin(0, 1)

	mobileButtonSelectedStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("255")).
					Background(activeColor).
					Bold(true).
					Padding(0, 2).
					Margin(0, 1)

	mobileButtonDangerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(errorColor).
				Bold(true).
				Padding(0, 2).
				Margin(0, 1)

	mobileHintStyle = lipgloss.NewStyle().
			Foreground(dimColor).
			Align(lipgloss.Center)

	mobileTimeStyle = lipgloss.NewStyle().
			Foreground(dimColor)

	mobileActiveIndicator = lipgloss.NewStyle().
				Foreground(activeColor).
				Render("*")
)

// MobileButton represents a button in the mobile button bar
type MobileButton int

const (
	MobileButtonAttach MobileButton = iota
	MobileButtonKill
	MobileButtonNew
	MobileButtonCount
)

// shouldUseMobileLayout determines if mobile layout should be used
func shouldUseMobileLayout(width int, forceMobile bool) bool {
	if forceMobile {
		return true
	}
	if os.Getenv("ATMUX_MOBILE") == "1" {
		return true
	}
	return width > 0 && width < mobileWidthThreshold
}

// renderMobileView renders the mobile-optimized view
func (m Model) renderMobileView() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	var sections []string

	// Header
	header := m.renderMobileHeader()
	sections = append(sections, header)

	// Sessions list (single column, no tree drill-down)
	sessionsList := m.renderMobileSessionsList()
	sections = append(sections, sessionsList)

	// Button bar at bottom (large touch-friendly buttons)
	buttonBar := m.renderMobileButtonBar()
	sections = append(sections, buttonBar)

	// Hints
	hints := m.renderMobileHints()
	sections = append(sections, hints)

	base := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Show kill confirmation overlay if active
	if m.confirmKill {
		return m.renderMobileKillConfirm(base)
	}

	// Show help overlay if active
	if m.showHelp {
		return m.renderMobileHelp(base)
	}

	return base
}

// renderMobileHeader renders the mobile header bar
func (m Model) renderMobileHeader() string {
	title := mobileHeaderStyle.Render("atmux")

	// Help button on right
	helpBtn := helpButtonStyle.Render("?")

	// Build header with title on left, help on right
	titleWidth := lipgloss.Width(title)
	helpWidth := lipgloss.Width(helpBtn)
	padding := m.width - titleWidth - helpWidth - 2
	if padding < 0 {
		padding = 0
	}

	header := title + strings.Repeat(" ", padding) + helpBtn
	return header
}

// renderMobileSessionsList renders the sessions list for mobile
func (m Model) renderMobileSessionsList() string {
	var lines []string

	// Calculate available height for sessions
	// Total height minus header (1) minus button bar (3) minus hints (2) minus borders (2)
	availableHeight := m.height - 1 - mobileButtonHeight - 2 - 2
	if availableHeight < 3 {
		availableHeight = 3
	}

	if m.tree == nil || len(m.tree.Sessions) == 0 {
		emptyMsg := lipgloss.NewStyle().
			Foreground(dimColor).
			Italic(true).
			Padding(1, 1).
			Render("No sessions")
		lines = append(lines, emptyMsg)
	} else {
		// Section header
		sectionHeader := mobileSectionStyle.Render(fmt.Sprintf("Sessions (%d)", len(m.tree.Sessions)))
		lines = append(lines, sectionHeader)
		lines = append(lines, "")

		// List sessions - show only session-level items (no window/pane drill-down)
		sessionIdx := 0
		for i, sess := range m.tree.Sessions {
			if len(lines) >= availableHeight-1 {
				// Show "more..." indicator
				remaining := len(m.tree.Sessions) - i
				moreMsg := lipgloss.NewStyle().
					Foreground(dimColor).
					Padding(0, 1).
					Render(fmt.Sprintf("  ... +%d more", remaining))
				lines = append(lines, moreMsg)
				break
			}

			selected := sessionIdx == m.selectedIndex
			line := m.renderMobileSessionLine(sess, selected)
			lines = append(lines, line)
			sessionIdx++
		}
	}

	// Pad to fill available height
	for len(lines) < availableHeight {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")
	return borderStyle.Width(m.width - 2).Height(availableHeight).Render(content)
}

// renderMobileSessionLine renders a single session line for mobile view
func (m Model) renderMobileSessionLine(sess tmux.TmuxSession, selected bool) string {
	// Format: "> sessionname      2w  *"
	// Where 2w = 2 windows, * = attached indicator

	name := sess.Name
	maxNameLen := m.width - 15 // Leave room for windows count and indicators
	if maxNameLen < 10 {
		maxNameLen = 10
	}
	if len(name) > maxNameLen {
		name = name[:maxNameLen-3] + "..."
	}

	// Window count
	windowCount := fmt.Sprintf("%dw", len(sess.Windows))

	// Attached indicator
	attachedIndicator := "  "
	if sess.Attached {
		attachedIndicator = mobileActiveIndicator + " "
	}

	// Calculate padding
	lineContent := name
	rightPart := windowCount + " " + attachedIndicator
	padding := m.width - 6 - len(lineContent) - len(rightPart)
	if padding < 1 {
		padding = 1
	}

	fullLine := lineContent + strings.Repeat(" ", padding) + rightPart

	// Apply style based on selection state
	var style lipgloss.Style
	if selected {
		style = mobileSessionSelectedStyle
		fullLine = "> " + fullLine
	} else if sess.Attached {
		style = mobileSessionAttachedStyle
		fullLine = "  " + fullLine
	} else {
		style = mobileSessionStyle
		fullLine = "  " + fullLine
	}

	// Format session name with dimmed prefix
	return style.Width(m.width - 4).Render(fullLine)
}

// renderMobileButtonBar renders the large touch-friendly button bar
func (m Model) renderMobileButtonBar() string {
	// Three main buttons: [Attach] [Kill] [New]
	// Each should be large and touch-friendly

	attachBtn := mobileButtonSelectedStyle.Render("Attach")
	killBtn := mobileButtonDangerStyle.Render("Kill")
	newBtn := mobileButtonStyle.Render("New")

	// Center the buttons
	buttons := lipgloss.JoinHorizontal(lipgloss.Center, attachBtn, killBtn, newBtn)
	centered := lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(buttons)

	return centered
}

// renderMobileHints renders the keyboard/touch hints
func (m Model) renderMobileHints() string {
	hints := mobileHintStyle.Width(m.width).Render("j/k navigate  Enter attach  ? help  q quit")
	return hints
}

// renderMobileHelp renders the mobile help overlay
func (m Model) renderMobileHelp(base string) string {
	title := helpTitleStyle.Render("Help")

	helpLines := []string{
		"",
		helpKeyStyle.Render("j/k or Up/Down") + "  Navigate",
		helpKeyStyle.Render("Enter") + "          Attach to session",
		helpKeyStyle.Render("x or d") + "         Kill session",
		helpKeyStyle.Render("n") + "              New session",
		helpKeyStyle.Render("r") + "              Refresh list",
		helpKeyStyle.Render("?") + "              Toggle help",
		helpKeyStyle.Render("q or Esc") + "       Quit",
		"",
		helpDescStyle.Render("Press any key to close"),
	}

	helpContent := lipgloss.JoinVertical(lipgloss.Left,
		append([]string{title}, helpLines...)...,
	)

	helpBox := helpOverlayStyle.Render(helpContent)
	helpWidth := lipgloss.Width(helpBox)
	helpHeight := lipgloss.Height(helpBox)

	x := (m.width - helpWidth) / 2
	y := (m.height - helpHeight) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	return placeOverlay(x, y, helpBox, base)
}

// renderMobileKillConfirm renders the kill confirmation overlay for mobile
func (m Model) renderMobileKillConfirm(base string) string {
	title := helpTitleStyle.Render("Kill Session?")

	nameDisplay := m.killNodeName
	if nameDisplay == "" {
		nameDisplay = m.killNodeTarget
	}

	message := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Bold(true).
		Render(fmt.Sprintf("'%s'", nameDisplay))

	hint := lipgloss.NewStyle().
		Foreground(dimColor).
		Render("[y] confirm  [n] cancel")

	confirmContent := lipgloss.JoinVertical(lipgloss.Center,
		title,
		"",
		message,
		"",
		hint,
	)

	confirmBox := helpOverlayStyle.
		Width(m.width - 8).
		Render(confirmContent)

	confirmWidth := lipgloss.Width(confirmBox)
	confirmHeight := lipgloss.Height(confirmBox)

	x := (m.width - confirmWidth) / 2
	y := (m.height - confirmHeight) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	return placeOverlay(x, y, confirmBox, base)
}

// handleMobileKeyMsg handles keyboard input in mobile mode
func (m Model) handleMobileKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle kill confirmation if active
	if m.confirmKill {
		switch msg.String() {
		case "y", "Y":
			m.confirmKill = false
			return m, killTarget(m.killNodeType, m.killNodeTarget)
		case "n", "N", "esc":
			m.confirmKill = false
			return m, nil
		}
		return m, nil
	}

	// Close help overlay if open
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	switch msg.String() {
	case "?":
		m.showHelp = true
		return m, nil
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.moveMobileSelection(-1)
		return m, nil
	case "down", "j":
		m.moveMobileSelection(1)
		return m, nil
	case "enter", " ":
		// Attach to selected session
		if sess := m.selectedMobileSession(); sess != nil {
			m.attachSession = sess.Name
			return m, tea.Quit
		}
		return m, nil
	case "x", "d":
		// Kill selected session
		if sess := m.selectedMobileSession(); sess != nil {
			m.confirmKill = true
			m.killNodeType = "session"
			m.killNodeTarget = sess.Name
			m.killNodeName = sess.Name
			return m, nil
		}
		return m, nil
	case "r":
		return m, fetchTree
	case "n":
		// New session - for now just refresh (could add new session wizard later)
		return m, fetchTree
	}

	return m, nil
}

// handleMobileMouseMsg handles mouse input in mobile mode
func (m Model) handleMobileMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Close help on any click
	if m.showHelp && msg.Action == tea.MouseActionPress {
		m.showHelp = false
		return m, nil
	}

	// Close kill confirm on click outside
	if m.confirmKill && msg.Action == tea.MouseActionPress {
		m.confirmKill = false
		return m, nil
	}

	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		// Check if clicking in session list area
		// Header is 1 line, session list starts at line 2
		sessionListStart := 4 // header + section header + empty line + border
		sessionListEnd := m.height - mobileButtonHeight - 2 - 2

		if msg.Y >= sessionListStart && msg.Y < sessionListEnd {
			clickedIdx := msg.Y - sessionListStart
			if m.tree != nil && clickedIdx >= 0 && clickedIdx < len(m.tree.Sessions) {
				// Check for double-click
				if clickedIdx == m.selectedIndex &&
					time.Since(m.lastClickAt) <= doubleClickThreshold {
					// Double-click: attach
					if sess := m.selectedMobileSession(); sess != nil {
						m.attachSession = sess.Name
						return m, tea.Quit
					}
				}
				m.selectedIndex = clickedIdx
				m.lastClickIdx = clickedIdx
				m.lastClickAt = time.Now()
				return m, nil
			}
		}

		// Check if clicking button bar
		buttonBarY := m.height - mobileButtonHeight - 2
		if msg.Y >= buttonBarY && msg.Y < buttonBarY+mobileButtonHeight {
			// Determine which button was clicked based on X position
			// Buttons are roughly evenly distributed
			buttonWidth := m.width / 3
			buttonIdx := msg.X / buttonWidth

			switch MobileButton(buttonIdx) {
			case MobileButtonAttach:
				if sess := m.selectedMobileSession(); sess != nil {
					m.attachSession = sess.Name
					return m, tea.Quit
				}
			case MobileButtonKill:
				if sess := m.selectedMobileSession(); sess != nil {
					m.confirmKill = true
					m.killNodeType = "session"
					m.killNodeTarget = sess.Name
					m.killNodeName = sess.Name
				}
			case MobileButtonNew:
				// Refresh for now
				return m, fetchTree
			}
			return m, nil
		}

		// Check if clicking help button (top right)
		if msg.Y == 0 && msg.X >= m.width-5 {
			m.showHelp = !m.showHelp
			return m, nil
		}
	}

	return m, nil
}

// moveMobileSelection moves the selection in mobile mode (sessions only)
func (m *Model) moveMobileSelection(delta int) {
	if m.tree == nil || len(m.tree.Sessions) == 0 {
		return
	}

	m.selectedIndex += delta
	if m.selectedIndex < 0 {
		m.selectedIndex = 0
	}
	if m.selectedIndex >= len(m.tree.Sessions) {
		m.selectedIndex = len(m.tree.Sessions) - 1
	}
}

// selectedMobileSession returns the currently selected session in mobile mode
func (m *Model) selectedMobileSession() *tmux.TmuxSession {
	if m.tree == nil || m.selectedIndex < 0 || m.selectedIndex >= len(m.tree.Sessions) {
		return nil
	}
	return &m.tree.Sessions[m.selectedIndex]
}

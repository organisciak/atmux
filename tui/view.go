package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View renders the TUI
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Build the layout
	inputBar := m.renderInputBar()
	mainContent := m.renderMainContent()
	statusBar := m.renderStatusBar()

	base := lipgloss.JoinVertical(lipgloss.Left,
		inputBar,
		mainContent,
		statusBar,
	)

	// Show help overlay if active
	if m.showHelp {
		return m.renderHelpOverlay(base)
	}

	return base
}

// renderInputBar renders the command input area
func (m *Model) renderInputBar() string {
	style := inputStyle
	if m.focused == FocusInput {
		style = inputFocusedStyle
	}

	label := lipgloss.NewStyle().Bold(true).Render("Command: ")
	input := m.commandInput.View()

	// Help button
	helpBtn := helpButtonStyle.Render("?")

	// Track help button zone (top-right area)
	helpBtnWidth := lipgloss.Width(helpBtn)
	m.buttonZones = append(m.buttonZones, buttonZone{
		x:      m.width - helpBtnWidth - 4,
		y:      1,
		width:  helpBtnWidth,
		height: 1,
		action: buttonActionHelp,
	})

	// Refresh hint
	hint := " [r]efresh [a]ttach [/]input [?]help"
	if m.options.DebugMode {
		hint += " [m]ethod"
	}
	hintStyled := lipgloss.NewStyle().Foreground(dimColor).Render(hint)

	content := lipgloss.JoinHorizontal(lipgloss.Center, label, input, hintStyled, " ", helpBtn)
	return style.Width(m.width - 4).Render(content)
}

// renderMainContent renders the tree and preview side by side
func (m Model) renderMainContent() string {
	tree := m.renderTree()
	preview := m.renderPreview()

	return lipgloss.JoinHorizontal(lipgloss.Top, tree, preview)
}

// renderTree renders the session/window/pane tree
func (m *Model) renderTree() string {
	var lines []string
	m.buttonZones = nil // Reset button zones

	treeHeight := m.height - inputHeight - statusHeight - 4
	if treeHeight < 1 {
		treeHeight = 1
	}

	// Calculate the Y offset for button zones (input bar + border)
	buttonYOffset := inputHeight + 2

	for i, node := range m.flatNodes {
		if i >= treeHeight {
			break
		}

		selected := i == m.selectedIndex
		indent := strings.Repeat("  ", node.Level)
		icon := getNodeIcon(node.Type, node.Expanded, node.Active)
		style := getNodeStyle(node.Type, node.Active, selected)

		// Build the line
		name := node.Name

		// Calculate button widths based on node type
		attButton := attachButtonStyle.Render("ATT")
		buttonGap := " "
		buttonsWidth := lipgloss.Width(attButton)

		if node.Type == "pane" {
			sendButton := sendButtonStyle.Render("SEND")
			escButton := escapeButtonStyle.Render("ESC")
			buttonsWidth = lipgloss.Width(sendButton) + len(buttonGap) + lipgloss.Width(escButton) + len(buttonGap) + lipgloss.Width(attButton)
		}

		maxNameLen := m.treeWidth - (node.Level * 2) - 4 - buttonsWidth // indent + icon + spacing + buttons
		if len(name) > maxNameLen && maxNameLen > 3 {
			name = name[:maxNameLen-3] + "..."
		}

		line := indent + icon + " " + style.Render(name)

		// Add buttons based on node type
		if node.Type == "pane" {
			// Panes get SEND, ESC, and ATT buttons
			sendButton := sendButtonStyle.Render("SEND")
			escButton := escapeButtonStyle.Render("ESC")
			attButton := attachButtonStyle.Render("ATT")
			buttonsWidth := lipgloss.Width(sendButton) + len(buttonGap) + lipgloss.Width(escButton) + len(buttonGap) + lipgloss.Width(attButton)
			buttonStartX := m.treeWidth - buttonsWidth

			m.buttonZones = append(m.buttonZones, buttonZone{
				x:      buttonStartX,
				y:      buttonYOffset + i,
				width:  lipgloss.Width(sendButton),
				height: 1,
				target: node.Target,
				action: buttonActionSend,
			})
			sendEndX := buttonStartX + lipgloss.Width(sendButton) + len(buttonGap)
			m.buttonZones = append(m.buttonZones, buttonZone{
				x:      sendEndX,
				y:      buttonYOffset + i,
				width:  lipgloss.Width(escButton),
				height: 1,
				target: node.Target,
				action: buttonActionEscape,
			})
			escEndX := sendEndX + lipgloss.Width(escButton) + len(buttonGap)
			m.buttonZones = append(m.buttonZones, buttonZone{
				x:      escEndX,
				y:      buttonYOffset + i,
				width:  lipgloss.Width(attButton),
				height: 1,
				target: node.Target,
				action: buttonActionAttach,
			})

			// Pad line to push buttons to the right
			lineLen := lipgloss.Width(line)
			padding := m.treeWidth - lineLen - buttonsWidth
			if padding < 1 {
				padding = 1
			}
			line = line + strings.Repeat(" ", padding) + sendButton + buttonGap + escButton + buttonGap + attButton
		} else {
			// Sessions and windows get only ATT button
			attButton := attachButtonStyle.Render("ATT")
			buttonsWidth := lipgloss.Width(attButton)
			buttonStartX := m.treeWidth - buttonsWidth

			m.buttonZones = append(m.buttonZones, buttonZone{
				x:      buttonStartX,
				y:      buttonYOffset + i,
				width:  lipgloss.Width(attButton),
				height: 1,
				target: node.Target,
				action: buttonActionAttach,
			})

			// Pad line to push button to the right
			lineLen := lipgloss.Width(line)
			padding := m.treeWidth - lineLen - buttonsWidth
			if padding < 1 {
				padding = 1
			}
			line = line + strings.Repeat(" ", padding) + attButton
		}

		lines = append(lines, line)
	}

	// Pad with empty lines
	for len(lines) < treeHeight {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")

	// Apply border style
	style := borderStyle
	if m.focused == FocusTree {
		style = activeBorderStyle
	}

	return style.
		Width(m.treeWidth).
		Height(treeHeight).
		Render(content)
}

// renderPreview renders the pane preview panel
func (m Model) renderPreview() string {
	previewHeight := m.height - inputHeight - statusHeight - 4
	if previewHeight < 1 {
		previewHeight = 1
	}

	var content string
	if node := m.selectedNode(); node != nil {
		if node.Type == "pane" {
			if m.previewContent != "" {
				content = m.previewPort.View()
			} else {
				content = lipgloss.NewStyle().
					Foreground(dimColor).
					Render("Loading preview...")
			}
		} else {
			content = lipgloss.NewStyle().
				Foreground(dimColor).
				Italic(true).
				Render("Select a pane to preview")
		}
	} else {
		content = lipgloss.NewStyle().
			Foreground(dimColor).
			Italic(true).
			Render("No pane selected")
	}

	// Header showing target
	header := ""
	if node := m.selectedNode(); node != nil && node.Type == "pane" {
		header = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			Render(node.Target) + "\n"
	}

	// Apply border style
	style := borderStyle
	if m.focused == FocusPreview {
		style = activeBorderStyle
	}

	return style.
		Width(m.previewWidth).
		Height(previewHeight).
		Render(header + content)
}

// renderStatusBar renders the status bar at the bottom
func (m Model) renderStatusBar() string {
	var parts []string

	// Debug mode: show send method
	if m.options.DebugMode {
		methodStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")). // Orange
			Bold(true)
		parts = append(parts, methodStyle.Render(fmt.Sprintf("Method: %s", m.sendMethod.String())))
	}

	// Focus indicator
	focusName := "Tree"
	switch m.focused {
	case FocusInput:
		focusName = "Input"
	case FocusPreview:
		focusName = "Preview"
	}
	parts = append(parts, fmt.Sprintf("Focus: %s", focusName))
	if m.mouseEnabled {
		parts = append(parts, "Mouse: on")
	} else {
		parts = append(parts, "Mouse: off")
	}

	// Selected target
	if node := m.selectedNode(); node != nil {
		parts = append(parts, statusSelectedStyle.Render(node.Target))
	}

	// Last sent command
	if m.lastSent != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(activeColor).Render("Sent: "+m.lastSent))
	}

	// Error display
	if m.lastError != nil {
		parts = append(parts, lipgloss.NewStyle().Foreground(errorColor).Render("Error: "+m.lastError.Error()))
	}

	status := strings.Join(parts, " | ")
	return statusBarStyle.Width(m.width - 2).Render(status)
}

// renderHelpOverlay renders the help overlay on top of the base view
func (m Model) renderHelpOverlay(base string) string {
	// Build help content
	title := helpTitleStyle.Render("agent-tmux browse - Help")

	keyboardSection := helpSectionStyle.Render("Keyboard Shortcuts")
	keyboard := []struct{ key, desc string }{
		{"↑/↓ or j/k", "Navigate tree"},
		{"Enter/Space", "Expand/collapse node"},
		{"a", "Attach to selected session"},
		{"s", "Send command to selected pane"},
		{"/", "Focus command input"},
		{"r", "Refresh tree"},
		{"M", "Toggle mouse support"},
		{"Tab", "Cycle focus (Tree → Input → Preview)"},
		{"Esc", "Clear input / Quit"},
		{"q", "Quit"},
		{"?", "Toggle this help"},
	}

	var keyboardLines []string
	for _, k := range keyboard {
		key := helpKeyStyle.Width(16).Render(k.key)
		desc := helpDescStyle.Render(k.desc)
		keyboardLines = append(keyboardLines, key+desc)
	}

	mouseSection := helpSectionStyle.Render("\nMouse Actions")
	mouse := []struct{ action, desc string }{
		{"Click tree node", "Select node"},
		{"Click [+]/[-]", "Expand/collapse"},
		{"Double-click", "Attach to session"},
		{"Click SEND", "Send command to pane"},
		{"Click ESC", "Send Escape to pane"},
		{"Click ATT", "Attach to session"},
		{"Drag divider", "Resize panels"},
		{"Scroll", "Scroll preview pane"},
	}

	var mouseLines []string
	for _, m := range mouse {
		action := helpKeyStyle.Width(16).Render(m.action)
		desc := helpDescStyle.Render(m.desc)
		mouseLines = append(mouseLines, action+desc)
	}

	buttonsSection := helpSectionStyle.Render("\nTree Buttons")
	buttons := []struct{ btn, desc string }{
		{"SEND", "Send command input to this pane"},
		{"ESC", "Send Escape key to this pane"},
		{"ATT", "Attach/switch to this session"},
	}

	var buttonLines []string
	for _, b := range buttons {
		btn := helpKeyStyle.Width(16).Render(b.btn)
		desc := helpDescStyle.Render(b.desc)
		buttonLines = append(buttonLines, btn+desc)
	}

	footer := helpDescStyle.Render("\nPress ? or Esc to close")

	helpContent := strings.Join([]string{
		title,
		"",
		keyboardSection,
		strings.Join(keyboardLines, "\n"),
		mouseSection,
		strings.Join(mouseLines, "\n"),
		buttonsSection,
		strings.Join(buttonLines, "\n"),
		footer,
	}, "\n")

	// Calculate overlay dimensions
	helpBox := helpOverlayStyle.Render(helpContent)
	helpWidth := lipgloss.Width(helpBox)
	helpHeight := lipgloss.Height(helpBox)

	// Center the overlay
	x := (m.width - helpWidth) / 2
	y := (m.height - helpHeight) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	// Place overlay on top of base
	return placeOverlay(x, y, helpBox, base)
}

// placeOverlay places an overlay string on top of a background at position (x, y)
func placeOverlay(x, y int, overlay, background string) string {
	bgLines := strings.Split(background, "\n")
	ovLines := strings.Split(overlay, "\n")

	for i, ovLine := range ovLines {
		bgY := y + i
		if bgY < 0 || bgY >= len(bgLines) {
			continue
		}

		bgLine := bgLines[bgY]
		bgRunes := []rune(bgLine)

		// Pad background line if needed
		for len(bgRunes) < x+lipgloss.Width(ovLine) {
			bgRunes = append(bgRunes, ' ')
		}

		// Insert overlay line
		ovRunes := []rune(ovLine)
		for j, r := range ovRunes {
			if x+j >= 0 && x+j < len(bgRunes) {
				bgRunes[x+j] = r
			}
		}

		bgLines[bgY] = string(bgRunes)
	}

	return strings.Join(bgLines, "\n")
}

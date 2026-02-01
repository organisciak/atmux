package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	primaryColor   = lipgloss.Color("39")  // Cyan
	secondaryColor = lipgloss.Color("170") // Magenta
	activeColor    = lipgloss.Color("82")  // Green
	dimColor       = lipgloss.Color("240") // Gray
	errorColor     = lipgloss.Color("196") // Red
	buttonColor    = lipgloss.Color("33")  // Blue

	// Border styles
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(dimColor)

	activeBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor)

	// Tree styles
	sessionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)

	sessionAttachedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(activeColor)

	windowStyle = lipgloss.NewStyle().
			Foreground(secondaryColor)

	windowActiveStyle = lipgloss.NewStyle().
				Foreground(activeColor)

	paneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	paneActiveStyle = lipgloss.NewStyle().
			Foreground(activeColor)

	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Bold(true)

	// Button styles
	sendButtonStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(buttonColor).
			Padding(0, 1)

	sendButtonHoverStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(activeColor).
				Padding(0, 1)

	escapeButtonStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(errorColor).
				Padding(0, 1)

	attachButtonStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(activeColor).
				Padding(0, 1)

	helpButtonStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("99")). // Purple
			Padding(0, 1)

	// Help overlay styles
	helpOverlayStyle = lipgloss.NewStyle().
				Border(lipgloss.DoubleBorder()).
				BorderForeground(primaryColor).
				Padding(1, 2)

	helpTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)

	helpSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(secondaryColor)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(activeColor).
			Bold(true)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	// Input styles
	inputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(dimColor).
			Padding(0, 1)

	inputFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor).
				Padding(0, 1)

	// Preview styles
	previewStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250"))

	// Status bar styles
	statusBarStyle = lipgloss.NewStyle().
			Foreground(dimColor).
			Padding(0, 1)

	statusSelectedStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true)

	// Expand/collapse indicators
	expandedIcon   = "[-]"
	collapsedIcon  = "[+]"
	paneIcon       = " > "
	paneActiveIcon = "[*]"

	// Layout constants
	treeWidthPercent    = 35
	previewWidthPercent = 65
	minTreeWidth        = 30
	minPreviewWidth     = 40
	inputHeight         = 3
	statusHeight        = 1

	// Scheduler-specific styles
	schedTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)

	schedAddBtnStyle = lipgloss.NewStyle().
				Background(activeColor).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)

	schedStatusActiveStyle = lipgloss.NewStyle().Foreground(activeColor)
	schedStatusDimStyle    = lipgloss.NewStyle().Foreground(dimColor)

	schedIDStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))

	schedTargetStyle = lipgloss.NewStyle().Foreground(dimColor)

	schedHintStyle = lipgloss.NewStyle().Foreground(dimColor)

	schedSeparatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	schedConfirmStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("196")).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)

	// Wizard styles
	wizSubtitleStyle = lipgloss.NewStyle().Foreground(dimColor)

	wizBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2)

	wizInputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(0, 1).
			Width(40)

	wizCronFieldStyle = lipgloss.NewStyle().Width(10).Align(lipgloss.Center)

	wizCronFieldFocusStyle = lipgloss.NewStyle().
				Width(10).
				Align(lipgloss.Center).
				Bold(true).
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("255"))

	wizCronHeaderStyle = lipgloss.NewStyle().Width(10).Align(lipgloss.Center)

	wizCronHeaderFocusStyle = lipgloss.NewStyle().
					Width(10).
					Align(lipgloss.Center).
					Bold(true).
					Foreground(primaryColor)

	wizCronRangeStyle = lipgloss.NewStyle().
				Width(10).
				Align(lipgloss.Center).
				Foreground(dimColor)

	wizCronRangeFocusStyle = lipgloss.NewStyle().
				Width(10).
				Align(lipgloss.Center).
				Foreground(primaryColor)

	wizPreviewOKStyle  = lipgloss.NewStyle().Foreground(activeColor)
	wizPreviewErrStyle = lipgloss.NewStyle().Foreground(errorColor)
	wizRefStyle        = lipgloss.NewStyle().Foreground(dimColor)
	wizLabelStyle      = lipgloss.NewStyle().Foreground(dimColor)
	wizValueStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	wizSaveBtnStyle = lipgloss.NewStyle().
			Background(activeColor).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)

	wizCancelBtnStyle = lipgloss.NewStyle().
				Background(dimColor).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)

	wizSaveBtnActiveStyle = lipgloss.NewStyle().
				Background(activeColor).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1).
				Bold(true)

	wizCancelBtnActiveStyle = lipgloss.NewStyle().
					Background(errorColor).
					Foreground(lipgloss.Color("255")).
					Padding(0, 1).
					Bold(true)

	wizSaveBtnInactiveStyle = lipgloss.NewStyle().
					Background(dimColor).
					Foreground(lipgloss.Color("255")).
					Padding(0, 1)

	wizSeparatorStyle = lipgloss.NewStyle().Foreground(dimColor)
)

// Helper to get tree node style based on type and state
func getNodeStyle(nodeType string, active, selected bool) lipgloss.Style {
	var style lipgloss.Style

	switch nodeType {
	case "session":
		style = sessionStyle
	case "window":
		if active {
			style = windowActiveStyle
		} else {
			style = windowStyle
		}
	case "pane":
		if active {
			style = paneActiveStyle
		} else {
			style = paneStyle
		}
	default:
		style = paneStyle
	}

	if selected {
		style = style.Inherit(selectedStyle)
	}

	return style
}

// Helper to get the appropriate icon for a node
func getNodeIcon(nodeType string, expanded, active bool) string {
	switch nodeType {
	case "session":
		if expanded {
			return expandedIcon
		}
		return collapsedIcon
	case "window":
		if expanded {
			return expandedIcon
		}
		return collapsedIcon
	case "pane":
		if active {
			return paneActiveIcon
		}
		return paneIcon
	}
	return "   "
}

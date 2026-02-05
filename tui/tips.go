package tui

import (
	"math/rand"

	"github.com/charmbracelet/lipgloss"
)

// tips contains helpful hints shown to users across different views
var tips = []string{
	"Double-click to attach to a session",
	"Press ? for full keyboard shortcuts",
	"Use / to focus the command input",
	"Press Enter to attach to selected session",
	"Tab cycles between sections",
	"Press r to refresh the session list",
	"Use j/k or arrow keys to navigate",
	"Press a to attach to the selected session",
	"Space expands or collapses tree nodes",
	"Press M to toggle mouse support",
	"Run `atmux onboard` for a quick setup guide",
	"Use Esc to exit input mode or quit",
	// Additional tips for discoverability
	"Click SEND button to send input to a specific pane",
	"Click ESC button to send Escape key to a pane",
	"Drag the divider to resize tree and preview panels",
	"Scroll wheel works in the preview pane",
	"Shift+Tab cycles focus in reverse order",
	"Press x or Delete to remove a history entry",
	"Up/Down arrows recall previous commands in input",
	"Run `atmux keybind` to add a tmux hotkey for browse",
	"Run `atmux init` to create a project config file",
	"Use `atmux send` to script commands to panes",
	"Run `atmux sessions NAME` to attach directly by name",
	"Project .agent-tmux.conf overrides global config",
	"Set your default startup behavior in landing page",
	"Run `atmux --reset-defaults` to restore landing page",
	"Press s in browse view to send command to selected pane",
	"Ctrl+C twice to quit from any view",
}

// tipStyle defines the subtle appearance for tips
var tipStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("238"))

// GetRandomTip returns a random tip string
func GetRandomTip() string {
	return tips[rand.Intn(len(tips))]
}

// RenderTip returns a styled tip string
func RenderTip() string {
	return tipStyle.Render(GetRandomTip())
}

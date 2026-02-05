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

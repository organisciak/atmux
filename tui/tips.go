package tui

import (
	"math/rand"

	"github.com/charmbracelet/lipgloss"
)

// TipContext represents the screen context for filtering tips
type TipContext int

const (
	TipAll       TipContext = 0
	TipBrowse    TipContext = 1
	TipLanding   TipContext = 2
	TipSessions  TipContext = 3
	TipRecents   TipContext = 4
	TipScheduler TipContext = 5
)

// Tip holds a tip string and the contexts where it should appear.
// An empty Contexts slice means the tip is shown in all contexts.
type Tip struct {
	Text     string
	Contexts []TipContext
}

// tips contains helpful hints shown to users across different views
var tips = []Tip{
	{Text: "Double-click to attach to a session", Contexts: []TipContext{TipSessions, TipLanding}},
	{Text: "Press ? for full keyboard shortcuts", Contexts: nil},
	{Text: "Use / to focus the command input", Contexts: []TipContext{TipBrowse}},
	{Text: "Press Enter to attach to selected session", Contexts: []TipContext{TipSessions, TipLanding}},
	{Text: "Tab cycles between sections", Contexts: nil},
	{Text: "Press r to refresh the session list", Contexts: nil},
	{Text: "Use j/k or arrow keys to navigate", Contexts: nil},
	{Text: "Press a to attach to the selected session", Contexts: []TipContext{TipSessions, TipLanding}},
	{Text: "Use --remote to include remote hosts in sessions", Contexts: []TipContext{TipSessions}},
	{Text: "Space expands or collapses tree nodes", Contexts: []TipContext{TipBrowse}},
	{Text: "Press M to toggle mouse support", Contexts: []TipContext{TipBrowse}},
	{Text: "Use `atmux browse --remote=devbox` to include remote panes", Contexts: []TipContext{TipBrowse}},
	{Text: "Run `atmux onboard` for a quick setup guide", Contexts: nil},
	{Text: "Use Esc to exit input mode or quit", Contexts: nil},
	// Additional tips for discoverability
	{Text: "Click SEND button to send input to a specific pane", Contexts: []TipContext{TipBrowse}},
	{Text: "Click ESC button to send Escape key to a pane", Contexts: []TipContext{TipBrowse}},
	{Text: "Drag the divider to resize tree and preview panels", Contexts: []TipContext{TipBrowse}},
	{Text: "Scroll wheel works in the preview pane", Contexts: []TipContext{TipBrowse}},
	{Text: "Shift+Tab cycles focus in reverse order", Contexts: nil},
	{Text: "Press x or Delete to remove a history entry", Contexts: []TipContext{TipRecents}},
	{Text: "Remote recents are marked with @host", Contexts: []TipContext{TipRecents}},
	{Text: "Up/Down arrows recall previous commands in input", Contexts: []TipContext{TipBrowse}},
	{Text: "Use `atmux sessions -p` for a popup session picker", Contexts: nil},
	{Text: "Run `atmux keybind --command sessions` for quick popup access", Contexts: nil},
	{Text: "Run `atmux recents` to quickly reopen recent projects", Contexts: nil},
	{Text: "Run `atmux init` to create a project config file", Contexts: nil},
	{Text: "Use `atmux send` to script commands to panes", Contexts: nil},
	{Text: "Run `atmux sessions NAME` to attach directly by name", Contexts: nil},
	{Text: "Project .agent-tmux.conf overrides global config", Contexts: nil},
	{Text: "Set your default startup behavior in landing page", Contexts: []TipContext{TipLanding}},
	{Text: "Run `atmux --reset-defaults` to restore landing page", Contexts: []TipContext{TipLanding}},
	{Text: "Press s in browse view to send command to selected pane", Contexts: []TipContext{TipBrowse}},
	{Text: "Ctrl+C twice to quit from any view", Contexts: nil},
}

// tipStyle defines the subtle appearance for tips
var tipStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("238"))

// tipLabelStyle is slightly brighter than the tip text for the "Tip:" prefix
var tipLabelStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("243"))

// GetRandomTip returns a random tip string (from all tips)
func GetRandomTip() string {
	return tips[rand.Intn(len(tips))].Text
}

// GetRandomTipForContext returns a random tip appropriate for the given context.
// Tips with an empty Contexts slice are included in every context.
func GetRandomTipForContext(ctx TipContext) string {
	var filtered []Tip
	for _, t := range tips {
		if len(t.Contexts) == 0 {
			filtered = append(filtered, t)
			continue
		}
		for _, c := range t.Contexts {
			if c == ctx {
				filtered = append(filtered, t)
				break
			}
		}
	}
	if len(filtered) == 0 {
		return tips[rand.Intn(len(tips))].Text
	}
	return filtered[rand.Intn(len(filtered))].Text
}

// RenderTip returns a styled tip string with a "Tip: " prefix (all contexts)
func RenderTip() string {
	return tipLabelStyle.Render("Tip: ") + tipStyle.Render(GetRandomTip())
}

// RenderTipForContext returns a styled tip string filtered by context
func RenderTipForContext(ctx TipContext) string {
	return tipLabelStyle.Render("Tip: ") + tipStyle.Render(GetRandomTipForContext(ctx))
}

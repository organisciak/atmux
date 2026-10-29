package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// HubView is another atmux view that the sessions list can open. They sit in a
// strip of tabs across the top of the list, so everything atmux can show is
// one arrow key away and stays discoverable as views are added.
type HubView struct {
	Name    string // Tab label, and what SessionsResult.OpenView reports
	Summary string // What the view is for, in a sentence or two
	Command string // How to open it directly, e.g. "atmux agents"
	Key     string // The tmux key bound to it, e.g. "prefix a"; empty if none
}

// sessionsTabName labels the list itself, always the first tab.
const sessionsTabName = "Sessions"

const viewStripGap = "  "

// viewStripLabels returns every tab label, the list's first.
func viewStripLabels(views []HubView) []string {
	labels := make([]string, 0, len(views)+1)
	labels = append(labels, sessionsTabName)
	for _, v := range views {
		labels = append(labels, v.Name)
	}
	return labels
}

// renderViewStrip draws the tab strip with the selected tab highlighted.
func renderViewStrip(views []HubView, selected, width int) string {
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(primaryColor)
	if selected == 0 {
		// On the list itself the strip doubles as its title.
		selectedStyle = lipgloss.NewStyle().Bold(true)
	}
	otherStyle := lipgloss.NewStyle().Foreground(dimColor)

	var parts []string
	for i, label := range viewStripLabels(views) {
		if i == selected {
			parts = append(parts, selectedStyle.Render(label))
		} else {
			parts = append(parts, otherStyle.Render(label))
		}
	}
	strip := strings.Join(parts, viewStripGap)
	if width > 0 {
		strip = ansi.Truncate(strip, width, "…")
	}
	return strip
}

// viewStripTabAt returns the tab under column x of the strip, or -1.
func viewStripTabAt(views []HubView, x int) int {
	start := 0
	for i, label := range viewStripLabels(views) {
		end := start + ansi.StringWidth(label)
		if x >= start && x < end {
			return i
		}
		start = end + len(viewStripGap)
	}
	return -1
}

// renderViewCard describes a view in place of the list, with how to open it.
func renderViewCard(v HubView, width int) string {
	textWidth := width - 2
	if textWidth < 20 {
		textWidth = 20
	}
	indent := func(s string) string {
		return "  " + strings.ReplaceAll(s, "\n", "\n  ")
	}
	dim := lipgloss.NewStyle().Foreground(dimColor)

	lines := []string{
		indent(lipgloss.NewStyle().Bold(true).Render(v.Name)),
		indent(ansi.Wrap(v.Summary, textWidth, "")),
		"",
	}
	var how []string
	if v.Key != "" {
		how = append(how, v.Key)
	}
	if v.Command != "" {
		how = append(how, v.Command)
	}
	if len(how) > 0 {
		lines = append(lines, indent(dim.Render("Also: "+strings.Join(how, "  ·  "))))
	}
	lines = append(lines, indent(lipgloss.NewStyle().Foreground(buttonColor).Render("Enter or click to open")))
	return strings.Join(lines, "\n")
}

// selectedView returns the view whose tab is selected, if it is not the list.
func (m sessionsModel) selectedView() (HubView, bool) {
	if m.viewTab <= 0 || m.viewTab > len(m.views) {
		return HubView{}, false
	}
	return m.views[m.viewTab-1], true
}

// handleViewTabKey moves between tabs and opens views. It reports false for
// keys the list should handle as usual. Letters stay with the list, where
// typing starts a search.
func (m sessionsModel) handleViewTabKey(key string) (sessionsModel, tea.Cmd, bool) {
	if len(m.views) == 0 {
		return m, nil, false
	}
	tabs := len(m.views) + 1
	switch key {
	case "right", "tab":
		m.viewTab = (m.viewTab + 1) % tabs
		return m, nil, true
	case "left", "shift+tab":
		m.viewTab = (m.viewTab + tabs - 1) % tabs
		return m, nil, true
	}
	v, ok := m.selectedView()
	if !ok {
		return m, nil, false
	}
	switch key {
	case "enter":
		m.openView = v.Name
		return m, tea.Quit, true
	case "esc":
		m.viewTab = 0
	case "q", "ctrl+c":
		return m, nil, false
	}
	// Anything else would act on the hidden list, so it does nothing here.
	return m, nil, true
}

// handleViewStripClick selects a tab, or opens the view when its tab is
// clicked again or its card is clicked. It reports false for clicks on the
// list.
func (m sessionsModel) handleViewStripClick(x, y int) (sessionsModel, tea.Cmd, bool) {
	if len(m.views) == 0 {
		return m, nil, false
	}
	if y == 0 {
		tab := viewStripTabAt(m.views, x)
		if tab < 0 {
			return m, nil, true
		}
		if tab == m.viewTab && tab > 0 {
			m.openView = m.views[tab-1].Name
			return m, tea.Quit, true
		}
		m.viewTab = tab
		return m, nil, true
	}
	if v, ok := m.selectedView(); ok {
		m.openView = v.Name
		return m, tea.Quit, true
	}
	return m, nil, false
}

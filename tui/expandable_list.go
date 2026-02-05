package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ListItem represents a single item in an expandable list.
// Implementations should provide rendering and identification.
type ListItem interface {
	// Render returns the styled string representation of the item.
	// selected indicates if this item is currently selected.
	// width is the available width for rendering.
	Render(selected bool, width int) string
	// ID returns a unique identifier for this item.
	ID() string
}

// ExpandableList is a reusable component for displaying lists with
// expand/collapse functionality for browsing more items.
type ExpandableList struct {
	Items         []ListItem
	MaxCollapsed  int            // Items shown when collapsed (default: 5)
	MaxExpanded   int            // Items shown when expanded (default: 20)
	Expanded      bool           // Whether the list is currently expanded
	SelectedIndex int            // Currently selected item index
	OnSelect      func(ListItem) // Called when an item is selected (Enter key)
	OnExpand      func()         // Called when 'show more' is triggered

	// Internal state
	showMoreSelected bool // True when "show more/less" footer is selected
}

// DefaultMaxCollapsed is the default number of items shown when collapsed.
const DefaultMaxCollapsed = 5

// DefaultMaxExpanded is the default number of items shown when expanded.
const DefaultMaxExpanded = 20

// NewExpandableList creates a new ExpandableList with default values.
func NewExpandableList(items []ListItem) *ExpandableList {
	return &ExpandableList{
		Items:        items,
		MaxCollapsed: DefaultMaxCollapsed,
		MaxExpanded:  DefaultMaxExpanded,
		Expanded:     false,
	}
}

// VisibleCount returns the number of items currently visible.
func (e *ExpandableList) VisibleCount() int {
	if len(e.Items) <= e.maxCollapsed() {
		return len(e.Items)
	}
	if e.Expanded {
		if len(e.Items) <= e.maxExpanded() {
			return len(e.Items)
		}
		return e.maxExpanded()
	}
	return e.maxCollapsed()
}

// TotalSelectableCount returns the total number of selectable positions
// (visible items + show more/less footer if applicable).
func (e *ExpandableList) TotalSelectableCount() int {
	count := e.VisibleCount()
	if e.HasFooter() {
		count++ // Add 1 for the "show more/less" footer
	}
	return count
}

// HasFooter returns true if the expand/collapse footer should be shown.
func (e *ExpandableList) HasFooter() bool {
	return len(e.Items) > e.maxCollapsed()
}

// HiddenCount returns the number of items currently hidden.
func (e *ExpandableList) HiddenCount() int {
	total := len(e.Items)
	visible := e.VisibleCount()
	return total - visible
}

// maxCollapsed returns MaxCollapsed or the default if not set.
func (e *ExpandableList) maxCollapsed() int {
	if e.MaxCollapsed <= 0 {
		return DefaultMaxCollapsed
	}
	return e.MaxCollapsed
}

// maxExpanded returns MaxExpanded or the default if not set.
func (e *ExpandableList) maxExpanded() int {
	if e.MaxExpanded <= 0 {
		return DefaultMaxExpanded
	}
	return e.MaxExpanded
}

// VisibleItems returns the slice of currently visible items.
func (e *ExpandableList) VisibleItems() []ListItem {
	count := e.VisibleCount()
	if count > len(e.Items) {
		count = len(e.Items)
	}
	return e.Items[:count]
}

// IsFooterSelected returns true if the "show more/less" footer is currently selected.
func (e *ExpandableList) IsFooterSelected() bool {
	if !e.HasFooter() {
		return false
	}
	return e.SelectedIndex >= e.VisibleCount()
}

// SelectedItem returns the currently selected item, or nil if footer is selected or no items.
func (e *ExpandableList) SelectedItem() ListItem {
	if e.IsFooterSelected() || e.SelectedIndex < 0 || e.SelectedIndex >= len(e.Items) {
		return nil
	}
	visible := e.VisibleItems()
	if e.SelectedIndex >= len(visible) {
		return nil
	}
	return visible[e.SelectedIndex]
}

// Update handles keyboard and mouse messages for the expandable list.
// Returns the updated list and any command to execute.
func (e *ExpandableList) Update(msg tea.Msg) (*ExpandableList, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return e.handleKeyMsg(msg)
	case tea.MouseMsg:
		return e.handleMouseMsg(msg)
	}
	return e, nil
}

// handleKeyMsg handles keyboard input.
func (e *ExpandableList) handleKeyMsg(msg tea.KeyMsg) (*ExpandableList, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		e.MoveSelection(-1)
		return e, nil
	case "down", "j":
		e.MoveSelection(1)
		return e, nil
	case "enter", " ":
		return e.handleSelect()
	}
	return e, nil
}

// handleMouseMsg handles mouse input.
func (e *ExpandableList) handleMouseMsg(msg tea.MouseMsg) (*ExpandableList, tea.Cmd) {
	// Mouse handling is expected to be coordinated by the parent component
	// which tracks y-positions. This is a placeholder for direct click handling
	// if the parent provides transformed coordinates.
	return e, nil
}

// handleSelect handles Enter key or click on the selected item.
func (e *ExpandableList) handleSelect() (*ExpandableList, tea.Cmd) {
	if e.IsFooterSelected() {
		e.ToggleExpanded()
		return e, nil
	}
	if item := e.SelectedItem(); item != nil && e.OnSelect != nil {
		e.OnSelect(item)
	}
	return e, nil
}

// MoveSelection moves the selection by delta positions.
func (e *ExpandableList) MoveSelection(delta int) {
	total := e.TotalSelectableCount()
	if total == 0 {
		return
	}

	e.SelectedIndex += delta
	if e.SelectedIndex < 0 {
		e.SelectedIndex = 0
	}
	if e.SelectedIndex >= total {
		e.SelectedIndex = total - 1
	}
}

// ToggleExpanded toggles between expanded and collapsed state.
func (e *ExpandableList) ToggleExpanded() {
	e.Expanded = !e.Expanded

	// Clamp selection to new visible range
	total := e.TotalSelectableCount()
	if e.SelectedIndex >= total {
		e.SelectedIndex = total - 1
	}
	if e.SelectedIndex < 0 {
		e.SelectedIndex = 0
	}

	if e.OnExpand != nil {
		e.OnExpand()
	}
}

// SelectByIndex sets the selection to the given index (for mouse clicks).
// Returns true if the index was valid and selection changed.
func (e *ExpandableList) SelectByIndex(index int) bool {
	total := e.TotalSelectableCount()
	if index < 0 || index >= total {
		return false
	}
	e.SelectedIndex = index
	return true
}

// Styles for the expandable list footer
var (
	expandFooterStyle = lipgloss.NewStyle().
				Foreground(primaryColor)

	expandFooterSelectedStyle = lipgloss.NewStyle().
					Foreground(primaryColor).
					Bold(true).
					Background(lipgloss.Color("236"))

	expandFooterDimStyle = lipgloss.NewStyle().
				Foreground(dimColor)
)

// View renders the expandable list.
func (e *ExpandableList) View(width int) string {
	var lines []string

	// Render visible items
	visible := e.VisibleItems()
	for i, item := range visible {
		selected := i == e.SelectedIndex && !e.IsFooterSelected()
		lines = append(lines, item.Render(selected, width))
	}

	// Render footer if needed
	if e.HasFooter() {
		footer := e.renderFooter(width)
		lines = append(lines, footer)
	}

	return strings.Join(lines, "\n")
}

// renderFooter renders the "Show more (N)" or "Show less" footer.
func (e *ExpandableList) renderFooter(width int) string {
	var text string
	icon := expandedIcon

	if e.Expanded {
		icon = collapsedIcon
		text = "Show less"
	} else {
		icon = expandedIcon
		hidden := e.HiddenCount()
		text = fmt.Sprintf("Show more (%d)", hidden)
	}

	// Use a down/up arrow indicator
	if e.Expanded {
		icon = lipgloss.NewStyle().Foreground(primaryColor).Render("\u25b2") // Up arrow
	} else {
		icon = lipgloss.NewStyle().Foreground(primaryColor).Render("\u25bc") // Down arrow
	}

	style := expandFooterDimStyle
	if e.IsFooterSelected() {
		style = expandFooterSelectedStyle
	}

	return "  " + icon + " " + style.Render(text)
}

// RenderFooterLine is a helper to render just the footer text (for external layout).
func (e *ExpandableList) RenderFooterLine(selected bool) string {
	if !e.HasFooter() {
		return ""
	}

	var text string
	var icon string

	if e.Expanded {
		icon = "\u25b2" // Up arrow
		text = "Show less"
	} else {
		icon = "\u25bc" // Down arrow
		hidden := e.HiddenCount()
		text = fmt.Sprintf("Show more (%d)", hidden)
	}

	iconStyled := lipgloss.NewStyle().Foreground(primaryColor).Render(icon)

	style := expandFooterDimStyle
	if selected {
		style = expandFooterSelectedStyle
	}

	return "  " + iconStyled + " " + style.Render(text)
}

// Reset resets the list to its initial collapsed state.
func (e *ExpandableList) Reset() {
	e.Expanded = false
	e.SelectedIndex = 0
}

// SetItems updates the items in the list and resets selection if needed.
func (e *ExpandableList) SetItems(items []ListItem) {
	e.Items = items
	total := e.TotalSelectableCount()
	if e.SelectedIndex >= total {
		e.SelectedIndex = total - 1
	}
	if e.SelectedIndex < 0 {
		e.SelectedIndex = 0
	}
}

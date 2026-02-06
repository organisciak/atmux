package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ContextMenu represents a right-click context menu
type ContextMenu struct {
	Items    []MenuItem
	Position Position // x, y where menu appears
	Selected int
	Width    int
	Visible  bool
	NodeType string // "session", "window", or "pane"
	Target   string // Target of the node this menu is for
	NodeName string // Display name of the node
}

// Position represents an x, y coordinate
type Position struct {
	X int
	Y int
}

// MenuItem represents a single menu item
type MenuItem struct {
	Label    string
	Shortcut string // Shown on right side
	Action   string // Action identifier
	Disabled bool
	Divider  bool // Render as separator line
}

// Menu styles
var (
	menuBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(0, 1)

	menuItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	menuItemSelectedStyle = lipgloss.NewStyle().
				Background(primaryColor).
				Foreground(lipgloss.Color("255"))

	menuItemDisabledStyle = lipgloss.NewStyle().
				Foreground(dimColor)

	menuShortcutStyle = lipgloss.NewStyle().
				Foreground(dimColor)

	menuDividerStyle = lipgloss.NewStyle().
				Foreground(dimColor)
)

// Menu action constants
const (
	MenuActionAttach       = "attach"
	MenuActionAttachPopup  = "attach_popup"
	MenuActionNewWindow    = "new_window"
	MenuActionRename       = "rename"
	MenuActionKillSession  = "kill_session"
	MenuActionSelectWindow = "select_window"
	MenuActionNewPaneH     = "new_pane_h"
	MenuActionNewPaneV     = "new_pane_v"
	MenuActionMoveWindow   = "move_window"
	MenuActionKillWindow   = "kill_window"
	MenuActionSelectPane   = "select_pane"
	MenuActionZoomPane     = "zoom_pane"
	MenuActionSendKeys     = "send_keys"
	MenuActionSwapPane     = "swap_pane"
	MenuActionKillPane     = "kill_pane"
)

// NewContextMenu creates a new context menu for the given node type
func NewContextMenu(nodeType, target, name string, x, y int) *ContextMenu {
	menu := &ContextMenu{
		Position: Position{X: x, Y: y},
		Selected: 0,
		Visible:  true,
		NodeType: nodeType,
		Target:   target,
		NodeName: name,
	}

	switch nodeType {
	case "session":
		menu.Items = sessionMenuItems()
	case "window":
		menu.Items = windowMenuItems()
	case "pane":
		menu.Items = paneMenuItems()
	}

	menu.calculateWidth()
	return menu
}

// sessionMenuItems returns the menu items for a session context menu
func sessionMenuItems() []MenuItem {
	return []MenuItem{
		{Label: "Attach", Shortcut: "a", Action: MenuActionAttach},
		{Label: "Attach (popup)", Action: MenuActionAttachPopup},
		{Divider: true},
		{Label: "New window", Action: MenuActionNewWindow},
		{Label: "Rename...", Action: MenuActionRename},
		{Divider: true},
		{Label: "Kill session", Shortcut: "x", Action: MenuActionKillSession},
	}
}

// windowMenuItems returns the menu items for a window context menu
func windowMenuItems() []MenuItem {
	return []MenuItem{
		{Label: "Select window", Action: MenuActionSelectWindow},
		{Divider: true},
		{Label: "New pane (horizontal)", Shortcut: "h", Action: MenuActionNewPaneH},
		{Label: "New pane (vertical)", Shortcut: "v", Action: MenuActionNewPaneV},
		{Label: "Rename...", Action: MenuActionRename},
		{Label: "Move to session...", Action: MenuActionMoveWindow, Disabled: true},
		{Divider: true},
		{Label: "Kill window", Shortcut: "x", Action: MenuActionKillWindow},
	}
}

// paneMenuItems returns the menu items for a pane context menu
func paneMenuItems() []MenuItem {
	return []MenuItem{
		{Label: "Select pane", Action: MenuActionSelectPane},
		{Label: "Zoom toggle", Shortcut: "z", Action: MenuActionZoomPane},
		{Divider: true},
		{Label: "Send keys...", Action: MenuActionSendKeys},
		{Label: "Swap with...", Action: MenuActionSwapPane, Disabled: true},
		{Divider: true},
		{Label: "Kill pane", Shortcut: "x", Action: MenuActionKillPane},
	}
}

// calculateWidth calculates the menu width based on items
func (m *ContextMenu) calculateWidth() {
	maxLen := 0
	for _, item := range m.Items {
		if item.Divider {
			continue
		}
		itemLen := len(item.Label)
		if item.Shortcut != "" {
			itemLen += 4 + len(item.Shortcut) // "  " + shortcut + " "
		}
		if itemLen > maxLen {
			maxLen = itemLen
		}
	}
	m.Width = maxLen + 4 // padding
}

// MoveSelection moves the selection up or down, skipping dividers
func (m *ContextMenu) MoveSelection(delta int) {
	if len(m.Items) == 0 {
		return
	}

	newIdx := m.Selected
	for {
		newIdx += delta
		if newIdx < 0 {
			newIdx = len(m.Items) - 1
		}
		if newIdx >= len(m.Items) {
			newIdx = 0
		}

		// Found a non-divider item
		if !m.Items[newIdx].Divider {
			m.Selected = newIdx
			return
		}

		// Prevent infinite loop if all items are dividers
		if newIdx == m.Selected {
			return
		}
	}
}

// SelectedItem returns the currently selected menu item
func (m *ContextMenu) SelectedItem() *MenuItem {
	if m.Selected >= 0 && m.Selected < len(m.Items) {
		return &m.Items[m.Selected]
	}
	return nil
}

// Render renders the context menu
func (m *ContextMenu) Render() string {
	if !m.Visible || len(m.Items) == 0 {
		return ""
	}

	var lines []string
	for i, item := range m.Items {
		if item.Divider {
			divider := strings.Repeat("-", m.Width-2)
			lines = append(lines, menuDividerStyle.Render(divider))
			continue
		}

		// Build the line
		label := item.Label
		shortcut := ""
		if item.Shortcut != "" {
			shortcut = item.Shortcut
		}

		// Calculate padding between label and shortcut
		paddingLen := m.Width - len(label) - len(shortcut) - 2
		if paddingLen < 2 {
			paddingLen = 2
		}
		padding := strings.Repeat(" ", paddingLen)

		line := label + padding + shortcut

		// Apply style based on state
		var styledLine string
		if item.Disabled {
			styledLine = menuItemDisabledStyle.Width(m.Width).Render(line)
		} else if i == m.Selected {
			styledLine = menuItemSelectedStyle.Width(m.Width).Render(line)
		} else {
			styledLine = menuItemStyle.Width(m.Width).Render(line)
		}

		lines = append(lines, styledLine)
	}

	content := strings.Join(lines, "\n")
	return menuBorderStyle.Render(content)
}

// Height returns the height of the rendered menu
func (m *ContextMenu) Height() int {
	if !m.Visible || len(m.Items) == 0 {
		return 0
	}
	return len(m.Items) + 2 // items + border
}

// Contains checks if a point is inside the menu
func (m *ContextMenu) Contains(x, y int) bool {
	if !m.Visible {
		return false
	}

	menuWidth := m.Width + 4  // including border and padding
	menuHeight := m.Height()

	return x >= m.Position.X && x < m.Position.X+menuWidth &&
		y >= m.Position.Y && y < m.Position.Y+menuHeight
}

// ItemAtPosition returns the menu item at the given y position, if any
func (m *ContextMenu) ItemAtPosition(x, y int) (int, *MenuItem) {
	if !m.Visible {
		return -1, nil
	}

	// Check if click is within menu bounds
	menuWidth := m.Width + 4
	if x < m.Position.X || x >= m.Position.X+menuWidth {
		return -1, nil
	}

	// Calculate which item was clicked (accounting for border)
	itemY := y - m.Position.Y - 1 // -1 for top border
	if itemY < 0 || itemY >= len(m.Items) {
		return -1, nil
	}

	item := &m.Items[itemY]
	if item.Divider || item.Disabled {
		return -1, nil
	}

	return itemY, item
}

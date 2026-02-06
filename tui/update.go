package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/tmux"
)

// Update handles messages and updates state
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Route to mobile handler if in mobile mode
		if m.mobileMode {
			return m.handleMobileKeyMsg(msg)
		}
		return m.handleKeyMsg(msg)

	case tea.MouseMsg:
		// Route to mobile handler if in mobile mode
		if m.mobileMode {
			return m.handleMobileMouseMsg(msg)
		}
		return m.handleMouseMsg(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Auto-detect mobile mode based on terminal width (unless forced via --mobile)
		if !m.mobileForcedMode {
			m.mobileMode = shouldUseMobileLayout(m.width, false)
		}
		m.calculateLayout()
		m.calculateButtonZones()
		m.commandInput.Width = m.width - 20
		return m, nil

	case TreeRefreshedMsg:
		if msg.Err != nil {
			m.lastError = msg.Err
		} else {
			m.tree = msg.Tree
			m.rebuildFlatNodes()
			m.calculateButtonZones()
			m.lastError = nil
			// Re-filter recent sessions against active tree
			m.filterRecentSessions()

			// Fetch preview for selected node
			if node := m.selectedNode(); node != nil && node.Type == "pane" {
				cmds = append(cmds, fetchPreview(node.Target))
			}
		}
		// Schedule next refresh
		if m.options.RefreshInterval > 0 {
			cmds = append(cmds, tickCmd(m.options.RefreshInterval))
		}
		return m, tea.Batch(cmds...)

	case RecentSessionsMsg:
		if msg.Err == nil {
			m.recentSessions = msg.Entries
			m.filterRecentSessions()
			// Clamp selection
			if m.recentSelectedIndex >= len(m.recentSessions) {
				m.recentSelectedIndex = len(m.recentSessions) - 1
			}
			if m.recentSelectedIndex < 0 {
				m.recentSelectedIndex = 0
			}
		}
		return m, nil

	case RecentDeletedMsg:
		if msg.Err == nil {
			// Remove the deleted entry from the list
			for i, e := range m.recentSessions {
				if e.ID == msg.ID {
					m.recentSessions = append(m.recentSessions[:i], m.recentSessions[i+1:]...)
					break
				}
			}
			// Clamp selection
			if m.recentSelectedIndex >= len(m.recentSessions) {
				m.recentSelectedIndex = len(m.recentSessions) - 1
			}
			if m.recentSelectedIndex < 0 {
				m.recentSelectedIndex = 0
			}
			// If no more recent sessions and focus was on recent, move back to tree
			if len(m.recentSessions) == 0 && m.focusRecent {
				m.focusRecent = false
			}
		}
		return m, nil

	case PreviewUpdatedMsg:
		if msg.Err == nil && msg.Target == m.previewTarget {
			m.previewContent = msg.Content
			m.previewPort.SetContent(msg.Content)
			m.previewPort.GotoBottom()
		}
		return m, nil

	case CommandSentMsg:
		if msg.Err != nil {
			m.lastError = msg.Err
		} else {
			m.lastSent = msg.Command + " -> " + msg.Target
			// Refresh preview after sending
			cmds = append(cmds, fetchPreview(msg.Target))
		}
		return m, tea.Batch(cmds...)

	case TickMsg:
		// Auto-refresh tree and recent sessions
		cmds = append(cmds, fetchTree)
		cmds = append(cmds, fetchRecentSessions)
		// Also refresh preview if we have a selected pane
		if node := m.selectedNode(); node != nil && node.Type == "pane" {
			cmds = append(cmds, fetchPreview(node.Target))
		}
		return m, tea.Batch(cmds...)

	case KillCompletedMsg:
		if msg.Err != nil {
			m.lastError = msg.Err
		} else {
			// Successfully killed, refresh tree and recent sessions
			return m, tea.Batch(fetchTree, fetchRecentSessions)
		}
		return m, nil
	}

	// Update focused component
	switch m.focused {
	case FocusInput:
		var cmd tea.Cmd
		m.commandInput, cmd = m.commandInput.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case FocusPreview:
		var cmd tea.Cmd
		m.previewPort, cmd = m.previewPort.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// handleKeyMsg handles keyboard input
func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if msg.String() != "ctrl+c" {
		m.ctrlCPrimed = false
	}

	// Handle context menu keyboard navigation
	if m.contextMenu != nil && m.contextMenu.Visible {
		switch msg.String() {
		case "up", "k":
			m.contextMenu.MoveSelection(-1)
			return m, nil
		case "down", "j":
			m.contextMenu.MoveSelection(1)
			return m, nil
		case "enter", " ":
			if item := m.contextMenu.SelectedItem(); item != nil && !item.Disabled {
				return m.executeMenuAction(item.Action)
			}
			return m, nil
		case "esc", "q":
			m.contextMenu = nil
			return m, nil
		}
		return m, nil // Ignore other keys while menu is open
	}

	// Handle kill confirmation if active
	if m.confirmKill {
		switch msg.String() {
		case "y", "Y":
			// Confirm kill
			m.confirmKill = false
			return m, killTarget(m.killNodeType, m.killNodeTarget)
		case "n", "N", "esc":
			// Cancel kill
			m.confirmKill = false
			return m, nil
		}
		return m, nil // Ignore other keys while confirmation is shown
	}

	// Close help overlay first if open
	if m.showHelp {
		switch msg.String() {
		case "?", "esc", "q", "enter", " ":
			m.showHelp = false
			return m, nil
		}
		return m, nil // Ignore other keys while help is open
	}

	// Global keys
	switch msg.String() {
	case "?":
		m.showHelp = true
		return m, nil
	case "ctrl+c", "q":
		if msg.String() == "q" && m.focused != FocusInput {
			return m, tea.Quit
		}
		if msg.String() == "ctrl+c" {
			if m.focused == FocusInput {
				if m.commandInput.Value() != "" {
					m.pushInputHistory(m.commandInput.Value())
					m.commandInput.SetValue("")
					m.commandInput.CursorEnd()
					m.lastInputVal = ""
					m.ctrlCPrimed = true
					return m, nil
				}
				if m.ctrlCPrimed {
					return m, tea.Quit
				}
				m.ctrlCPrimed = true
				return m, nil
			}
			if m.ctrlCPrimed {
				return m, tea.Quit
			}
			m.ctrlCPrimed = true
			return m, nil
		}
	case "esc":
		if m.focused == FocusInput {
			if m.commandInput.Value() != "" {
				// First Esc: clear input and save to history
				m.pushInputHistory(m.commandInput.Value())
				m.commandInput.SetValue("")
				m.commandInput.CursorEnd()
				m.lastInputVal = ""
			} else {
				// Second Esc (input already empty): switch to tree panel
				m.focused = FocusTree
				m.commandInput.Blur()
			}
			m.ctrlCPrimed = false
			return m, nil
		}
		return m, tea.Quit
	case "tab":
		m.cycleFocus(1)
		return m, nil
	case "shift+tab":
		m.cycleFocus(-1)
		return m, nil
	case "/":
		// Only focus input if not already focused (so "/" can be typed)
		if m.focused != FocusInput {
			m.focused = FocusInput
			m.commandInput.Focus()
			return m, nil
		}
	case "r":
		if m.focused != FocusInput {
			return m, tea.Batch(fetchTree, fetchRecentSessions)
		}
	case "m":
		// Cycle through send methods (debug mode)
		if m.focused != FocusInput && m.options.DebugMode {
			m.sendMethod = (m.sendMethod + 1) % tmux.SendMethodCount
			return m, nil
		}
	case "M":
		if m.focused != FocusInput {
			m.mouseEnabled = !m.mouseEnabled
			if m.mouseEnabled {
				return m, tea.EnableMouseCellMotion
			}
			return m, tea.DisableMouse
		}
	}

	// Focus-specific keys
	switch m.focused {
	case FocusTree:
		return m.handleTreeKeys(msg)
	case FocusInput:
		return m.handleInputKeys(msg)
	case FocusPreview:
		return m.handlePreviewKeys(msg)
	}

	return m, tea.Batch(cmds...)
}

// handleTreeKeys handles keys when tree is focused
func (m Model) handleTreeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// If focus is on the recent section, handle recent-specific keys
	if m.focusRecent {
		return m.handleRecentKeys(msg)
	}

	switch msg.String() {
	case "up", "k":
		m.moveSelection(-1)
		return m, m.updatePreviewForSelection()
	case "down", "j":
		m.moveSelection(1)
		if m.focusRecent {
			// Moved into recent section, no preview update needed
			return m, nil
		}
		return m, m.updatePreviewForSelection()
	case "enter", " ":
		m.toggleExpand()
		m.calculateButtonZones()
		return m, nil
	case "a":
		// Attach to selected session/window/pane
		if node := m.selectedNode(); node != nil {
			if session := sessionFromNode(node); session != "" {
				m.attachSession = session
				return m, tea.Quit
			}
		}
	case "s":
		// Send command to selected pane
		if node := m.selectedNode(); node != nil && node.Type == "pane" {
			cmd := m.commandInput.Value()
			if cmd != "" {
				m.pushInputHistory(cmd)
				return m, sendCommand(node.Target, cmd, m.sendMethod)
			}
		}
	case "x", "d":
		// Kill selected session/window/pane (with confirmation)
		if node := m.selectedNode(); node != nil {
			m.confirmKill = true
			m.killNodeType = node.Type
			m.killNodeTarget = node.Target
			m.killNodeName = node.Name
			return m, nil
		}
	case "c":
		// Show context menu for selected item (alternative to right-click)
		m.showContextMenuForSelected()
		return m, nil
	}
	return m, nil
}

// handleRecentKeys handles keys when the recent section is focused
func (m Model) handleRecentKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.moveSelection(-1)
		if !m.focusRecent {
			// Moved back into tree section
			return m, m.updatePreviewForSelection()
		}
		return m, nil
	case "down", "j":
		m.moveSelection(1)
		return m, nil
	case "enter":
		// Revive selected recent session (quit with working dir set)
		if entry := m.selectedRecentEntry(); entry != nil {
			m.attachSession = entry.SessionName
			return m, tea.Quit
		}
		return m, nil
	case "x", "d", "delete", "backspace":
		// Delete selected recent entry from history
		if entry := m.selectedRecentEntry(); entry != nil {
			return m, deleteRecentEntry(entry.ID)
		}
		return m, nil
	case "a":
		// Revive (same as enter for recent entries)
		if entry := m.selectedRecentEntry(); entry != nil {
			m.attachSession = entry.SessionName
			return m, tea.Quit
		}
		return m, nil
	}
	return m, nil
}

// handleInputKeys handles keys when input is focused
func (m Model) handleInputKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		if len(m.inputHistory) == 0 {
			return m, nil
		}
		if m.historyIndex == -1 {
			m.historyIndex = len(m.inputHistory)
		}
		if m.historyIndex == len(m.inputHistory) {
			m.historyDraft = m.commandInput.Value()
		}
		if m.historyIndex > 0 {
			m.historyIndex--
		}
		m.commandInput.SetValue(m.inputHistory[m.historyIndex])
		m.commandInput.CursorEnd()
		return m, nil
	case "down":
		if len(m.inputHistory) == 0 || m.historyIndex == -1 {
			return m, nil
		}
		if m.historyIndex < len(m.inputHistory) {
			m.historyIndex++
		}
		if m.historyIndex >= len(m.inputHistory) {
			m.commandInput.SetValue(m.historyDraft)
			m.commandInput.CursorEnd()
			return m, nil
		}
		m.commandInput.SetValue(m.inputHistory[m.historyIndex])
		m.commandInput.CursorEnd()
		return m, nil
	case "enter":
		// Send to selected pane
		if node := m.selectedNode(); node != nil && node.Type == "pane" {
			cmd := m.commandInput.Value()
			if cmd != "" {
				m.pushInputHistory(cmd)
				return m, sendCommand(node.Target, cmd, m.sendMethod)
			}
		}
		return m, nil
	}

	// Pass to text input
	prevValue := m.commandInput.Value()
	var cmd tea.Cmd
	m.commandInput, cmd = m.commandInput.Update(msg)
	newValue := m.commandInput.Value()
	if newValue != "" && !isDeletionKey(msg) {
		m.lastInputVal = newValue
	}
	if prevValue != "" && newValue == "" {
		candidate := prevValue
		if m.lastInputVal != "" {
			candidate = m.lastInputVal
		}
		m.pushInputHistory(candidate)
		m.lastInputVal = ""
	}
	return m, cmd
}

func isDeletionKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyBackspace, tea.KeyDelete:
		return true
	}
	switch msg.String() {
	case "ctrl+w", "ctrl+u", "ctrl+k":
		return true
	}
	return false
}

// handlePreviewKeys handles keys when preview is focused
func (m Model) handlePreviewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.previewPort, cmd = m.previewPort.Update(msg)
	return m, cmd
}

// handleMouseMsg handles mouse input
func (m Model) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Close help on any click
	if m.showHelp && msg.Action == tea.MouseActionPress {
		m.showHelp = false
		return m, nil
	}

	// Handle context menu interactions
	if m.contextMenu != nil && m.contextMenu.Visible {
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// Check if clicking inside the menu
			if idx, item := m.contextMenu.ItemAtPosition(msg.X, msg.Y); item != nil {
				m.contextMenu.Selected = idx
				return m.executeMenuAction(item.Action)
			}
			// Click outside menu - dismiss it
			m.contextMenu = nil
			return m, nil
		}
		// Ignore other mouse events while menu is open
		return m, nil
	}

	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button == tea.MouseButtonLeft {
			if m.isOnDivider(msg.X, msg.Y) {
				m.resizing = true
				m.resizeTreeWidth(msg.X)
				return m, nil
			}
			return m.handleLeftClick(msg.X, msg.Y)
		}
		if msg.Button == tea.MouseButtonRight {
			return m.handleRightClick(msg.X, msg.Y)
		}
	case tea.MouseActionMotion:
		if m.resizing {
			m.resizeTreeWidth(msg.X)
			return m, nil
		}
		// Track hover for button highlighting
		m.hoverIndex = -1
		// Could track hover state here for button highlighting
	case tea.MouseActionRelease:
		if m.resizing {
			m.resizing = false
			return m, nil
		}
	}

	// Pass scroll to appropriate component
	if msg.Action == tea.MouseActionPress {
		switch msg.Button {
		case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
			if m.focused == FocusPreview {
				var cmd tea.Cmd
				m.previewPort, cmd = m.previewPort.Update(msg)
				return m, cmd
			}
		}
	}

	return m, nil
}

// handleLeftClick handles left mouse clicks
func (m Model) handleLeftClick(x, y int) (tea.Model, tea.Cmd) {
	// Check if clicking a button
	if zone, ok := m.findButtonAt(x, y); ok {
		switch zone.action {
		case buttonActionSend:
			cmd := m.commandInput.Value()
			if cmd != "" {
				m.pushInputHistory(cmd)
				return m, sendCommand(zone.target, cmd, m.sendMethod)
			}
			return m, nil
		case buttonActionEscape:
			return m, sendEscape(zone.target)
		case buttonActionAttach:
			// Extract session from target and attach
			session := sessionFromTarget(zone.target)
			if session != "" {
				m.attachSession = session
				return m, tea.Quit
			}
			return m, nil
		case buttonActionHelp:
			m.showHelp = !m.showHelp
			return m, nil
		case buttonActionRefresh:
			return m, fetchTree
		case buttonActionKillHint:
			if node := m.selectedNode(); node != nil {
				m.confirmKill = true
				m.killNodeType = node.Type
				m.killNodeTarget = node.Target
				m.killNodeName = node.Name
			}
			return m, nil
		case buttonActionFocusInput:
			m.focused = FocusInput
			m.commandInput.Focus()
			return m, nil
		}
	}

	// Check regions for focus change
	// Input area is at the top (rows 1-3)
	if y <= inputHeight {
		m.focused = FocusInput
		m.commandInput.Focus()
		return m, nil
	}

	// Tree is on the left
	if x < m.treeWidth+2 {
		m.focused = FocusTree
		m.commandInput.Blur()

		// Calculate which tree item was clicked
		// inputHeight (3) + tree top border (1) + tree content padding (1) = 5
		treeStartY := inputHeight + 2
		clickedIdx := y - treeStartY
		if clickedIdx >= 0 && clickedIdx < len(m.flatNodes) {
			m.focusRecent = false
			node := m.flatNodes[clickedIdx]
			m.selectedIndex = clickedIdx
			if node.Type == "session" || node.Type == "window" {
				indent := node.Level * 2
				icon := getNodeIcon(node.Type, node.Expanded, node.Active)
				iconStartX := indent
				iconEndX := iconStartX + lipgloss.Width(icon)
				if x >= iconStartX && x < iconEndX {
					m.toggleExpand()
					m.calculateButtonZones()
					return m, nil
				}
			}
			// Double-click to attach (works for sessions, windows, and panes)
			if clickedIdx == m.lastClickIdx &&
				time.Since(m.lastClickAt) <= doubleClickThreshold {
				if session := sessionFromNode(node); session != "" {
					m.attachSession = session
					return m, tea.Quit
				}
			}
			m.lastClickIdx = clickedIdx
			m.lastClickAt = time.Now()
			return m, m.updatePreviewForSelection()
		}

		// Check if clicking in the recent section area
		// Recent section starts at: tree nodes + 1 (empty line) + 1 (header)
		if len(m.recentSessions) > 0 {
			recentStartLine := len(m.flatNodes) + 2 // blank line + header
			recentIdx := clickedIdx - recentStartLine
			if recentIdx >= 0 && recentIdx < len(m.recentSessions) {
				m.focusRecent = true
				m.recentSelectedIndex = recentIdx

				// Double-click to revive
				if recentIdx == m.lastClickIdx-10000 && // Use offset to distinguish from tree clicks
					time.Since(m.lastClickAt) <= doubleClickThreshold {
					entry := m.recentSessions[recentIdx]
					m.attachSession = entry.SessionName
					return m, tea.Quit
				}
				m.lastClickIdx = recentIdx + 10000 // Offset to distinguish
				m.lastClickAt = time.Now()
				return m, nil
			}
		}
	} else {
		// Preview is on the right
		m.focused = FocusPreview
		m.commandInput.Blur()
	}

	return m, nil
}

// cycleFocus cycles through focusable components
func (m *Model) cycleFocus(delta int) {
	m.commandInput.Blur()
	m.focusRecent = false // Reset recent focus when cycling panels

	focusOrder := []FocusedComponent{FocusTree, FocusInput, FocusPreview}
	current := 0
	for i, f := range focusOrder {
		if f == m.focused {
			current = i
			break
		}
	}

	current = (current + delta + len(focusOrder)) % len(focusOrder)
	m.focused = focusOrder[current]

	if m.focused == FocusInput {
		m.commandInput.Focus()
	}
}

// updatePreviewForSelection fetches preview if a pane is selected
func (m *Model) updatePreviewForSelection() tea.Cmd {
	if node := m.selectedNode(); node != nil && node.Type == "pane" {
		m.previewTarget = node.Target
		return fetchPreview(node.Target)
	}
	return nil
}

func (m *Model) pushInputHistory(value string) {
	entry := value
	if entry == "" {
		return
	}
	if len(m.inputHistory) > 0 && m.inputHistory[len(m.inputHistory)-1] == entry {
		m.historyIndex = -1
		m.historyDraft = ""
		return
	}
	m.inputHistory = append(m.inputHistory, entry)
	m.historyIndex = -1
	m.historyDraft = ""
}

func sessionFromNode(node *tmux.TreeNode) string {
	if node == nil {
		return ""
	}
	if node.Type == "session" {
		if node.Target != "" {
			return node.Target
		}
		return node.Name
	}
	if idx := strings.Index(node.Target, ":"); idx != -1 {
		return node.Target[:idx]
	}
	return node.Target
}

func sessionFromTarget(target string) string {
	if target == "" {
		return ""
	}
	if idx := strings.Index(target, ":"); idx != -1 {
		return target[:idx]
	}
	return target
}

func (m *Model) isOnDivider(x, y int) bool {
	if y <= inputHeight || y >= m.height-statusHeight {
		return false
	}
	dividerX := m.treeWidth - 1
	return x >= dividerX-1 && x <= dividerX+1
}

func (m *Model) resizeTreeWidth(x int) {
	availableWidth := m.width - 4
	maxTreeWidth := availableWidth - minPreviewWidth
	if maxTreeWidth < minTreeWidth {
		return
	}

	newTreeWidth := x + 1
	if newTreeWidth < minTreeWidth {
		newTreeWidth = minTreeWidth
	}
	if newTreeWidth > maxTreeWidth {
		newTreeWidth = maxTreeWidth
	}
	if newTreeWidth == m.treeWidth {
		return
	}

	m.treeWidth = newTreeWidth
	m.previewWidth = availableWidth - m.treeWidth
	previewHeight := m.height - inputHeight - statusHeight - 4
	if previewHeight < 5 {
		previewHeight = 5
	}
	m.previewPort.Width = m.previewWidth - 2
	m.previewPort.Height = previewHeight
}

// handleRightClick handles right mouse clicks to show context menus
func (m Model) handleRightClick(x, y int) (tea.Model, tea.Cmd) {
	// Only show context menu when clicking in tree area
	if x >= m.treeWidth+2 {
		return m, nil
	}

	// Calculate which tree item was clicked
	treeStartY := inputHeight + 2
	clickedIdx := y - treeStartY
	if clickedIdx < 0 || clickedIdx >= len(m.flatNodes) {
		return m, nil
	}

	node := m.flatNodes[clickedIdx]
	m.selectedIndex = clickedIdx

	// Create context menu at click position
	// Adjust position to stay within screen bounds
	menuX := x
	menuY := y

	menu := NewContextMenu(node.Type, node.Target, node.Name, menuX, menuY)

	// Adjust menu position to stay within screen bounds
	menuWidth := menu.Width + 4
	menuHeight := menu.Height()

	if menuX+menuWidth > m.width {
		menuX = m.width - menuWidth - 1
	}
	if menuX < 0 {
		menuX = 0
	}
	if menuY+menuHeight > m.height {
		menuY = m.height - menuHeight - 1
	}
	if menuY < 0 {
		menuY = 0
	}

	menu.Position.X = menuX
	menu.Position.Y = menuY

	m.contextMenu = menu
	return m, nil
}

// showContextMenuForSelected shows a context menu for the currently selected node
func (m *Model) showContextMenuForSelected() {
	node := m.selectedNode()
	if node == nil {
		return
	}

	// Position menu near the selected item in the tree
	treeStartY := inputHeight + 2
	menuY := treeStartY + m.selectedIndex
	menuX := node.Level*2 + 5 // Indent based on level

	menu := NewContextMenu(node.Type, node.Target, node.Name, menuX, menuY)

	// Adjust menu position to stay within screen bounds
	menuWidth := menu.Width + 4
	menuHeight := menu.Height()

	if menuX+menuWidth > m.width {
		menuX = m.width - menuWidth - 1
	}
	if menuX < 0 {
		menuX = 0
	}
	if menuY+menuHeight > m.height {
		menuY = m.height - menuHeight - 1
	}
	if menuY < 0 {
		menuY = 0
	}

	menu.Position.X = menuX
	menu.Position.Y = menuY

	m.contextMenu = menu
}

// executeMenuAction executes the action for a menu item
func (m Model) executeMenuAction(action string) (tea.Model, tea.Cmd) {
	if m.contextMenu == nil {
		return m, nil
	}

	target := m.contextMenu.Target
	nodeType := m.contextMenu.NodeType

	// Close the menu
	m.contextMenu = nil

	switch action {
	case MenuActionAttach:
		// Attach to session
		session := sessionFromTarget(target)
		if session != "" {
			m.attachSession = session
			return m, tea.Quit
		}

	case MenuActionAttachPopup:
		// Attach in popup mode - for now just attach normally
		session := sessionFromTarget(target)
		if session != "" {
			m.attachSession = session
			return m, tea.Quit
		}

	case MenuActionNewWindow:
		// Create new window in session
		return m, createNewWindow(target)

	case MenuActionRename:
		// TODO: Implement rename dialog
		// For now, just show a message
		return m, nil

	case MenuActionKillSession, MenuActionKillWindow, MenuActionKillPane:
		// Show kill confirmation
		node := m.selectedNode()
		if node != nil {
			m.confirmKill = true
			m.killNodeType = nodeType
			m.killNodeTarget = target
			m.killNodeName = node.Name
		}
		return m, nil

	case MenuActionSelectWindow:
		// Switch to window
		return m, switchToTarget(target)

	case MenuActionNewPaneH:
		// Create horizontal split
		return m, createNewPane(target, false)

	case MenuActionNewPaneV:
		// Create vertical split
		return m, createNewPane(target, true)

	case MenuActionSelectPane:
		// Switch to pane
		return m, switchToTarget(target)

	case MenuActionZoomPane:
		// Toggle zoom on pane
		return m, toggleZoomPane(target)

	case MenuActionSendKeys:
		// Focus the input and set target
		m.focused = FocusInput
		m.commandInput.Focus()
		return m, nil
	}

	return m, nil
}

// createNewWindow creates a new window in the specified session
func createNewWindow(sessionTarget string) tea.Cmd {
	return func() tea.Msg {
		err := tmux.CreateNewWindow(sessionTarget)
		return TreeRefreshedMsg{Err: err}
	}
}

// createNewPane creates a new pane in the specified window
func createNewPane(windowTarget string, vertical bool) tea.Cmd {
	return func() tea.Msg {
		err := tmux.CreateNewPane(windowTarget, vertical)
		return TreeRefreshedMsg{Err: err}
	}
}

// switchToTarget switches the client to the specified target
func switchToTarget(target string) tea.Cmd {
	return func() tea.Msg {
		err := tmux.SwitchToTarget(target)
		return CommandSentMsg{Target: target, Command: "switch", Err: err}
	}
}

// toggleZoomPane toggles zoom on the specified pane
func toggleZoomPane(target string) tea.Cmd {
	return func() tea.Msg {
		err := tmux.ToggleZoom(target)
		return CommandSentMsg{Target: target, Command: "zoom", Err: err}
	}
}

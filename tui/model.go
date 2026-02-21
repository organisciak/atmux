package tui

import (
	"os"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/porganisciak/agent-tmux/history"
	"github.com/porganisciak/agent-tmux/tmux"
)

// FocusedComponent tracks which component has focus
type FocusedComponent int

const (
	FocusTree FocusedComponent = iota
	FocusInput
	FocusPreview
)

const (
	buttonActionSend       = "send"
	buttonActionEscape     = "escape"
	buttonActionAttach     = "attach"
	buttonActionHelp       = "help"
	buttonActionRefresh    = "refresh"
	buttonActionKillHint   = "killhint"
	buttonActionFocusInput = "focusinput"
)

const doubleClickThreshold = 400 * time.Millisecond

// Options for initializing the TUI
type Options struct {
	RefreshInterval time.Duration
	PopupMode       bool
	DebugMode       bool
	MobileMode      bool // Force mobile layout (auto-detected if width < 60)
	Executors       []tmux.TmuxExecutor // Executors for multi-host browsing (nil = local only)
}

// Model is the main TUI state
type Model struct {
	// Data
	tree      *tmux.Tree
	flatNodes []*tmux.TreeNode

	// Selection
	selectedIndex int
	hoverIndex    int // For mouse hover

	// Components
	commandInput textinput.Model
	previewPort  viewport.Model

	// State
	focused        FocusedComponent
	command        string
	previewContent string
	previewTarget  string

	// Dimensions
	width        int
	height       int
	treeWidth    int
	previewWidth int

	// Options
	options Options

	// Multi-host support
	executors  []tmux.TmuxExecutor // Executors (nil = local-only)
	hostTrees  []tmux.HostTree     // Per-host tree data (used for routing)
	hostErrors map[string]error    // Per-host errors from last fetch

	// Status
	lastError     error
	lastSent      string // Last command sent (for status display)
	ctrlCPrimed   bool   // Tracks double Ctrl-C to exit
	attachSession string
	reviveDir     string // Working directory for reviving a recent session

	// Debug mode
	sendMethod tmux.SendMethod

	// Mouse tracking
	buttonZones  []buttonZone // Clickable button zones
	lastClickAt  time.Time
	lastClickIdx int
	resizing     bool
	mouseEnabled bool

	// Input history
	inputHistory []string
	historyIndex int
	historyDraft string
	lastInputVal string

	// Tree expansion state
	expanded map[string]bool

	// Help overlay
	showHelp bool

	// Kill confirmation state
	confirmKill    bool   // Whether we're showing kill confirmation
	killNodeType   string // Type of node being killed (session/window/pane)
	killNodeTarget string // Target of node being killed
	killNodeName   string // Name of node being killed (for display)
	killNodeHost   string // Host of node being killed (for executor routing)

	// Context menu state
	contextMenu *ContextMenu // Active context menu, nil if not showing

	// Mobile mode
	mobileMode       bool // True when using mobile-optimized layout
	mobileForcedMode bool // True when --mobile flag was passed (prevents auto-switching)

	// Recent sessions (history entries not currently active)
	recentSessions      []history.Entry
	recentSelectedIndex int  // Selection index within recent section
	focusRecent         bool // Whether focus is on recent section vs tree
}

// buttonZone tracks a clickable button area
type buttonZone struct {
	x, y, width, height int
	target              string
	action              string
}

// NewModel creates a new TUI model
func NewModel(opts Options) Model {
	ti := textinput.New()
	ti.Placeholder = "Enter command to send..."
	ti.CharLimit = 256
	ti.Width = 50

	vp := viewport.New(40, 20)
	mouseEnabled := os.Getenv("TMUX") == ""

	return Model{
		commandInput:     ti,
		previewPort:      vp,
		focused:          FocusTree,
		options:          opts,
		executors:        opts.Executors,
		flatNodes:        []*tmux.TreeNode{},
		historyIndex:     -1,
		sendMethod:       tmux.SendMethodEnterDelayed, // 500ms delay works for both Claude and Codex
		lastClickIdx:     -1,
		mouseEnabled:     mouseEnabled,
		expanded:         map[string]bool{},
		mobileMode:       opts.MobileMode,
		mobileForcedMode: opts.MobileMode,
		hostErrors:       map[string]error{},
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchTreeCmd(),
		fetchRecentSessions,
		tea.SetWindowTitle("atmux browse"),
	)
}

// fetchTreeCmd returns a command that fetches the tree, using executors if available.
func (m *Model) fetchTreeCmd() tea.Cmd {
	if len(m.executors) > 0 {
		execs := m.executors
		return func() tea.Msg {
			hostTrees := tmux.FetchTreeWithExecutors(execs)
			return MultiTreeRefreshedMsg{HostTrees: hostTrees}
		}
	}
	return fetchTree
}

// fetchTree fetches the tmux tree structure (local only)
func fetchTree() tea.Msg {
	tree, err := tmux.FetchTree()
	return TreeRefreshedMsg{Tree: tree, Err: err}
}

// fetchRecentSessions loads history entries for the recent section
func fetchRecentSessions() tea.Msg {
	store, err := history.Open()
	if err != nil {
		return RecentSessionsMsg{Err: err}
	}
	defer store.Close()
	entries, err := store.LoadHistory()
	return RecentSessionsMsg{Entries: entries, Err: err}
}

// filterRecentSessions removes history entries that match active sessions.
func (m *Model) filterRecentSessions() {
	if m.tree == nil || m.recentSessions == nil {
		return
	}
	activeNames := make(map[string]bool)
	for _, sess := range m.tree.Sessions {
		activeNames[sess.Name] = true
	}
	var filtered []history.Entry
	for _, e := range m.recentSessions {
		if !activeNames[e.SessionName] {
			filtered = append(filtered, e)
		}
	}
	m.recentSessions = filtered
}

// deleteRecentEntry deletes a history entry by ID
func deleteRecentEntry(id int64) tea.Cmd {
	return func() tea.Msg {
		store, err := history.Open()
		if err != nil {
			return RecentDeletedMsg{ID: id, Err: err}
		}
		defer store.Close()
		return RecentDeletedMsg{ID: id, Err: store.DeleteEntry(id)}
	}
}

// selectedRecentEntry returns the currently selected recent entry, if any.
func (m *Model) selectedRecentEntry() *history.Entry {
	if !m.focusRecent || m.recentSelectedIndex < 0 || m.recentSelectedIndex >= len(m.recentSessions) {
		return nil
	}
	e := m.recentSessions[m.recentSelectedIndex]
	return &e
}

// maxVisibleRecentEntries returns how many recent entries can be shown
// given the current tree height and number of tree nodes.
func (m *Model) maxVisibleRecentEntries() int {
	if len(m.recentSessions) == 0 {
		return 0
	}
	treeHeight := m.height - inputHeight - statusHeight - 4
	if treeHeight < 1 {
		treeHeight = 1
	}
	// Tree nodes take up space, plus 2 lines for separator + header
	nodeCount := len(m.flatNodes)
	if nodeCount > treeHeight {
		nodeCount = treeHeight
	}
	remaining := treeHeight - nodeCount - 2 // -2 for blank line + header
	if remaining < 0 {
		return 0
	}
	if remaining > len(m.recentSessions) {
		return len(m.recentSessions)
	}
	return remaining
}

// fetchPreview fetches pane content
func fetchPreview(target string) tea.Cmd {
	return func() tea.Msg {
		content, err := tmux.CapturePane(target)
		return PreviewUpdatedMsg{Content: content, Target: target, Err: err}
	}
}

// fetchPreviewWithExecutor fetches pane content via a specific executor.
func fetchPreviewWithExecutor(target string, exec tmux.TmuxExecutor) tea.Cmd {
	return func() tea.Msg {
		content, err := tmux.CapturePaneWithExecutor(target, exec)
		return PreviewUpdatedMsg{Content: content, Target: target, Err: err}
	}
}

// sendCommand sends a command to a pane using a specific method
func sendCommand(target, command string, method tmux.SendMethod) tea.Cmd {
	return func() tea.Msg {
		err := tmux.SendCommandWithMethod(target, command, method)
		return CommandSentMsg{Target: target, Command: command, Err: err}
	}
}

// sendCommandWithExecutor sends a command via a specific executor.
func sendCommandWithExecutor(target, command string, method tmux.SendMethod, exec tmux.TmuxExecutor) tea.Cmd {
	return func() tea.Msg {
		err := tmux.SendCommandWithMethodAndExecutor(target, command, method, exec)
		return CommandSentMsg{Target: target, Command: command, Err: err}
	}
}

// sendEscape sends an escape key to a pane.
func sendEscape(target string) tea.Cmd {
	return func() tea.Msg {
		err := tmux.SendEscape(target)
		return CommandSentMsg{Target: target, Command: "Escape", Err: err}
	}
}

// sendEscapeWithExecutor sends an escape key via a specific executor.
func sendEscapeWithExecutor(target string, exec tmux.TmuxExecutor) tea.Cmd {
	return func() tea.Msg {
		err := tmux.SendEscapeWithExecutor(target, exec)
		return CommandSentMsg{Target: target, Command: "Escape", Err: err}
	}
}

// killTarget kills a session, window, or pane.
func killTarget(nodeType, target string) tea.Cmd {
	return func() tea.Msg {
		err := tmux.KillTarget(nodeType, target)
		return KillCompletedMsg{NodeType: nodeType, Target: target, Err: err}
	}
}

// killTargetWithExecutor kills a session, window, or pane via a specific executor.
func killTargetWithExecutor(nodeType, target string, exec tmux.TmuxExecutor) tea.Cmd {
	return func() tea.Msg {
		err := tmux.KillTargetWithExecutor(nodeType, target, exec)
		return KillCompletedMsg{NodeType: nodeType, Target: target, Err: err}
	}
}

// tickCmd creates a tick for auto-refresh
func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

// selectedNode returns the currently selected node
func (m *Model) selectedNode() *tmux.TreeNode {
	if m.selectedIndex >= 0 && m.selectedIndex < len(m.flatNodes) {
		return m.flatNodes[m.selectedIndex]
	}
	return nil
}

// nodeForTarget returns the first node matching the given target.
func (m *Model) nodeForTarget(target string) *tmux.TreeNode {
	for _, node := range m.flatNodes {
		if node.Target == target {
			return node
		}
	}
	return nil
}

// rebuildFlatNodes rebuilds the flat node list from the tree
func (m *Model) rebuildFlatNodes() {
	if m.tree == nil {
		m.flatNodes = []*tmux.TreeNode{}
		return
	}
	m.flatNodes = m.buildFlatNodes()
}

// toggleExpand toggles expansion of the selected node
func (m *Model) toggleExpand() {
	node := m.selectedNode()
	if node == nil {
		return
	}
	if node.Type == "session" || node.Type == "window" || node.Type == "host" {
		key := m.expandKey(node)
		expanded := node.Expanded
		if val, ok := m.expanded[key]; ok {
			expanded = val
		}
		m.expanded[key] = !expanded
		m.rebuildFlatNodes()
	}
}

// expandKey returns the expansion key for a node, including host prefix for multi-host mode.
func (m *Model) expandKey(node *tmux.TreeNode) string {
	if node.Type == "host" {
		return nodeKey("host", node.Target)
	}
	if len(m.hostTrees) > 0 {
		hostLabel := node.Host
		if hostLabel == "" {
			hostLabel = "local"
		}
		return nodeKey(node.Type, hostLabel+"/"+node.Target)
	}
	return nodeKey(node.Type, node.Target)
}

// moveSelection moves selection up or down, transitioning between tree and recent sections.
func (m *Model) moveSelection(delta int) {
	maxVisible := m.maxVisibleRecentEntries()

	if m.focusRecent {
		// Currently in recent section
		m.recentSelectedIndex += delta
		if m.recentSelectedIndex < 0 {
			// Move back up into tree
			m.focusRecent = false
			m.recentSelectedIndex = 0
			m.selectedIndex = len(m.flatNodes) - 1
			if m.selectedIndex < 0 {
				m.selectedIndex = 0
			}
			return
		}
		if m.recentSelectedIndex >= maxVisible {
			m.recentSelectedIndex = maxVisible - 1
		}
		if m.recentSelectedIndex < 0 {
			m.recentSelectedIndex = 0
		}
		return
	}

	// Currently in tree section
	newIndex := m.selectedIndex + delta
	if newIndex < 0 {
		newIndex = 0
	}
	if newIndex >= len(m.flatNodes) {
		// Check if we can move into recent section (only if visible)
		if maxVisible > 0 && delta > 0 {
			m.focusRecent = true
			m.recentSelectedIndex = 0
			return
		}
		newIndex = len(m.flatNodes) - 1
	}
	if newIndex < 0 {
		newIndex = 0
	}
	m.selectedIndex = newIndex
}

// calculateLayout calculates panel widths based on terminal size
func (m *Model) calculateLayout() {
	// Account for borders
	availableWidth := m.width - 4
	m.treeWidth = (availableWidth * treeWidthPercent) / 100
	m.previewWidth = availableWidth - m.treeWidth

	if m.treeWidth < minTreeWidth {
		m.treeWidth = minTreeWidth
	}
	if m.previewWidth < minPreviewWidth {
		m.previewWidth = minPreviewWidth
	}

	// Update viewport dimensions
	previewHeight := m.height - inputHeight - statusHeight - 4
	if previewHeight < 5 {
		previewHeight = 5
	}
	m.previewPort.Width = m.previewWidth - 2
	m.previewPort.Height = previewHeight
}

// findButtonAt returns the button at the given coordinates, if any
func (m *Model) findButtonAt(x, y int) (buttonZone, bool) {
	for i := range m.buttonZones {
		zone := m.buttonZones[i]
		if x >= zone.x && x < zone.x+zone.width &&
			y >= zone.y && y < zone.y+zone.height {
			return zone, true
		}
	}
	return buttonZone{}, false
}

// calculateButtonZones pre-calculates clickable button zones based on current layout.
// This must be called after layout changes or tree data changes to keep zones in sync.
func (m *Model) calculateButtonZones() {
	m.buttonZones = nil

	if m.width == 0 || m.height == 0 || m.treeWidth == 0 {
		return
	}

	// Help button in top-right
	helpBtnWidth := 3 // "[?]" rendered width with padding
	m.buttonZones = append(m.buttonZones, buttonZone{
		x:      m.width - helpBtnWidth - 4,
		y:      1,
		width:  helpBtnWidth,
		height: 1,
		action: buttonActionHelp,
	})

	// Tree node buttons
	treeHeight := m.height - inputHeight - statusHeight - 4
	if treeHeight < 1 {
		treeHeight = 1
	}

	// inputHeight (3) + tree top border (1) + tree content padding (1) = 5
	buttonYOffset := inputHeight + 2
	buttonGap := 1

	// Button widths (text + padding(0,1) on each side)
	sendWidth := 6 // " SEND "
	escWidth := 5  // " ESC "
	attWidth := 5  // " ATT "

	for i, node := range m.flatNodes {
		if i >= treeHeight {
			break
		}

		nodeY := buttonYOffset + i

		if node.Type == "pane" {
			// Panes get SEND, ESC, and ATT buttons
			buttonsWidth := sendWidth + buttonGap + escWidth + buttonGap + attWidth
			buttonStartX := m.treeWidth - buttonsWidth

			m.buttonZones = append(m.buttonZones, buttonZone{
				x:      buttonStartX,
				y:      nodeY,
				width:  sendWidth,
				height: 1,
				target: node.Target,
				action: buttonActionSend,
			})

			escStartX := buttonStartX + sendWidth + buttonGap
			m.buttonZones = append(m.buttonZones, buttonZone{
				x:      escStartX,
				y:      nodeY,
				width:  escWidth,
				height: 1,
				target: node.Target,
				action: buttonActionEscape,
			})

			attStartX := escStartX + escWidth + buttonGap
			m.buttonZones = append(m.buttonZones, buttonZone{
				x:      attStartX,
				y:      nodeY,
				width:  attWidth,
				height: 1,
				target: node.Target,
				action: buttonActionAttach,
			})
		} else {
			// Sessions and windows get only ATT button
			buttonStartX := m.treeWidth - attWidth
			m.buttonZones = append(m.buttonZones, buttonZone{
				x:      buttonStartX,
				y:      nodeY,
				width:  attWidth,
				height: 1,
				target: node.Target,
				action: buttonActionAttach,
			})
		}
	}

	// Status bar hint zones (only shown when not in input mode)
	if m.focused != FocusInput {
		// Status bar Y: inputHeight + mainContent (treeHeight + 2 borders)
		statusY := inputHeight + treeHeight + 2

		// Status bar has Padding(0,1), so content starts at x=1
		// Hints: [r]efresh [a]ttach [x]kill [/]input [?]help
		// Each hint is rendered individually with spacing between them
		type hintDef struct {
			text   string
			action string
		}
		hints := []hintDef{
			{"[r]efresh", buttonActionRefresh},
			{"[a]ttach", buttonActionAttach},
			{"[x]kill", buttonActionKillHint},
			{"[/]input", buttonActionFocusInput},
			{"[?]help", buttonActionHelp},
		}

		xOffset := 1 // statusBarStyle has Padding(0,1)
		for i, h := range hints {
			if i > 0 {
				xOffset++ // space between hints
			}
			w := len(h.text)
			m.buttonZones = append(m.buttonZones, buttonZone{
				x:      xOffset,
				y:      statusY,
				width:  w,
				height: 1,
				action: h.action,
			})
			xOffset += w
		}
	}
}

func nodeKey(nodeType, target string) string {
	return nodeType + ":" + target
}

func (m *Model) isExpanded(nodeType, target string, defaultValue bool) bool {
	if val, ok := m.expanded[nodeKey(nodeType, target)]; ok {
		return val
	}
	return defaultValue
}

func (m *Model) buildFlatNodes() []*tmux.TreeNode {
	// Multi-host mode: build from hostTrees with host grouping
	if len(m.hostTrees) > 0 {
		return m.buildMultiHostFlatNodes()
	}

	// Single-host (local) mode: build from m.tree
	var nodes []*tmux.TreeNode
	for _, sess := range m.tree.Sessions {
		sessExpanded := m.isExpanded("session", sess.Name, true)
		sessNode := &tmux.TreeNode{
			Type:     "session",
			Name:     sess.Name,
			Target:   sess.Name,
			Expanded: sessExpanded,
			Level:    0,
			Attached: sess.Attached,
		}
		nodes = append(nodes, sessNode)

		if sessExpanded {
			for _, win := range sess.Windows {
				winTarget := sess.Name + ":" + strconv.Itoa(win.Index)
				winExpanded := m.isExpanded("window", winTarget, true)
				winNode := &tmux.TreeNode{
					Type:     "window",
					Name:     win.Name,
					Target:   winTarget,
					Expanded: winExpanded,
					Level:    1,
					Active:   win.Active,
				}
				sessNode.Children = append(sessNode.Children, winNode)
				nodes = append(nodes, winNode)

				if winExpanded {
					for _, pane := range win.Panes {
						paneNode := &tmux.TreeNode{
							Type:   "pane",
							Name:   pane.Title,
							Target: pane.Target,
							Level:  2,
							Active: pane.Active,
						}
						if paneNode.Name == "" {
							paneNode.Name = pane.Command
						}
						if paneNode.Name == "" {
							paneNode.Name = "pane " + strconv.Itoa(pane.Index)
						}
						winNode.Children = append(winNode.Children, paneNode)
						nodes = append(nodes, paneNode)
					}
				}
			}
		}
	}

	return nodes
}

// buildMultiHostFlatNodes builds flat nodes from multiple host trees with host headers.
func (m *Model) buildMultiHostFlatNodes() []*tmux.TreeNode {
	var nodes []*tmux.TreeNode

	for _, ht := range m.hostTrees {
		hostLabel := ht.Host
		if hostLabel == "" {
			hostLabel = "local"
		}

		hostKey := "host:" + hostLabel
		hostExpanded := m.isExpanded("host", hostKey, true)

		hostNode := &tmux.TreeNode{
			Type:     "host",
			Name:     hostLabel,
			Target:   hostKey,
			Expanded: hostExpanded,
			Level:    0,
			Host:     ht.Host,
		}
		nodes = append(nodes, hostNode)

		if ht.Err != nil {
			// Show error node for unreachable hosts
			if hostExpanded {
				errNode := &tmux.TreeNode{
					Type:  "pane", // Use pane type for leaf rendering
					Name:  "unreachable: " + ht.Err.Error(),
					Level: 1,
					Host:  ht.Host,
				}
				nodes = append(nodes, errNode)
			}
			continue
		}

		if ht.Tree == nil || !hostExpanded {
			continue
		}

		for _, sess := range ht.Tree.Sessions {
			sessExpanded := m.isExpanded("session", hostLabel+"/"+sess.Name, true)
			sessNode := &tmux.TreeNode{
				Type:     "session",
				Name:     sess.Name,
				Target:   sess.Name,
				Expanded: sessExpanded,
				Level:    1,
				Attached: sess.Attached,
				Host:     ht.Host,
			}
			nodes = append(nodes, sessNode)

			if sessExpanded {
				for _, win := range sess.Windows {
					winTarget := sess.Name + ":" + strconv.Itoa(win.Index)
					winExpanded := m.isExpanded("window", hostLabel+"/"+winTarget, true)
					winNode := &tmux.TreeNode{
						Type:     "window",
						Name:     win.Name,
						Target:   winTarget,
						Expanded: winExpanded,
						Level:    2,
						Active:   win.Active,
						Host:     ht.Host,
					}
					sessNode.Children = append(sessNode.Children, winNode)
					nodes = append(nodes, winNode)

					if winExpanded {
						for _, pane := range win.Panes {
							paneNode := &tmux.TreeNode{
								Type:   "pane",
								Name:   pane.Title,
								Target: pane.Target,
								Level:  3,
								Active: pane.Active,
								Host:   ht.Host,
							}
							if paneNode.Name == "" {
								paneNode.Name = pane.Command
							}
							if paneNode.Name == "" {
								paneNode.Name = "pane " + strconv.Itoa(pane.Index)
							}
							winNode.Children = append(winNode.Children, paneNode)
							nodes = append(nodes, paneNode)
						}
					}
				}
			}
		}
	}

	return nodes
}

// executorForHost returns the executor for the given host label.
// Returns nil if no matching executor is found.
func (m *Model) executorForHost(host string) tmux.TmuxExecutor {
	for _, ht := range m.hostTrees {
		if ht.Host == host && ht.Executor != nil {
			return ht.Executor
		}
	}
	return nil
}

// fetchPreviewForNode returns the appropriate preview command for a node,
// routing through the correct executor for remote nodes.
func (m *Model) fetchPreviewForNode(node *tmux.TreeNode) tea.Cmd {
	if node == nil || node.Type != "pane" {
		return nil
	}
	if node.Host != "" {
		if exec := m.executorForHost(node.Host); exec != nil {
			return fetchPreviewWithExecutor(node.Target, exec)
		}
	}
	return fetchPreview(node.Target)
}

// sendCommandForNode sends a command to the correct executor for a node.
func (m *Model) sendCommandForNode(node *tmux.TreeNode, command string) tea.Cmd {
	if node == nil || node.Type != "pane" {
		return nil
	}
	if node.Host != "" {
		if exec := m.executorForHost(node.Host); exec != nil {
			return sendCommandWithExecutor(node.Target, command, m.sendMethod, exec)
		}
	}
	return sendCommand(node.Target, command, m.sendMethod)
}

// sendEscapeForNode sends escape to the correct executor for a node.
func (m *Model) sendEscapeForNode(node *tmux.TreeNode) tea.Cmd {
	if node == nil || node.Type != "pane" {
		return nil
	}
	if node.Host != "" {
		if exec := m.executorForHost(node.Host); exec != nil {
			return sendEscapeWithExecutor(node.Target, exec)
		}
	}
	return sendEscape(node.Target)
}

// killTargetForNode kills a target via the correct executor.
func (m *Model) killTargetForNode(nodeType, target, host string) tea.Cmd {
	if host != "" {
		if exec := m.executorForHost(host); exec != nil {
			return killTargetWithExecutor(nodeType, target, exec)
		}
	}
	return killTarget(nodeType, target)
}

// Run starts the TUI
func Run(opts Options) error {
	m := NewModel(opts)
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(), // Enable mouse support
	)
	finalModel, err := p.Run()
	if err != nil {
		return err
	}
	model, ok := finalModel.(Model)
	if !ok || model.attachSession == "" {
		return nil
	}

	if model.reviveDir != "" {
		session := tmux.NewSession(model.reviveDir)
		if !session.Exists() {
			if err := session.Create(nil); err != nil {
				return err
			}
			session.SelectDefault()
		}
		return tmux.AttachToSession(session.Name)
	}

	return tmux.AttachToSession(model.attachSession)
}

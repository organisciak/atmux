package tui

import (
	"os"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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
	buttonActionSend   = "send"
	buttonActionEscape = "escape"
	buttonActionAttach = "attach"
	buttonActionHelp   = "help"
)

const doubleClickThreshold = 400 * time.Millisecond

// Options for initializing the TUI
type Options struct {
	RefreshInterval time.Duration
	PopupMode       bool
	DebugMode       bool
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

	// Status
	lastError     error
	lastSent      string // Last command sent (for status display)
	ctrlCPrimed   bool   // Tracks double Ctrl-C to exit
	attachSession string

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
		commandInput: ti,
		previewPort:  vp,
		focused:      FocusTree,
		options:      opts,
		flatNodes:    []*tmux.TreeNode{},
		historyIndex: -1,
		sendMethod:   tmux.SendMethodEnterDelayed, // 500ms delay works for both Claude and Codex
		lastClickIdx: -1,
		mouseEnabled: mouseEnabled,
		expanded:     map[string]bool{},
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchTree,
		tea.SetWindowTitle("agent-tmux browse"),
	)
}

// fetchTree fetches the tmux tree structure
func fetchTree() tea.Msg {
	tree, err := tmux.FetchTree()
	return TreeRefreshedMsg{Tree: tree, Err: err}
}

// fetchPreview fetches pane content
func fetchPreview(target string) tea.Cmd {
	return func() tea.Msg {
		content, err := tmux.CapturePane(target)
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

// sendEscape sends an escape key to a pane.
func sendEscape(target string) tea.Cmd {
	return func() tea.Msg {
		err := tmux.SendEscape(target)
		return CommandSentMsg{Target: target, Command: "Escape", Err: err}
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
	if node.Type == "session" || node.Type == "window" {
		key := nodeKey(node.Type, node.Target)
		expanded := node.Expanded
		if val, ok := m.expanded[key]; ok {
			expanded = val
		}
		m.expanded[key] = !expanded
		m.rebuildFlatNodes()
	}
}

// moveSelection moves selection up or down
func (m *Model) moveSelection(delta int) {
	m.selectedIndex += delta
	if m.selectedIndex < 0 {
		m.selectedIndex = 0
	}
	if m.selectedIndex >= len(m.flatNodes) {
		m.selectedIndex = len(m.flatNodes) - 1
	}
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
	if model, ok := finalModel.(Model); ok && model.attachSession != "" {
		return tmux.AttachToSession(model.attachSession)
	}
	return nil
}

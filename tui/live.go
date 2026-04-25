package tui

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/history"
	"github.com/porganisciak/agent-tmux/tmux"
)

// LiveOptions configures the live browser TUI.
type LiveOptions struct {
	RightPaneID     string        // Tmux pane ID for the display slot
	OriginalSession string        // Session to switch back to on exit
	RefreshInterval time.Duration // How often to refresh the tree
}

// LiveModel is the Bubbletea model for the live browser tree.
type LiveModel struct {
	tree      *tmux.Tree
	flatNodes []*tmux.TreeNode

	selectedIdx int
	expanded    map[string]bool

	width, height int

	opts            LiveOptions
	displayedPaneID string // Pane ID currently swapped into the right slot
	lastSwapTarget  string // Target string of the currently displayed node

	statusMsg string
	lastError error

	// Search state
	searchActive bool             // Whether the search box is visible
	searchQuery  string           // Current search text
	searchManual bool             // True if opened with '/' (allows numbers in query)
	allFlatNodes []*tmux.TreeNode // Unfiltered nodes (flatNodes may be filtered)

	// Recent (closed) projects shown below active sessions, mirroring
	// `atmux sessions`. Loaded async; filtered to exclude active sessions.
	allRecents     []history.Entry // Raw history (post active-session filter)
	visibleRecents []history.Entry // After search filter
	recentsHidden  bool            // Toggle with 'h'

	// Attach-on-quit: set to switch to a session after cleanup
	attachSession    string
	attachWorkingDir string // Set when reviving from a recent entry
	attachIsRecent   bool   // True when attach target needs creation from history

	// Quick-link URLs surfaced for the selected project (loaded from
	// .agent-tmux.conf in the session's working directory). Cached by
	// directory so repeated navigation doesn't re-parse configs.
	currentURLs []config.URLConfig
	urlsCache   map[string][]config.URLConfig

	currentBeads liveBeadsSummary
	beadsPanel   bool // Toggle with 'b'
}

// urlClickBounds records the rendered position of a URL button so the
// mouse handler can map a click X to the right URL index. Computed by
// urlButtonBounds in live_view.go from the same layout as the renderer.
type urlClickBounds struct {
	startX, endX int
	idx          int
}

// NewLiveModel creates a new live browser TUI model.
func NewLiveModel(opts LiveOptions) LiveModel {
	if opts.RefreshInterval == 0 {
		opts.RefreshInterval = 2 * time.Second
	}
	return LiveModel{
		opts:     opts,
		expanded: map[string]bool{},
	}
}

// Init initializes the live model.
func (m LiveModel) Init() tea.Cmd {
	return tea.Batch(
		fetchTree,
		fetchRecents,
		tea.SetWindowTitle("atmux live"),
	)
}

// fetchRecents loads recent history entries asynchronously.
func fetchRecents() tea.Msg {
	store, err := history.Open()
	if err != nil {
		return RecentSessionsMsg{Err: err}
	}
	defer store.Close()
	entries, err := store.LoadHistory()
	return RecentSessionsMsg{Entries: entries, Err: err}
}

// Update handles messages for the live model.
func (m LiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case TreeRefreshedMsg:
		if msg.Err != nil {
			m.lastError = msg.Err
			return m, tickCmd(m.opts.RefreshInterval)
		}
		m.tree = msg.Tree
		m.rebuildFlatNodes()
		m.refreshProjectMetadata()

		// Auto-swap on first load
		if m.displayedPaneID == "" && len(m.flatNodes) > 0 {
			m.swapToSelected()
		}

		return m, tickCmd(m.opts.RefreshInterval)

	case TickMsg:
		return m, fetchTree

	case RecentSessionsMsg:
		if msg.Err == nil {
			m.allRecents = msg.Entries
			m.refilterRecents()
			m.refreshProjectMetadata()
		}
		return m, nil

	case LiveActionMsg:
		if msg.Err != nil {
			m.lastError = msg.Err
			m.statusMsg = msg.Action + " failed"
			return m, nil
		}
		m.lastError = nil
		if msg.Target != "" {
			m.statusMsg = msg.Action + ": " + msg.Target
		} else {
			m.statusMsg = msg.Action
		}
		if msg.Action == "default set" {
			m.rebuildFlatNodes()
			m.refreshProjectMetadata()
			return m, nil
		}
		if msg.Action == "remote control" {
			return m, nil
		}
		return m, tea.Batch(fetchTree, fetchRecents)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	return m, nil
}

func (m LiveModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Search mode key handling
	if m.searchActive {
		switch key {
		case "esc":
			m.clearSearch()
			return m, nil
		case "backspace":
			if len(m.searchQuery) > 0 {
				m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
				m.applySearchFilter()
				if m.searchQuery == "" {
					m.clearSearch()
				}
			} else {
				m.clearSearch()
			}
			return m, nil
		case "enter":
			// Accept current selection, close search
			m.searchActive = false
			m.searchManual = false
			m.swapToSelected()
			return m, nil
		case "up", "ctrl+p":
			m.moveUp()
			m.swapToSelected()
			return m, nil
		case "down", "ctrl+n":
			m.moveDown()
			m.swapToSelected()
			return m, nil
		default:
			// Append character to search query
			if len(key) == 1 {
				ch := rune(key[0])
				if unicode.IsLetter(ch) || (m.searchManual && unicode.IsDigit(ch)) || ch == '-' || ch == '_' {
					m.searchQuery += string(unicode.ToLower(ch))
					m.applySearchFilter()
					m.swapToSelected()
					return m, nil
				}
			}
		}
		return m, nil
	}

	// Normal mode key handling
	switch key {
	case "q":
		m.cleanup()
		return m, tea.Quit

	case "esc":
		m.cleanup()
		return m, tea.Quit

	case "up", "k":
		m.moveUp()
		m.swapToSelected()
		return m, nil

	case "down", "j":
		m.moveDown()
		m.swapToSelected()
		return m, nil

	case "enter":
		// Recent entry: revive (create if needed) and attach.
		if entry, ok := m.selectedRecent(); ok {
			m.attachSession = entry.SessionName
			m.attachWorkingDir = entry.WorkingDirectory
			m.attachIsRecent = true
			m.cleanup()
			return m, tea.Quit
		}
		node := m.selectedNode()
		if node == nil {
			return m, nil
		}
		if node.Type == "session" || node.Type == "window" {
			m.toggleExpand()
			return m, nil
		}
		// Pane selected: swap it in (if needed) and move tmux focus
		// to the display slot so the user can interact with it.
		m.swapToSelected()
		tmux.FocusPane(m.opts.RightPaneID)
		return m, nil

	case " ":
		m.toggleExpand()
		return m, nil

	case "a":
		if entry, ok := m.selectedRecent(); ok {
			m.attachSession = entry.SessionName
			m.attachWorkingDir = entry.WorkingDirectory
			m.attachIsRecent = true
			m.cleanup()
			return m, tea.Quit
		}
		node := m.selectedNode()
		if node == nil {
			return m, nil
		}
		sessName := m.sessionForNode(node)
		if sessName != "" {
			m.attachSession = sessName
			m.cleanup()
			return m, tea.Quit
		}
		return m, nil

	case "r":
		return m, tea.Batch(fetchTree, fetchRecents)

	case "p":
		if entry, ok := m.selectedRecent(); ok {
			m.statusMsg = "opening popup..."
			return m, openRecentPopupCmd(entry)
		}
		return m, nil

	case "d":
		return m, m.setSelectedDefault()

	case "x":
		return m, m.killSelectedSession()

	case "c":
		return m, m.startRemoteControl()

	case "b":
		m.beadsPanel = !m.beadsPanel
		return m, nil

	case "h":
		m.recentsHidden = !m.recentsHidden
		m.clampSelection()
		m.refreshProjectMetadata()
		return m, nil

	case "/":
		// Manually open search (allows numbers too)
		m.searchActive = true
		m.searchManual = true
		m.searchQuery = ""
		return m, nil

	default:
		// Numbers: open URL N when the selected project has URLs defined,
		// otherwise fall back to tree quick-jump.
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			idx := int(key[0]-'0') - 1
			if idx < len(m.currentURLs) {
				m.openURL(idx)
				return m, nil
			}
			if idx < len(m.flatNodes) {
				m.selectedIdx = idx
				m.swapToSelected()
			}
			return m, nil
		}

		// Letters: auto-activate search
		if len(key) == 1 {
			ch := rune(key[0])
			if unicode.IsLetter(ch) && ch != 'a' && ch != 'b' && ch != 'c' && ch != 'd' && ch != 'p' && ch != 'q' && ch != 'r' && ch != 'h' && ch != 'j' && ch != 'k' && ch != 'x' {
				m.searchActive = true
				m.searchManual = false
				m.searchQuery = string(unicode.ToLower(ch))
				m.applySearchFilter()
				m.swapToSelected()
				return m, nil
			}
		}
	}

	return m, nil
}

func (m LiveModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		// URL row sits one above the status bar when URLs are present.
		if len(m.currentURLs) > 0 && msg.Y == m.height-2 {
			for _, b := range urlButtonBounds(m.currentURLs) {
				if msg.X >= b.startX && msg.X < b.endX {
					m.openURL(b.idx)
					return m, nil
				}
			}
			return m, nil
		}
		// Click on a row. Header(2) + optional search(1) line offset.
		offset := 2
		if m.searchActive {
			offset++
		}
		rowY := msg.Y - offset
		// Walk the flat node list, then the recent header + entries.
		if rowY >= 0 && rowY < len(m.flatNodes) {
			m.selectedIdx = rowY
			m.swapToSelected()
			return m, nil
		}
		// Past the tree: account for the recent-section header row.
		recents := m.shownRecents()
		if len(recents) > 0 {
			recIdx := rowY - len(m.flatNodes) - 1 // -1 for header row
			if recIdx >= 0 && recIdx < len(recents) {
				m.selectedIdx = len(m.flatNodes) + recIdx
			}
		}
		return m, nil

	case tea.MouseButtonWheelUp:
		m.moveUp()
		m.swapToSelected()
		return m, nil

	case tea.MouseButtonWheelDown:
		m.moveDown()
		m.swapToSelected()
		return m, nil
	}

	return m, nil
}

// swapToSelected swaps the right pane to show the selected node's pane.
// No-op when the selection is on a recent (closed) entry — those have no
// live pane to display, so the right slot keeps showing the previous one.
// Always refreshes URL quick-links for the new selection.
//
// On swap failure (typically because the target pane was killed externally),
// refreshes the tree and tries the next available pane in the same session,
// then advances to subsequent items if that session is gone entirely. Avoids
// leaving the user stuck on a stale selection with a "swap failed" message.
func (m *LiveModel) swapToSelected() {
	m.refreshProjectMetadata()
	node := m.selectedNode()
	if node == nil || m.tree == nil {
		return
	}

	paneID := tmux.FindPaneIDForNode(node, m.tree)
	if paneID == "" || paneID == m.displayedPaneID {
		return
	}

	if err := tmux.SwapPaneIn(m.opts.RightPaneID, paneID, &m.displayedPaneID); err != nil {
		m.recoverFromSwapFailure(err)
	} else {
		m.lastError = nil
		m.lastSwapTarget = node.Target
		m.statusMsg = node.Name
	}
}

// recoverFromSwapFailure handles the case where swap-pane errored — usually
// because the user killed the target pane (or the whole session) from
// another client. Refetches the tree, rebuilds the flat node list, and walks
// forward from the current selection looking for the first selectable pane
// that swaps successfully. Falls back to a "no live panes" status if every
// remaining target also fails.
func (m *LiveModel) recoverFromSwapFailure(origErr error) {
	tree, ferr := tmux.FetchTree()
	if ferr != nil {
		m.lastError = origErr
		m.statusMsg = "swap failed"
		return
	}
	m.tree = tree
	m.displayedPaneID = "" // previously displayed pane is presumed gone
	m.rebuildFlatNodes()

	// Walk forward from the current selection, trying each candidate.
	total := m.totalItems()
	if total == 0 {
		m.statusMsg = "no sessions"
		m.lastError = nil
		return
	}
	start := m.selectedIdx
	if start < 0 {
		start = 0
	}
	for offset := 0; offset < total; offset++ {
		idx := (start + offset) % total
		m.selectedIdx = idx
		node := m.selectedNode()
		if node == nil {
			continue // recent entry; can't display
		}
		paneID := tmux.FindPaneIDForNode(node, m.tree)
		if paneID == "" {
			continue
		}
		if err := tmux.SwapPaneIn(m.opts.RightPaneID, paneID, &m.displayedPaneID); err == nil {
			m.lastError = nil
			m.lastSwapTarget = node.Target
			m.statusMsg = node.Name
			m.refreshProjectMetadata()
			return
		}
	}
	m.lastError = origErr
	m.statusMsg = "no live panes"
}

// refreshURLs reloads the quick-link URLs for whatever is selected. Looks
// up the working directory for the active session (or the recent entry's
// stored dir), parses .agent-tmux.conf, and caches the result.
func (m *LiveModel) refreshURLs() {
	workDir := m.selectedWorkingDir()
	if workDir == "" {
		m.currentURLs = nil
		return
	}
	if m.urlsCache == nil {
		m.urlsCache = map[string][]config.URLConfig{}
	}
	if cached, ok := m.urlsCache[workDir]; ok {
		m.currentURLs = cached
		return
	}
	cfg, _ := config.LoadConfig(filepath.Join(workDir, config.DefaultConfigName))
	var urls []config.URLConfig
	if cfg != nil {
		urls = cfg.URLs
	}
	m.urlsCache[workDir] = urls
	m.currentURLs = urls
}

func (m *LiveModel) refreshProjectMetadata() {
	m.refreshURLs()
	m.refreshBeads()
}

func (m *LiveModel) refreshBeads() {
	workDir := m.selectedWorkingDir()
	if workDir == "" {
		m.currentBeads = liveBeadsSummary{}
		return
	}
	m.currentBeads = loadLiveBeadsSummary(workDir)
}

// selectedWorkingDir resolves a filesystem path for the current selection,
// preferring tmux's session_path for live sessions and falling back to the
// stored history WorkingDirectory for recent entries.
func (m *LiveModel) selectedWorkingDir() string {
	if entry, ok := m.selectedRecent(); ok {
		return entry.WorkingDirectory
	}
	node := m.selectedNode()
	if node == nil {
		return ""
	}
	sess := m.sessionForNode(node)
	if sess == "" {
		return ""
	}
	return tmux.GetSessionPath(sess)
}

// openURL launches the URL at the given index in the platform browser
// and surfaces the action in the status bar.
func (m *LiveModel) openURL(idx int) {
	if idx < 0 || idx >= len(m.currentURLs) {
		return
	}
	u := m.currentURLs[idx]
	if err := tmux.OpenURL(u.URL); err != nil {
		m.lastError = err
		m.statusMsg = "open failed"
		return
	}
	m.lastError = nil
	m.statusMsg = "opened: " + u.Label
}

func openRecentPopupCmd(entry history.Entry) tea.Cmd {
	return func() tea.Msg {
		name, err := reviveRecentSession(entry.WorkingDirectory)
		if err != nil {
			return LiveActionMsg{Action: "popup", Target: entry.Name, Err: err}
		}
		if err := tmux.OpenSessionPopup(name); err != nil {
			return LiveActionMsg{Action: "popup", Target: name, Err: err}
		}
		return LiveActionMsg{Action: "popup closed", Target: name}
	}
}

func (m LiveModel) setSelectedDefault() tea.Cmd {
	node := m.selectedNode()
	if node == nil || m.tree == nil {
		return nil
	}
	sessionName := m.sessionForNode(node)
	target := tmux.FindPaneTargetForNode(node, m.tree)
	if sessionName == "" || target == "" {
		return nil
	}
	return func() tea.Msg {
		err := tmux.SetSessionDefaultPane(sessionName, target)
		return LiveActionMsg{Action: "default set", Target: target, Err: err}
	}
}

func (m *LiveModel) killSelectedSession() tea.Cmd {
	node := m.selectedNode()
	if node == nil {
		return nil
	}
	sessionName := m.sessionForNode(node)
	if sessionName == "" || sessionName == tmux.LiveSessionName {
		return nil
	}
	rightPaneID := m.opts.RightPaneID
	displayedPaneID := m.displayedPaneID
	displayedSession := sessionNameFromTarget(m.lastSwapTarget)
	if displayedPaneID != "" && displayedSession == sessionName {
		m.displayedPaneID = ""
		m.lastSwapTarget = ""
	}
	return func() tea.Msg {
		if displayedPaneID != "" && displayedSession == sessionName {
			tmux.RestoreDisplayedPane(rightPaneID, &displayedPaneID)
		}
		err := tmux.KillSession(sessionName)
		return LiveActionMsg{Action: "killed", Target: sessionName, Err: err}
	}
}

func (m LiveModel) startRemoteControl() tea.Cmd {
	session, ok := m.selectedSession()
	if !ok {
		return nil
	}
	pane, ok := tmux.FindClaudePaneInSession(session)
	if !ok {
		return func() tea.Msg {
			return LiveActionMsg{Action: "remote control", Target: session.Name, Err: errNoClaudePane{}}
		}
	}
	target := pane.ID
	if target == "" {
		target = pane.Target
	}
	command := "/remote-control " + liveRemoteControlName(session.Name)
	return func() tea.Msg {
		err := tmux.SendCommandWithMethod(target, command, tmux.SendMethodEnterDelayed)
		return LiveActionMsg{Action: "remote control", Target: session.Name, Err: err}
	}
}

type errNoClaudePane struct{}

func (errNoClaudePane) Error() string {
	return "no Claude Code pane in selected session"
}

func liveRemoteControlName(sessionName string) string {
	name := strings.TrimPrefix(sessionName, "agent-")
	name = strings.TrimPrefix(name, "atmux-")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	return strings.Join(strings.Fields(name), " ")
}

func sessionNameFromTarget(target string) string {
	if target == "" {
		return ""
	}
	if idx := strings.Index(target, ":"); idx >= 0 {
		return target[:idx]
	}
	return target
}

// totalItems returns the count of selectable rows (tree nodes + visible recents).
func (m *LiveModel) totalItems() int {
	return len(m.flatNodes) + len(m.shownRecents())
}

// shownRecents returns the recent entries that are currently visible.
func (m *LiveModel) shownRecents() []history.Entry {
	if m.recentsHidden {
		return nil
	}
	return m.visibleRecents
}

// selectedRecent returns the recent entry under the cursor, if any.
func (m *LiveModel) selectedRecent() (history.Entry, bool) {
	recents := m.shownRecents()
	if len(recents) == 0 {
		return history.Entry{}, false
	}
	idx := m.selectedIdx - len(m.flatNodes)
	if idx < 0 || idx >= len(recents) {
		return history.Entry{}, false
	}
	return recents[idx], true
}

func (m *LiveModel) selectedSession() (tmux.TmuxSession, bool) {
	node := m.selectedNode()
	if node == nil || m.tree == nil {
		return tmux.TmuxSession{}, false
	}
	sessionName := m.sessionForNode(node)
	if sessionName == "" {
		return tmux.TmuxSession{}, false
	}
	for _, sess := range m.tree.Sessions {
		if sess.Name == sessionName {
			return sess, true
		}
	}
	return tmux.TmuxSession{}, false
}

// activeSessionNames returns the set of session names currently in the tree.
func (m *LiveModel) activeSessionNames() map[string]bool {
	names := map[string]bool{}
	if m.tree == nil {
		return names
	}
	for _, sess := range m.tree.Sessions {
		if sess.Name == tmux.LiveSessionName {
			continue
		}
		names[sess.Name] = true
	}
	return names
}

// refilterRecents updates visibleRecents from allRecents, removing entries
// whose sessions are currently active and (when search is active) applying
// the search filter on entry name.
func (m *LiveModel) refilterRecents() {
	active := m.activeSessionNames()
	out := make([]history.Entry, 0, len(m.allRecents))
	for _, e := range m.allRecents {
		if active[e.SessionName] {
			continue
		}
		if m.searchActive && m.searchQuery != "" && !fuzzyMatch(m.searchQuery, e.Name) && !fuzzyMatch(m.searchQuery, e.SessionName) {
			continue
		}
		out = append(out, e)
	}
	m.visibleRecents = out
}

// cleanup restores the displayed pane and prepares for exit.
func (m *LiveModel) cleanup() {
	tmux.CleanupLiveSession(m.opts.RightPaneID, &m.displayedPaneID, m.opts.OriginalSession)
}

// selectedNode returns the currently selected tree node, or nil if the
// selection is on a recent entry (or out of range).
func (m *LiveModel) selectedNode() *tmux.TreeNode {
	if m.selectedIdx >= 0 && m.selectedIdx < len(m.flatNodes) {
		return m.flatNodes[m.selectedIdx]
	}
	return nil
}

func (m *LiveModel) moveUp() {
	if m.selectedIdx > 0 {
		m.selectedIdx--
	}
}

func (m *LiveModel) moveDown() {
	if m.selectedIdx < m.totalItems()-1 {
		m.selectedIdx++
	}
}

// clampSelection ensures selectedIdx is within [0, totalItems-1].
func (m *LiveModel) clampSelection() {
	total := m.totalItems()
	if m.selectedIdx >= total {
		m.selectedIdx = total - 1
	}
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
}

func (m *LiveModel) toggleExpand() {
	node := m.selectedNode()
	if node == nil {
		return
	}
	if node.Type == "session" || node.Type == "window" {
		key := nodeKey(node.Type, node.Target)
		expanded := m.isExpanded(node.Type, node.Target)
		m.expanded[key] = !expanded
		m.rebuildFlatNodes()
		m.refreshProjectMetadata()
	}
}

func (m *LiveModel) isExpanded(nodeType, target string) bool {
	key := nodeKey(nodeType, target)
	if val, ok := m.expanded[key]; ok {
		return val
	}
	// Default: collapsed for sessions, expanded for windows
	return nodeType == "window"
}

func (m *LiveModel) rebuildFlatNodes() {
	if m.tree == nil {
		m.flatNodes = []*tmux.TreeNode{}
		return
	}

	var nodes []*tmux.TreeNode
	for _, sess := range m.tree.Sessions {
		// Filter out the live session itself
		if sess.Name == tmux.LiveSessionName {
			continue
		}

		sessExpanded := m.isExpanded("session", sess.Name)
		defaultTarget := tmux.FindDefaultPaneTarget(sess)
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
				winExpanded := m.isExpanded("window", winTarget)
				winNode := &tmux.TreeNode{
					Type:     "window",
					Name:     win.Name,
					Target:   winTarget,
					Expanded: winExpanded,
					Level:    1,
					Active:   win.Active,
				}
				nodes = append(nodes, winNode)

				if winExpanded {
					for _, pane := range win.Panes {
						paneNode := &tmux.TreeNode{
							Type:    "pane",
							Name:    pane.Title,
							Target:  pane.Target,
							Level:   2,
							Active:  pane.Active,
							Default: pane.Target == defaultTarget,
						}
						if paneNode.Name == "" {
							paneNode.Name = pane.Command
						}
						if paneNode.Name == "" {
							paneNode.Name = "pane " + strconv.Itoa(pane.Index)
						}
						nodes = append(nodes, paneNode)
					}
				}
			}
		}
	}

	m.allFlatNodes = nodes
	m.flatNodes = nodes

	// Reapply search filter if active
	if m.searchActive && m.searchQuery != "" {
		m.applySearchFilter()
	}

	// Recents may need re-filtering since active sessions changed.
	m.refilterRecents()

	m.clampSelection()
}

// fuzzyMatch checks if pattern appears as a subsequence in text (case-insensitive).
// E.g., "abc" matches "agent-bar-config" (a...b...c in order).
func fuzzyMatch(pattern, text string) bool {
	text = strings.ToLower(text)
	pattern = strings.ToLower(pattern)
	pi := 0
	for i := 0; i < len(text) && pi < len(pattern); i++ {
		if text[i] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

// fuzzyMatchIndices returns the matching character indices for highlighting.
// Returns nil if no match.
func fuzzyMatchIndices(pattern, text string) []int {
	lower := strings.ToLower(text)
	pattern = strings.ToLower(pattern)
	var indices []int
	pi := 0
	for i := 0; i < len(lower) && pi < len(pattern); i++ {
		if lower[i] == pattern[pi] {
			indices = append(indices, i)
			pi++
		}
	}
	if pi < len(pattern) {
		return nil
	}
	return indices
}

// applySearchFilter filters flatNodes to sessions matching the search query.
func (m *LiveModel) applySearchFilter() {
	if m.searchQuery == "" || m.allFlatNodes == nil {
		m.flatNodes = m.allFlatNodes
		return
	}

	var filtered []*tmux.TreeNode
	// Track which sessions match
	matchingSessions := map[string]bool{}
	for _, node := range m.allFlatNodes {
		if node.Type == "session" && fuzzyMatch(m.searchQuery, node.Name) {
			matchingSessions[node.Target] = true
		}
	}

	// Include matching sessions and their visible children
	var currentSession string
	for _, node := range m.allFlatNodes {
		if node.Type == "session" {
			currentSession = node.Target
			if matchingSessions[currentSession] {
				filtered = append(filtered, node)
			}
		} else if matchingSessions[currentSession] {
			filtered = append(filtered, node)
		}
	}

	m.flatNodes = filtered
	m.refilterRecents()
	m.clampSelection()
}

// clearSearch clears the search and restores the full node list.
func (m *LiveModel) clearSearch() {
	m.searchActive = false
	m.searchManual = false
	m.searchQuery = ""
	m.flatNodes = m.allFlatNodes
	m.refilterRecents()
	m.clampSelection()
	m.refreshProjectMetadata()
}

// sessionForNode finds the session name that a node belongs to.
func (m *LiveModel) sessionForNode(node *tmux.TreeNode) string {
	if node == nil {
		return ""
	}
	if node.Type == "session" {
		return node.Target
	}
	// Walk flatNodes backwards from current position to find parent session
	for i := m.selectedIdx; i >= 0; i-- {
		if m.flatNodes[i].Type == "session" {
			return m.flatNodes[i].Target
		}
	}
	return ""
}

// RunLive starts the live browser TUI.
func RunLive(opts LiveOptions) error {
	m := NewLiveModel(opts)
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	model, ok := finalModel.(LiveModel)
	if !ok {
		return nil
	}

	// If user requested attach to a recent (closed) project, create the
	// session if needed before attaching. The name may differ from the
	// stored history entry (NewSession derives it from the working dir).
	target := model.attachSession
	if model.attachIsRecent && model.attachWorkingDir != "" {
		name, err := reviveRecentSession(model.attachWorkingDir)
		if err != nil {
			return err
		}
		target = name
	}

	// If user requested attach, switch to that session
	if target != "" {
		return tmux.AttachToSession(target)
	}

	return nil
}

// reviveRecentSession creates the tmux session for a recent history entry
// (using its on-disk config) when it isn't already running. Returns the
// real session name. Mirrors the revival path in cmd/root.go for consistency.
func reviveRecentSession(workingDir string) (string, error) {
	session := tmux.NewSession(workingDir)
	if session.Exists() {
		return session.Name, nil
	}
	cfg, _ := config.LoadConfig(filepath.Join(workingDir, config.DefaultConfigName))
	if err := session.Create(cfg); err != nil {
		return "", err
	}
	if cfg != nil {
		_ = session.ApplyConfig(cfg)
	}
	session.SelectDefault()

	if store, err := history.Open(); err == nil {
		_ = store.SaveEntry(filepath.Base(workingDir), workingDir, session.Name, "", "")
		store.Close()
	}
	return session.Name, nil
}

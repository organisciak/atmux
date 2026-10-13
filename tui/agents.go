package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/porganisciak/agent-tmux/tmux"
)

// AgentsOptions configures the agents overview.
type AgentsOptions struct {
	Executors []tmux.TmuxExecutor
	// Columns fixes the number of card columns; 0 fits as many as the width
	// allows.
	Columns int
	// OnColumnsChange is called when the user resizes the grid, so the choice
	// can be remembered. Optional.
	OnColumnsChange func(columns int)
}

// AgentsResult is the pane the user picked, if any.
type AgentsResult struct {
	Target      string // pane id to switch to ("" = quit)
	SessionName string
	Executor    tmux.TmuxExecutor
}

// RunAgents shows the overview grid and returns the chosen pane.
func RunAgents(opts AgentsOptions) (*AgentsResult, error) {
	m := agentsModel{
		executors:       opts.Executors,
		byHost:          make([][]tmux.AgentPane, len(opts.Executors)),
		gens:            make([]int, len(opts.Executors)),
		columnsOverride: max(0, opts.Columns),
		onColumnsChange: opts.OnColumnsChange,
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	if fm, ok := final.(agentsModel); ok && fm.chosen != nil {
		return &AgentsResult{
			Target:      fm.chosen.Pane.ID,
			SessionName: fm.chosen.SessionName,
			Executor:    fm.chosen.Executor,
		}, nil
	}
	return &AgentsResult{}, nil
}

const (
	// agentsRefreshInterval is how often states are re-read. The registry is a
	// handful of small files, so this can be brisk.
	agentsRefreshInterval = 2 * time.Second
	// agentCardMinWidth is the narrowest card the automatic layout uses.
	agentCardMinWidth = 38
	// agentCardNarrowest bounds how many columns the user can ask for.
	agentCardNarrowest = 20
	agentCardBodyLines = 5
	// Card chrome: border (2) + status line + title line.
	agentCardChrome = 4
	// agentCardMinHeight is a card with agentCardBodyLines of text. Cards
	// grow past it to fill the screen when every row fits.
	agentCardMinHeight = agentCardChrome + agentCardBodyLines
	agentsHeaderLines  = 2
	agentsFooterLines  = 1
)

// Each host loads and refreshes on its own schedule, so a slow or unreachable
// remote never holds up the local cards.
type agentsLoadedMsg struct {
	host   int
	agents []tmux.AgentPane
	at     time.Time
}

// agentsTickMsg carries the generation of the load that scheduled it; a tick
// from before a manual refresh is stale and dropped, so refreshing never
// leaves a second polling loop behind.
type agentsTickMsg struct{ host, gen int }

type agentsModel struct {
	executors []tmux.TmuxExecutor
	byHost    [][]tmux.AgentPane
	gens      []int
	agents    []tmux.AgentPane // byHost flattened, in executor order
	loadedAt  time.Time
	loaded    int // hosts that have answered at least once

	width, height int
	selected      int
	scrollRow     int

	// columnsOverride is the user's chosen column count; 0 means automatic.
	columnsOverride int
	onColumnsChange func(int)

	chosen *tmux.AgentPane
}

func (m agentsModel) Init() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.executors))
	for i := range m.executors {
		cmds[i] = m.load(i)
	}
	return tea.Batch(cmds...)
}

func (m agentsModel) load(host int) tea.Cmd {
	exec := m.executors[host]
	return func() tea.Msg {
		agents := tmux.ListAgentPanes([]tmux.TmuxExecutor{exec})
		tmux.LoadTranscripts(agents)
		return agentsLoadedMsg{host: host, agents: agents, at: time.Now()}
	}
}

func agentsTick(host, gen int) tea.Cmd {
	return tea.Tick(agentsRefreshInterval, func(time.Time) tea.Msg { return agentsTickMsg{host: host, gen: gen} })
}

func (m agentsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.keepSelectionVisible()
		return m, nil

	case agentsLoadedMsg:
		if m.byHost[msg.host] == nil {
			m.loaded++
		}
		if msg.agents == nil {
			msg.agents = []tmux.AgentPane{}
		}
		m.byHost[msg.host] = msg.agents
		m.agents = m.agents[:0:0]
		for _, host := range m.byHost {
			m.agents = append(m.agents, host...)
		}
		m.loadedAt = msg.at
		if m.selected >= len(m.agents) {
			m.selected = max(0, len(m.agents)-1)
		}
		m.keepSelectionVisible()
		m.gens[msg.host]++
		return m, agentsTick(msg.host, m.gens[msg.host])

	case agentsTickMsg:
		if msg.gen != m.gens[msg.host] {
			return m, nil
		}
		return m, m.load(msg.host)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}
	return m, nil
}

func (m agentsModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cols := m.columns()
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "enter":
		return m.choose(m.selected)
	case "r":
		return m, m.Init()
	case "left", "h":
		m.move(-1)
	case "right", "l", "tab":
		m.move(1)
	case "up", "k":
		m.move(-cols)
	case "down", "j":
		m.move(cols)
	case "+", "=":
		// Fewer, larger cards; stop at one rather than wrap to 0, which
		// means automatic.
		return m.setColumns(max(1, cols-1))
	case "-", "_":
		return m.setColumns(cols + 1)
	case "0":
		return m.setColumns(0)
	}
	return m, nil
}

// setColumns fixes the column count (0 = automatic), clamped to what the
// width can hold.
func (m agentsModel) setColumns(n int) (tea.Model, tea.Cmd) {
	if n != 0 {
		n = max(1, min(n, m.maxColumns()))
	}
	if n == m.columnsOverride {
		return m, nil
	}
	m.columnsOverride = n
	m.keepSelectionVisible()
	if save := m.onColumnsChange; save != nil {
		// Off the UI goroutine; remembering the size is best-effort.
		return m, func() tea.Msg { save(n); return nil }
	}
	return m, nil
}

func (m agentsModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if m.scrollRow > 0 {
			m.scrollRow--
		}
		return m, nil
	case tea.MouseButtonWheelDown:
		if m.scrollRow < m.rows()-m.visibleRows() {
			m.scrollRow++
		}
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if i, ok := m.cardAt(msg.X, msg.Y); ok {
		return m.choose(i)
	}
	return m, nil
}

func (m agentsModel) choose(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i >= len(m.agents) {
		return m, nil
	}
	a := m.agents[i]
	m.chosen = &a
	return m, tea.Quit
}

func (m *agentsModel) move(delta int) {
	next := m.selected + delta
	if next < 0 || next >= len(m.agents) {
		return
	}
	m.selected = next
	m.keepSelectionVisible()
}

// Layout. Cards share the width evenly, so a column's x range is fixed and a
// click maps back to a card with plain division.

func (m agentsModel) columns() int {
	if m.width <= 0 {
		return 1
	}
	if m.columnsOverride > 0 {
		return min(m.columnsOverride, m.maxColumns())
	}
	return max(1, m.width/agentCardMinWidth)
}

func (m agentsModel) maxColumns() int {
	return max(1, m.width/agentCardNarrowest)
}

func (m agentsModel) cardWidth() int {
	return max(1, m.width/m.columns())
}

// cardHeight is shared by every card so rows line up and a click maps back
// to a card by division. When all rows fit at the minimum height, cards
// stretch to fill the screen, so fewer columns means more text per card.
func (m agentsModel) cardHeight() int {
	avail := m.height - agentsHeaderLines - agentsFooterLines
	rows := m.rows()
	if rows == 0 || rows*agentCardMinHeight > avail {
		return agentCardMinHeight
	}
	return avail / rows
}

func (m agentsModel) rows() int {
	cols := m.columns()
	return (len(m.agents) + cols - 1) / cols
}

func (m agentsModel) visibleRows() int {
	return max(1, (m.height-agentsHeaderLines-agentsFooterLines)/m.cardHeight())
}

func (m *agentsModel) keepSelectionVisible() {
	row := m.selected / m.columns()
	if row < m.scrollRow {
		m.scrollRow = row
	}
	if row >= m.scrollRow+m.visibleRows() {
		m.scrollRow = row - m.visibleRows() + 1
	}
	m.scrollRow = max(0, min(m.scrollRow, m.rows()-m.visibleRows()))
}

func (m agentsModel) cardAt(x, y int) (int, bool) {
	y -= agentsHeaderLines
	if x < 0 || y < 0 {
		return 0, false
	}
	col := x / m.cardWidth()
	row := y/m.cardHeight() + m.scrollRow
	if col >= m.columns() {
		return 0, false
	}
	i := row*m.columns() + col
	return i, i < len(m.agents)
}

func (m agentsModel) View() string {
	if m.width == 0 {
		return ""
	}
	now := m.loadedAt
	var b strings.Builder
	b.WriteString(m.header(now))
	b.WriteString("\n\n")

	if len(m.agents) == 0 {
		if m.loaded < len(m.executors) {
			b.WriteString(agentsDimStyle.Render("  Looking for Claude Code sessions…"))
		} else {
			b.WriteString(agentsDimStyle.Render("  No Claude Code sessions running."))
		}
		return b.String()
	}

	cols := m.columns()
	first := m.scrollRow * cols
	last := min(len(m.agents), (m.scrollRow+m.visibleRows())*cols)
	var rows []string
	for start := first; start < last; start += cols {
		var cards []string
		for i := start; i < min(start+cols, last); i++ {
			cards = append(cards, m.renderCard(i, now))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cards...))
	}
	b.WriteString(strings.Join(rows, "\n"))

	footer := "click or enter: jump · arrows: move · +/-: card size · 0: auto · r: refresh · q: quit"
	if waiting := len(m.executors) - m.loaded; waiting > 0 {
		footer = fmt.Sprintf("waiting on %d host(s) · ", waiting) + footer
	}
	if hidden := len(m.agents) - (last - first); hidden > 0 {
		footer = fmt.Sprintf("%d more (scroll) · ", hidden) + footer
	}
	// One line exactly: the height math reserves agentsFooterLines for it.
	b.WriteString("\n" + agentsDimStyle.Render(ansi.Truncate(footer, m.width, "…")))
	return b.String()
}

func (m agentsModel) header(now time.Time) string {
	counts := make(map[tmux.Activity]int)
	for _, a := range m.agents {
		counts[a.Activity(now)]++
	}
	parts := []string{agentsTitleStyle.Render("Claude Code sessions")}
	for _, act := range []tmux.Activity{tmux.ActivityNeedsInput, tmux.ActivityReady, tmux.ActivityWorking, tmux.ActivityFresh, tmux.ActivityDormant} {
		if n := counts[act]; n > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(activityColor(act)).Render(fmt.Sprintf("%d %s", n, activityName(act))))
		}
	}
	return strings.Join(parts, "  ")
}

func (m agentsModel) renderCard(i int, now time.Time) string {
	a := m.agents[i]
	act := a.Activity(now)
	inner := m.cardWidth() - 2

	name := a.SessionName
	if a.Host != "" {
		name = "[" + a.Host + "] " + name
	}
	// atmux names its agent window "agents"; tmux's automatic rename names it
	// after Claude's version string. Neither tells sessions apart.
	if a.WindowName != "" && a.WindowName != "agents" && !tmux.LooksLikeVersion(a.WindowName) {
		name += ":" + a.WindowName
	}
	status := lipgloss.NewStyle().Foreground(activityColor(act)).Bold(true).Render(AgentActivityLabel(a, now))
	head := ansi.Truncate(status+"  "+lipgloss.NewStyle().Bold(true).Render(name), inner, "…")

	_, title := tmux.SplitAgentTitle(a.Pane.Title)
	titleLine := agentsDimStyle.Render(ansi.Truncate(title, inner, "…"))

	bodyHeight := m.cardHeight() - agentCardChrome
	bodyLines := wrapCardBody(agentCardSections(a, act), inner, bodyHeight)

	border := lipgloss.RoundedBorder()
	if i == m.selected {
		border = lipgloss.ThickBorder()
	}
	return lipgloss.NewStyle().
		Border(border).
		BorderForeground(activityColor(act)).
		Width(inner).
		Render(strings.Join(append([]string{head, titleLine}, bodyLines...), "\n"))
}

// agentCardSections is what a card says, most important first; a small card
// shows the first section, a large one reads on.
func agentCardSections(a tmux.AgentPane, act tmux.Activity) []string {
	t := a.Transcript
	prompt := ""
	if t.LastPrompt != "" {
		prompt = "▸ " + t.LastPrompt
	}
	summary := AgentSummaryText(a)

	var sections []string
	switch {
	case act == tmux.ActivityWorking:
		// A running turn is best described by what it was asked to do.
		sections = []string{prompt, summary}
	case summary == t.LastReply:
		sections = []string{t.LastReply, t.ReplyMore, prompt}
	default:
		// The recap leads; then what was asked and how Claude answered.
		sections = []string{summary, prompt, t.LastReply, t.ReplyMore}
	}

	var out []string
	for _, s := range sections {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		out = []string{placeholderSummary(act, a)}
	}
	return out
}

// wrapCardBody wraps sections to width, a blank line between them, and fits
// the result to exactly height lines, ending in "…" when text was cut.
func wrapCardBody(sections []string, width, height int) []string {
	var lines []string
	for i, section := range sections {
		if i > 0 {
			lines = append(lines, "")
		}
		for _, line := range strings.Split(section, "\n") {
			line = strings.Join(strings.Fields(line), " ")
			lines = append(lines, strings.Split(ansi.Wrap(line, width, ""), "\n")...)
		}
	}
	if len(lines) > height {
		lines = lines[:height]
		// Put the ellipsis on the last line of text, not on a section gap.
		for len(lines) > 1 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		last := strings.TrimRight(lines[len(lines)-1], " ")
		lines[len(lines)-1] = ansi.Truncate(last, width-1, "") + "…"
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

func placeholderSummary(act tmux.Activity, a tmux.AgentPane) string {
	switch {
	case act == tmux.ActivityFresh:
		return "No conversation yet."
	case a.Host != "":
		return "Install this atmux version on " + a.Host + " to see its recaps."
	}
	return "No recap yet."
}

// AgentSummaryText is the best short description of where a session is: the
// recap while it is current, otherwise Claude's latest reply, otherwise an
// older recap.
func AgentSummaryText(a tmux.AgentPane) string {
	t := a.Transcript
	switch {
	case t.RecapIsCurrent():
		return t.Recap
	case t.LastReply != "":
		return t.LastReply
	}
	return t.Recap
}

// AgentActivityLabel is the short status shown on a card, e.g. "✓ ready 4m".
func AgentActivityLabel(a tmux.AgentPane, now time.Time) string {
	act := a.Activity(now)
	label := activityGlyph(act) + " " + activityName(act)
	if act == tmux.ActivityNeedsInput && a.WaitingFor != "" {
		label += ": " + a.WaitingFor
	}
	if !a.Since.IsZero() && act != tmux.ActivityFresh {
		label += " " + shortDuration(now.Sub(a.Since))
	}
	return label
}

func activityName(act tmux.Activity) string {
	switch act {
	case tmux.ActivityNeedsInput:
		return "needs input"
	case tmux.ActivityUnknown:
		return "unknown"
	}
	return string(act)
}

func activityGlyph(act tmux.Activity) string {
	switch act {
	case tmux.ActivityWorking:
		return "●"
	case tmux.ActivityNeedsInput:
		return "▲"
	case tmux.ActivityReady:
		return "✓"
	case tmux.ActivityDormant:
		return "○"
	case tmux.ActivityFresh:
		return "◌"
	}
	return "?"
}

func activityColor(act tmux.Activity) lipgloss.Color {
	switch act {
	case tmux.ActivityWorking:
		return lipgloss.Color("33") // blue
	case tmux.ActivityNeedsInput:
		return lipgloss.Color("208") // orange
	case tmux.ActivityReady:
		return lipgloss.Color("42") // green
	case tmux.ActivityFresh:
		return lipgloss.Color("141") // lavender
	}
	return lipgloss.Color("243") // grey
}

func shortDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

var (
	agentsDimStyle   = lipgloss.NewStyle().Foreground(dimColor)
	agentsTitleStyle = lipgloss.NewStyle().Bold(true)
)

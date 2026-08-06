package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/history"
	"github.com/porganisciak/agent-tmux/tmux"
)

// View renders the live browser TUI.
func (m LiveModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	var b strings.Builder

	// Header
	title := lipgloss.NewStyle().Bold(true).Foreground(primaryColor).Render("atmux live")
	sessionCount := ""
	if m.tree != nil {
		count := 0
		for _, s := range m.tree.Sessions {
			if s.Name != "_atmux_live" {
				count++
			}
		}
		sessionCount = lipgloss.NewStyle().Foreground(dimColor).Render(
			fmt.Sprintf(" (%d sessions)", count))
	}
	b.WriteString(title + sessionCount + m.renderBeadsBadge() + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render(strings.Repeat("─", m.width)) + "\n")

	// Search box (if active)
	searchHeight := 0
	if m.searchActive {
		searchBox := m.renderSearchBox()
		b.WriteString(searchBox + "\n")
		searchHeight = 1
	}

	urlHeight := 0
	if len(m.currentURLs) > 0 {
		urlHeight = 1
	}
	beadsHeight := 0
	if m.beadsPanel && m.currentBeads.HasBeads {
		beadsHeight = minInt(liveBeadsPanelRows, maxInt(0, m.height-5-searchHeight-urlHeight))
	}
	statusHeight := m.statusHeight()

	// Tree + recents
	treeHeight := m.height - 3 - searchHeight - urlHeight - beadsHeight - statusHeight // header(2) + separator(1) + search + urls + beads + status
	if treeHeight < 1 {
		treeHeight = 1
	}

	// Build a unified list of display rows. Each row carries its rendered
	// string and the selectable index it corresponds to (-1 for headers).
	rows, rowSel := m.buildDisplayRows()

	if len(rows) == 0 {
		noSessions := lipgloss.NewStyle().Foreground(dimColor).Render("  No sessions found")
		b.WriteString(noSessions + "\n")
		for i := 1; i < treeHeight; i++ {
			b.WriteString("\n")
		}
	} else {
		// Find the visual line index of the currently selected row.
		selVisual := -1
		for i, sel := range rowSel {
			if sel == m.selectedIdx {
				selVisual = i
				break
			}
		}

		scrollStart := 0
		if selVisual >= treeHeight {
			scrollStart = selVisual - treeHeight + 1
		}
		scrollEnd := scrollStart + treeHeight
		if scrollEnd > len(rows) {
			scrollEnd = len(rows)
		}

		linesWritten := 0
		for i := scrollStart; i < scrollEnd; i++ {
			b.WriteString(rows[i] + "\n")
			linesWritten++
		}

		for i := linesWritten; i < treeHeight; i++ {
			b.WriteString("\n")
		}
	}

	// URL quick-links row (above status bar) when the selected project
	// has any urls defined.
	if urlHeight > 0 {
		b.WriteString(m.renderURLs() + "\n")
	}

	if beadsHeight > 0 {
		b.WriteString(m.renderBeadsPanel(beadsHeight) + "\n")
	}

	// Status bar
	b.WriteString(m.renderStatus())

	return b.String()
}

func (m LiveModel) renderBeadsBadge() string {
	if !m.currentBeads.HasBeads {
		return ""
	}
	label := fmt.Sprintf(" bd:%d", m.currentBeads.Count)
	if m.currentBeads.Count > 0 {
		return " " + beadsCountStyle.Render(label)
	}
	return " " + lipgloss.NewStyle().Foreground(dimColor).Render(label)
}

// urlButtonLabel formats the displayed text for a URL button. Kept short
// so several fit on one row.
func urlButtonLabel(idx int, u config.URLConfig) string {
	return fmt.Sprintf("[%d] %s", idx+1, u.Label)
}

// urlButtonBounds returns the X-coordinate ranges of each URL button as
// they will be rendered. Both the renderer and the click handler use this
// to keep the visual and hit-test layouts in lockstep.
func urlButtonBounds(urls []config.URLConfig) []urlClickBounds {
	const sep = "  "
	bounds := make([]urlClickBounds, 0, len(urls))
	x := 0
	for i, u := range urls {
		label := urlButtonLabel(i, u)
		w := lipgloss.Width(label)
		bounds = append(bounds, urlClickBounds{startX: x, endX: x + w, idx: i})
		x += w + len(sep)
	}
	return bounds
}

func (m LiveModel) renderURLs() string {
	const sep = "  "
	buttonStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true) // cyan-ish
	parts := make([]string, 0, len(m.currentURLs))
	for i, u := range m.currentURLs {
		parts = append(parts, buttonStyle.Render(urlButtonLabel(i, u)))
	}
	line := strings.Join(parts, sep)
	if m.width > 0 && lipgloss.Width(line) > m.width {
		// Truncate (rare — labels should be short)
		line = line[:m.width-1] + "…"
	}
	return line
}

// buildDisplayRows assembles the visible row list: tree nodes followed by
// (optionally) a "Recent" header and recent entries. Returns parallel slices
// of rendered rows and the selectable index each row maps to (-1 = header,
// non-selectable).
func (m LiveModel) buildDisplayRows() ([]string, []int) {
	var rows []string
	var sel []int

	for i, node := range m.flatNodes {
		rows = append(rows, m.renderNode(node, i == m.selectedIdx))
		sel = append(sel, i)
	}

	recents := m.shownRecents()
	if len(recents) > 0 {
		rows = append(rows, m.renderRecentsHeader(len(recents)))
		sel = append(sel, -1)
		base := len(m.flatNodes)
		for i, entry := range recents {
			selIdx := base + i
			rows = append(rows, m.renderRecentEntry(entry, selIdx == m.selectedIdx))
			sel = append(sel, selIdx)
		}
	} else if len(m.allRecents) > 0 && m.recentsHidden {
		rows = append(rows, lipgloss.NewStyle().Foreground(dimColor).Render(
			fmt.Sprintf("  ─ Recent (%d hidden, h to show) ─", len(m.allRecents))))
		sel = append(sel, -1)
	}

	return rows, sel
}

func (m LiveModel) renderRecentsHeader(count int) string {
	label := fmt.Sprintf(" Recent (%d) — h to hide ", count)
	return lipgloss.NewStyle().Foreground(dimColor).Render("─" + label + strings.Repeat("─", maxInt(0, m.width-lipgloss.Width(label)-1)))
}

func (m LiveModel) renderRecentEntry(entry history.Entry, selected bool) string {
	icon := "□ "
	nameStyle := lipgloss.NewStyle().Foreground(dimColor)
	name := formatSessionName(entry.Name, nameStyle)
	ago := lipgloss.NewStyle().Foreground(dimColor).Render(" (" + sessionsTimeAgo(entry.LastUsedAt) + ")")
	line := "  " + icon + name + ago

	if m.width > 0 && lipgloss.Width(line) > m.width {
		line = line[:m.width-1] + "…"
	}
	pad := m.width - lipgloss.Width(line)
	if pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	if selected {
		line = selectedStyle.Render(line)
	}
	return line
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// renderNode renders a single tree node line.
func (m LiveModel) renderNode(node *tmux.TreeNode, selected bool) string {
	indent := strings.Repeat("  ", node.Level)

	var icon string
	switch node.Type {
	case "session":
		if node.SinglePane {
			icon = "● "
		} else if m.isExpanded("session", node.Target) {
			icon = "▾ "
		} else {
			icon = "▸ "
		}
	case "window":
		if node.SinglePane {
			icon = "● "
		} else if m.isExpanded("window", node.Target) {
			icon = "▾ "
		} else {
			icon = "▸ "
		}
	case "pane":
		if node.Default {
			icon = "◆ "
		} else if node.Active {
			icon = "● "
		} else {
			icon = "○ "
		}
	}

	// Style the name
	var name string
	switch node.Type {
	case "session":
		nameStyle := sessionStyle
		if node.Attached {
			nameStyle = sessionAttachedStyle
		}
		// Highlight fuzzy match characters if searching
		if m.searchActive && m.searchQuery != "" {
			name = m.renderFuzzyHighlight(node.Name, nameStyle)
		} else {
			name = formatSessionName(node.Name, nameStyle)
		}
	case "window":
		if node.Active {
			name = windowActiveStyle.Render(node.Name)
		} else {
			name = windowStyle.Render(node.Name)
		}
	case "pane":
		if node.Active {
			name = paneActiveStyle.Render(node.Name)
		} else {
			name = paneStyle.Render(node.Name)
		}
	}

	line := indent + icon + name
	if node.Type == "pane" && node.Default {
		line += lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(" default")
	}

	// Truncate to width
	if m.width > 0 && lipgloss.Width(line) > m.width {
		// Simple truncation
		line = line[:m.width-1] + "…"
	}

	// Pad to full width
	lineWidth := lipgloss.Width(line)
	if lineWidth < m.width {
		line += strings.Repeat(" ", m.width-lineWidth)
	}

	if selected {
		line = selectedStyle.Render(line)
	}

	return line
}

func (m LiveModel) renderSearchBox() string {
	label := lipgloss.NewStyle().Foreground(primaryColor).Bold(true).Render("/")
	query := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render(m.searchQuery)
	cursor := lipgloss.NewStyle().Foreground(primaryColor).Render("▎")
	matchCount := ""
	if len(m.flatNodes) > 0 {
		// Count matching sessions
		count := 0
		for _, n := range m.flatNodes {
			if n.Type == "session" {
				count++
			}
		}
		matchCount = lipgloss.NewStyle().Foreground(dimColor).Render(
			fmt.Sprintf(" (%d)", count))
	}
	return label + query + cursor + matchCount
}

// renderFuzzyHighlight renders a session name with matched characters highlighted.
func (m LiveModel) renderFuzzyHighlight(name string, baseStyle lipgloss.Style) string {
	indices := fuzzyMatchIndices(m.searchQuery, name)
	if indices == nil {
		return formatSessionName(name, baseStyle)
	}

	highlightStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("226")). // Yellow
		Bold(true).
		Underline(true)

	matchSet := map[int]bool{}
	for _, idx := range indices {
		matchSet[idx] = true
	}

	// Check for agent prefix to dim it
	var prefix, rest string
	prefixLen := 0
	for _, p := range agentPrefixes {
		if strings.HasPrefix(name, p) {
			prefix = p
			rest = strings.TrimPrefix(name, p)
			prefixLen = len(p)
			break
		}
	}
	if prefix == "" {
		rest = name
	}

	var result strings.Builder
	// Render prefix (dimmed, but highlight matches within it)
	for i, ch := range prefix {
		if matchSet[i] {
			result.WriteString(highlightStyle.Render(string(ch)))
		} else {
			result.WriteString(agentPrefixStyle.Render(string(ch)))
		}
	}
	// Render rest
	for i, ch := range rest {
		absIdx := prefixLen + i
		if matchSet[absIdx] {
			result.WriteString(highlightStyle.Render(string(ch)))
		} else {
			result.WriteString(baseStyle.Render(string(ch)))
		}
	}
	return result.String()
}

func (m LiveModel) renderStatus() string {
	hintLines := m.statusHintLines()
	right := m.statusMessage()

	hintStyle := lipgloss.NewStyle().Foreground(dimColor)
	rendered := make([]string, 0, len(hintLines)+1)
	for i, line := range hintLines {
		if i == len(hintLines)-1 && right != "" {
			gap := m.width - lipgloss.Width(line) - lipgloss.Width(right)
			if gap >= 1 {
				rendered = append(rendered, hintStyle.Render(line)+strings.Repeat(" ", gap)+right)
				continue
			}
		}
		rendered = append(rendered, hintStyle.Render(line))
	}
	if right != "" && (len(hintLines) == 0 || m.width-lipgloss.Width(hintLines[len(hintLines)-1])-lipgloss.Width(right) < 1) {
		rendered = append(rendered, right)
	}
	return strings.Join(rendered, "\n")
}

func (m LiveModel) statusHeight() int {
	lines := m.statusHintLines()
	if len(lines) == 0 {
		return 1
	}
	right := m.statusMessage()
	if right != "" && m.width-lipgloss.Width(lines[len(lines)-1])-lipgloss.Width(right) < 1 {
		return len(lines) + 1
	}
	return len(lines)
}

func (m LiveModel) statusHintLines() []string {
	return wrapStatusParts(m.statusHintParts(), m.width)
}

func (m LiveModel) statusHintParts() []string {
	parts := []string{
		"[q]uit",
		"[a]ttach",
		"[↑↓]nav",
		"[⏎]expand/focus",
		"[r]efresh",
		"[d]efault",
		"[x]kill",
		"[c]rc",
		"[h]recents",
	}
	if _, ok := m.selectedRecent(); ok {
		parts = append(parts, "[p]opup")
	}
	if m.currentBeads.HasBeads {
		parts = append(parts, "[b]eads")
	}
	if len(m.currentURLs) > 0 {
		parts = append(parts, "[1-9]open-url")
	}
	return parts
}

func (m LiveModel) statusMessage() string {
	if m.lastError != nil {
		return lipgloss.NewStyle().Foreground(errorColor).Render(m.lastError.Error())
	}
	if m.statusMsg != "" {
		return lipgloss.NewStyle().Foreground(activeColor).Render(m.statusMsg)
	}
	return ""
}

func wrapStatusParts(parts []string, width int) []string {
	if len(parts) == 0 {
		return []string{""}
	}
	if width <= 0 {
		return []string{strings.Join(parts, " ")}
	}
	lines := []string{parts[0]}
	for _, part := range parts[1:] {
		last := lines[len(lines)-1]
		candidate := last + " " + part
		if lipgloss.Width(candidate) <= width {
			lines[len(lines)-1] = candidate
			continue
		}
		lines = append(lines, part)
	}
	return lines
}

func (m LiveModel) renderBeadsPanel(height int) string {
	if height < 1 {
		return ""
	}
	headerLabel := fmt.Sprintf(" beads %d open [b] hide ", m.currentBeads.Count)
	header := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).
		Render("─" + headerLabel + strings.Repeat("─", maxInt(0, m.width-lipgloss.Width(headerLabel)-1)))

	lines := []string{header}
	if m.currentBeads.Err != nil {
		lines = append(lines, lipgloss.NewStyle().Foreground(errorColor).Render("  "+m.currentBeads.Err.Error()))
	} else if m.currentBeads.Count == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(dimColor).Render("  no open beads issues"))
	} else {
		for _, issue := range m.currentBeads.Issues {
			lines = append(lines, m.renderBeadsIssueLines(issue)...)
			if len(lines) >= height {
				break
			}
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines[:height], "\n")
}

func (m LiveModel) renderBeadsIssueLines(issue liveBeadsIssue) []string {
	state := "blocked"
	lineStyle := lipgloss.NewStyle().Foreground(dimColor)
	if issue.Ready {
		state = "ready"
		lineStyle = lipgloss.NewStyle().Foreground(activeColor)
	}
	meta := truncatePlainLine(fmt.Sprintf("  P%d %-7s %s", issue.Priority, state, issue.ID), m.width)
	titleWidth := m.width - 4
	if titleWidth < 1 {
		titleWidth = m.width
	}
	title := truncatePlainLine(issue.Title, titleWidth)
	if m.width >= 4 {
		title = "    " + title
	}
	return []string{
		lineStyle.Render(meta),
		lipgloss.NewStyle().Foreground(dimColor).Render(title),
	}
}

func truncatePlainLine(line string, width int) string {
	if width <= 0 || lipgloss.Width(line) <= width {
		return line
	}
	runes := []rune(line)
	if width <= 1 {
		return ""
	}
	if len(runes) > width-1 {
		runes = runes[:width-1]
	}
	return string(runes) + "…"
}

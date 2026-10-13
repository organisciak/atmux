package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/claude"
	"github.com/porganisciak/agent-tmux/tmux"
)

func agentsModelWith(n, width, height int) agentsModel {
	m := agentsModel{width: width, height: height, agents: make([]tmux.AgentPane, n)}
	return m
}

func TestAgentsCardAtMapsClicksToCards(t *testing.T) {
	m := agentsModelWith(6, 160, 45) // 4 columns of 40, 2 rows stretched to fill
	h := m.cardHeight()
	if h == agentCardMinHeight {
		t.Fatalf("two rows on a 45-line screen should stretch past %d, got %d", agentCardMinHeight, h)
	}
	cases := []struct {
		x, y int
		want int
		ok   bool
	}{
		{0, 0, 0, false},                        // header
		{5, agentsHeaderLines, 0, true},         // first card, top border
		{45, agentsHeaderLines + 3, 1, true},    // second column
		{5, agentsHeaderLines + h - 1, 0, true}, // first card, bottom border
		{5, agentsHeaderLines + h, 4, true},     // second row
		{85, agentsHeaderLines + h, 0, false},   // past the last card
	}
	for _, c := range cases {
		got, ok := m.cardAt(c.x, c.y)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("cardAt(%d,%d) = %d,%v want %d,%v", c.x, c.y, got, ok, c.want, c.ok)
		}
	}
}

func TestAgentsCardAtAccountsForScroll(t *testing.T) {
	m := agentsModelWith(12, 80, agentsHeaderLines+agentCardMinHeight+agentsFooterLines) // 2 columns, one visible row
	m.scrollRow = 2
	if got, ok := m.cardAt(1, agentsHeaderLines); !ok || got != 4 {
		t.Fatalf("cardAt with scroll = %d,%v want 4,true", got, ok)
	}
}

func TestAgentsKeyboardSelectionScrolls(t *testing.T) {
	m := agentsModelWith(12, 80, agentsHeaderLines+agentCardMinHeight+agentsFooterLines)
	for i := 0; i < 3; i++ {
		m.move(m.columns())
	}
	if m.selected != 6 || m.scrollRow != 3 {
		t.Fatalf("selected=%d scrollRow=%d, want 6 and 3", m.selected, m.scrollRow)
	}
}

func TestAgentActivityLabel(t *testing.T) {
	now := time.Now()
	a := tmux.AgentPane{Status: claude.StatusWaiting, WaitingFor: "permission prompt", Since: now.Add(-3 * time.Minute)}
	if got := AgentActivityLabel(a, now); got != "▲ needs input: permission prompt 3m" {
		t.Fatalf("label = %q", got)
	}
}

func TestAgentSummaryPrefersCurrentRecap(t *testing.T) {
	at := time.Now()
	a := tmux.AgentPane{Transcript: claude.Transcript{Recap: "recap", RecapAt: at, LastPromptAt: at.Add(-time.Minute), LastReply: "reply"}}
	if got := AgentSummaryText(a); got != "recap" {
		t.Fatalf("current recap not preferred: %q", got)
	}
	a.Transcript.LastPromptAt = at.Add(time.Minute)
	if got := AgentSummaryText(a); got != "reply" {
		t.Fatalf("stale recap not replaced by the newer reply: %q", got)
	}
}

func TestAgentsCardHeightFillsScreenOnlyWhenEveryRowFits(t *testing.T) {
	avail := 45 - agentsHeaderLines - agentsFooterLines

	m := agentsModelWith(2, 80, 45) // one row of two
	if got := m.cardHeight(); got != avail {
		t.Fatalf("single row height = %d, want the full %d", got, avail)
	}

	m = agentsModelWith(40, 80, 45) // 20 rows cannot fit
	if got := m.cardHeight(); got != agentCardMinHeight {
		t.Fatalf("overflowing grid height = %d, want minimum %d", got, agentCardMinHeight)
	}
}

func TestAgentsColumnKeysResizeAndClamp(t *testing.T) {
	var saved []int
	m := agentsModelWith(8, 160, 45)
	m.onColumnsChange = func(n int) { saved = append(saved, n) }
	press := func(key string) {
		t.Helper()
		next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = next.(agentsModel)
		if cmd != nil {
			cmd()
		}
	}

	if m.columns() != 4 {
		t.Fatalf("auto columns = %d, want 4", m.columns())
	}
	press("+")
	if m.columns() != 3 {
		t.Fatalf("after + columns = %d, want 3", m.columns())
	}
	press("+")
	press("+")
	if m.columns() != 1 || m.columnsOverride != 1 {
		t.Fatalf("columns = %d (override %d), want 1", m.columns(), m.columnsOverride)
	}
	atOne := len(saved)
	press("+")
	if m.columns() != 1 || m.columnsOverride != 1 {
		t.Fatalf("+ at one column changed the layout: columns=%d override=%d", m.columns(), m.columnsOverride)
	}
	if len(saved) != atOne {
		t.Fatalf("+ at one column saved a new size: %v", saved)
	}
	for i := 0; i < 20; i++ {
		press("-")
	}
	if want := 160 / agentCardNarrowest; m.columns() != want {
		t.Fatalf("- should stop at %d columns, got %d", want, m.columns())
	}
	press("0")
	if m.columnsOverride != 0 || m.columns() != 4 {
		t.Fatalf("0 should restore automatic columns, got override=%d columns=%d", m.columnsOverride, m.columns())
	}
	if len(saved) == 0 || saved[0] != 3 || saved[len(saved)-1] != 0 {
		t.Fatalf("size changes not reported for saving: %v", saved)
	}
	// Presses that change nothing are not saved again.
	n := len(saved)
	press("0")
	if len(saved) != n {
		t.Fatalf("no-op press triggered a save: %v", saved)
	}
}

func TestAgentsColumnOverrideClampsWhenTheWindowShrinks(t *testing.T) {
	m := agentsModelWith(8, 160, 45)
	m.columnsOverride = 6
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 45})
	m = next.(agentsModel)
	if want := 60 / agentCardNarrowest; m.columns() != want {
		t.Fatalf("columns after shrink = %d, want %d", m.columns(), want)
	}
	if m.cardWidth()*m.columns() > 60 {
		t.Fatalf("cards overflow: %d x %d > 60", m.columns(), m.cardWidth())
	}
}

func TestAgentCardSectionsReadOnWhenThereIsRoom(t *testing.T) {
	at := time.Now()
	a := tmux.AgentPane{Transcript: claude.Transcript{
		Recap: "the recap", RecapAt: at,
		LastPrompt: "the prompt", LastPromptAt: at.Add(-time.Minute),
		LastReply: "first para", ReplyMore: "more of the reply",
	}}
	got := agentCardSections(a, tmux.ActivityReady)
	want := []string{"the recap", "▸ the prompt", "first para", "more of the reply"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("sections = %q, want %q", got, want)
	}

	small := wrapCardBody(got, 40, 1)
	if len(small) != 1 || small[0] != "the recap…" {
		t.Fatalf("one-line card = %q", small)
	}
	big := wrapCardBody(got, 40, 10)
	if len(big) != 10 || big[2] != "▸ the prompt" || big[6] != "more of the reply" {
		t.Fatalf("large card = %q", big)
	}
	gap := wrapCardBody(got, 40, 2) // cut falls on the gap after the recap
	if gap[0] != "the recap…" || gap[1] != "" {
		t.Fatalf("ellipsis should sit on text, not the gap: %q", gap)
	}
	cut := wrapCardBody(got, 40, 3)
	if cut[2] != "▸ the prompt…" {
		t.Fatalf("cut card should end in an ellipsis, got %q", cut)
	}
}

func TestAgentsFooterFitsOnOneLine(t *testing.T) {
	m := agentsModelWith(40, 60, 30)
	m.executors = make([]tmux.TmuxExecutor, 2) // one host still loading
	lines := strings.Split(m.View(), "\n")
	if footer := lines[len(lines)-1]; lipgloss.Width(footer) > 60 {
		t.Fatalf("footer wraps at width 60: %d cells", lipgloss.Width(footer))
	}
}

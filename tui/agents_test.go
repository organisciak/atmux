package tui

import (
	"testing"
	"time"

	"github.com/porganisciak/agent-tmux/claude"
	"github.com/porganisciak/agent-tmux/tmux"
)

func agentsModelWith(n, width, height int) agentsModel {
	m := agentsModel{width: width, height: height, agents: make([]tmux.AgentPane, n)}
	return m
}

func TestAgentsCardAtMapsClicksToCards(t *testing.T) {
	m := agentsModelWith(6, 160, 45) // 4 columns of 40
	cases := []struct {
		x, y int
		want int
		ok   bool
	}{
		{0, 0, 0, false},                                    // header
		{5, agentsHeaderLines, 0, true},                     // first card, top border
		{45, agentsHeaderLines + 3, 1, true},                // second column
		{5, agentsHeaderLines + agentCardHeight, 4, true},   // second row
		{85, agentsHeaderLines + agentCardHeight, 0, false}, // past the last card
	}
	for _, c := range cases {
		got, ok := m.cardAt(c.x, c.y)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("cardAt(%d,%d) = %d,%v want %d,%v", c.x, c.y, got, ok, c.want, c.ok)
		}
	}
}

func TestAgentsCardAtAccountsForScroll(t *testing.T) {
	m := agentsModelWith(12, 80, 2+agentCardHeight+1) // 2 columns, one visible row
	m.scrollRow = 2
	if got, ok := m.cardAt(1, agentsHeaderLines); !ok || got != 4 {
		t.Fatalf("cardAt with scroll = %d,%v want 4,true", got, ok)
	}
}

func TestAgentsKeyboardSelectionScrolls(t *testing.T) {
	m := agentsModelWith(12, 80, 2+agentCardHeight+1)
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

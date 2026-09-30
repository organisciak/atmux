package tmux

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/porganisciak/agent-tmux/claude"
)

func TestAgentActivity(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		a    AgentPane
		want Activity
	}{
		{"busy", AgentPane{Status: claude.StatusBusy}, ActivityWorking},
		{"waiting", AgentPane{Status: claude.StatusWaiting}, ActivityNeedsInput},
		{"just finished", AgentPane{Status: claude.StatusIdle, Since: now.Add(-5 * time.Minute)}, ActivityReady},
		{"long idle", AgentPane{Status: claude.StatusIdle, Since: now.Add(-DormantAfter)}, ActivityDormant},
		{"never prompted", AgentPane{Status: claude.StatusIdle, Since: now.Add(-time.Hour), Transcript: claude.Transcript{Empty: true}}, ActivityFresh},
		{"idle, unknown since", AgentPane{Status: claude.StatusIdle}, ActivityReady},
		{"no status", AgentPane{}, ActivityUnknown},
	}
	for _, c := range cases {
		if got := c.a.Activity(now); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestScreenStatus(t *testing.T) {
	busy := "⏺ Reading files\n✢ Osmosing… (21s)\n────\n❯ \n────\n  ⏵⏵ auto mode on · esc to interrupt\n"
	if s, _ := screenStatus(busy); s != claude.StatusBusy {
		t.Errorf("busy screen read as %q", s)
	}
	prompt := " Bash command\n   rm -rf build\n Do you want to proceed?\n ❯ 1. Yes\n   2. No\n"
	if s, why := screenStatus(prompt); s != claude.StatusWaiting || why != "permission prompt" {
		t.Errorf("prompt screen read as %q/%q", s, why)
	}
	idle := "✻ Worked for 1m 54s\n────\n❯ \n────\n  ⏵⏵ auto mode on (shift+tab to cycle)\n"
	if s, _ := screenStatus(idle); s != claude.StatusIdle {
		t.Errorf("idle screen read as %q", s)
	}
	if s, _ := screenStatus("  \n"); s != claude.StatusUnknown {
		t.Errorf("blank screen read as %q", s)
	}
}

func TestParseAgentPanesKeepsOnlyClaude(t *testing.T) {
	out := "%1\twork\t0\tagents\t0\t✳ Fix: login bug\t2.1.283\t1\t4242\t1790789277\n" +
		"%2\twork\t1\tdev\t0\thost\tzsh\t1\t4243\t1790789277\n"
	rows := parseAgentPanes(out, NewLocalExecutor())
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if r.pane.Pane.ID != "%1" || r.pane.SessionName != "work" || r.pane.Pane.Target != "work:0.0" || r.panePID != 4242 {
		t.Fatalf("unexpected row %+v", r)
	}
	if !r.lastActivity.Equal(time.Unix(1790789277, 0)) {
		t.Fatalf("last activity = %v", r.lastActivity)
	}
}

func TestAgentsPayloadRoundTrip(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	in := []AgentPane{{
		ClaudePane: ClaudePane{Pane: Pane{ID: "%3", Title: "✳ Cube light", Target: "cube:0.0"}, SessionName: "cube", WindowName: "agents"},
		Process:    claude.Process{SessionID: "abc"},
		Found:      true,
		Status:     claude.StatusWaiting,
		WaitingFor: "permission prompt",
		Since:      now.Add(-2 * time.Minute),
		Transcript: claude.Transcript{Recap: "Next: flash it.", RecapAt: now.Add(-time.Hour), LastPromptAt: now.Add(-2 * time.Hour)},
	}}
	payload := BuildAgentsPayload(in, now)
	if payload.Agents[0].Title != "Cube light" || payload.Agents[0].Activity != string(ActivityNeedsInput) || !payload.Agents[0].RecapCurrent {
		t.Fatalf("payload = %+v", payload.Agents[0])
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	remote := NewRemoteExecutor("vps.example", 22, "ssh", "vps")
	out, err := ParseAgentsPayload(data, remote)
	if err != nil {
		t.Fatal(err)
	}
	got := out[0]
	if got.Host != "vps" || got.Executor != remote || got.Pane.ID != "%3" {
		t.Fatalf("remote agent not attached to its host: %+v", got.ClaudePane)
	}
	if got.Activity(now) != ActivityNeedsInput || got.WaitingFor != "permission prompt" || !got.Transcript.RecapIsCurrent() {
		t.Fatalf("state lost in round trip: %+v", got)
	}
}

func TestParseAgentsPayloadRejectsNewerSchema(t *testing.T) {
	if _, err := ParseAgentsPayload([]byte(`{"schema":99,"agents":[]}`), NewLocalExecutor()); err == nil {
		t.Fatal("newer schema accepted")
	}
}

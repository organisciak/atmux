package tmux

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/porganisciak/agent-tmux/claude"
)

// AgentsSchemaVersion is bumped on incompatible changes to AgentsPayload.
// Additive fields do not need a bump.
const AgentsSchemaVersion = 1

// AgentsPayload is what `atmux agents --json` prints. It is read by scripts
// (status lights, MIDI controllers) and by other atmux instances, which is how
// a remote host's agents show up with their real state and recaps.
type AgentsPayload struct {
	Schema int         `json:"schema"`
	Agents []AgentJSON `json:"agents"`
}

// AgentJSON is one agent pane. Times are RFC 3339 in UTC.
type AgentJSON struct {
	Host         string `json:"host,omitempty"`
	Session      string `json:"session"`
	Window       string `json:"window"`
	Pane         string `json:"pane"`
	Target       string `json:"target"`
	Title        string `json:"title,omitempty"`
	Activity     string `json:"activity"`
	Status       string `json:"status,omitempty"`
	WaitingFor   string `json:"waiting_for,omitempty"`
	Since        string `json:"since,omitempty"`
	SinceSeconds int64  `json:"since_seconds,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	Recap        string `json:"recap,omitempty"`
	RecapAt      string `json:"recap_at,omitempty"`
	RecapCurrent bool   `json:"recap_current,omitempty"`
	LastPrompt   string `json:"last_prompt,omitempty"`
	LastPromptAt string `json:"last_prompt_at,omitempty"`
	LastReply    string `json:"last_reply,omitempty"`
	Fresh        bool   `json:"fresh,omitempty"`
}

// BuildAgentsPayload converts agent panes to their JSON form.
func BuildAgentsPayload(agents []AgentPane, now time.Time) AgentsPayload {
	out := AgentsPayload{Schema: AgentsSchemaVersion, Agents: make([]AgentJSON, 0, len(agents))}
	for _, a := range agents {
		_, title := SplitAgentTitle(a.Pane.Title)
		t := a.Transcript
		out.Agents = append(out.Agents, AgentJSON{
			Host:         a.Host,
			Session:      a.SessionName,
			Window:       a.WindowName,
			Pane:         a.Pane.ID,
			Target:       a.Pane.Target,
			Title:        title,
			Activity:     string(a.Activity(now)),
			Status:       string(a.Status),
			WaitingFor:   a.WaitingFor,
			Since:        formatJSONTime(a.Since),
			SinceSeconds: secondsSince(a.Since, now),
			SessionID:    a.Process.SessionID,
			Recap:        t.Recap,
			RecapAt:      formatJSONTime(t.RecapAt),
			RecapCurrent: t.RecapIsCurrent(),
			LastPrompt:   t.LastPrompt,
			LastPromptAt: formatJSONTime(t.LastPromptAt),
			LastReply:    t.LastReply,
			Fresh:        t.Empty,
		})
	}
	return out
}

// ParseAgentsPayload reads another atmux's agents and attaches them to exec,
// so they can be jumped to like local ones.
func ParseAgentsPayload(data []byte, exec TmuxExecutor) ([]AgentPane, error) {
	var payload AgentsPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if payload.Schema < 1 || payload.Schema > AgentsSchemaVersion {
		return nil, fmt.Errorf("unsupported agents schema %d", payload.Schema)
	}

	agents := make([]AgentPane, 0, len(payload.Agents))
	for _, j := range payload.Agents {
		agents = append(agents, AgentPane{
			ClaudePane: ClaudePane{
				Pane:        Pane{ID: j.Pane, Title: j.Title, Target: j.Target},
				SessionName: j.Session,
				WindowName:  j.Window,
				Host:        exec.HostLabel(),
				Executor:    exec,
			},
			Process:    claude.Process{SessionID: j.SessionID},
			Found:      j.SessionID != "",
			Status:     claude.Status(j.Status),
			WaitingFor: j.WaitingFor,
			Since:      parseJSONTime(j.Since),
			Transcript: claude.Transcript{
				Recap:        j.Recap,
				RecapAt:      parseJSONTime(j.RecapAt),
				LastPrompt:   j.LastPrompt,
				LastPromptAt: parseJSONTime(j.LastPromptAt),
				LastReply:    j.LastReply,
				Empty:        j.Fresh,
			},
		})
	}
	return agents, nil
}

func formatJSONTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func parseJSONTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func secondsSince(t, now time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return int64(now.Sub(t).Seconds())
}

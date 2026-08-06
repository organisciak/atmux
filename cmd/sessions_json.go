package cmd

import (
	"encoding/json"
	"io"
	"time"

	"github.com/porganisciak/agent-tmux/tmux"
)

// SessionsSchemaVersion identifies the shape of the `atmux sessions --json`
// payload. This output is a compatibility surface between atmux versions: a
// newer atmux reads it from an older remote host and vice versa. Bump the major
// version only for changes that break existing readers; adding an optional
// field does not qualify.
const SessionsSchemaVersion = 1

// SessionsPayload is the top-level envelope emitted by `atmux sessions --json`.
type SessionsPayload struct {
	SchemaVersion int           `json:"schema_version"`
	AtmuxVersion  string        `json:"atmux_version"`
	GeneratedAt   time.Time     `json:"generated_at"`
	Sessions      []SessionJSON `json:"sessions"`
}

// SessionJSON is one tmux session as reported to another atmux instance.
type SessionJSON struct {
	Name       string `json:"name"`
	Display    string `json:"display"`
	Attached   bool   `json:"attached"`
	WorkingDir string `json:"working_dir,omitempty"`
	Activity   int64  `json:"activity,omitempty"`
	Color      string `json:"color,omitempty"`

	// BeadsOpen is nil when the session's directory is not a beads project,
	// which is distinct from a project with zero open issues.
	BeadsOpen *int `json:"beads_open,omitempty"`
}

// buildSessionsPayload converts collected session lines into the wire format.
// It takes lines from the same collection path the TUI uses, so the two cannot
// report different data.
func buildSessionsPayload(lines []tmux.SessionLine, now time.Time, includeBeads bool) SessionsPayload {
	sessions := make([]SessionJSON, 0, len(lines))
	for _, line := range lines {
		s := SessionJSON{
			Name:       line.Name,
			Display:    line.Line,
			Attached:   line.Attached,
			WorkingDir: line.WorkingDir,
			Activity:   line.Activity,
			Color:      line.Color,
		}
		if includeBeads {
			if count, ok := tmux.BeadsOpenCount(line.WorkingDir); ok {
				s.BeadsOpen = &count
			}
		}
		sessions = append(sessions, s)
	}

	return SessionsPayload{
		SchemaVersion: SessionsSchemaVersion,
		AtmuxVersion:  Version,
		GeneratedAt:   now.UTC(),
		Sessions:      sessions,
	}
}

// runSessionsJSON writes the local session list as JSON and nothing else.
// A host with no tmux server is an empty list, not an error: the caller needs
// to tell "no sessions" apart from "could not ask".
func runSessionsJSON(out io.Writer, includeBeads bool) error {
	lines, err := tmux.ListSessionsRawWithExecutor(tmux.NewLocalExecutor())
	if err != nil {
		return err
	}

	payload := buildSessionsPayload(lines, time.Now(), includeBeads)

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}

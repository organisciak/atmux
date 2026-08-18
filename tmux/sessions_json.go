package tmux

import (
	"encoding/json"
	"fmt"
	"time"
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
	// Title is the agent's own session name, e.g. Claude Code's conversation
	// title. Omitted when the session has no agent pane.
	Title string `json:"title,omitempty"`

	// BeadsOpen is nil when the session's directory is not a beads project,
	// which is distinct from a project with zero open issues.
	BeadsOpen *int `json:"beads_open,omitempty"`
}

// BuildSessionsPayload converts collected session lines into the wire format.
// It takes lines from the same collection path the TUI uses, so the two cannot
// report different data.
func BuildSessionsPayload(lines []SessionLine, version string, now time.Time, includeBeads bool) SessionsPayload {
	sessions := make([]SessionJSON, 0, len(lines))
	for _, line := range lines {
		s := SessionJSON{
			Name:       line.Name,
			Display:    line.Line,
			Attached:   line.Attached,
			WorkingDir: line.WorkingDir,
			Activity:   line.Activity,
			Color:      line.Color,
			Title:      line.Title,
		}
		if includeBeads {
			if count, ok := BeadsOpenCount(line.WorkingDir); ok {
				s.BeadsOpen = &count
			}
		}
		sessions = append(sessions, s)
	}

	return SessionsPayload{
		SchemaVersion: SessionsSchemaVersion,
		AtmuxVersion:  version,
		GeneratedAt:   now.UTC(),
		Sessions:      sessions,
	}
}

// ParseSessionsPayload decodes a payload produced by `atmux sessions --json`
// and converts it back into session lines tagged with host.
//
// A payload whose schema is newer than this build understands is rejected
// rather than guessed at, so the caller can fall back to plain tmux.
func ParseSessionsPayload(data []byte, host string) ([]SessionLine, error) {
	var payload SessionsPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("malformed sessions payload from %s: %w", host, err)
	}
	if payload.SchemaVersion > SessionsSchemaVersion {
		return nil, fmt.Errorf("sessions payload from %s uses schema %d, this build understands %d",
			host, payload.SchemaVersion, SessionsSchemaVersion)
	}

	lines := make([]SessionLine, 0, len(payload.Sessions))
	for _, s := range payload.Sessions {
		lines = append(lines, SessionLine{
			Name:       s.Name,
			Line:       s.Display,
			Host:       host,
			Activity:   s.Activity,
			WorkingDir: s.WorkingDir,
			Color:      s.Color,
			Attached:   s.Attached,
			Beads:      s.BeadsOpen,
			Title:      s.Title,
		})
	}
	return lines, nil
}

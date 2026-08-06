package cmd

import (
	"encoding/json"
	"io"
	"time"

	"github.com/porganisciak/agent-tmux/tmux"
)

// runSessionsJSON writes the local session list as JSON and nothing else.
// A host with no tmux server is an empty list, not an error: the caller needs
// to tell "no sessions" apart from "could not ask".
func runSessionsJSON(out io.Writer, includeBeads bool) error {
	lines, err := tmux.ListSessionsRawWithExecutor(tmux.NewLocalExecutor())
	if err != nil {
		return err
	}

	payload := tmux.BuildSessionsPayload(lines, Version, time.Now(), includeBeads)

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}

// Package claude reads the state Claude Code leaves on disk: the per-process
// session registry and the per-session transcripts.
//
// Neither is a documented interface. Everything here degrades to "unknown"
// rather than failing, so a Claude Code release that changes the format costs
// atmux its status column, not its session list.
package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Status is what a Claude Code process reports it is doing.
type Status string

const (
	// StatusBusy means a turn is running: the model is thinking, streaming,
	// running tools, or waiting on a subagent.
	StatusBusy Status = "busy"
	// StatusWaiting means Claude is blocked on the user: a permission prompt,
	// a question, or an open dialog. WaitingFor says which.
	StatusWaiting Status = "waiting"
	// StatusIdle means the turn finished and the prompt is empty.
	StatusIdle Status = "idle"
	// StatusUnknown means the registry entry carried no status.
	StatusUnknown Status = ""
)

// Process is one live Claude Code process from the session registry.
type Process struct {
	PID       int
	SessionID string
	Cwd       string
	Name      string
	Version   string

	Status Status
	// WaitingFor explains a StatusWaiting, e.g. "permission prompt",
	// "input needed", "dialog open".
	WaitingFor string
	// Since is when Status last changed, so an idle process's Since is when
	// its last turn finished.
	Since time.Time

	// TmuxPane is the pane id ("%121") Claude recorded for itself, or "" when
	// it was not started inside tmux.
	TmuxPane string
}

// registryEntry mirrors the fields of ~/.claude/sessions/<pid>.json we use.
type registryEntry struct {
	PID             int    `json:"pid"`
	SessionID       string `json:"sessionId"`
	Cwd             string `json:"cwd"`
	Name            string `json:"name"`
	Version         string `json:"version"`
	Kind            string `json:"kind"`
	Status          string `json:"status"`
	WaitingFor      string `json:"waitingFor"`
	StatusUpdatedAt int64  `json:"statusUpdatedAt"`
	Tmux            string `json:"tmux"`
}

// Dir returns Claude Code's config directory, honouring CLAUDE_CONFIG_DIR.
func Dir() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// LiveProcesses reads the session registry and returns the entries whose
// process is still running. Claude removes its entry on a clean exit, but a
// crash or kill -9 leaves one behind, so liveness is checked rather than
// trusted.
func LiveProcesses() []Process {
	return liveProcessesIn(filepath.Join(Dir(), "sessions"), processAlive)
}

func liveProcessesIn(dir string, alive func(int) bool) []Process {
	paths, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	procs := make([]Process, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		p, ok := parseRegistryEntry(data)
		if !ok || !alive(p.PID) {
			continue
		}
		procs = append(procs, p)
	}
	return procs
}

func parseRegistryEntry(data []byte) (Process, bool) {
	var e registryEntry
	if err := json.Unmarshal(data, &e); err != nil || e.PID <= 0 {
		return Process{}, false
	}
	// Background jobs have no terminal to switch to.
	if e.Kind != "" && e.Kind != "interactive" {
		return Process{}, false
	}

	p := Process{
		PID:        e.PID,
		SessionID:  e.SessionID,
		Cwd:        e.Cwd,
		Name:       e.Name,
		Version:    e.Version,
		Status:     Status(e.Status),
		WaitingFor: e.WaitingFor,
		TmuxPane:   paneFromTmuxField(e.Tmux),
	}
	if e.StatusUpdatedAt > 0 {
		p.Since = time.UnixMilli(e.StatusUpdatedAt)
	}
	return p, true
}

// paneFromTmuxField extracts the pane id from Claude's "session:@window.%pane"
// record. Pane ids are stable for the pane's lifetime, unlike session and
// window names, which the user can rename.
func paneFromTmuxField(field string) string {
	i := strings.LastIndex(field, "%")
	if i < 0 {
		return ""
	}
	return field[i:]
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	// EPERM means the process exists but belongs to someone else.
	return err == nil || err == syscall.EPERM
}

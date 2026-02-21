package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/porganisciak/agent-tmux/config"
)

// Session represents a tmux session configuration
type Session struct {
	Name       string
	WorkingDir string
}

// SessionLine mirrors a single line from `tmux list-sessions`.
type SessionLine struct {
	Name     string
	Line     string
	Host     string // Remote host label (empty for local)
	Activity int64  // Unix timestamp of last activity (for sorting)
}

// NewSession creates a new session configuration based on the current directory
func NewSession(workingDir string) *Session {
	basename := filepath.Base(workingDir)
	// Sanitize: replace non-alphanumeric (except _ and -) with _
	reg := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	slug := reg.ReplaceAllString(basename, "_")

	return &Session{
		Name:       "agent-" + slug,
		WorkingDir: workingDir,
	}
}

// Exists checks if the tmux session already exists
func (s *Session) Exists() bool {
	cmd := exec.Command("tmux", "has-session", "-t", s.Name)
	return cmd.Run() == nil
}

// DefaultAgents returns the default agent commands when no config is provided
func DefaultAgents() []config.AgentConfig {
	return []config.AgentConfig{
		{Command: "claude --dangerously-skip-permissions"},
		{Command: "codex --full-auto"},
	}
}

// Create creates a new tmux session with the agents window
func (s *Session) Create(cfg *config.Config) error {
	// Determine which agents to use
	agents := DefaultAgents()
	if cfg != nil && len(cfg.CoreAgents) > 0 {
		agents = cfg.CoreAgents
	}

	// Create session with agents window
	if err := s.run("new-session", "-d", "-s", s.Name, "-n", "agents", "-c", s.WorkingDir); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Set up agent panes
	for i, agent := range agents {
		if i == 0 {
			// First agent uses the initial pane
			s.run("send-keys", "-t", s.Name+":agents.0", agent.Command, "C-m")
		} else {
			// Subsequent agents split horizontally
			s.run("split-window", "-h", "-t", s.Name+":agents", "-c", s.WorkingDir)
			s.run("send-keys", "-t", s.Name+":agents", agent.Command, "C-m")
		}
	}

	// Select first pane
	s.run("select-pane", "-t", s.Name+":agents.0")

	return nil
}

// ApplyConfig applies project-specific configuration
func (s *Session) ApplyConfig(cfg *config.Config) error {
	// Add panes to agents window
	for _, pane := range cfg.AgentPanes {
		splitFlag := "-h"
		if pane.Vertical {
			splitFlag = "-v"
		}
		s.run("split-window", splitFlag, "-t", s.Name+":agents", "-c", s.WorkingDir)
		s.run("send-keys", "-t", s.Name+":agents", pane.Command, "C-m")
	}

	// Create new windows
	for _, window := range cfg.Windows {
		s.run("new-window", "-t", s.Name, "-n", window.Name, "-c", s.WorkingDir)

		for i, pane := range window.Panes {
			if i == 0 {
				// First pane uses the existing pane in the new window
				s.run("send-keys", "-t", s.Name+":"+window.Name, pane.Command, "C-m")
			} else {
				// Subsequent panes need to split
				splitFlag := "-h"
				if pane.Vertical {
					splitFlag = "-v"
				}
				s.run("split-window", splitFlag, "-t", s.Name+":"+window.Name, "-c", s.WorkingDir)
				s.run("send-keys", "-t", s.Name+":"+window.Name, pane.Command, "C-m")
			}
		}
	}

	return nil
}

// SelectDefault selects the default window and pane
func (s *Session) SelectDefault() {
	s.run("select-window", "-t", s.Name+":agents")
	s.run("select-pane", "-t", s.Name+":agents.0")
}

// Attach attaches to the tmux session
func (s *Session) Attach() error {
	cmd := exec.Command("tmux", "attach-session", "-t", s.Name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// AttachToSession attaches or switches to the given tmux session.
func AttachToSession(name string) error {
	if name == "" {
		return nil
	}
	if os.Getenv("TMUX") != "" {
		return exec.Command("tmux", "switch-client", "-t", name).Run()
	}
	cmd := exec.Command("tmux", "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Kill kills the tmux session
func (s *Session) Kill() error {
	return s.run("kill-session", "-t", s.Name)
}

// run executes a tmux command
func (s *Session) run(args ...string) error {
	cmd := exec.Command("tmux", args...)
	cmd.Dir = s.WorkingDir
	return cmd.Run()
}

// ListSessions returns all atmux (agent-tmux) sessions
func ListSessions() ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		// No server running or no sessions is not an error
		return []string{}, nil
	}

	var sessions []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.HasPrefix(line, "agent-") || strings.HasPrefix(line, "atmux-") {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}

// sessionListFormat is the tmux format string used for list-sessions.
// It prepends the activity timestamp (tab-separated) to a display line
// that closely matches the default tmux output.
const sessionListFormat = `#{session_activity}	#{session_name}: #{session_windows} windows (created #{t:session_created})#{?session_attached, (attached),}`

// ListSessionsRaw returns tmux list-sessions output with parsed names,
// sorted by most recently active first.
func ListSessionsRaw() ([]SessionLine, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", sessionListFormat)
	output, err := cmd.Output()
	if err != nil {
		if isNoServerError(err) {
			return []SessionLine{}, nil
		}
		return nil, err
	}

	sessions := parseSessionLines(string(output))
	sortSessionsByActivity(sessions)
	return sessions, nil
}

// parseSessionLines parses lines from the activity-prefixed format.
func parseSessionLines(output string) []SessionLine {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var sessions []SessionLine
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		sessions = append(sessions, parseSessionLine(line))
	}
	return sessions
}

func parseSessionLine(line string) SessionLine {
	trimmed := strings.TrimSpace(line)

	var activity int64
	displayLine := trimmed

	// Parse "activity\tdisplay_line" format
	if idx := strings.IndexByte(trimmed, '\t'); idx != -1 {
		if ts, err := strconv.ParseInt(trimmed[:idx], 10, 64); err == nil {
			activity = ts
			displayLine = trimmed[idx+1:]
		}
	}

	name := displayLine
	if idx := strings.Index(displayLine, ":"); idx != -1 {
		name = displayLine[:idx]
	}
	return SessionLine{Name: name, Line: displayLine, Activity: activity}
}

// sortSessionsByActivity sorts sessions by activity timestamp, most recent first.
func sortSessionsByActivity(sessions []SessionLine) {
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].Activity > sessions[j].Activity
	})
}

// KillSession kills a session by name
func KillSession(name string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	return cmd.Run()
}

// ListSessionsRawWithExecutor returns tmux list-sessions output using the given executor,
// sorted by most recently active first.
func ListSessionsRawWithExecutor(exec TmuxExecutor) ([]SessionLine, error) {
	output, err := exec.Output("list-sessions", "-F", sessionListFormat)
	if err != nil {
		if isNoServerError(err) {
			return []SessionLine{}, nil
		}
		return nil, err
	}

	sessions := parseSessionLines(string(output))
	host := exec.HostLabel()
	for i := range sessions {
		sessions[i].Host = host
	}
	sortSessionsByActivity(sessions)
	return sessions, nil
}

// AttachToSessionWithExecutor attaches or switches to the given tmux session
// using the provided executor. For local sessions it behaves like AttachToSession;
// for remote sessions it uses the executor's Interactive method.
func AttachToSessionWithExecutor(name string, exec TmuxExecutor) error {
	return AttachToSessionWithStrategy(name, exec, config.AttachStrategyAuto)
}

// AttachToSessionWithStrategy attaches to a session using the given strategy.
// For local sessions the strategy is ignored (standard switch-client / attach behavior).
// For remote sessions:
//   - auto: new tmux window when inside tmux, direct attach otherwise
//   - replace: always attach directly (replaces current terminal)
//   - new-window: always open a new tmux window for the remote session
func AttachToSessionWithStrategy(name string, executor TmuxExecutor, strategy config.AttachStrategy) error {
	if name == "" {
		return nil
	}
	if !executor.IsRemote() {
		return AttachToSession(name)
	}

	insideTmux := os.Getenv("TMUX") != ""

	switch strategy {
	case config.AttachStrategyReplace:
		return executor.Interactive("attach-session", "-t", name)
	case config.AttachStrategyNewWindow:
		if !insideTmux {
			// Can't create a tmux window if we're not inside tmux
			return executor.Interactive("attach-session", "-t", name)
		}
		return attachRemoteInNewWindow(name, executor)
	default: // auto
		if insideTmux {
			return attachRemoteInNewWindow(name, executor)
		}
		return executor.Interactive("attach-session", "-t", name)
	}
}

// attachRemoteInNewWindow opens a new local tmux window that runs the remote
// attach command (SSH or mosh) for the given session.
func attachRemoteInNewWindow(name string, executor TmuxExecutor) error {
	re, ok := executor.(*RemoteExecutor)
	if !ok {
		// Fallback: not a RemoteExecutor, attach directly
		return executor.Interactive("attach-session", "-t", name)
	}

	windowName := "remote:" + name
	var shellCmd []string

	if re.AttachMethod == "mosh" && moshAvailable() {
		shellCmd = re.buildMoshArgs("attach-session", "-t", name)
		shellCmd = append([]string{"mosh"}, shellCmd...)
	} else {
		shellCmd = re.buildSSHInteractiveArgs("attach-session", "-t", name)
		shellCmd = append([]string{"ssh"}, shellCmd...)
	}

	// Join into a single shell string for tmux new-window
	cmdStr := shellQuoteJoin(shellCmd)
	tmuxCmd := exec.Command("tmux", "new-window", "-n", windowName, cmdStr)
	return tmuxCmd.Run()
}

// shellQuoteJoin joins args into a shell command string, quoting args that
// contain spaces.
func shellQuoteJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t'\"\\") {
			quoted[i] = "'" + strings.ReplaceAll(a, "'", "'\\''") + "'"
		} else {
			quoted[i] = a
		}
	}
	return strings.Join(quoted, " ")
}

// GetSessionPath returns the working directory of a tmux session.
func GetSessionPath(name string) string {
	cmd := exec.Command("tmux", "display-message", "-t", name, "-p", "#{session_path}")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func isNoServerError(err error) bool {
	if err == nil {
		return false
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		if isNoServerStderr(string(exitErr.Stderr)) {
			return true
		}
	}

	return isNoServerStderr(err.Error())
}

func isNoServerStderr(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "no server running") ||
		strings.Contains(lower, "failed to connect to server") ||
		strings.Contains(lower, "error connecting to")
}

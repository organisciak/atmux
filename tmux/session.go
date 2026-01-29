package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/porganisciak/agent-tmux/config"
)

// Session represents a tmux session configuration
type Session struct {
	Name       string
	WorkingDir string
	DiagScript string
}

// NewSession creates a new session configuration based on the current directory
func NewSession(workingDir string) *Session {
	basename := filepath.Base(workingDir)
	// Sanitize: replace non-alphanumeric (except _ and -) with _
	reg := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	slug := reg.ReplaceAllString(basename, "_")

	// Find the diag script (look relative to the executable)
	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)
	diagScript := filepath.Join(execDir, "agent-tmux-diag.sh")

	// Fallback to looking in the working dir's parent bin folder
	if _, err := os.Stat(diagScript); os.IsNotExist(err) {
		// Try common locations
		homeDir, _ := os.UserHomeDir()
		possiblePaths := []string{
			filepath.Join(homeDir, "bin", "agent-tmux-diag.sh"),
			"/usr/local/bin/agent-tmux-diag.sh",
			"/opt/homebrew/bin/agent-tmux-diag.sh",
		}
		for _, p := range possiblePaths {
			if _, err := os.Stat(p); err == nil {
				diagScript = p
				break
			}
		}
	}

	return &Session{
		Name:       "agent-" + slug,
		WorkingDir: workingDir,
		DiagScript: diagScript,
	}
}

// Exists checks if the tmux session already exists
func (s *Session) Exists() bool {
	cmd := exec.Command("tmux", "has-session", "-t", s.Name)
	return cmd.Run() == nil
}

// Create creates a new tmux session with the default layout
func (s *Session) Create() error {
	// Create session with agents window
	if err := s.run("new-session", "-d", "-s", s.Name, "-n", "agents", "-c", s.WorkingDir); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Split horizontally for claude pane
	if err := s.run("split-window", "-h", "-t", s.Name+":agents", "-c", s.WorkingDir); err != nil {
		return fmt.Errorf("failed to split window: %w", err)
	}

	// Set up codex pane (pane 0)
	s.run("select-pane", "-t", s.Name+":agents.0")
	s.run("select-pane", "-T", "codex")
	s.run("send-keys", "-t", s.Name+":agents.0", "codex --yolo", "C-m")

	// Set up claude pane (pane 1)
	s.run("select-pane", "-t", s.Name+":agents.1")
	s.run("select-pane", "-T", "claude")
	s.run("send-keys", "-t", s.Name+":agents.1", "claude code --yolo", "C-m")

	// Create diag window
	s.run("new-window", "-t", s.Name, "-n", "diag", "-c", s.WorkingDir)
	if s.DiagScript != "" {
		s.run("send-keys", "-t", s.Name+":diag", s.DiagScript, "C-m")
	}

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

// ListSessions returns all agent-tmux sessions
func ListSessions() ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		// No server running or no sessions is not an error
		return []string{}, nil
	}

	var sessions []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.HasPrefix(line, "agent-") {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}

// KillSession kills a session by name
func KillSession(name string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	return cmd.Run()
}

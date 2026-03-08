package tmux

import (
	"strings"
)

// ClaudePane represents a pane running Claude Code
type ClaudePane struct {
	Pane        Pane
	SessionName string
	WindowName  string
	Host        string // empty for local
	Executor    TmuxExecutor
}

// Label returns a human-readable label for display
func (cp ClaudePane) Label() string {
	var parts []string
	if cp.Host != "" {
		parts = append(parts, "["+cp.Host+"]")
	}
	parts = append(parts, cp.SessionName+":"+cp.WindowName+"."+strings.TrimPrefix(cp.Pane.Target, cp.SessionName+":"))
	return strings.Join(parts, " ")
}

// FindClaudePanes scans all panes across executors and returns those running Claude Code.
// Detection uses pane_title (Claude Code sets it to contain "Claude Code"),
// with fallbacks to pane_current_command and pane content inspection.
func FindClaudePanes(executors []TmuxExecutor) []ClaudePane {
	var results []ClaudePane

	for _, exec := range executors {
		tree, err := fetchTreeWithExecutor(exec)
		if err != nil {
			continue
		}

		host := exec.HostLabel()
		for _, sess := range tree.Sessions {
			for _, win := range sess.Windows {
				for _, pane := range win.Panes {
					if isClaudePane(pane) {
						results = append(results, ClaudePane{
							Pane:        pane,
							SessionName: sess.Name,
							WindowName:  win.Name,
							Host:        host,
							Executor:    exec,
						})
					}
				}
			}
		}
	}

	return results
}

// isClaudePane returns true if the pane appears to be running Claude Code.
// Claude Code sets pane_title to contain "Claude Code" and reports its semver
// as pane_current_command (e.g. "2.1.71"). A pane whose title says "Claude Code"
// but whose command is a regular shell (zsh, bash, etc.) has likely exited.
func isClaudePane(pane Pane) bool {
	cmd := strings.ToLower(pane.Command)

	// Direct command match
	if cmd == "claude" || cmd == "claude-code" {
		return true
	}
	// Version number as command (Claude's signature behavior)
	if looksLikeVersion(pane.Command) {
		return true
	}
	// Title match, but only if the process isn't a plain shell
	// (shell means Claude exited and left a stale title)
	if strings.Contains(pane.Title, "Claude Code") && !isShellCommand(cmd) {
		return true
	}
	return false
}

// isShellCommand returns true for common shell process names.
func isShellCommand(cmd string) bool {
	switch cmd {
	case "zsh", "bash", "sh", "fish", "tcsh", "csh", "dash", "ksh":
		return true
	}
	return false
}

// looksLikeVersion returns true if s looks like a semver (e.g. "2.1.71").
func looksLikeVersion(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

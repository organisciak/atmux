package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/porganisciak/agent-tmux/tui"
	"github.com/spf13/cobra"
)

var sessionsCmd = &cobra.Command{
	Use:     "sessions [session-name]",
	Aliases: []string{"lsessions", "list-sessions", "list", "ls", "attach"},
	Short:   "List sessions or attach directly by name",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runSessions,
}

var (
	sessionsInline         bool
	sessionsNoPopup        bool
	sessionsNonInteractive bool
)

func init() {
	rootCmd.AddCommand(sessionsCmd)
	sessionsCmd.Flags().BoolVar(&sessionsInline, "inline", true, "Render without alt screen (non-fullscreen)")
	sessionsCmd.Flags().BoolVar(&sessionsNoPopup, "no-popup", false, "Disable popup mode (default: popup when inside tmux)")
	sessionsCmd.Flags().BoolVarP(&sessionsNonInteractive, "non-interactive", "n", false, "Print sessions and exit (no TUI)")
}

func runSessions(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return attachToSession(args[0])
	}

	// Non-interactive mode: just print and exit
	if sessionsNonInteractive {
		lines, err := tmux.ListSessionsRaw()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		for _, line := range lines {
			fmt.Fprintln(out, line.Line)
		}
		return nil
	}

	// Default to popup when inside tmux, unless --no-popup is set
	insideTmux := os.Getenv("TMUX") != ""
	if insideTmux && !sessionsNoPopup && !sessionsInline {
		return launchAsPopup("sessions")
	}

	result, err := tui.RunSessionsList(tui.SessionsOptions{
		AltScreen: !sessionsInline,
	})
	if err != nil {
		return err
	}
	if result.SessionName == "" {
		return nil
	}

	if result.IsFromHistory {
		// Revival from history - create new session in that directory
		session := tmux.NewSession(result.WorkingDir)
		return runDirectAttach(session, result.WorkingDir)
	}

	// Attach to existing session
	if sessionPath := tmux.GetSessionPath(result.SessionName); sessionPath != "" {
		saveHistory(filepath.Base(sessionPath), sessionPath, result.SessionName)
	}
	return tmux.AttachToSession(result.SessionName)
}

func attachToSession(name string) error {
	sessionName := name
	if !strings.HasPrefix(sessionName, "agent-") && !strings.HasPrefix(sessionName, "atmux-") {
		sessionName = "agent-" + sessionName
	}

	session := &tmux.Session{Name: sessionName}
	if !session.Exists() {
		return fmt.Errorf("session %s does not exist\nUse 'atmux sessions' to see active sessions", sessionName)
	}

	return session.Attach()
}

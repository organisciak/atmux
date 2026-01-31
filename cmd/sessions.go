package cmd

import (
	"fmt"
	"os"

	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/porganisciak/agent-tmux/tui"
	"github.com/spf13/cobra"
)

var sessionsCmd = &cobra.Command{
	Use:     "sessions",
	Aliases: []string{"lsessions", "list-sessions"},
	Short:   "List all tmux sessions with click-to-attach",
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

	return tui.RunSessionsList(tui.SessionsOptions{
		AltScreen: !sessionsInline,
	})
}

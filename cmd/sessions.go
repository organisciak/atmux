package cmd

import (
	"github.com/porganisciak/agent-tmux/tui"
	"github.com/spf13/cobra"
)

var sessionsCmd = &cobra.Command{
	Use:     "sessions",
	Aliases: []string{"lsessions", "list-sessions"},
	Short:   "List all tmux sessions with click-to-attach",
	RunE:    runSessions,
}

func init() {
	rootCmd.AddCommand(sessionsCmd)
	sessionsCmd.Flags().BoolVar(&sessionsInline, "inline", false, "Render without alt screen (non-fullscreen)")
}

func runSessions(cmd *cobra.Command, args []string) error {
	return tui.RunSessionsList(tui.SessionsOptions{
		AltScreen: !sessionsInline,
	})
}

var sessionsInline bool

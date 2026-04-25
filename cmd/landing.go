package cmd

import (
	"github.com/spf13/cobra"
)

// landingCmd is kept for backwards compatibility; it now delegates to sessions.
var landingCmd = &cobra.Command{
	Use:    "landing",
	Short:  "Show sessions (deprecated: use 'sessions')",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessions(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(landingCmd)
}

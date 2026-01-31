package cmd

import (
	"fmt"

	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List active atmux sessions",
	RunE:    runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	sessions, err := tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No active atmux sessions")
		return nil
	}

	fmt.Println("Active sessions:")
	for _, s := range sessions {
		fmt.Printf("  %s\n", s)
	}
	return nil
}

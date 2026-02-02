package cmd

import (
	"fmt"
	"strings"

	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/spf13/cobra"
)

var killAll bool

var killCmd = &cobra.Command{
	Use:   "kill [session-name]",
	Short: "Kill an atmux session",
	Long: `Kill a specific atmux session by name, or all sessions with --all.

If no session name is provided and you're in a project directory,
it will kill the session for that project.`,
	RunE: runKill,
}

func init() {
	rootCmd.AddCommand(killCmd)
	killCmd.Flags().BoolVarP(&killAll, "all", "a", false, "Kill all atmux sessions")
}

func runKill(cmd *cobra.Command, args []string) error {
	if killAll {
		return killAllSessions()
	}

	if len(args) == 0 {
		// Try to kill session for current directory
		session := tmux.NewSession(".")
		if !session.Exists() {
			return fmt.Errorf("no session found for current directory\nUse 'atmux sessions' to see active sessions")
		}
		return killSession(session.Name)
	}

	// Kill specified session
	name := args[0]
	if !strings.HasPrefix(name, "agent-") && !strings.HasPrefix(name, "atmux-") {
		name = "agent-" + name
	}
	return killSession(name)
}

func killSession(name string) error {
	if err := tmux.KillSession(name); err != nil {
		return fmt.Errorf("failed to kill session %s: %w", name, err)
	}
	fmt.Printf("Killed session: %s\n", name)
	return nil
}

func killAllSessions() error {
	sessions, err := tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No active atmux sessions to kill")
		return nil
	}

	for _, s := range sessions {
		if err := tmux.KillSession(s); err != nil {
			fmt.Printf("Failed to kill %s: %v\n", s, err)
		} else {
			fmt.Printf("Killed: %s\n", s)
		}
	}
	return nil
}

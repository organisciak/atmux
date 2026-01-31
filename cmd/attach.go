package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/spf13/cobra"
)

var attachCmd = &cobra.Command{
	Use:   "attach [session-name]",
	Short: "Attach to an existing atmux session",
	Long: `Attach to an existing atmux session by name.

If no session name is provided and you're in a project directory,
it will attach to the session for that project (if it exists).`,
	RunE: runAttach,
}

func init() {
	rootCmd.AddCommand(attachCmd)
}

func runAttach(cmd *cobra.Command, args []string) error {
	var sessionName string

	if len(args) == 0 {
		// Try to attach to session for current directory
		workingDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
		session := tmux.NewSession(workingDir)
		sessionName = session.Name
	} else {
		sessionName = args[0]
		if !strings.HasPrefix(sessionName, "agent-") && !strings.HasPrefix(sessionName, "atmux-") {
			sessionName = "agent-" + sessionName
		}
	}

	// Create a temporary session object for attaching
	session := &tmux.Session{Name: sessionName}
	if !session.Exists() {
		return fmt.Errorf("session %s does not exist\nUse 'atmux list' to see active sessions", sessionName)
	}

	return session.Attach()
}

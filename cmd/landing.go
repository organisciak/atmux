package cmd

import (
	"fmt"
	"os"

	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/spf13/cobra"
)

var landingCmd = &cobra.Command{
	Use:   "landing",
	Short: "Show the landing page",
	Long:  "Show the interactive landing page for session selection.",
	RunE:  runLandingCmd,
}

func init() {
	rootCmd.AddCommand(landingCmd)
}

func runLandingCmd(cmd *cobra.Command, args []string) error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	session := tmux.NewSession(workingDir)
	return runLandingPage(session, workingDir)
}

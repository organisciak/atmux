package cmd

import (
	"os"

	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/porganisciak/agent-tmux/tui"
	"github.com/spf13/cobra"
)

var openCmd = &cobra.Command{
	Use:   "open",
	Short: "Quick session selector with number shortcuts",
	Long: `Open provides a streamlined TUI for quickly jumping into sessions.

Press 1-9 to instantly select a session, or use arrow keys and Enter.
Tab switches between Active and Recent sessions.`,
	RunE: runOpen,
}

var openNoPopup bool

func init() {
	rootCmd.AddCommand(openCmd)
	openCmd.Flags().BoolVar(&openNoPopup, "no-popup", false, "Disable popup mode (default: popup when inside tmux)")
}

func runOpen(cmd *cobra.Command, args []string) error {
	// Default to popup when inside tmux
	insideTmux := os.Getenv("TMUX") != ""
	if insideTmux && !openNoPopup {
		return launchAsPopup("open --no-popup")
	}

	result, err := tui.RunOpen()
	if err != nil {
		return err
	}

	if result.SessionName == "" {
		return nil
	}

	if result.IsFromHistory {
		// Revival from history
		session := tmux.NewSession(result.WorkingDir)
		return runDirectAttach(session, result.WorkingDir)
	}

	// Attach to existing session
	return tmux.AttachToSession(result.SessionName)
}

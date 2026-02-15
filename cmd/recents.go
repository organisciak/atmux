package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/history"
	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/porganisciak/agent-tmux/tui"
	"github.com/spf13/cobra"
)

var recentsCmd = &cobra.Command{
	Use:     "recents",
	Aliases: []string{"recent"},
	Short:   "Browse and revive recent sessions from history",
	Long: `Opens an interactive TUI to browse recent sessions from history.

Select a session to revive it (create a new session in that directory).

Controls:
  Up/Down or j/k   Navigate list
  Enter            Revive selected session
  x/Delete         Remove entry from history
  /                Filter sessions (type to search)
  q/Esc            Quit`,
	RunE: runRecents,
}

var (
	recentsNoPopup   bool
	recentsList      bool
	recentsLimit     int
	recentsHidePaths bool
)

func init() {
	rootCmd.AddCommand(recentsCmd)
	recentsCmd.Flags().BoolVar(&recentsNoPopup, "no-popup", false, "Disable popup mode (default: popup when inside tmux)")
	recentsCmd.Flags().BoolVarP(&recentsList, "list", "l", false, "List recent sessions (non-interactive)")
	recentsCmd.Flags().IntVar(&recentsLimit, "limit", 20, "Maximum number of sessions to show")
	recentsCmd.Flags().BoolVar(&recentsHidePaths, "hide-paths", false, hidePathsHelpText)
}

func runRecents(cmd *cobra.Command, args []string) error {
	// Non-interactive list mode
	if recentsList {
		return runRecentsList(cmd)
	}

	// Default to popup when inside tmux, unless --no-popup is set
	insideTmux := os.Getenv("TMUX") != ""
	if insideTmux && !recentsNoPopup {
		return launchAsPopup("recents")
	}

	// Run TUI
	result, err := tui.RunRecents(tui.RecentsOptions{
		AltScreen: false,
		Limit:     recentsLimit,
	})
	if err != nil {
		return err
	}

	if result.SessionName == "" {
		return nil // User quit without selection
	}

	// Revive session in the selected working directory
	session := tmux.NewSession(result.WorkingDir)
	return runDirectAttach(session, result.WorkingDir)
}

func runRecentsList(cmd *cobra.Command) error {
	store, err := history.Open()
	if err != nil {
		return fmt.Errorf("failed to open history: %w", err)
	}
	defer store.Close()

	entries, err := store.LoadHistory()
	if err != nil {
		return fmt.Errorf("failed to load history: %w", err)
	}

	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No recent sessions.")
		return nil
	}

	// Limit entries
	if recentsLimit > 0 && len(entries) > recentsLimit {
		entries = entries[:recentsLimit]
	}

	// Styles
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	out := cmd.OutOrStdout()
	for _, e := range entries {
		ago := timeAgo(e.LastUsedAt)
		displayPath := displayPathForList(e.WorkingDirectory, recentsHidePaths, true)
		fmt.Fprintf(out, "%s  %s  %s\n",
			nameStyle.Render(e.Name),
			dimStyle.Render(displayPath),
			dimStyle.Render("("+ago+")"))
	}

	return nil
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if len(path) == 0 {
		return path
	}
	if path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/history"
	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Manage session history",
	Long:  "View and manage the history of agent sessions.",
}

var historyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent agent sessions",
	RunE:  runHistoryList,
}

var historyClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all session history",
	RunE:  runHistoryClear,
}

var historyRemoveCmd = &cobra.Command{
	Use:   "remove <id-or-name>",
	Short: "Remove a session from history",
	Args:  cobra.ExactArgs(1),
	RunE:  runHistoryRemove,
}

var historyJSON bool
var historyHidePaths bool

func init() {
	rootCmd.AddCommand(historyCmd)
	historyCmd.AddCommand(historyListCmd)
	historyCmd.AddCommand(historyClearCmd)
	historyCmd.AddCommand(historyRemoveCmd)

	historyListCmd.Flags().BoolVar(&historyJSON, "json", false, "Output as JSON")
	historyListCmd.Flags().BoolVar(&historyHidePaths, "hide-paths", false, hidePathsHelpText)
}

func runHistoryList(cmd *cobra.Command, args []string) error {
	store, err := history.Open()
	if err != nil {
		return fmt.Errorf("failed to open history: %w", err)
	}
	defer store.Close()

	entries, err := store.LoadHistory()
	if err != nil {
		return fmt.Errorf("failed to load history: %w", err)
	}

	if historyJSON {
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No session history.")
		return nil
	}

	// Styles
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))

	out := cmd.OutOrStdout()
	fmt.Fprintln(out, headerStyle.Render("Recent Sessions"))
	fmt.Fprintln(out)

	for i, e := range entries {
		ago := timeAgo(e.LastUsedAt)
		fmt.Fprintf(out, "%s %s\n",
			dimStyle.Render(fmt.Sprintf("%2d.", i+1)),
			nameStyle.Render(e.Name))
		fmt.Fprintf(out, "    %s  %s\n",
			dimStyle.Render(displayPathForList(e.WorkingDirectory, historyHidePaths, false)),
			dimStyle.Render("("+ago+")"))
	}

	return nil
}

func runHistoryClear(cmd *cobra.Command, args []string) error {
	store, err := history.Open()
	if err != nil {
		return fmt.Errorf("failed to open history: %w", err)
	}
	defer store.Close()

	if err := store.ClearHistory(); err != nil {
		return fmt.Errorf("failed to clear history: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "History cleared.")
	return nil
}

func runHistoryRemove(cmd *cobra.Command, args []string) error {
	store, err := history.Open()
	if err != nil {
		return fmt.Errorf("failed to open history: %w", err)
	}
	defer store.Close()

	target := args[0]

	// Try to parse as numeric ID first
	if id, err := strconv.ParseInt(target, 10, 64); err == nil {
		if err := store.DeleteEntry(id); err != nil {
			return fmt.Errorf("failed to remove entry: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed entry with ID %d.\n", id)
		return nil
	}

	// Otherwise treat as session name
	if err := store.DeleteBySessionName(target); err != nil {
		return fmt.Errorf("failed to remove entry: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Removed entry for session '%s'.\n", target)
	return nil
}

func timeAgo(t time.Time) string {
	d := time.Since(t)

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("Jan 2")
	}
}

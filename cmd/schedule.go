package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/scheduler"
	"github.com/porganisciak/agent-tmux/tui"
	"github.com/spf13/cobra"
)

var scheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Manage scheduled commands",
	Long:  "Manage scheduled commands to send to tmux panes at specified times.",
	RunE:  runScheduleTUI,
}

var scheduleAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new scheduled command",
	Long:  "Add a new scheduled command. Run without flags for interactive mode.",
	RunE:  runScheduleAdd,
}

var scheduleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all scheduled commands",
	RunE:  runScheduleList,
}

var scheduleRemoveCmd = &cobra.Command{
	Use:     "rm <id>",
	Aliases: []string{"remove", "delete"},
	Short:   "Remove a scheduled command",
	Args:    cobra.ExactArgs(1),
	RunE:    runScheduleRemove,
}

var scheduleEnableCmd = &cobra.Command{
	Use:   "enable <id>",
	Short: "Enable a scheduled command",
	Args:  cobra.ExactArgs(1),
	RunE:  runScheduleEnable,
}

var scheduleDisableCmd = &cobra.Command{
	Use:   "disable <id>",
	Short: "Disable a scheduled command",
	Args:  cobra.ExactArgs(1),
	RunE:  runScheduleDisable,
}

var scheduleRunPendingCmd = &cobra.Command{
	Use:   "run-pending",
	Short: "Run all pending scheduled commands",
	Long:  "Execute all scheduled commands that are due. Designed for cron integration.",
	RunE:  runScheduleRunPending,
}

var scheduleDaemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the scheduler daemon",
	Long:  "Run a background daemon that checks and executes scheduled commands.",
	RunE:  runScheduleDaemon,
}

// Flags
var (
	scheduleJSON     bool
	scheduleCron     string
	scheduleTarget   string
	scheduleCommand  string
	schedulePreAction string
	daemonInterval   time.Duration
)

func init() {
	rootCmd.AddCommand(scheduleCmd)
	scheduleCmd.AddCommand(scheduleAddCmd)
	scheduleCmd.AddCommand(scheduleListCmd)
	scheduleCmd.AddCommand(scheduleRemoveCmd)
	scheduleCmd.AddCommand(scheduleEnableCmd)
	scheduleCmd.AddCommand(scheduleDisableCmd)
	scheduleCmd.AddCommand(scheduleRunPendingCmd)
	scheduleCmd.AddCommand(scheduleDaemonCmd)

	// List flags
	scheduleListCmd.Flags().BoolVar(&scheduleJSON, "json", false, "Output as JSON")

	// Add flags for scripted mode
	scheduleAddCmd.Flags().StringVar(&scheduleCron, "cron", "", "Cron expression (e.g., '0 9 * * *')")
	scheduleAddCmd.Flags().StringVar(&scheduleTarget, "target", "", "Target pane (e.g., 'session:window.pane')")
	scheduleAddCmd.Flags().StringVar(&scheduleCommand, "command", "", "Command to send")
	scheduleAddCmd.Flags().StringVar(&schedulePreAction, "pre", "none", "Pre-action: none, new, or compact")

	// Daemon flags
	scheduleDaemonCmd.Flags().DurationVar(&daemonInterval, "interval", time.Minute, "Check interval for pending jobs")
}

func runScheduleTUI(cmd *cobra.Command, args []string) error {
	result, err := tui.RunScheduleTUI()
	if err != nil {
		return err
	}
	if result != nil && result.Message != "" {
		fmt.Fprintln(cmd.OutOrStdout(), result.Message)
	}
	return nil
}

func runScheduleAdd(cmd *cobra.Command, args []string) error {
	// If no flags provided, run interactive mode
	if scheduleCron == "" && scheduleTarget == "" && scheduleCommand == "" {
		result, err := tui.RunScheduleFormTUI()
		if err != nil {
			return err
		}
		if result != nil && result.Added {
			fmt.Fprintf(cmd.OutOrStdout(), "Added schedule: %s\n", result.JobID)
		}
		return nil
	}

	// Scripted mode - validate all required flags
	if scheduleCron == "" {
		return fmt.Errorf("--cron is required")
	}
	if scheduleTarget == "" {
		return fmt.Errorf("--target is required")
	}
	if scheduleCommand == "" {
		return fmt.Errorf("--command is required")
	}

	// Validate cron expression
	if _, err := scheduler.ParseSchedule(scheduleCron); err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}

	// Parse pre-action
	var preAction scheduler.PreAction
	switch schedulePreAction {
	case "none", "":
		preAction = scheduler.PreActionNone
	case "new":
		preAction = scheduler.PreActionNew
	case "compact":
		preAction = scheduler.PreActionCompact
	default:
		return fmt.Errorf("invalid pre-action: %s (must be none, new, or compact)", schedulePreAction)
	}

	// Create and save job
	store, err := scheduler.Load()
	if err != nil {
		return fmt.Errorf("failed to load schedules: %w", err)
	}

	job := scheduler.ScheduledJob{
		ID:        scheduler.GenerateID(),
		Schedule:  scheduleCron,
		Target:    scheduleTarget,
		Command:   scheduleCommand,
		PreAction: preAction,
		Enabled:   true,
		CreatedAt: time.Now(),
	}

	if err := store.Add(job); err != nil {
		return fmt.Errorf("failed to add schedule: %w", err)
	}

	if err := store.Save(); err != nil {
		return fmt.Errorf("failed to save schedules: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Added schedule %s: %s\n", job.ID, scheduler.CronToEnglish(job.Schedule))
	return nil
}

func runScheduleList(cmd *cobra.Command, args []string) error {
	store, err := scheduler.Load()
	if err != nil {
		return fmt.Errorf("failed to load schedules: %w", err)
	}

	if scheduleJSON {
		data, err := json.MarshalIndent(store.Jobs, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	if len(store.Jobs) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No scheduled commands.")
		fmt.Fprintln(cmd.OutOrStdout(), "Run 'atmux schedule add' to create one.")
		return nil
	}

	// Styles
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	idStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	disabledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Strikethrough(true)
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	out := cmd.OutOrStdout()
	fmt.Fprintln(out, headerStyle.Render("Scheduled Commands"))
	fmt.Fprintln(out)

	for _, job := range store.Jobs {
		// Status indicator
		status := "●"
		if !job.Enabled {
			status = "○"
		}

		// ID and schedule
		scheduleDesc := scheduler.CronToEnglish(job.Schedule)
		if job.Enabled {
			fmt.Fprintf(out, "%s %s  %s\n", status, idStyle.Render(job.ID), scheduleDesc)
		} else {
			fmt.Fprintf(out, "%s %s  %s\n", dimStyle.Render(status), dimStyle.Render(job.ID), disabledStyle.Render(scheduleDesc))
		}

		// Target
		fmt.Fprintf(out, "  %s %s\n", dimStyle.Render("→"), job.Target)

		// Command (with pre-action if set)
		cmdDesc := job.Command
		if job.PreAction != scheduler.PreActionNone {
			cmdDesc = fmt.Sprintf("/%s then %s", job.PreAction, job.Command)
		}
		fmt.Fprintf(out, "  %s\n", cmdDesc)

		// Next run
		if job.Enabled && !job.NextRun.IsZero() {
			fmt.Fprintf(out, "  %s\n", dimStyle.Render("Next: "+job.NextRun.Format("Mon Jan 2 15:04")))
		}

		// Last error
		if job.LastError != "" {
			fmt.Fprintf(out, "  %s\n", errorStyle.Render("Error: "+job.LastError))
		}

		fmt.Fprintln(out)
	}

	return nil
}

func runScheduleRemove(cmd *cobra.Command, args []string) error {
	id := args[0]

	store, err := scheduler.Load()
	if err != nil {
		return fmt.Errorf("failed to load schedules: %w", err)
	}

	if err := store.Remove(id); err != nil {
		return fmt.Errorf("failed to remove schedule: %w", err)
	}

	if err := store.Save(); err != nil {
		return fmt.Errorf("failed to save schedules: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed schedule %s\n", id)
	return nil
}

func runScheduleEnable(cmd *cobra.Command, args []string) error {
	id := args[0]

	store, err := scheduler.Load()
	if err != nil {
		return fmt.Errorf("failed to load schedules: %w", err)
	}

	job, err := store.GetByID(id)
	if err != nil {
		return fmt.Errorf("schedule not found: %s", id)
	}

	job.Enabled = true
	// Recalculate next run
	nextRun, err := scheduler.NextRun(job.Schedule)
	if err != nil {
		return fmt.Errorf("failed to calculate next run: %w", err)
	}
	job.NextRun = nextRun

	if err := store.Update(*job); err != nil {
		return fmt.Errorf("failed to update schedule: %w", err)
	}

	if err := store.Save(); err != nil {
		return fmt.Errorf("failed to save schedules: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Enabled schedule %s\n", id)
	fmt.Fprintf(cmd.OutOrStdout(), "Next run: %s\n", job.NextRun.Format("Mon Jan 2 15:04"))
	return nil
}

func runScheduleDisable(cmd *cobra.Command, args []string) error {
	id := args[0]

	store, err := scheduler.Load()
	if err != nil {
		return fmt.Errorf("failed to load schedules: %w", err)
	}

	job, err := store.GetByID(id)
	if err != nil {
		return fmt.Errorf("schedule not found: %s", id)
	}

	job.Enabled = false

	if err := store.Update(*job); err != nil {
		return fmt.Errorf("failed to update schedule: %w", err)
	}

	if err := store.Save(); err != nil {
		return fmt.Errorf("failed to save schedules: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Disabled schedule %s\n", id)
	return nil
}

func runScheduleRunPending(cmd *cobra.Command, args []string) error {
	results, err := scheduler.ExecutePending()
	if err != nil {
		return fmt.Errorf("failed to execute pending jobs: %w", err)
	}

	if len(results) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No pending jobs to run.")
		return nil
	}

	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
			fmt.Fprintf(cmd.OutOrStdout(), "✓ %s executed successfully\n", r.JobID)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "✗ %s failed: %v\n", r.JobID, r.Error)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nExecuted %d/%d jobs successfully.\n", successCount, len(results))
	return nil
}

func runScheduleDaemon(cmd *cobra.Command, args []string) error {
	fmt.Fprintf(cmd.OutOrStdout(), "Starting scheduler daemon (interval: %s)\n", daemonInterval)
	fmt.Fprintln(cmd.OutOrStdout(), "Press Ctrl+C to stop.")

	// Set up signal handling
	stop := make(chan struct{})
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Fprintln(cmd.OutOrStdout(), "\nStopping daemon...")
		close(stop)
	}()

	return scheduler.RunDaemon(daemonInterval, stop)
}

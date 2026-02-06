package cmd

import (
	"github.com/porganisciak/agent-tmux/tui"
	"github.com/spf13/cobra"
)

var scheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Manage scheduled commands",
	Long: `Opens an interactive TUI to manage scheduled commands.

Scheduled commands are sent to tmux panes on a cron schedule.
You can create, edit, enable/disable, and delete scheduled jobs.

Controls:
  Up/Down or j/k  Navigate jobs list
  Enter           Edit selected job
  a               Add new job
  e               Toggle enabled/disabled
  d/x             Delete selected job
  q/Esc           Quit

Note: The scheduler daemon must be running for jobs to execute.
Use 'atmux schedule daemon' to start the background scheduler.`,
	RunE: runSchedule,
}

func init() {
	rootCmd.AddCommand(scheduleCmd)
}

func runSchedule(cmd *cobra.Command, args []string) error {
	return tui.RunScheduler(tui.SchedulerOptions{
		AltScreen: true,
	})
}

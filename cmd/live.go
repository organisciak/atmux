package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/porganisciak/agent-tmux/tui"
	"github.com/spf13/cobra"
)

var (
	liveTUIMode         bool   // --tui (internal: run as TUI subprocess)
	liveRightPane       string // --right-pane (internal: pane ID for display slot)
	liveOriginalSess    string // --original-session (internal: session to return to)
	liveRefreshInterval int    // --refresh
)

var liveCmd = &cobra.Command{
	Use:   "live",
	Short: "Live split-pane browser for tmux sessions",
	Long: `Opens a tmux-native split-pane browser where the left side shows a
compact session tree and the right side displays the actual live tmux pane
from the selected session.

Navigate between sessions with j/k or arrow keys. The right pane updates
to show each session's Claude Code instance (or first pane).

Sessions are collapsed by default — press Enter or Space to expand and
see individual windows and panes.

Controls:
  Up/Down or j/k   Navigate sessions
  Enter/Space       Expand/collapse session
  a                 Attach to selected session (exit live browser)
  p                 Open selected recent project in a popup
  d                 Set selected pane/window/session as default view
  x                 Kill selected open session
  c                 Start Claude Remote Control for selected session
  b                 Toggle beads task panel for selected project
  r                 Refresh session list
  q/Esc             Quit (restores all panes)`,
	RunE: runLive,
}

func init() {
	rootCmd.AddCommand(liveCmd)
	liveCmd.Flags().BoolVar(&liveTUIMode, "tui", false, "Run as TUI subprocess (internal)")
	liveCmd.Flags().StringVar(&liveRightPane, "right-pane", "", "Right pane ID (internal)")
	liveCmd.Flags().StringVar(&liveOriginalSess, "original-session", "", "Original session name (internal)")
	liveCmd.Flags().IntVarP(&liveRefreshInterval, "refresh", "r", 2, "Auto-refresh interval in seconds")

	// Hide internal flags
	liveCmd.Flags().MarkHidden("tui")
	liveCmd.Flags().MarkHidden("right-pane")
	liveCmd.Flags().MarkHidden("original-session")
}

func runLive(cmd *cobra.Command, args []string) error {
	if liveTUIMode {
		return runLiveTUI()
	}
	return runLiveOrchestrator()
}

// runLiveTUI runs the Bubbletea tree TUI in the left pane.
func runLiveTUI() error {
	if liveRightPane == "" {
		return fmt.Errorf("--right-pane is required in TUI mode")
	}

	opts := tui.LiveOptions{
		RightPaneID:     liveRightPane,
		OriginalSession: liveOriginalSess,
		RefreshInterval: time.Duration(liveRefreshInterval) * time.Second,
	}

	return tui.RunLive(opts)
}

// runLiveOrchestrator sets up the tmux layout and launches the TUI.
func runLiveOrchestrator() error {
	// Check if tmux server is running
	if !tmuxServerRunning() {
		return fmt.Errorf("tmux server not running - start a tmux session first")
	}

	// If a previous live session crashed mid-swap, swap the stranded
	// display pane back to its original location before deciding what to do.
	tmux.RecoverOrphanedDisplay()

	// If a healthy live session already exists, just switch to it.
	// Otherwise rebuild from scratch — a session with a missing sidebar
	// pane is unusable and we can't recover it in place.
	if tmux.LiveSessionExists() {
		if tmux.IsLiveSessionHealthy() {
			tmux.PinLeftWidth()
			return tmux.AttachToSession(tmux.LiveSessionName)
		}
		// Broken layout (e.g., sidebar pane closed, swap-back never ran).
		// Kill it and fall through to rebuild.
		tmux.KillLiveSession()
	}

	// Create the live session with split layout
	ls, err := tmux.CreateLiveSession()
	if err != nil {
		return fmt.Errorf("failed to create live session: %w", err)
	}

	// Get our executable path
	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}

	// Respawn the left pane with the TUI
	if err := ls.RespawnLeftWithTUI(selfPath); err != nil {
		// Clean up on failure
		tmux.CleanupLiveSession(ls.RightPaneID, &ls.DisplayedPaneID, ls.OriginalSession)
		return fmt.Errorf("failed to launch TUI: %w", err)
	}

	// Switch to the live session
	return tmux.AttachToSession(tmux.LiveSessionName)
}

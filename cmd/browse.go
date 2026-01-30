package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/porganisciak/agent-tmux/tui"
	"github.com/spf13/cobra"
)

var (
	popupMode       bool
	refreshInterval int
	debugMode       bool
)

var browseCmd = &cobra.Command{
	Use:   "browse",
	Short: "Interactive browser for all tmux sessions",
	Long: `Opens an interactive TUI to browse, preview, and send commands to tmux panes.

Features:
  - Tree view of all sessions, windows, and panes
  - Live preview of selected pane content
  - Send commands to any pane with a click
  - Mouse and keyboard navigation

Controls:
  Tab/Shift+Tab  Cycle focus between tree, input, preview
  Up/Down or j/k Navigate tree
  Enter/Space    Expand/collapse session or window
  a (att)        Attach to session for selected window/pane
  s              Send command to selected pane
  M              Toggle mouse capture (for text selection)
  r              Refresh tree
  /              Focus command input
  q/Esc          Quit

Mouse:
  Click tree item to select
  Double-click window/pane to attach
  Click [SEND] button to send command to that pane
  Click [ESC] button to send Escape to that pane
  Click input/preview area to focus

Debug Mode (--debug):
  m              Cycle through send methods (Enter separate, C-m separate, etc.)

  Send methods available:
    - Enter (separate):    send-keys 'text'; send-keys Enter
    - C-m (separate):      send-keys 'text'; send-keys C-m
    - Enter (appended):    send-keys 'text' Enter
    - C-m (appended):      send-keys 'text' C-m
    - Enter (literal):     send-keys -l 'text'; send-keys Enter
    - Enter (500ms delay): send-keys 'text'; sleep 500ms; send-keys Enter
    - Enter (1500ms delay): send-keys 'text'; sleep 1.5s; send-keys Enter`,
	RunE: runBrowse,
}

func init() {
	rootCmd.AddCommand(browseCmd)
	browseCmd.Flags().BoolVarP(&popupMode, "popup", "p", false, "Launch as tmux popup overlay (requires tmux 3.2+)")
	browseCmd.Flags().IntVarP(&refreshInterval, "refresh", "r", 2, "Auto-refresh interval in seconds (0 to disable)")
	browseCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "Enable debug mode to test different send methods")
}

func runBrowse(cmd *cobra.Command, args []string) error {
	// Check if tmux server is running
	if !tmuxServerRunning() {
		return fmt.Errorf("tmux server not running - start a tmux session first")
	}

	if popupMode {
		return launchAsPopup()
	}

	// Run TUI directly
	opts := tui.Options{
		RefreshInterval: time.Duration(refreshInterval) * time.Second,
		PopupMode:       false,
		DebugMode:       debugMode,
	}
	return tui.Run(opts)
}

func tmuxServerRunning() bool {
	cmd := exec.Command("tmux", "list-sessions")
	return cmd.Run() == nil
}

func launchAsPopup() error {
	// Check if we're inside tmux
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("--popup requires running inside a tmux session")
	}

	// Get the path to ourselves
	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}

	// Launch as popup
	// Using display-popup which is available in tmux 3.2+
	tmuxCmd := exec.Command("tmux", "display-popup",
		"-E",        // Close popup when command exits
		"-w", "90%", // Width
		"-h", "90%", // Height
		selfPath, "browse", // Run ourselves without --popup
	)
	tmuxCmd.Stdin = os.Stdin
	tmuxCmd.Stdout = os.Stdout
	tmuxCmd.Stderr = os.Stderr

	return tmuxCmd.Run()
}

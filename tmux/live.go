package tmux

import (
	"fmt"
	"os/exec"
	"strings"
)

// LiveSessionName is the tmux session used by the live browser.
const LiveSessionName = "_atmux_live"

// Tmux global option keys used to recover from a crashed/aborted live session.
// These let a fresh atmux process detect and undo a half-finished swap so the
// sidebar pane is always present when the user re-enters live.
const (
	optLiveRightPane = "@atmux_live_right_pane"
	optLiveDisplayed = "@atmux_live_displayed"
	optLiveLeftPane  = "@atmux_live_left_pane"
	optLiveLeftWidth = "@atmux_live_left_width"
)

// LiveSession holds state for the live browser session.
type LiveSession struct {
	SessionName     string // "_atmux_live"
	LeftPaneID      string // Pane ID (%nn) running the TUI
	RightPaneID     string // Pane ID (%nn) the display slot
	DisplayedPaneID string // Pane ID currently swapped into the right slot
	OriginalSession string // Session the user was in before launching live
}

// maxLeftPaneWidth is the upper bound for the left tree pane width.
const maxLeftPaneWidth = 35

// minLeftPaneWidth is the minimum width for the left pane.
const minLeftPaneWidth = 16

// leftPanePadding accounts for tree icons (▸ ), selection highlight, and margin.
const leftPanePadding = 6

// calculateLeftWidth computes the left pane width based on session names.
// It uses the longest session name + padding, clamped to [minLeftPaneWidth, maxLeftPaneWidth].
func calculateLeftWidth() int {
	sessions, err := ListSessions()
	if err != nil || len(sessions) == 0 {
		return minLeftPaneWidth
	}
	maxLen := 0
	for _, name := range sessions {
		if name == LiveSessionName {
			continue
		}
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}
	width := maxLen + leftPanePadding
	if width < minLeftPaneWidth {
		width = minLeftPaneWidth
	}
	if width > maxLeftPaneWidth {
		width = maxLeftPaneWidth
	}
	return width
}

// CreateLiveSession creates the _atmux_live session with a left/right split.
// Left pane width is sized to fit session names (max 35 chars).
func CreateLiveSession() (*LiveSession, error) {
	ls := &LiveSession{SessionName: LiveSessionName}

	// Save original session
	ls.OriginalSession = GetCurrentSession()

	// Kill any existing live session
	exec.Command("tmux", "kill-session", "-t", LiveSessionName).Run()

	// Create new detached session
	if err := exec.Command("tmux", "new-session", "-d", "-s", LiveSessionName, "-n", "browse").Run(); err != nil {
		return nil, fmt.Errorf("failed to create live session: %w", err)
	}

	// Calculate left pane width based on session names
	leftWidth := calculateLeftWidth()

	// Split the initial pane horizontally using -b (before) so the new pane
	// becomes the left side with an exact character width; the original pane
	// stays on the right and takes the rest of the terminal width.
	if err := exec.Command("tmux", "split-window", "-h", "-b",
		"-l", fmt.Sprintf("%d", leftWidth),
		"-t", LiveSessionName+":0").Run(); err != nil {
		exec.Command("tmux", "kill-session", "-t", LiveSessionName).Run()
		return nil, fmt.Errorf("failed to split live session: %w", err)
	}

	// Get pane IDs — after a -b split the new (left) pane is pane index 0,
	// the original (right) pane is index 1.
	output, err := exec.Command("tmux", "list-panes", "-t", LiveSessionName+":0",
		"-F", "#{pane_id}", "-O", "left").Output()
	if err != nil {
		// Fallback: list without ordering flag (older tmux)
		output, err = exec.Command("tmux", "list-panes", "-t", LiveSessionName+":0",
			"-F", "#{pane_id}").Output()
		if err != nil {
			exec.Command("tmux", "kill-session", "-t", LiveSessionName).Run()
			return nil, fmt.Errorf("failed to get pane IDs: %w", err)
		}
	}

	paneIDs := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(paneIDs) < 2 {
		exec.Command("tmux", "kill-session", "-t", LiveSessionName).Run()
		return nil, fmt.Errorf("expected 2 panes, got %d", len(paneIDs))
	}

	ls.LeftPaneID = paneIDs[0]
	ls.RightPaneID = paneIDs[1]

	// Persist pane IDs so a fresh atmux process can detect and recover
	// from a stranded display pane after a crash. Width is pinned after
	// every swap (tmux otherwise reflows toward equal proportions).
	setTmuxGlobalOption(optLiveLeftPane, ls.LeftPaneID)
	setTmuxGlobalOption(optLiveRightPane, ls.RightPaneID)
	setTmuxGlobalOption(optLiveLeftWidth, fmt.Sprintf("%d", leftWidth))
	unsetTmuxGlobalOption(optLiveDisplayed)

	// Select the left pane
	exec.Command("tmux", "select-pane", "-t", ls.LeftPaneID).Run()

	return ls, nil
}

// setTmuxGlobalOption sets a global tmux user option (must start with @).
func setTmuxGlobalOption(name, value string) {
	exec.Command("tmux", "set-option", "-g", name, value).Run()
}

// unsetTmuxGlobalOption removes a global tmux user option.
func unsetTmuxGlobalOption(name string) {
	exec.Command("tmux", "set-option", "-gu", name).Run()
}

// getTmuxGlobalOption reads a global tmux user option, returning "" if unset.
func getTmuxGlobalOption(name string) string {
	out, err := exec.Command("tmux", "show-option", "-gv", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// paneExists returns true if the given pane ID is currently known to tmux.
func paneExists(paneID string) bool {
	if paneID == "" {
		return false
	}
	return exec.Command("tmux", "display-message", "-t", paneID, "-p", "#{pane_id}").Run() == nil
}

// liveSessionPaneCount returns the number of panes in the live session's
// first window, or -1 if the session doesn't exist.
func liveSessionPaneCount() int {
	out, err := exec.Command("tmux", "list-panes",
		"-t", LiveSessionName+":0", "-F", "#{pane_id}").Output()
	if err != nil {
		return -1
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0
	}
	return len(lines)
}

// IsLiveSessionHealthy reports whether the live session has the expected
// 2-pane sidebar+display layout. Used to decide whether to attach to an
// existing live session or rebuild it from scratch.
func IsLiveSessionHealthy() bool {
	return liveSessionPaneCount() == 2
}

// RecoverOrphanedDisplay swaps a stranded display pane back to its original
// location if the previous live session crashed mid-swap. Best-effort: if
// the recorded panes no longer exist, the options are just cleared.
//
// Call this BEFORE deciding whether to recreate the live session, so we don't
// kill a pane that's still alive in the user's other session layout.
func RecoverOrphanedDisplay() {
	displayed := getTmuxGlobalOption(optLiveDisplayed)
	rightPane := getTmuxGlobalOption(optLiveRightPane)

	if displayed != "" && rightPane != "" && paneExists(displayed) && paneExists(rightPane) {
		// Best-effort swap-back; ignore errors (could happen if the
		// panes are now in incompatible layouts).
		exec.Command("tmux", "swap-pane", "-d", "-s", displayed, "-t", rightPane).Run()
	}
	unsetTmuxGlobalOption(optLiveDisplayed)
}

// ClearLiveSessionOptions wipes all live-session recovery options. Call after
// the live session has been fully cleaned up (or never existed).
func ClearLiveSessionOptions() {
	unsetTmuxGlobalOption(optLiveLeftPane)
	unsetTmuxGlobalOption(optLiveRightPane)
	unsetTmuxGlobalOption(optLiveDisplayed)
	unsetTmuxGlobalOption(optLiveLeftWidth)
}

// PinLeftWidth re-asserts the sidebar pane's width to the value persisted
// at create time. Tmux reflows panes toward equal proportions across swaps
// and resize events, so we re-pin after every swap to keep the sidebar narrow.
func PinLeftWidth() {
	leftPane := getTmuxGlobalOption(optLiveLeftPane)
	widthStr := getTmuxGlobalOption(optLiveLeftWidth)
	if leftPane == "" || widthStr == "" {
		return
	}
	exec.Command("tmux", "resize-pane", "-t", leftPane, "-x", widthStr).Run()
}

// RespawnLeftWithTUI replaces the left pane's shell with the TUI process.
func (ls *LiveSession) RespawnLeftWithTUI(selfPath string) error {
	cmd := fmt.Sprintf("%s live --tui --right-pane=%s --original-session=%s",
		selfPath, ls.RightPaneID, ls.OriginalSession)
	return exec.Command("tmux", "respawn-pane", "-k", "-t", ls.LeftPaneID, cmd).Run()
}

// SwapPaneIn swaps the target pane into the display slot.
// If a pane is already displayed, it is swapped back first.
// The -d flag keeps the active pane unchanged so cursor focus stays in the TUI.
func SwapPaneIn(rightPaneID, targetPaneID string, currentDisplayed *string) error {
	// Swap back the currently displayed pane if any
	if *currentDisplayed != "" {
		if err := exec.Command("tmux", "swap-pane", "-d", "-s", *currentDisplayed, "-t", rightPaneID).Run(); err != nil {
			// The pane may have been killed; clear and continue
			*currentDisplayed = ""
		} else {
			*currentDisplayed = ""
		}
		unsetTmuxGlobalOption(optLiveDisplayed)
	}

	// Swap in the new target
	if err := exec.Command("tmux", "swap-pane", "-d", "-s", targetPaneID, "-t", rightPaneID).Run(); err != nil {
		return fmt.Errorf("swap-pane failed: %w", err)
	}
	*currentDisplayed = targetPaneID
	// Persist so a crashed TUI can be recovered on next launch.
	setTmuxGlobalOption(optLiveDisplayed, targetPaneID)
	// Re-pin sidebar width — swap-pane often reflows toward 50/50.
	PinLeftWidth()
	return nil
}

// RestoreDisplayedPane swaps the displayed pane back to its original location.
func RestoreDisplayedPane(rightPaneID string, displayedPaneID *string) {
	if *displayedPaneID == "" {
		return
	}
	exec.Command("tmux", "swap-pane", "-d", "-s", *displayedPaneID, "-t", rightPaneID).Run()
	*displayedPaneID = ""
	unsetTmuxGlobalOption(optLiveDisplayed)
}

// FocusPane makes the given pane the active pane in its window.
func FocusPane(paneID string) {
	if paneID == "" {
		return
	}
	exec.Command("tmux", "select-pane", "-t", paneID).Run()
}

// CleanupLiveSession restores panes and kills the live session.
func CleanupLiveSession(rightPaneID string, displayedPaneID *string, originalSession string) {
	RestoreDisplayedPane(rightPaneID, displayedPaneID)
	exec.Command("tmux", "kill-session", "-t", LiveSessionName).Run()
	ClearLiveSessionOptions()
	if originalSession != "" {
		exec.Command("tmux", "switch-client", "-t", originalSession).Run()
	}
}

// GetCurrentSession returns the name of the session the current client is attached to.
func GetCurrentSession() string {
	output, err := exec.Command("tmux", "display-message", "-p", "#{client_session}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// GetPaneID returns the pane ID (%nn) for a given tmux target.
func GetPaneID(target string) string {
	output, err := exec.Command("tmux", "display-message", "-t", target, "-p", "#{pane_id}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// LiveSessionExists returns true if the _atmux_live session exists.
func LiveSessionExists() bool {
	return exec.Command("tmux", "has-session", "-t", LiveSessionName).Run() == nil
}

// KillLiveSession force-kills the live session and clears its recovery
// options. Used when a broken layout can't be repaired in place.
func KillLiveSession() {
	exec.Command("tmux", "kill-session", "-t", LiveSessionName).Run()
	ClearLiveSessionOptions()
}

// FindBestPaneID returns the pane ID of the best pane to display for a session.
// Priority: (1) first Claude Code pane, (2) active pane of active window, (3) first pane.
func FindBestPaneID(sess TmuxSession) string {
	// 1. Look for a Claude Code pane
	for _, win := range sess.Windows {
		for _, pane := range win.Panes {
			if isClaudePane(pane) {
				return GetPaneID(pane.Target)
			}
		}
	}

	// 2. Active pane of active window
	for _, win := range sess.Windows {
		if win.Active {
			for _, pane := range win.Panes {
				if pane.Active {
					return GetPaneID(pane.Target)
				}
			}
			// Active window but no active pane marked? Use first pane
			if len(win.Panes) > 0 {
				return GetPaneID(win.Panes[0].Target)
			}
		}
	}

	// 3. First pane of first window
	if len(sess.Windows) > 0 && len(sess.Windows[0].Panes) > 0 {
		return GetPaneID(sess.Windows[0].Panes[0].Target)
	}

	return ""
}

// FindPaneIDForNode returns the best pane ID for the given tree node.
// For sessions: finds the best pane (Claude or first).
// For windows: finds the active pane of that window.
// For panes: returns that pane's ID directly.
func FindPaneIDForNode(node *TreeNode, tree *Tree) string {
	if node == nil || tree == nil {
		return ""
	}

	switch node.Type {
	case "pane":
		return GetPaneID(node.Target)
	case "window":
		// Find this window and return its active/first pane
		for _, sess := range tree.Sessions {
			for _, win := range sess.Windows {
				winTarget := sess.Name + ":" + fmt.Sprintf("%d", win.Index)
				if winTarget == node.Target {
					for _, pane := range win.Panes {
						if pane.Active {
							return GetPaneID(pane.Target)
						}
					}
					if len(win.Panes) > 0 {
						return GetPaneID(win.Panes[0].Target)
					}
				}
			}
		}
	case "session":
		for _, sess := range tree.Sessions {
			if sess.Name == node.Target {
				return FindBestPaneID(sess)
			}
		}
	}
	return ""
}

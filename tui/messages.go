package tui

import (
	"github.com/porganisciak/agent-tmux/history"
	"github.com/porganisciak/agent-tmux/tmux"
)

// TreeRefreshedMsg is sent when tree data is fetched
type TreeRefreshedMsg struct {
	Tree *tmux.Tree
	Err  error
}

// PreviewUpdatedMsg is sent when pane preview is captured
type PreviewUpdatedMsg struct {
	Content string
	Target  string
	Err     error
}

// CommandSentMsg is sent after command dispatch
type CommandSentMsg struct {
	Target  string
	Command string
	Err     error
}

// TickMsg for auto-refresh
type TickMsg struct{}

// AttachMsg is sent after attempting to switch to a target
type AttachMsg struct {
	Target string
	Err    error
}

// ErrorMsg for displaying errors
type ErrorMsg struct {
	Err error
}

// KillCompletedMsg is sent after a kill operation completes
type KillCompletedMsg struct {
	NodeType string
	Target   string
	Err      error
}

// RecentSessionsMsg is sent when recent history entries are loaded
type RecentSessionsMsg struct {
	Entries []history.Entry
	Err     error
}

// RecentDeletedMsg is sent after deleting a recent history entry
type RecentDeletedMsg struct {
	ID  int64
	Err error
}

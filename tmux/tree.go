package tmux

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Pane represents a tmux pane
type Pane struct {
	ID      string
	Index   int
	Title   string
	Command string
	Active  bool
	Width   int
	Height  int
	Target  string // Full target: session:window.pane
}

// Window represents a tmux window
type Window struct {
	ID     string
	Index  int
	Name   string
	Active bool
	Panes  []Pane
}

// TmuxSession represents a tmux session (distinct from Session config type)
type TmuxSession struct {
	Name     string
	Attached bool
	Windows  []Window
}

// Tree represents the full tmux hierarchy
type Tree struct {
	Sessions []TmuxSession
}

// TreeNode is used for the tree browser display
type TreeNode struct {
	Type     string // "session", "window", or "pane"
	Name     string // Display name
	Target   string // Tmux target (session:window.pane)
	Expanded bool
	Level    int
	Active   bool
	Attached bool // For sessions
	Host     string // Remote host label (empty for local)
	Children []*TreeNode
}

// FetchTree queries tmux and builds the complete tree
func FetchTree() (*Tree, error) {
	tree := &Tree{}

	// Get all sessions
	sessions, err := listAllSessions()
	if err != nil {
		return nil, err
	}

	for _, sess := range sessions {
		// Get windows for this session
		windows, err := listWindows(sess.Name)
		if err != nil {
			continue // Skip sessions we can't query
		}

		for i := range windows {
			// Get panes for this window
			panes, err := listPanes(sess.Name, windows[i].Index)
			if err != nil {
				continue
			}
			windows[i].Panes = panes
		}

		sess.Windows = windows
		tree.Sessions = append(tree.Sessions, sess)
	}

	return tree, nil
}

// listAllSessions returns all tmux sessions (not just agent-* ones)
func listAllSessions() ([]TmuxSession, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}:#{session_attached}")
	output, err := cmd.Output()
	if err != nil {
		// No server running or no sessions
		return []TmuxSession{}, nil
	}

	var sessions []TmuxSession
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}
		sessions = append(sessions, TmuxSession{
			Name:     parts[0],
			Attached: parts[1] == "1",
		})
	}
	return sessions, nil
}

// listWindows returns all windows for a session
func listWindows(sessionName string) ([]Window, error) {
	cmd := exec.Command("tmux", "list-windows", "-t", sessionName,
		"-F", "#{window_id}:#{window_index}:#{window_name}:#{window_active}")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var windows []Window
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 4)
		if len(parts) < 4 {
			continue
		}
		idx, _ := strconv.Atoi(parts[1])
		windows = append(windows, Window{
			ID:     parts[0],
			Index:  idx,
			Name:   parts[2],
			Active: parts[3] == "1",
		})
	}
	return windows, nil
}

// listPanes returns all panes for a window
func listPanes(sessionName string, windowIndex int) ([]Pane, error) {
	target := sessionName + ":" + strconv.Itoa(windowIndex)
	cmd := exec.Command("tmux", "list-panes", "-t", target,
		"-F", "#{pane_id}:#{pane_index}:#{pane_title}:#{pane_current_command}:#{pane_active}:#{pane_width}:#{pane_height}")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var panes []Pane
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 7)
		if len(parts) < 7 {
			continue
		}
		idx, _ := strconv.Atoi(parts[1])
		width, _ := strconv.Atoi(parts[5])
		height, _ := strconv.Atoi(parts[6])

		paneTarget := target + "." + parts[1]
		panes = append(panes, Pane{
			ID:      parts[0],
			Index:   idx,
			Title:   parts[2],
			Command: parts[3],
			Active:  parts[4] == "1",
			Width:   width,
			Height:  height,
			Target:  paneTarget,
		})
	}
	return panes, nil
}

// BuildTreeNodes converts the Tree to a flat list of TreeNodes for rendering
func (t *Tree) BuildTreeNodes() []*TreeNode {
	var nodes []*TreeNode

	for _, sess := range t.Sessions {
		sessNode := &TreeNode{
			Type:     "session",
			Name:     sess.Name,
			Target:   sess.Name,
			Expanded: true,
			Level:    0,
			Attached: sess.Attached,
		}
		nodes = append(nodes, sessNode)

		if sessNode.Expanded {
			for _, win := range sess.Windows {
				winNode := &TreeNode{
					Type:     "window",
					Name:     win.Name,
					Target:   sess.Name + ":" + strconv.Itoa(win.Index),
					Expanded: true,
					Level:    1,
					Active:   win.Active,
				}
				sessNode.Children = append(sessNode.Children, winNode)
				nodes = append(nodes, winNode)

				if winNode.Expanded {
					for _, pane := range win.Panes {
						paneNode := &TreeNode{
							Type:   "pane",
							Name:   pane.Title,
							Target: pane.Target,
							Level:  2,
							Active: pane.Active,
						}
						if pane.Title == "" {
							paneNode.Name = pane.Command
						}
						if paneNode.Name == "" {
							paneNode.Name = "pane " + strconv.Itoa(pane.Index)
						}
						winNode.Children = append(winNode.Children, paneNode)
						nodes = append(nodes, paneNode)
					}
				}
			}
		}
	}

	return nodes
}

// CapturePane captures the content of a pane
func CapturePane(target string) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-t", target, "-p", "-e")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// SendMethod represents different ways to send the "execute" signal
type SendMethod int

const (
	SendMethodEnterSeparate    SendMethod = iota // text, then "Enter" separately
	SendMethodCmSeparate                         // text, then "C-m" separately
	SendMethodEnterAppended                      // text + "Enter" in one call
	SendMethodCmAppended                         // text + "C-m" in one call
	SendMethodEnterLiteral                       // text, then literal Enter key
	SendMethodEnterDelayed                       // text, sleep 500ms, then Enter
	SendMethodEnterDelayedLong                   // text, sleep 1500ms, then Enter (like tmux-cli)
	SendMethodCount                              // number of methods (for cycling)
)

// SendMethodNames returns human-readable names for send methods
func (m SendMethod) String() string {
	switch m {
	case SendMethodEnterSeparate:
		return "Enter (separate)"
	case SendMethodCmSeparate:
		return "C-m (separate)"
	case SendMethodEnterAppended:
		return "Enter (appended)"
	case SendMethodCmAppended:
		return "C-m (appended)"
	case SendMethodEnterLiteral:
		return "Enter (literal)"
	case SendMethodEnterDelayed:
		return "Enter (500ms delay)"
	case SendMethodEnterDelayedLong:
		return "Enter (1500ms delay)"
	default:
		return "unknown"
	}
}

// SendMethodDescription returns a description of what tmux commands are used
func (m SendMethod) Description() string {
	switch m {
	case SendMethodEnterSeparate:
		return "send-keys 'text'; send-keys Enter"
	case SendMethodCmSeparate:
		return "send-keys 'text'; send-keys C-m"
	case SendMethodEnterAppended:
		return "send-keys 'text' Enter"
	case SendMethodCmAppended:
		return "send-keys 'text' C-m"
	case SendMethodEnterLiteral:
		return "send-keys -l 'text'; send-keys Enter"
	case SendMethodEnterDelayed:
		return "send-keys 'text'; sleep 500ms; send-keys Enter"
	case SendMethodEnterDelayedLong:
		return "send-keys 'text'; sleep 1500ms; send-keys Enter"
	default:
		return ""
	}
}

// SendCommand sends a command to a pane using the default method
func SendCommand(target, command string) error {
	return SendCommandWithMethod(target, command, SendMethodEnterDelayed)
}

// SendEscape sends an Escape key to a pane.
func SendEscape(target string) error {
	return exec.Command("tmux", "send-keys", "-t", target, "Escape").Run()
}

// KillTarget kills a session, window, or pane by target.
// For sessions: target is the session name
// For windows: target is session:window_index
// For panes: target is session:window_index.pane_index
func KillTarget(nodeType, target string) error {
	switch nodeType {
	case "session":
		return exec.Command("tmux", "kill-session", "-t", target).Run()
	case "window":
		return exec.Command("tmux", "kill-window", "-t", target).Run()
	case "pane":
		return exec.Command("tmux", "kill-pane", "-t", target).Run()
	default:
		return nil
	}
}

// SwitchToTarget switches the client to the specified session/window/pane target.
// This is equivalent to what tmux choose-tree does when you select a target.
func SwitchToTarget(target string) error {
	return exec.Command("tmux", "switch-client", "-t", target).Run()
}

// SendCommandWithMethodAndExecutor sends a command using the specified method and executor.
func SendCommandWithMethodAndExecutor(target, command string, method SendMethod, exec TmuxExecutor) error {
	switch method {
	case SendMethodEnterSeparate:
		if err := exec.Run("send-keys", "-t", target, command); err != nil {
			return err
		}
		return exec.Run("send-keys", "-t", target, "Enter")
	case SendMethodCmSeparate:
		if err := exec.Run("send-keys", "-t", target, command); err != nil {
			return err
		}
		return exec.Run("send-keys", "-t", target, "C-m")
	case SendMethodEnterAppended:
		return exec.Run("send-keys", "-t", target, command, "Enter")
	case SendMethodCmAppended:
		return exec.Run("send-keys", "-t", target, command, "C-m")
	case SendMethodEnterLiteral:
		if err := exec.Run("send-keys", "-t", target, "-l", command); err != nil {
			return err
		}
		return exec.Run("send-keys", "-t", target, "Enter")
	case SendMethodEnterDelayed:
		if err := exec.Run("send-keys", "-t", target, command); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		return exec.Run("send-keys", "-t", target, "Enter")
	case SendMethodEnterDelayedLong:
		if err := exec.Run("send-keys", "-t", target, command); err != nil {
			return err
		}
		time.Sleep(1500 * time.Millisecond)
		return exec.Run("send-keys", "-t", target, "Enter")
	default:
		return SendCommandWithMethodAndExecutor(target, command, SendMethodEnterSeparate, exec)
	}
}

// SendCommandWithMethod sends a command using the specified method
func SendCommandWithMethod(target, command string, method SendMethod) error {
	switch method {
	case SendMethodEnterSeparate:
		// Send text, then Enter separately
		if err := exec.Command("tmux", "send-keys", "-t", target, command).Run(); err != nil {
			return err
		}
		return exec.Command("tmux", "send-keys", "-t", target, "Enter").Run()

	case SendMethodCmSeparate:
		// Send text, then C-m separately
		if err := exec.Command("tmux", "send-keys", "-t", target, command).Run(); err != nil {
			return err
		}
		return exec.Command("tmux", "send-keys", "-t", target, "C-m").Run()

	case SendMethodEnterAppended:
		// Send text and Enter in one command
		return exec.Command("tmux", "send-keys", "-t", target, command, "Enter").Run()

	case SendMethodCmAppended:
		// Send text and C-m in one command
		return exec.Command("tmux", "send-keys", "-t", target, command, "C-m").Run()

	case SendMethodEnterLiteral:
		// Send text literally (no interpretation), then Enter
		if err := exec.Command("tmux", "send-keys", "-t", target, "-l", command).Run(); err != nil {
			return err
		}
		return exec.Command("tmux", "send-keys", "-t", target, "Enter").Run()

	case SendMethodEnterDelayed:
		// Send text, wait 500ms, then Enter (for slow terminals/apps)
		if err := exec.Command("tmux", "send-keys", "-t", target, command).Run(); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		return exec.Command("tmux", "send-keys", "-t", target, "Enter").Run()

	case SendMethodEnterDelayedLong:
		// Send text, wait 1500ms, then Enter (like tmux-cli)
		if err := exec.Command("tmux", "send-keys", "-t", target, command).Run(); err != nil {
			return err
		}
		time.Sleep(1500 * time.Millisecond)
		return exec.Command("tmux", "send-keys", "-t", target, "Enter").Run()

	default:
		return SendCommandWithMethod(target, command, SendMethodEnterSeparate)
	}
}

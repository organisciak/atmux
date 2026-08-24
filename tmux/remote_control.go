package tmux

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// agentReadyTimeout bounds how long we wait for an agent to finish booting
	// before giving up on enabling remote control. Slow machines and cold
	// caches make this take a few seconds; waiting forever would hang the
	// launch.
	agentReadyTimeout = 30 * time.Second

	// agentReadyPoll is how often the pane is checked while starting.
	agentReadyPoll = 250 * time.Millisecond

	// agentSettleDelay gives the agent a moment after it starts to finish
	// drawing its input box. Typing into it before then can drop characters.
	agentSettleDelay = 1500 * time.Millisecond
)

// WaitForAgentPane blocks until a pane in the session is running an agent, and
// returns its target. Readiness is detected the same way panes are identified
// elsewhere: Claude Code reports its own semver as pane_current_command, so the
// pane stops being a shell once the agent is up.
func WaitForAgentPane(exec TmuxExecutor, sessionName string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)

	for {
		sessions, err := listAllSessionsWithExecutor(exec)
		if err == nil {
			for _, sess := range sessions {
				if sess.Name != sessionName {
					continue
				}
				windows, err := listWindowsWithExecutor(exec, sess.Name)
				if err != nil {
					break
				}
				for i := range windows {
					panes, err := listPanesWithExecutor(exec, sess.Name, windows[i].Index)
					if err != nil {
						continue
					}
					for _, pane := range panes {
						if isClaudePane(pane) {
							return pane.Target, nil
						}
					}
				}
			}
		}

		if time.Now().After(deadline) {
			return "", fmt.Errorf("no agent pane appeared in %s within %s", sessionName, timeout)
		}
		time.Sleep(agentReadyPoll)
	}
}

// EnableRemoteControl waits for the session's agent to come up and then sends
// /remote-control, naming the remote session after the tmux session.
//
// This exists so a session launched over a slow link is reachable from the
// Claude app without a second command: connect, run atmux, switch to the phone.
func EnableRemoteControl(exec TmuxExecutor, sessionName, displayName string) error {
	target, err := WaitForAgentPane(exec, sessionName, agentReadyTimeout)
	if err != nil {
		return err
	}

	// The agent process exists, but its input box may not be drawn yet.
	time.Sleep(agentSettleDelay)

	command := "/remote-control"
	if displayName != "" {
		command += " " + displayName
	}
	return SendCommandWithMethodAndExecutor(target, command, SendMethodEnterDelayed, exec)
}

// RemoteControlDisplayName converts a tmux session name into the label shown
// in the Claude app: "agent-my-app" becomes "my app". Shared with `atmux rc`
// so a session enabled at launch and one enabled later get the same name.
func RemoteControlDisplayName(sessionName string) string {
	name := sessionName
	for _, prefix := range []string{"agent-", "atmux-"} {
		name = strings.TrimPrefix(name, prefix)
	}
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	name = displayNameSpaces.ReplaceAllString(strings.TrimSpace(name), " ")
	if name == "" {
		name = filepath.Base(sessionName)
	}
	return name
}

var displayNameSpaces = regexp.MustCompile(`\s+`)

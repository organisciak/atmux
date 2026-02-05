package cmd

import (
	"fmt"
	"strings"

	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/spf13/cobra"
)

var (
	sendMethod  string
	sendRemote  string
	sendNoEnter bool
)

var sendCmd = &cobra.Command{
	Use:   "send <target> <text>",
	Short: "Send text to a tmux pane",
	Long: `Send text to a specific tmux pane.

Target format: session:window.pane
  - agent-project:agents.0   (session:window.pane)
  - agent-foo:0.1            (session:windowIndex.paneIndex)

Methods:
  - enter         Send text, then "Enter" separately
  - enter-delayed Send text, wait 500ms, then "Enter" (default)
  - enter-literal Send text with -l flag, then "Enter"
  - cm            Send text, then "C-m" separately

Examples:
  atmux send agent-project:agents.0 "Take a beads task"
  atmux send --no-enter agent-foo:0.0 "/compact"
  atmux send --method=enter-literal agent-foo:0.0 "text with special chars"
  atmux send --remote=server1 agent-foo:0.0 "hello"`,
	Args: cobra.ExactArgs(2),
	RunE: runSend,
}

func init() {
	sendCmd.Flags().StringVarP(&sendMethod, "method", "m", "enter-delayed",
		"Send method: enter, enter-delayed, enter-literal, cm")
	sendCmd.Flags().StringVarP(&sendRemote, "remote", "r", "",
		"Remote host(s) to send to (comma-separated)")
	sendCmd.Flags().BoolVarP(&sendNoEnter, "no-enter", "n", false,
		"Send text without pressing Enter")

	rootCmd.AddCommand(sendCmd)
}

func runSend(cmd *cobra.Command, args []string) error {
	target := args[0]
	text := args[1]

	// Build executor(s)
	var executors []tmux.TmuxExecutor
	if sendRemote != "" {
		// Use only remote executors specified by --remote flag
		for _, host := range strings.Split(sendRemote, ",") {
			host = strings.TrimSpace(host)
			if host == "" {
				continue
			}
			executors = append(executors, tmux.NewRemoteExecutor(host, 22, "ssh", host))
		}
	} else {
		// Use local executor
		executors = []tmux.TmuxExecutor{tmux.NewLocalExecutor()}
	}
	defer closeExecutors(executors)

	// Parse send method
	method := parseMethod(sendMethod)

	// Send to each executor
	for _, exec := range executors {
		var err error
		if sendNoEnter {
			// Send text without Enter
			err = exec.Run("send-keys", "-t", target, text)
		} else {
			// Use the standard send method
			err = tmux.SendCommandWithMethodAndExecutor(target, text, method, exec)
		}

		if err != nil {
			hostLabel := "local"
			if exec.IsRemote() {
				hostLabel = exec.HostLabel()
			}
			return fmt.Errorf("failed to send to %s: %w", hostLabel, err)
		}
	}

	return nil
}

// parseMethod converts a method string to a SendMethod enum value
func parseMethod(s string) tmux.SendMethod {
	switch strings.ToLower(s) {
	case "enter":
		return tmux.SendMethodEnterSeparate
	case "enter-delayed":
		return tmux.SendMethodEnterDelayed
	case "enter-literal":
		return tmux.SendMethodEnterLiteral
	case "cm":
		return tmux.SendMethodCmSeparate
	case "enter-appended":
		return tmux.SendMethodEnterAppended
	case "cm-appended":
		return tmux.SendMethodCmAppended
	case "enter-delayed-long":
		return tmux.SendMethodEnterDelayedLong
	default:
		return tmux.SendMethodEnterDelayed
	}
}

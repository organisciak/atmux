package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/spf13/cobra"
)

// buildExecutors builds a list of TmuxExecutors from config and --remote flag.
// The local executor is always first. Remote executors follow.
func buildExecutors(remoteFlag string) ([]tmux.TmuxExecutor, error) {
	executors := []tmux.TmuxExecutor{tmux.NewLocalExecutor()}

	cfg, err := loadRemoteConfig()
	if err != nil {
		return nil, err
	}
	remoteHosts, err := config.ResolveRemoteHosts(cfg, remoteFlag, true)
	if err != nil {
		return nil, err
	}
	for _, rh := range remoteHosts {
		executors = append(executors, tmux.NewRemoteExecutor(
			rh.Host, rh.Port, rh.AttachMethod, rh.Alias,
		))
	}

	return executors, nil
}

// loadRemoteConfig loads remote host config from global and local configs.
func loadRemoteConfig() (*config.Config, error) {
	localPath := filepath.Join(".", config.DefaultConfigName)
	cfg, err := config.LoadConfig(localPath)
	if err != nil || cfg == nil {
		if err != nil {
			return nil, err
		}
		return &config.Config{}, nil
	}
	return cfg, nil
}

// closeExecutors releases per-process executor resources. SSH ControlMaster
// sockets are intentionally left running so the next atmux invocation reuses
// them; `atmux remote disconnect` tears them down explicitly.
func closeExecutors(executors []tmux.TmuxExecutor) {
	for _, exec := range executors {
		exec.Close()
	}
}

var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Manage saved remote hosts and their connections",
}

var remoteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved remote hosts and their connection state",
	Args:  cobra.NoArgs,
	RunE:  runRemoteList,
}

var remoteDisconnectCmd = &cobra.Command{
	Use:   "disconnect [host-or-alias...]",
	Short: "Close persistent SSH connections to remote hosts",
	Long: "Close the shared SSH ControlMaster for one or more hosts.\n" +
		"With no arguments, disconnects every saved host.",
	RunE: runRemoteDisconnect,
}

func init() {
	rootCmd.AddCommand(remoteCmd)
	remoteCmd.AddCommand(remoteListCmd)
	remoteCmd.AddCommand(remoteDisconnectCmd)
}

// remoteExecutors resolves saved hosts (optionally filtered by name) into
// RemoteExecutors, skipping the local executor.
func remoteExecutors(filter string) ([]*tmux.RemoteExecutor, error) {
	cfg, err := loadRemoteConfig()
	if err != nil {
		return nil, err
	}
	hosts, err := config.ResolveRemoteHosts(cfg, filter, filter == "")
	if err != nil {
		return nil, err
	}

	executors := make([]*tmux.RemoteExecutor, 0, len(hosts))
	for _, rh := range hosts {
		executors = append(executors, tmux.NewRemoteExecutor(rh.Host, rh.Port, rh.AttachMethod, rh.Alias))
	}
	return executors, nil
}

func runRemoteList(cmd *cobra.Command, args []string) error {
	executors, err := remoteExecutors("")
	if err != nil {
		return err
	}
	if len(executors) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No remote hosts configured. Add one with remote_host: in your config.")
		return nil
	}

	for _, exec := range executors {
		state := "disconnected"
		if exec.ControlSocketActive() {
			state = "connected"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-30s %-6s %s\n",
			exec.Alias, exec.Host, exec.AttachMethod, state)
	}
	return nil
}

func runRemoteDisconnect(cmd *cobra.Command, args []string) error {
	executors, err := remoteExecutors(strings.Join(args, ","))
	if err != nil {
		return err
	}

	var failures []string
	for _, exec := range executors {
		if err := exec.Disconnect(); err != nil {
			failures = append(failures, err.Error())
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Disconnected %s\n", exec.Alias)
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

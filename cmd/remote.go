package cmd

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/tmux"
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

// closeExecutors cleans up all executors (e.g., SSH ControlMaster sockets).
func closeExecutors(executors []tmux.TmuxExecutor) {
	for _, exec := range executors {
		exec.Close()
	}
}

// registerCleanupSignals registers signal handlers that clean up executors
// on SIGINT/SIGTERM before exiting.
func registerCleanupSignals(executors []tmux.TmuxExecutor) {
	hasRemote := false
	for _, exec := range executors {
		if exec.IsRemote() {
			hasRemote = true
			break
		}
	}
	if !hasRemote {
		return
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		closeExecutors(executors)
		os.Exit(1)
	}()
}

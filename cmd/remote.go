package cmd

import (
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/tmux"
)

// buildExecutors builds a list of TmuxExecutors from config and --remote flag.
// The local executor is always first. Remote executors follow.
func buildExecutors(remoteFlag string) []tmux.TmuxExecutor {
	executors := []tmux.TmuxExecutor{tmux.NewLocalExecutor()}

	// Add executors from config
	cfg := loadRemoteConfig()
	for _, rh := range cfg.RemoteHosts {
		executors = append(executors, tmux.NewRemoteExecutor(
			rh.Host, rh.Port, rh.AttachMethod, rh.Alias,
		))
	}

	// Add executors from --remote flag (comma-separated user@host values)
	if remoteFlag != "" {
		for _, host := range strings.Split(remoteFlag, ",") {
			host = strings.TrimSpace(host)
			if host == "" {
				continue
			}
			// Check if this host is already in the config
			found := false
			for _, rh := range cfg.RemoteHosts {
				if rh.Host == host || rh.Alias == host {
					found = true
					break
				}
			}
			if !found {
				executors = append(executors, tmux.NewRemoteExecutor(host, 22, "ssh", host))
			}
		}
	}

	return executors
}

// loadRemoteConfig loads remote host config from global and local configs.
func loadRemoteConfig() *config.Config {
	localPath := filepath.Join(".", config.DefaultConfigName)
	cfg, err := config.LoadConfig(localPath)
	if err != nil || cfg == nil {
		return &config.Config{}
	}
	return cfg
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

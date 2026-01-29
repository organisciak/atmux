package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "agent-tmux",
	Short: "Manage tmux sessions for AI coding agents",
	Long: `agent-tmux creates and manages tmux sessions optimized for AI coding workflows.

It creates a session with:
  - An 'agents' window with codex and claude panes
  - A 'diag' window for diagnostics
  - Project-specific windows/panes from .agent-tmux.conf`,
	RunE: runRoot,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runRoot(cmd *cobra.Command, args []string) error {
	// Get working directory
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Create session config
	session := tmux.NewSession(workingDir)

	// Check if session already exists
	if session.Exists() {
		fmt.Printf("Attaching to existing session: %s\n", session.Name)
		return session.Attach()
	}

	// Create new session
	fmt.Printf("Creating new session: %s\n", session.Name)
	if err := session.Create(); err != nil {
		return err
	}

	// Check for project-specific config
	configPath := filepath.Join(workingDir, config.DefaultConfigName)
	if config.Exists(configPath) {
		cfg, err := config.Parse(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to parse config: %v\n", err)
		} else {
			if err := session.ApplyConfig(cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to apply config: %v\n", err)
			}
		}
	}

	// Select default window/pane and attach
	session.SelectDefault()
	return session.Attach()
}

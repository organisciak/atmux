package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/porganisciak/agent-tmux/config"
	"github.com/spf13/cobra"
)

var forceInit bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a .agent-tmux.conf template in the current directory",
	Long: `Creates a new .agent-tmux.conf configuration file in the current directory.

The config file allows you to define project-specific tmux windows and panes
that will be created when you start an atmux session.`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVarP(&forceInit, "force", "f", false, "Overwrite existing config file")
}

func runInit(cmd *cobra.Command, args []string) error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	configPath := filepath.Join(workingDir, config.DefaultConfigName)

	// Check if file already exists
	if config.Exists(configPath) && !forceInit {
		return fmt.Errorf("config file already exists: %s\nUse --force to overwrite", configPath)
	}

	// Write template
	if err := os.WriteFile(configPath, []byte(config.DefaultTemplate()), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Created %s\n", configPath)
	fmt.Println("Edit this file to configure project-specific windows and panes.")
	return nil
}

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/porganisciak/agent-tmux/config"
	"github.com/spf13/cobra"
)

var forceInit bool
var globalInit bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a config template",
	Long: `Creates a configuration file for atmux.

By default, creates .agent-tmux.conf in the current directory.
Use --global to create the global config at ~/.config/atmux/config.`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVarP(&forceInit, "force", "f", false, "Overwrite existing config file")
	initCmd.Flags().BoolVarP(&globalInit, "global", "g", false, "Create global config (~/.config/atmux/config)")
}

func runInit(cmd *cobra.Command, args []string) error {
	var configPath string
	var template string

	if globalInit {
		// Global config
		path, err := config.GlobalConfigPath()
		if err != nil {
			return fmt.Errorf("failed to get global config path: %w", err)
		}
		configPath = path
		template = config.GlobalTemplate()

		// Ensure directory exists
		dir, err := config.SettingsDir()
		if err != nil {
			return fmt.Errorf("failed to get settings directory: %w", err)
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
	} else {
		// Local config
		workingDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
		configPath = filepath.Join(workingDir, config.DefaultConfigName)
		template = config.DefaultTemplate()
	}

	// Check if file already exists
	if config.Exists(configPath) && !forceInit {
		return fmt.Errorf("config file already exists: %s\nUse --force to overwrite", configPath)
	}

	// Write template
	if err := os.WriteFile(configPath, []byte(template), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Created %s\n", configPath)
	if globalInit {
		fmt.Println("Edit this file to configure your default agent setup.")
	} else {
		fmt.Println("Edit this file to configure project-specific windows and panes.")
	}
	return nil
}

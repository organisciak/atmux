package config

import (
	"bufio"
	"os"
	"strings"
)

type PaneConfig struct {
	Command  string
	Vertical bool
}

type WindowConfig struct {
	Name  string
	Panes []PaneConfig
}

type Config struct {
	Windows     []WindowConfig // New windows to create
	AgentPanes  []PaneConfig   // Panes to add to agents window
}

// DefaultConfigName is the name of the config file to look for
const DefaultConfigName = ".agent-tmux.conf"

// Parse reads and parses an agent-tmux config file
func Parse(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	config := &Config{}
	var currentWindow *WindowConfig

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse directive:value
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		directive := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch directive {
		case "window":
			// Start a new window
			config.Windows = append(config.Windows, WindowConfig{
				Name:  value,
				Panes: []PaneConfig{},
			})
			currentWindow = &config.Windows[len(config.Windows)-1]

		case "pane":
			// Add horizontal pane to current window
			if currentWindow != nil {
				currentWindow.Panes = append(currentWindow.Panes, PaneConfig{
					Command:  value,
					Vertical: false,
				})
			}

		case "vpane":
			// Add vertical pane to current window
			if currentWindow != nil {
				currentWindow.Panes = append(currentWindow.Panes, PaneConfig{
					Command:  value,
					Vertical: true,
				})
			}

		case "agents":
			// Add horizontal pane to agents window
			config.AgentPanes = append(config.AgentPanes, PaneConfig{
				Command:  value,
				Vertical: false,
			})

		case "vagents":
			// Add vertical pane to agents window
			config.AgentPanes = append(config.AgentPanes, PaneConfig{
				Command:  value,
				Vertical: true,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return config, nil
}

// Exists checks if a config file exists at the given path
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DefaultTemplate returns a template for a new config file
func DefaultTemplate() string {
	return `# agent-tmux configuration
# This file configures additional windows and panes for your tmux session

# Directives:
#   window:name    - Create a new window with the given name
#   pane:command   - Add a horizontal pane to the current window
#   vpane:command  - Add a vertical pane to the current window
#   agents:command - Add a horizontal pane to the agents window
#   vagents:command - Add a vertical pane to the agents window

# Example: Development server window
# window:dev
# pane:pnpm dev
# pane:pnpm run emulators

# Example: Add monitoring to agents window
# agents:htop
`
}

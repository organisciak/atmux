package config

import (
	"bufio"
	"os"
	"path/filepath"
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

// AgentConfig represents a core agent pane configuration
type AgentConfig struct {
	Command string
}

// RemoteHostConfig represents a remote host configuration
type RemoteHostConfig struct {
	Host         string
	Port         int
	AttachMethod string
	Alias        string
}

type Config struct {
	Windows     []WindowConfig     // New windows to create
	AgentPanes  []PaneConfig       // Extra panes to add to agents window
	CoreAgents  []AgentConfig      // Core agent panes (from agent: directive)
	RemoteHosts []RemoteHostConfig // Remote hosts for sessions list
}

// DefaultConfigName is the name of the config file to look for
const DefaultConfigName = ".agent-tmux.conf"

// GlobalConfigName is the name of the global config file
const GlobalConfigName = "config"

// GlobalConfigPath returns the path to the global config file
func GlobalConfigPath() (string, error) {
	dir, err := SettingsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, GlobalConfigName), nil
}

// LoadConfig loads configuration, merging global and local configs.
// Local config takes precedence over global config.
func LoadConfig(localPath string) (*Config, error) {
	// Start with global config
	globalPath, err := GlobalConfigPath()
	if err != nil {
		globalPath = ""
	}

	var globalCfg, localCfg *Config

	if globalPath != "" && Exists(globalPath) {
		globalCfg, _ = Parse(globalPath)
	}

	if localPath != "" && Exists(localPath) {
		localCfg, _ = Parse(localPath)
	}

	return mergeConfigs(globalCfg, localCfg), nil
}

// mergeConfigs merges global and local configs. Local takes precedence.
func mergeConfigs(global, local *Config) *Config {
	result := &Config{}

	// If no configs, return empty
	if global == nil && local == nil {
		return result
	}

	// Start with global
	if global != nil {
		result.CoreAgents = append(result.CoreAgents, global.CoreAgents...)
		result.AgentPanes = append(result.AgentPanes, global.AgentPanes...)
		result.Windows = append(result.Windows, global.Windows...)
	}

	// Override/add from local
	if local != nil {
		// If local defines core agents, replace global ones
		if len(local.CoreAgents) > 0 {
			result.CoreAgents = local.CoreAgents
		}
		// Append additional panes and windows from local
		result.AgentPanes = append(result.AgentPanes, local.AgentPanes...)
		result.Windows = append(result.Windows, local.Windows...)
	}

	return result
}

// Parse reads and parses an atmux (agent-tmux) config file
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

		case "agent":
			// Core agent pane
			config.CoreAgents = append(config.CoreAgents, AgentConfig{
				Command: value,
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
	return `# atmux configuration
# This file configures windows and panes for your tmux session

# Directives:
#   agent:command   - Define a core agent pane (replaces defaults if set)
#   agents:command  - Add an extra horizontal pane to the agents window
#   vagents:command - Add an extra vertical pane to the agents window
#   window:name     - Create a new window with the given name
#   pane:command    - Add a horizontal pane to the current window
#   vpane:command   - Add a vertical pane to the current window

# Example: Custom agent setup (overrides defaults)
# agent:claude --dangerously-skip-permissions
# agent:codex --full-auto

# Example: Development server window
# window:dev
# pane:pnpm dev
# pane:pnpm run emulators

# Example: Add monitoring to agents window
# agents:htop
`
}

// GlobalTemplate returns a template for the global config file
func GlobalTemplate() string {
	return `# atmux global configuration
# Located at: ~/.config/atmux/config
# Local .agent-tmux.conf files override these settings

# Core agent panes (shown in every session's agents window)
agent:claude --dangerously-skip-permissions
agent:codex --full-auto

# Directives:
#   agent:command   - Define a core agent pane
#   agents:command  - Add an extra horizontal pane to agents window
#   vagents:command - Add an extra vertical pane to agents window
#   window:name     - Create a window in every session
#   pane:command    - Add pane to the current window
#   vpane:command   - Add vertical pane to the current window
`
}

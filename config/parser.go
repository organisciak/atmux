package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

// RemoteProjectConfig represents a reusable remote atmux project entry.
type RemoteProjectConfig struct {
	Name        string
	Host        string // Remote host alias/host (resolves against remote_host entries)
	WorkingDir  string // Remote working directory
	SessionName string // Preferred tmux session name on the remote host
}

type Config struct {
	Windows        []WindowConfig        // New windows to create
	AgentPanes     []PaneConfig          // Extra panes to add to agents window
	CoreAgents     []AgentConfig         // Core agent panes (from agent: directive)
	RemoteHosts    []RemoteHostConfig    // Remote hosts for sessions list
	RemoteProjects []RemoteProjectConfig // Reusable remote projects
}

const (
	defaultRemotePort         = 22
	defaultRemoteAttachMethod = "ssh"
)

var remoteProjectSessionSlug = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// NormalizeRemoteHost validates and normalizes a remote host config.
func NormalizeRemoteHost(rh RemoteHostConfig) (RemoteHostConfig, error) {
	rh.Host = strings.TrimSpace(rh.Host)
	if rh.Host == "" {
		return RemoteHostConfig{}, fmt.Errorf("host is required")
	}

	rh.Alias = strings.TrimSpace(rh.Alias)
	if rh.Alias == "" {
		rh.Alias = rh.Host
	}

	if rh.Port <= 0 {
		rh.Port = defaultRemotePort
	}

	rh.AttachMethod = strings.ToLower(strings.TrimSpace(rh.AttachMethod))
	if rh.AttachMethod == "" {
		rh.AttachMethod = defaultRemoteAttachMethod
	}
	switch rh.AttachMethod {
	case "ssh", "mosh":
	default:
		return RemoteHostConfig{}, fmt.Errorf("attach method must be 'ssh' or 'mosh'")
	}

	return rh, nil
}

// NormalizeRemoteProject validates and normalizes a remote project config.
func NormalizeRemoteProject(rp RemoteProjectConfig) (RemoteProjectConfig, error) {
	rp.Name = strings.TrimSpace(rp.Name)
	if rp.Name == "" {
		return RemoteProjectConfig{}, fmt.Errorf("name is required")
	}

	rp.Host = strings.TrimSpace(rp.Host)
	if rp.Host == "" {
		return RemoteProjectConfig{}, fmt.Errorf("host is required")
	}

	rp.WorkingDir = strings.TrimSpace(rp.WorkingDir)
	if rp.WorkingDir == "" {
		return RemoteProjectConfig{}, fmt.Errorf("working directory is required")
	}

	rp.SessionName = strings.TrimSpace(rp.SessionName)
	if rp.SessionName == "" {
		rp.SessionName = defaultRemoteProjectSessionName(rp.Name)
	}

	return rp, nil
}

func defaultRemoteProjectSessionName(name string) string {
	slug := remoteProjectSessionSlug.ReplaceAllString(strings.TrimSpace(name), "_")
	if slug == "" {
		slug = "project"
	}
	return "agent-" + slug
}

// ResolveRemoteHosts resolves a comma-separated remote host flag against config
// entries. When includeConfigured is true, configured hosts are included even if
// they are not explicitly listed in remoteFlag.
func ResolveRemoteHosts(cfg *Config, remoteFlag string, includeConfigured bool) ([]RemoteHostConfig, error) {
	if cfg == nil {
		cfg = &Config{}
	}

	configured := make([]RemoteHostConfig, 0, len(cfg.RemoteHosts))
	lookup := make(map[string]RemoteHostConfig, len(cfg.RemoteHosts)*2)
	for _, rh := range cfg.RemoteHosts {
		normalized, err := NormalizeRemoteHost(rh)
		if err != nil {
			return nil, fmt.Errorf("invalid configured remote host %q: %w", rh.Host, err)
		}
		configured = append(configured, normalized)
		lookup[normalized.Host] = normalized
		lookup[normalized.Alias] = normalized
	}

	remoteFlag = strings.TrimSpace(remoteFlag)
	if remoteFlag == "" {
		if !includeConfigured {
			return []RemoteHostConfig{}, nil
		}
		return dedupeRemoteHosts(configured), nil
	}

	var resolved []RemoteHostConfig
	seen := map[string]bool{}
	for _, token := range strings.Split(remoteFlag, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		rh, ok := lookup[token]
		if !ok {
			rh = RemoteHostConfig{Host: token, Alias: token}
		}

		normalized, err := NormalizeRemoteHost(rh)
		if err != nil {
			return nil, fmt.Errorf("invalid remote host %q: %w", token, err)
		}

		key := remoteHostKey(normalized)
		if seen[key] {
			continue
		}
		seen[key] = true
		resolved = append(resolved, normalized)
	}

	if includeConfigured {
		for _, rh := range configured {
			key := remoteHostKey(rh)
			if seen[key] {
				continue
			}
			seen[key] = true
			resolved = append(resolved, rh)
		}
	}

	return resolved, nil
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
		globalCfg, err = Parse(globalPath)
		if err != nil {
			return nil, err
		}
	}

	if localPath != "" && Exists(localPath) {
		localCfg, err = Parse(localPath)
		if err != nil {
			return nil, err
		}
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
		result.RemoteHosts = append(result.RemoteHosts, global.RemoteHosts...)
		result.RemoteProjects = append(result.RemoteProjects, global.RemoteProjects...)
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
		result.RemoteHosts = mergeRemoteHosts(result.RemoteHosts, local.RemoteHosts)
		result.RemoteProjects = mergeRemoteProjects(result.RemoteProjects, local.RemoteProjects)
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
	var currentRemote *RemoteHostConfig
	var currentRemoteProject *RemoteProjectConfig

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
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

		case "remote_host":
			if value == "" {
				return nil, fmt.Errorf("%s:%d: remote_host requires a host value", path, lineNumber)
			}
			config.RemoteHosts = append(config.RemoteHosts, RemoteHostConfig{
				Host: value,
			})
			currentRemote = &config.RemoteHosts[len(config.RemoteHosts)-1]

		case "remote_alias":
			if currentRemote == nil {
				return nil, fmt.Errorf("%s:%d: remote_alias requires a preceding remote_host", path, lineNumber)
			}
			if value == "" {
				return nil, fmt.Errorf("%s:%d: remote_alias requires a value", path, lineNumber)
			}
			currentRemote.Alias = value

		case "remote_port":
			if currentRemote == nil {
				return nil, fmt.Errorf("%s:%d: remote_port requires a preceding remote_host", path, lineNumber)
			}
			port, err := strconv.Atoi(value)
			if err != nil || port <= 0 {
				return nil, fmt.Errorf("%s:%d: invalid remote_port %q", path, lineNumber, value)
			}
			currentRemote.Port = port

		case "remote_attach":
			if currentRemote == nil {
				return nil, fmt.Errorf("%s:%d: remote_attach requires a preceding remote_host", path, lineNumber)
			}
			attach := strings.ToLower(strings.TrimSpace(value))
			if attach != "ssh" && attach != "mosh" {
				return nil, fmt.Errorf("%s:%d: remote_attach must be 'ssh' or 'mosh'", path, lineNumber)
			}
			currentRemote.AttachMethod = attach

		case "remote_project":
			if value == "" {
				return nil, fmt.Errorf("%s:%d: remote_project requires a name", path, lineNumber)
			}
			config.RemoteProjects = append(config.RemoteProjects, RemoteProjectConfig{
				Name: value,
			})
			currentRemoteProject = &config.RemoteProjects[len(config.RemoteProjects)-1]

		case "remote_project_host":
			if currentRemoteProject == nil {
				return nil, fmt.Errorf("%s:%d: remote_project_host requires a preceding remote_project", path, lineNumber)
			}
			if value == "" {
				return nil, fmt.Errorf("%s:%d: remote_project_host requires a host value", path, lineNumber)
			}
			currentRemoteProject.Host = value

		case "remote_project_dir":
			if currentRemoteProject == nil {
				return nil, fmt.Errorf("%s:%d: remote_project_dir requires a preceding remote_project", path, lineNumber)
			}
			if value == "" {
				return nil, fmt.Errorf("%s:%d: remote_project_dir requires a directory value", path, lineNumber)
			}
			currentRemoteProject.WorkingDir = value

		case "remote_project_session":
			if currentRemoteProject == nil {
				return nil, fmt.Errorf("%s:%d: remote_project_session requires a preceding remote_project", path, lineNumber)
			}
			if value == "" {
				return nil, fmt.Errorf("%s:%d: remote_project_session requires a value", path, lineNumber)
			}
			currentRemoteProject.SessionName = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	for i, rh := range config.RemoteHosts {
		normalized, err := NormalizeRemoteHost(rh)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid remote host %q: %w", path, rh.Host, err)
		}
		config.RemoteHosts[i] = normalized
	}

	for i, rp := range config.RemoteProjects {
		normalized, err := NormalizeRemoteProject(rp)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid remote project %q: %w", path, rp.Name, err)
		}
		config.RemoteProjects[i] = normalized
	}

	return config, nil
}

func mergeRemoteHosts(base, overrides []RemoteHostConfig) []RemoteHostConfig {
	merged := append([]RemoteHostConfig{}, base...)
	for _, override := range overrides {
		replaced := false
		for i := range merged {
			if sameRemoteIdentity(merged[i], override) {
				merged[i] = override
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, override)
		}
	}
	return dedupeRemoteHosts(merged)
}

func mergeRemoteProjects(base, overrides []RemoteProjectConfig) []RemoteProjectConfig {
	merged := append([]RemoteProjectConfig{}, base...)
	for _, override := range overrides {
		replaced := false
		for i := range merged {
			if sameRemoteProjectIdentity(merged[i], override) {
				merged[i] = override
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, override)
		}
	}
	return dedupeRemoteProjects(merged)
}

func dedupeRemoteHosts(hosts []RemoteHostConfig) []RemoteHostConfig {
	var deduped []RemoteHostConfig
	for _, host := range hosts {
		replaced := false
		for i := range deduped {
			if sameRemoteIdentity(deduped[i], host) {
				deduped[i] = host
				replaced = true
				break
			}
		}
		if !replaced {
			deduped = append(deduped, host)
		}
	}
	return deduped
}

func dedupeRemoteProjects(projects []RemoteProjectConfig) []RemoteProjectConfig {
	var deduped []RemoteProjectConfig
	for _, project := range projects {
		replaced := false
		for i := range deduped {
			if sameRemoteProjectIdentity(deduped[i], project) {
				deduped[i] = project
				replaced = true
				break
			}
		}
		if !replaced {
			deduped = append(deduped, project)
		}
	}
	return deduped
}

func sameRemoteIdentity(a, b RemoteHostConfig) bool {
	if a.Host != "" && b.Host != "" && a.Host == b.Host {
		return true
	}
	if a.Alias != "" && b.Alias != "" && a.Alias == b.Alias {
		return true
	}
	if a.Host != "" && b.Alias != "" && a.Host == b.Alias {
		return true
	}
	if a.Alias != "" && b.Host != "" && a.Alias == b.Host {
		return true
	}
	return false
}

func sameRemoteProjectIdentity(a, b RemoteProjectConfig) bool {
	return strings.EqualFold(strings.TrimSpace(a.Name), strings.TrimSpace(b.Name))
}

func remoteHostKey(rh RemoteHostConfig) string {
	return fmt.Sprintf("%s:%d", rh.Host, rh.Port)
}

// Exists checks if a config file exists at the given path
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DefaultTemplate returns a template for a new config file
func DefaultTemplate() string {
	return `# atmux (agent-tmux) configuration
# Tips and docs: https://github.com/organisciak/atmux
# This file configures windows and panes for your tmux session

# ── Directive Reference ──────────────────────────────────────────────
#   agent:command    - Define a core agent pane (replaces defaults if set)
#   agents:command   - Add an extra horizontal pane to the agents window
#   vagents:command  - Add an extra vertical pane to the agents window
#   window:name      - Create a new window with the given name
#   pane:command     - Add a horizontal split pane to the current window
#   vpane:command    - Add a vertical split pane to the current window
#   remote_host:...  - Define a remote host for --remote alias resolution
#   remote_alias:..  - Optional alias for the last remote_host
#   remote_port:...  - Optional SSH port for the last remote_host
#   remote_attach:.  - Optional attach method for the last remote_host (ssh|mosh)
#   remote_project:........ - Define a reusable remote project name
#   remote_project_host:... - Host/alias for the last remote_project
#   remote_project_dir:.... - Remote working dir for the last remote_project
#   remote_project_session: - Optional tmux session name for the last remote_project

# ── Custom Agent Setup ───────────────────────────────────────────────
# Override the default agent panes. When any agent: line is present,
# it replaces the built-in defaults entirely.
#
# agent:claude --dangerously-skip-permissions
# agent:codex --full-auto

# ── Extra Panes in the Agents Window ─────────────────────────────────
# These panes are added alongside your agent panes in the first window.
# Use "agents:" for a horizontal split (side-by-side) or
# "vagents:" for a vertical split (top/bottom).
#
# agents:htop
# vagents:tail -f /var/log/syslog

# ── Additional Windows ──────────────────────────────────────────────
# Each "window:" directive creates a new tmux window. Panes defined
# after it (with pane: or vpane:) are added to that window.
#
# --- Dev server window: run your app and bundler side by side ---
# window:dev
# pane:npm run dev
# pane:npm run watch
#
# --- Logs window: tail multiple log files ---
# window:logs
# pane:tail -f ./logs/app.log
# vpane:tail -f ./logs/error.log
#
# --- Tests window: run tests in watch mode ---
# window:tests
# pane:npm test -- --watch
#
# --- Database window: interactive DB shell with a monitoring pane ---
# window:database
# pane:psql -d myapp
# vpane:watch -n5 'pg_isready && echo "DB OK"'

# ── Layout Guide ─────────────────────────────────────────────────────
# pane:  splits horizontally (left | right) — good for side-by-side views
# vpane: splits vertically  (top / bottom)  — good for stacked output
#
# Example: three-pane layout (editor + server + tests)
# window:workspace
# pane:nvim .
# vpane:npm run dev
# vpane:npm test -- --watch

# ── Remote Hosts ─────────────────────────────────────────────────────
# Define remote hosts so you can use short aliases with --remote=devbox
#
# remote_host:user@devbox.example.com
# remote_alias:devbox
# remote_port:22
# remote_attach:ssh

# ── Remote Projects ──────────────────────────────────────────────────
# Define reusable remote project entries for quick access
#
# remote_project:atmux
# remote_project_host:devbox
# remote_project_dir:/home/user/projects/atmux
# remote_project_session:agent-atmux
`
}

// GlobalTemplate returns a template for the global config file
func GlobalTemplate() string {
	return `# atmux (agent-tmux) global configuration
# Tips and docs: https://github.com/organisciak/atmux
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
#   remote_host:... - Define a remote host
#   remote_alias:.. - Optional alias for the last remote_host
#   remote_port:... - Optional SSH port for the last remote_host
#   remote_attach:. - Optional attach method for the last remote_host (ssh|mosh)
#   remote_project:........ - Define a reusable remote project name
#   remote_project_host:... - Host/alias for the last remote_project
#   remote_project_dir:.... - Remote working dir for the last remote_project
#   remote_project_session: - Optional tmux session name for the last remote_project

# Example remote host
# remote_host:user@devbox.example.com
# remote_alias:devbox
# remote_port:22
# remote_attach:ssh

# Example remote project
# remote_project:atmux
# remote_project_host:devbox
# remote_project_dir:/home/user/projects/atmux
# remote_project_session:agent-atmux
`
}

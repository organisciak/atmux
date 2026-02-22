package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// AttachStrategy controls how remote sessions are attached when inside tmux.
type AttachStrategy string

const (
	// AttachStrategyAuto opens a new tmux window when inside tmux, otherwise attaches directly.
	AttachStrategyAuto AttachStrategy = "auto"
	// AttachStrategyReplace attaches directly in the current terminal (replaces the TUI).
	AttachStrategyReplace AttachStrategy = "replace"
	// AttachStrategyNewWindow always opens a new tmux window for the remote session.
	AttachStrategyNewWindow AttachStrategy = "new-window"
)

// ValidAttachStrategy reports whether s is a recognized attach strategy.
func ValidAttachStrategy(s AttachStrategy) bool {
	switch s {
	case AttachStrategyAuto, AttachStrategyReplace, AttachStrategyNewWindow:
		return true
	}
	return false
}

const (
	settingsDirName       = "atmux"
	legacySettingsDirName = "agent-tmux"
)

// StalenessConfig controls session staleness indicators in the sessions TUI.
type StalenessConfig struct {
	FreshDuration       string `json:"fresh_duration,omitempty"`       // default "24h"
	StaleDuration       string `json:"stale_duration,omitempty"`       // default "48h"
	SuggestionThreshold int    `json:"suggestion_threshold,omitempty"` // default 7
	Disabled            bool   `json:"disabled,omitempty"`
}

const (
	defaultFreshDuration       = 24 * time.Hour
	defaultStaleDuration       = 48 * time.Hour
	defaultSuggestionThreshold = 7
)

// ParsedStalenessThresholds returns the fresh and stale durations, falling back to defaults.
func (c *StalenessConfig) ParsedStalenessThresholds() (fresh, stale time.Duration) {
	fresh = defaultFreshDuration
	stale = defaultStaleDuration
	if c == nil {
		return
	}
	if c.FreshDuration != "" {
		if d, err := time.ParseDuration(c.FreshDuration); err == nil {
			fresh = d
		}
	}
	if c.StaleDuration != "" {
		if d, err := time.ParseDuration(c.StaleDuration); err == nil {
			stale = d
		}
	}
	return
}

// EffectiveSuggestionThreshold returns the suggestion threshold, falling back to the default.
func (c *StalenessConfig) EffectiveSuggestionThreshold() int {
	if c == nil || c.SuggestionThreshold <= 0 {
		return defaultSuggestionThreshold
	}
	return c.SuggestionThreshold
}

// Settings stores user preferences for atmux (agent-tmux)
type Settings struct {
	// DefaultAction controls what happens when running `atmux` with no subcommand
	// Values: "landing" (show landing page), "resume" (start/attach directly), "sessions" (show sessions list)
	DefaultAction string `json:"default_action"`

	// RemoteAttachStrategy controls how remote sessions are attached when inside tmux.
	// Values: "auto" (default), "replace", "new-window"
	RemoteAttachStrategy AttachStrategy `json:"remote_attach_strategy,omitempty"`

	// Staleness controls session staleness indicators in the sessions TUI.
	Staleness *StalenessConfig `json:"staleness,omitempty"`
}

// DefaultSettings returns settings with default values
func DefaultSettings() *Settings {
	return &Settings{
		DefaultAction: "landing",
	}
}

// SettingsDir returns the config directory path
func SettingsDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, settingsDirName), nil
}

func legacySettingsDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, legacySettingsDirName), nil
}

// SettingsPath returns the full path to the settings file
func SettingsPath() (string, error) {
	dir, err := SettingsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

func legacySettingsPath() (string, error) {
	dir, err := legacySettingsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// LoadSettings loads settings from the config file
// Returns default settings if file doesn't exist
func LoadSettings() (*Settings, error) {
	path, err := SettingsPath()
	if err != nil {
		return DefaultSettings(), err
	}

	var data []byte
	data, err = os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			legacyPath, legacyErr := legacySettingsPath()
			if legacyErr != nil {
				return DefaultSettings(), nil
			}
			legacyData, legacyReadErr := os.ReadFile(legacyPath)
			if legacyReadErr != nil {
				if os.IsNotExist(legacyReadErr) {
					return DefaultSettings(), nil
				}
				return DefaultSettings(), legacyReadErr
			}
			data = legacyData
		} else {
			return DefaultSettings(), err
		}
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return DefaultSettings(), err
	}

	// Validate and set defaults for invalid values
	if settings.DefaultAction == "" {
		settings.DefaultAction = "landing"
	}

	return &settings, nil
}

// Save writes settings to the config file
func (s *Settings) Save() error {
	dir, err := SettingsDir()
	if err != nil {
		return err
	}

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path, err := SettingsPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// ResetSettings resets settings to defaults
func ResetSettings() error {
	settings := DefaultSettings()
	return settings.Save()
}

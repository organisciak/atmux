package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// Settings stores user preferences for atmux (agent-tmux)
type Settings struct {
	// DefaultAction controls what happens when running `atmux` with no subcommand
	// Values: "landing" (show landing page), "resume" (start/attach directly), "sessions" (show sessions list)
	DefaultAction string `json:"default_action"`

	// RemoteAttachStrategy controls how remote sessions are attached when inside tmux.
	// Values: "auto" (default), "replace", "new-window"
	RemoteAttachStrategy AttachStrategy `json:"remote_attach_strategy,omitempty"`
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

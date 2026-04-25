package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/history"
	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/porganisciak/agent-tmux/tui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "atmux",
	Short: "Manage tmux sessions for AI coding agents",
	Long: `atmux (short for agent-tmux) creates and manages tmux sessions optimized for AI coding workflows.

It creates a session with an 'agents' window configured via:
  - Global config: ~/.config/atmux/config
  - Project config: .agent-tmux.conf (overrides global)`,
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

	// Create session config to get session name for current directory
	session := tmux.NewSession(workingDir)

	// Check settings — "resume" skips the TUI entirely
	settings, _ := config.LoadSettings()
	if settings.DefaultAction == "resume" {
		return runDirectAttach(session, workingDir)
	}

	// Default: show sessions list with current-directory context
	result, err := tui.RunSessionsList(tui.SessionsOptions{
		AltScreen:      false,
		CurrentSession: session.Name,
		CurrentDir:     workingDir,
	})
	if err != nil {
		return err
	}
	if result.SessionName == "" {
		return nil
	}

	// When invoked inside a tmux popup, attaching/switching from this process
	// would target the popup client. Instead, write the resolved target to a
	// tmux global option; the parent (or a wrapper) can read it after the
	// popup closes and switch the outer client.
	if tmuxClientIsPopup() {
		target := result.SessionName
		if result.IsFromHistory {
			session := tmux.NewSession(result.WorkingDir)
			if !session.Exists() {
				localConfigPath := filepath.Join(result.WorkingDir, config.DefaultConfigName)
				cfg, _ := config.LoadConfig(localConfigPath)
				if err := session.Create(cfg); err != nil {
					return err
				}
				if cfg != nil {
					session.ApplyConfig(cfg)
				}
				session.SelectDefault()
			}
			target = session.Name
			saveHistory(filepath.Base(result.WorkingDir), result.WorkingDir, target, "", "")
		} else {
			if sessionPath := tmux.GetSessionPath(target); sessionPath != "" {
				saveHistory(filepath.Base(sessionPath), sessionPath, target, "", "")
			}
		}
		// Use the same handoff key as `atmux sessions` so wrappers can
		// pick it up uniformly.
		if err := exec.Command("tmux", "set-option", "-g", "@atmux-popup-target", target).Run(); err != nil {
			return err
		}
		// Also switch directly. If the user invoked us via run-shell from a
		// keybind (no parent atmux process to do the handoff), this gets the
		// outer client to the right place; if a parent is waiting, the
		// duplicate switch is a no-op.
		return exec.Command("tmux", "switch-client", "-t", target).Run()
	}

	if result.IsFromHistory {
		histSession := tmux.NewSession(result.WorkingDir)
		return runDirectAttach(histSession, result.WorkingDir)
	}
	if sessionPath := tmux.GetSessionPath(result.SessionName); sessionPath != "" {
		saveHistory(filepath.Base(sessionPath), sessionPath, result.SessionName, "", "")
	}
	return tmux.AttachToSession(result.SessionName)
}

// runDirectAttach performs the original behavior: create/attach directly
func runDirectAttach(session *tmux.Session, workingDir string) error {
	// Check if session already exists
	if session.Exists() {
		fmt.Printf("Attaching to existing session: %s\n", session.Name)
		saveHistory(filepath.Base(workingDir), workingDir, session.Name, "", "")
		return session.Attach()
	}

	// Load merged config (global + local)
	localConfigPath := filepath.Join(workingDir, config.DefaultConfigName)
	cfg, err := config.LoadConfig(localConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", err)
		cfg = nil
	}

	// Create new session with agent config
	fmt.Printf("Creating new session: %s\n", session.Name)
	if err := session.Create(cfg); err != nil {
		return err
	}

	// Apply additional windows/panes from config
	if cfg != nil {
		if err := session.ApplyConfig(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to apply config: %v\n", err)
		}
	}

	// Save to history and attach
	saveHistory(filepath.Base(workingDir), workingDir, session.Name, "", "")
	session.SelectDefault()
	return session.Attach()
}

// saveHistory saves a session to history, logging any errors.
// host and attachMethod should be empty for local sessions.
func saveHistory(name, workingDir, sessionName, host, attachMethod string) {
	store, err := history.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to open history: %v\n", err)
		return
	}
	defer store.Close()

	if err := store.SaveEntry(name, workingDir, sessionName, host, attachMethod); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save history: %v\n", err)
	}
}

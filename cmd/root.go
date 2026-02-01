package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/history"
	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/porganisciak/agent-tmux/tui"
	"github.com/spf13/cobra"
)

var resetDefaults bool

var rootCmd = &cobra.Command{
	Use:   "atmux",
	Short: "Manage tmux sessions for AI coding agents",
	Long: `atmux (short for agent-tmux) creates and manages tmux sessions optimized for AI coding workflows.

It creates a session with an 'agents' window configured via:
  - Global config: ~/.config/atmux/config
  - Project config: .agent-tmux.conf (overrides global)`,
	RunE: runRoot,
}

func init() {
	rootCmd.Flags().BoolVar(&resetDefaults, "reset-defaults", false,
		"Reset default startup behavior to show landing page")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runRoot(cmd *cobra.Command, args []string) error {
	// Handle --reset-defaults flag
	if resetDefaults {
		if err := config.ResetSettings(); err != nil {
			return fmt.Errorf("failed to reset settings: %w", err)
		}
		fmt.Println("Settings reset to show landing page by default")
		return nil
	}

	// Get working directory
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Create session config to get session name
	session := tmux.NewSession(workingDir)

	// Check settings for default behavior
	settings, _ := config.LoadSettings()
	switch settings.DefaultAction {
	case "resume":
		return runDirectAttach(session, workingDir)
	case "sessions":
		result, err := tui.RunSessionsList(tui.SessionsOptions{AltScreen: false})
		if err != nil {
			return err
		}
		if result.SessionName == "" {
			return nil
		}
		if result.IsFromHistory {
			// Revival from history
			histSession := tmux.NewSession(result.WorkingDir)
			return runDirectAttach(histSession, result.WorkingDir)
		}
		if sessionPath := tmux.GetSessionPath(result.SessionName); sessionPath != "" {
			saveHistory(filepath.Base(sessionPath), sessionPath, result.SessionName)
		}
		return tmux.AttachToSession(result.SessionName)
	default: // "landing" or empty
		return runLandingPage(session, workingDir)
	}
}

// runDirectAttach performs the original behavior: create/attach directly
func runDirectAttach(session *tmux.Session, workingDir string) error {
	// Check if session already exists
	if session.Exists() {
		fmt.Printf("Attaching to existing session: %s\n", session.Name)
		saveHistory(filepath.Base(workingDir), workingDir, session.Name)
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
	saveHistory(filepath.Base(workingDir), workingDir, session.Name)
	session.SelectDefault()
	return session.Attach()
}

// saveHistory saves a session to history, logging any errors.
func saveHistory(name, workingDir, sessionName string) {
	store, err := history.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to open history: %v\n", err)
		return
	}
	defer store.Close()

	if err := store.SaveEntry(name, workingDir, sessionName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save history: %v\n", err)
	}
}

// runLandingPage shows the interactive landing page
func runLandingPage(session *tmux.Session, workingDir string) error {
	result, err := tui.RunLanding(tui.LandingOptions{
		SessionName: session.Name,
		AltScreen:   false,
	})
	if err != nil {
		return err
	}

	switch result.Action {
	case "resume":
		return runDirectAttach(session, workingDir)
	case "attach":
		// Save to history before attaching to another session
		if sessionPath := tmux.GetSessionPath(result.Target); sessionPath != "" {
			saveHistory(filepath.Base(sessionPath), sessionPath, result.Target)
		}
		return tmux.AttachToSession(result.Target)
	default:
		// User quit without action
		return nil
	}
}

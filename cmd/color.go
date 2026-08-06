package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/spf13/cobra"
)

var colorCmd = &cobra.Command{
	Use:   "color [value|surprise|clear|show]",
	Short: "Set a tmux color theme for this project",
	Long: `Tint the tmux status bar and active pane border for this project,
saved in .agent-tmux.conf so it sticks across sessions.

The local config file is created if it doesn't exist; if a "color:" line
already exists it is updated in place, otherwise the directive is appended.

Examples:
  atmux color                # show the current color (or "(none)")
  atmux color show           # same as above
  atmux color #42b883        # set an explicit color (hex, named, or colour###)
  atmux color set red
  atmux color surprise       # pick a random vibrant color
  atmux color clear          # remove the color directive`,
	Args: cobra.ArbitraryArgs,
	RunE: runColor,
}

func init() {
	rootCmd.AddCommand(colorCmd)

	colorCmd.AddCommand(&cobra.Command{
		Use:   "set <value>",
		Short: "Set a specific color (hex, named, or colour###)",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return setColor(args[0]) },
	})
	colorCmd.AddCommand(&cobra.Command{
		Use:   "surprise",
		Short: "Pick a random color from the curated palette",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return surpriseColor() },
	})
	colorCmd.AddCommand(&cobra.Command{
		Use:   "clear",
		Short: "Remove the color directive from the project config",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return clearColor() },
	})
	colorCmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print the current color (or \"(none)\")",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return showColor() },
	})
}

func runColor(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return showColor()
	}
	// Bare-word convenience: `atmux color surprise|clear|show|<value>`
	switch strings.ToLower(args[0]) {
	case "surprise", "random":
		return surpriseColor()
	case "clear", "none", "off":
		return clearColor()
	case "show":
		return showColor()
	default:
		return setColor(args[0])
	}
}

func showColor() error {
	cfg, path, err := loadCurrentProjectConfig()
	if err != nil {
		return err
	}
	if cfg == nil || strings.TrimSpace(cfg.Color) == "" {
		fmt.Println("(none)")
		return nil
	}
	fmt.Println(cfg.Color)
	_ = path // reserved for future verbose mode
	return nil
}

func setColor(raw string) error {
	normalized, err := config.NormalizeColor(raw)
	if err != nil {
		return err
	}
	path, err := localConfigPath()
	if err != nil {
		return err
	}
	if err := config.SetLocalDirective(path, "color", normalized); err != nil {
		return fmt.Errorf("failed to write color to %s: %w", path, err)
	}
	fmt.Printf("Set color %s in %s\n", normalized, path)
	applyColorToCurrentSession(normalized)
	return nil
}

func surpriseColor() error {
	cfg, _, _ := loadCurrentProjectConfig()
	current := ""
	if cfg != nil {
		current = cfg.Color
	}
	picked, err := config.RandomSurpriseColor(current)
	if err != nil {
		return err
	}
	return setColor(picked)
}

func clearColor() error {
	path, err := localConfigPath()
	if err != nil {
		return err
	}
	if err := config.RemoveLocalDirective(path, "color"); err != nil {
		return fmt.Errorf("failed to update %s: %w", path, err)
	}
	fmt.Printf("Cleared color in %s\n", path)
	clearColorOnCurrentSession()
	return nil
}

// loadCurrentProjectConfig loads the merged (global + local) config for the
// current working directory.
func loadCurrentProjectConfig() (*config.Config, string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get working directory: %w", err)
	}
	path := filepath.Join(workingDir, config.DefaultConfigName)
	cfg, err := config.LoadConfig(path)
	if err != nil {
		return nil, path, err
	}
	return cfg, path, nil
}

func localConfigPath() (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	return filepath.Join(workingDir, config.DefaultConfigName), nil
}

// applyColorToCurrentSession tints a live tmux session for this project, if
// one happens to be running. Errors are non-fatal — the directive is already
// saved, so the next session will pick it up regardless.
func applyColorToCurrentSession(color string) {
	name := currentProjectSessionName()
	if name == "" || !sessionExists(name) {
		return
	}
	if err := tmux.ApplyColorToSession(name, color); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to apply color to live session %s: %v\n", name, err)
		return
	}
	fmt.Printf("Applied to live session %s\n", name)
}

func clearColorOnCurrentSession() {
	name := currentProjectSessionName()
	if name == "" || !sessionExists(name) {
		return
	}
	if err := tmux.ClearColorOnSession(name); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to clear color on live session %s: %v\n", name, err)
		return
	}
	fmt.Printf("Cleared on live session %s\n", name)
}

func currentProjectSessionName() string {
	workingDir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return tmux.NewSession(workingDir).Name
}

func sessionExists(name string) bool {
	return exec.Command("tmux", "has-session", "-t", name).Run() == nil
}

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var (
	keybindKey     string
	keybindYes     bool
	keybindCommand string
)

var keybindCmd = &cobra.Command{
	Use:   "keybind",
	Short: "Add a tmux keybinding for an atmux popup",
	Long: `Adds a keybinding to ~/.tmux.conf that opens an atmux popup, and binds it
in the running tmux server so it works immediately.

Each command has a default key; --key overrides it.
  browse    prefix + S   tree-style session browser
  sessions  prefix + s   quick session list
  agents    prefix + a   every Claude session's state and recap

Examples:
  atmux keybind                      # Adds: bind-key S run-shell "atmux browse"
  atmux keybind --command agents     # Adds: bind-key a run-shell "atmux agents"
  atmux keybind --key T              # Adds: bind-key T run-shell "atmux browse"
  atmux keybind --command agents -y  # No prompts; for install scripts

Subcommands:
  atmux keybind show         # Print the keybinding snippet (ready to copy-paste)

After adding the keybinding, reload your tmux config:
  tmux source-file ~/.tmux.conf`,
	RunE: runKeybind,
}

var keybindShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the recommended tmux keybinding snippet",
	Long: `Prints the recommended keybinding snippet for copy-pasting into ~/.tmux.conf.

This is useful if you want to manually add the binding or include it in a
dotfiles repository.

Examples:
  atmux keybind show                  # Show default binding (prefix + S)
  atmux keybind show --key T          # Show binding for prefix + T
  atmux keybind show --command sessions  # Show binding for sessions command
  atmux keybind show --command agents    # Show binding for the agents overview`,
	Run: runKeybindShow,
}

func init() {
	rootCmd.AddCommand(keybindCmd)
	keybindCmd.Flags().StringVarP(&keybindKey, "key", "k", "S", "Key to bind (e.g., S, C-s, M-s)")
	keybindCmd.Flags().BoolVarP(&keybindYes, "yes", "y", false, "Skip confirmation prompt")
	keybindCmd.Flags().StringVar(&keybindCommand, "command", "browse", "Command to run (browse, sessions, or agents)")

	// Add show subcommand
	keybindCmd.AddCommand(keybindShowCmd)
	keybindShowCmd.Flags().StringVarP(&keybindKey, "key", "k", "S", "Key to bind (e.g., S, C-s, M-s)")
	keybindShowCmd.Flags().StringVar(&keybindCommand, "command", "browse", "Command to run (browse, sessions, or agents)")
}

// keybindTarget is a command atmux knows how to bind.
type keybindTarget struct {
	defaultKey  string
	description string
}

var keybindTargets = map[string]keybindTarget{
	"browse":   {"S", "open session browser popup"},
	"sessions": {"s", "open session list popup"},
	// Next to s on the keyboard, and unbound in stock tmux.
	"agents": {"a", "open agents overview (state and recap of every Claude session)"},
}

// resolveKeybind validates --command and applies its default key unless
// --key was given.
func resolveKeybind(cmd *cobra.Command) (keybindTarget, error) {
	target, ok := keybindTargets[keybindCommand]
	if !ok {
		return keybindTarget{}, fmt.Errorf("--command must be 'browse', 'sessions', or 'agents'")
	}
	if !cmd.Flags().Changed("key") {
		keybindKey = target.defaultKey
	}
	return target, nil
}

// bindLive applies the binding to the running tmux server, if there is one,
// so it works without reloading the config. Reports whether it did.
func bindLive(key, command string) bool {
	if exec.Command("tmux", "has-session").Run() != nil {
		return false
	}
	return exec.Command("tmux", "bind-key", key, "run-shell", command).Run() == nil
}

func runKeybindShow(cmd *cobra.Command, args []string) {
	target, err := resolveKeybind(cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Build the binding line
	bindingLine := fmt.Sprintf("bind-key %s run-shell \"atmux %s\"", keybindKey, keybindCommand)
	commentLine := "# atmux: " + target.description

	fmt.Println("# Add this to ~/.tmux.conf:")
	fmt.Println(commentLine)
	fmt.Println(bindingLine)
	fmt.Println()
	fmt.Println("# Then reload your config:")
	fmt.Println("# tmux source-file ~/.tmux.conf")
	fmt.Printf("#\n# Press prefix + %s to %s.\n", keybindKey, target.description)
}

func runKeybind(cmd *cobra.Command, args []string) error {
	// Get tmux config path
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}
	tmuxConfPath := filepath.Join(home, ".tmux.conf")

	target, err := resolveKeybind(cmd)
	if err != nil {
		return err
	}

	// Build the binding line
	bindingLine := fmt.Sprintf("bind-key %s run-shell \"atmux %s\"", keybindKey, keybindCommand)
	commentLine := "# atmux: " + target.description
	fullBinding := fmt.Sprintf("\n%s\n%s\n", commentLine, bindingLine)

	// Read existing config (if any)
	existingContent := ""
	if _, err := os.Stat(tmuxConfPath); err == nil {
		content, err := os.ReadFile(tmuxConfPath)
		if err != nil {
			return fmt.Errorf("could not read %s: %w", tmuxConfPath, err)
		}
		existingContent = string(content)
	}

	// Check for duplicate bindings
	duplicateKey, duplicateLine := findDuplicateBinding(existingContent, keybindKey)
	if duplicateKey {
		fmt.Printf("Warning: Key '%s' is already bound in %s:\n", keybindKey, tmuxConfPath)
		fmt.Printf("  %s\n\n", duplicateLine)
		if !keybindYes {
			fmt.Print("Do you want to add this binding anyway? [y/N] ")
			if !confirmPrompt() {
				fmt.Println("Aborted.")
				return nil
			}
		}
	}

	// Check if exact binding already exists
	if strings.Contains(existingContent, bindingLine) {
		fmt.Printf("Binding already exists in %s:\n", tmuxConfPath)
		fmt.Printf("  %s\n", bindingLine)
		if bindLive(keybindKey, "atmux "+keybindCommand) {
			fmt.Println("Bound in the running tmux server too.")
		}
		return nil
	}

	// Show what we'll add and confirm
	fmt.Printf("Will add to %s:\n", tmuxConfPath)
	fmt.Printf("  %s\n", commentLine)
	fmt.Printf("  %s\n\n", bindingLine)

	if !keybindYes {
		fmt.Print("Proceed? [Y/n] ")
		if !confirmPromptDefault(true) {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Append to file
	f, err := os.OpenFile(tmuxConfPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("could not open %s for writing: %w", tmuxConfPath, err)
	}
	defer f.Close()

	if _, err := f.WriteString(fullBinding); err != nil {
		return fmt.Errorf("could not write to %s: %w", tmuxConfPath, err)
	}

	fmt.Printf("\n✓ Keybinding added to %s\n", tmuxConfPath)
	if bindLive(keybindKey, "atmux "+keybindCommand) {
		fmt.Printf("\nActive now: press prefix + %s to %s.\n", keybindKey, target.description)
	} else {
		fmt.Println("\nTo activate, start tmux or run:")
		fmt.Println("  tmux source-file ~/.tmux.conf")
	}

	return nil
}

// findDuplicateBinding checks if the key is already bound in the config
func findDuplicateBinding(content, key string) (bool, string) {
	// Match bind-key or bind followed by the key
	pattern := regexp.MustCompile(`(?m)^\s*bind(?:-key)?\s+` + regexp.QuoteMeta(key) + `\s+.*$`)
	match := pattern.FindString(content)
	if match != "" {
		return true, strings.TrimSpace(match)
	}
	return false, ""
}

// confirmPrompt asks for y/n with default no
func confirmPrompt() bool {
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

// confirmPromptDefault asks for y/n with specified default
func confirmPromptDefault(defaultYes bool) bool {
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return defaultYes
	}
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return defaultYes
	}
	return input == "y" || input == "yes"
}

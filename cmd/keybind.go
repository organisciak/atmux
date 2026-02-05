package cmd

import (
	"bufio"
	"fmt"
	"os"
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
	Short: "Add a tmux keybinding for the session browser popup",
	Long: `Adds a keybinding to ~/.tmux.conf that opens the atmux session browser.

By default, binds prefix + S to open the sessions popup.
The binding can be customized with --key.

Examples:
  atmux keybind              # Adds: bind-key S run-shell "atmux browse"
  atmux keybind --key T      # Adds: bind-key T run-shell "atmux browse"
  atmux keybind --key C-s    # Adds: bind-key C-s run-shell "atmux browse"
  atmux keybind -y           # Skip confirmation

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
  atmux keybind show --command sessions  # Show binding for sessions command`,
	Run: runKeybindShow,
}

func init() {
	rootCmd.AddCommand(keybindCmd)
	keybindCmd.Flags().StringVarP(&keybindKey, "key", "k", "S", "Key to bind (e.g., S, C-s, M-s)")
	keybindCmd.Flags().BoolVarP(&keybindYes, "yes", "y", false, "Skip confirmation prompt")
	keybindCmd.Flags().StringVar(&keybindCommand, "command", "browse", "Command to run (browse or sessions)")

	// Add show subcommand
	keybindCmd.AddCommand(keybindShowCmd)
	keybindShowCmd.Flags().StringVarP(&keybindKey, "key", "k", "S", "Key to bind (e.g., S, C-s, M-s)")
	keybindShowCmd.Flags().StringVar(&keybindCommand, "command", "browse", "Command to run (browse or sessions)")
}

func runKeybindShow(cmd *cobra.Command, args []string) {
	// Validate command
	if keybindCommand != "browse" && keybindCommand != "sessions" {
		fmt.Fprintf(os.Stderr, "Error: --command must be 'browse' or 'sessions'\n")
		os.Exit(1)
	}

	// Build the binding line
	bindingLine := fmt.Sprintf("bind-key %s run-shell \"atmux %s\"", keybindKey, keybindCommand)
	commentLine := "# atmux: open session browser popup"

	fmt.Println("# Add this to ~/.tmux.conf:")
	fmt.Println(commentLine)
	fmt.Println(bindingLine)
	fmt.Println()
	fmt.Println("# Then reload your config:")
	fmt.Println("# tmux source-file ~/.tmux.conf")
	fmt.Printf("#\n# Press prefix + %s to open the session browser.\n", keybindKey)
}

func runKeybind(cmd *cobra.Command, args []string) error {
	// Get tmux config path
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}
	tmuxConfPath := filepath.Join(home, ".tmux.conf")

	// Validate command
	if keybindCommand != "browse" && keybindCommand != "sessions" {
		return fmt.Errorf("--command must be 'browse' or 'sessions'")
	}

	// Build the binding line
	bindingLine := fmt.Sprintf("bind-key %s run-shell \"atmux %s\"", keybindKey, keybindCommand)
	commentLine := "# atmux: open session browser popup"
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
	fmt.Println("\nTo activate, run:")
	fmt.Println("  tmux source-file ~/.tmux.conf")
	fmt.Printf("\nThen press prefix + %s to open the session browser.\n", keybindKey)

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

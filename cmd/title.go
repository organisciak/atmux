package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/spf13/cobra"
)

var titleCmd = &cobra.Command{
	Use:   "title [on|off|show]",
	Short: "Show the agent's session name in the tmux status bar",
	Long: `Show the running agent's session name in this project's tmux status bar,
saved in .agent-tmux.conf so it sticks across sessions.

Claude Code keeps the terminal title set to the current conversation's name,
which tmux exposes as pane_title. The leading glyph comes along with it and
doubles as an activity indicator: a spinner while the agent is working.

To rename a Claude Code conversation, run /resume, highlight the session, and
press Ctrl+R.

Examples:
  atmux title            # show whether it is on
  atmux title on         # enable for this project
  atmux title off        # disable
  atmux title --all      # apply to every running atmux session`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTitle,
}

var titleAll bool

func init() {
	rootCmd.AddCommand(titleCmd)
	titleCmd.Flags().BoolVar(&titleAll, "all", false, "Apply to every running atmux session, not just this project")
}

func runTitle(cmd *cobra.Command, args []string) error {
	if titleAll {
		enabled := true
		if len(args) > 0 {
			parsed, err := parseOnOff(args[0])
			if err != nil {
				return err
			}
			enabled = parsed
		}
		return applyTitleToAllSessions(cmd, enabled)
	}

	if len(args) == 0 {
		return showTitle()
	}

	enabled, err := parseOnOff(args[0])
	if err != nil {
		if strings.EqualFold(args[0], "show") {
			return showTitle()
		}
		return err
	}
	return setTitle(enabled)
}

func parseOnOff(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "on", "true", "yes", "1", "enable":
		return true, nil
	case "off", "false", "no", "0", "disable", "clear":
		return false, nil
	}
	return false, fmt.Errorf("expected on or off, got %q", raw)
}

func showTitle() error {
	cfg, _, err := loadCurrentProjectConfig()
	if err != nil {
		return err
	}
	if cfg != nil && cfg.SessionTitle {
		fmt.Println("on")
		return nil
	}
	fmt.Println("off")
	return nil
}

func setTitle(enabled bool) error {
	path, err := localConfigPath()
	if err != nil {
		return err
	}

	value := "off"
	if enabled {
		value = "on"
	}
	if err := config.SetLocalDirective(path, "session_title", value); err != nil {
		return fmt.Errorf("failed to write session_title to %s: %w", path, err)
	}
	fmt.Printf("Set session_title %s in %s\n", value, path)

	name := currentProjectSessionName()
	if name == "" || !sessionExists(name) {
		return nil
	}
	if err := tmux.ApplySessionTitleToSession(name, enabled); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to update live session %s: %v\n", name, err)
		return nil
	}
	fmt.Printf("Applied to live session %s\n", name)
	return nil
}

// applyTitleToAllSessions toggles the status bar on every running atmux
// session. This is the escape hatch for sessions that were created before the
// directive existed, since the option is otherwise only applied at create time.
func applyTitleToAllSessions(cmd *cobra.Command, enabled bool) error {
	names, err := tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}
	if len(names) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No atmux sessions running.")
		return nil
	}

	var failed []string
	for _, name := range names {
		if err := tmux.ApplySessionTitleToSession(name, enabled); err != nil {
			failed = append(failed, name)
		}
	}

	state := "off"
	if enabled {
		state = "on"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Turned session title %s for %d session(s)\n", state, len(names)-len(failed))
	if len(failed) > 0 {
		return fmt.Errorf("failed for: %s", strings.Join(failed, ", "))
	}
	return nil
}

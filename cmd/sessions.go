package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/porganisciak/agent-tmux/tui"
	"github.com/spf13/cobra"
)

var sessionsCmd = &cobra.Command{
	Use:     "sessions [session-name]",
	Aliases: []string{"lsessions", "list-sessions", "list", "ls", "attach"},
	Short:   "List sessions or attach directly by name",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runSessions,
}

var (
	sessionsInline         bool
	sessionsPopup          bool
	sessionsNoPopup        bool
	sessionsNonInteractive bool
	sessionsNoBeads        bool
	sessionsNoStaleness    bool
	sessionsRemote         string
	sessionsStrategy       string
)

func init() {
	rootCmd.AddCommand(sessionsCmd)
	sessionsCmd.Flags().BoolVar(&sessionsInline, "inline", true, "Render without alt screen (non-fullscreen)")
	sessionsCmd.Flags().BoolVarP(&sessionsPopup, "popup", "p", false, "Force popup mode (even outside tmux conditions)")
	sessionsCmd.Flags().BoolVar(&sessionsNoPopup, "no-popup", false, "Disable popup mode (default: popup when inside tmux)")
	sessionsCmd.Flags().BoolVarP(&sessionsNonInteractive, "non-interactive", "n", false, "Print sessions and exit (no TUI)")
	sessionsCmd.Flags().BoolVar(&sessionsNoBeads, "no-beads", false, "Hide beads issue counts per session")
	sessionsCmd.Flags().BoolVar(&sessionsNoStaleness, "no-staleness", false, "Disable staleness indicators and kill-stale")
	sessionsCmd.Flags().StringVarP(&sessionsRemote, "remote", "r", "", "Remote host(s) or aliases to include (comma-separated)")
	sessionsCmd.Flags().StringVar(&sessionsStrategy, "strategy", "", "Remote attach strategy: auto, replace, new-window")
}

func runSessions(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return attachToSession(args[0])
	}

	// Build executors (local + configured remotes + --remote flag)
	executors, err := buildExecutors(sessionsRemote)
	if err != nil {
		return fmt.Errorf("failed to build executors: %w", err)
	}
	defer closeExecutors(executors)
	registerCleanupSignals(executors)

	// Non-interactive mode: print all sessions and exit
	if sessionsNonInteractive {
		return runSessionsNonInteractive(cmd, executors)
	}

	// Force popup with -p, or default to popup when inside tmux (unless --no-popup)
	insideTmux := os.Getenv("TMUX") != ""
	if sessionsPopup || (insideTmux && !sessionsNoPopup && !sessionsInline) {
		return launchAsPopup("sessions")
	}

	result, err := tui.RunSessionsList(tui.SessionsOptions{
		AltScreen:        !sessionsInline,
		Executors:        executors,
		ShowBeads:        !sessionsNoBeads,
		DisableStaleness: sessionsNoStaleness,
	})
	if err != nil {
		return err
	}
	if result.SessionName == "" {
		return nil
	}

	if result.IsFromHistory {
		// Revival from history - create new session in that directory
		session := tmux.NewSession(result.WorkingDir)
		return runDirectAttach(session, result.WorkingDir)
	}

	// Attach to existing session via the appropriate executor
	executor := result.Executor
	if executor == nil {
		executor = tmux.NewLocalExecutor()
	}

	if executor.IsRemote() {
		// Save remote session to history with host identity
		host := executor.HostLabel()
		attachMethod := ""
		if re, ok := executor.(*tmux.RemoteExecutor); ok {
			attachMethod = re.AttachMethod
		}
		saveHistory(result.SessionName, "", result.SessionName, host, attachMethod)
	} else {
		if sessionPath := tmux.GetSessionPath(result.SessionName); sessionPath != "" {
			saveHistory(filepath.Base(sessionPath), sessionPath, result.SessionName, "", "")
		}
	}
	strategy := resolveAttachStrategy(executor)
	return tmux.AttachToSessionWithStrategy(result.SessionName, executor, strategy)
}

// resolveAttachStrategy determines the attach strategy from (in order):
// 1. --strategy flag, 2. per-host override, 3. global setting, 4. "auto".
func resolveAttachStrategy(executor tmux.TmuxExecutor) config.AttachStrategy {
	// 1. CLI flag takes precedence
	if sessionsStrategy != "" {
		s := config.AttachStrategy(sessionsStrategy)
		if config.ValidAttachStrategy(s) {
			return s
		}
	}

	// 2. Per-host override on RemoteExecutor
	if re, ok := executor.(*tmux.RemoteExecutor); ok && re.AttachStrategy != "" {
		s := config.AttachStrategy(re.AttachStrategy)
		if config.ValidAttachStrategy(s) {
			return s
		}
	}

	// 3. Global setting
	settings, err := config.LoadSettings()
	if err == nil && settings.RemoteAttachStrategy != "" {
		if config.ValidAttachStrategy(settings.RemoteAttachStrategy) {
			return settings.RemoteAttachStrategy
		}
	}

	return config.AttachStrategyAuto
}

// runSessionsNonInteractive prints sessions from all executors and exits.
func runSessionsNonInteractive(cmd *cobra.Command, executors []tmux.TmuxExecutor) error {
	out := cmd.OutOrStdout()
	for _, exec := range executors {
		lines, err := tmux.ListSessionsRawWithExecutor(exec)
		if err != nil {
			// Skip unreachable hosts with a warning
			if exec.IsRemote() {
				fmt.Fprintf(out, "# %s: unreachable\n", exec.HostLabel())
				continue
			}
			return err
		}
		// Print host header when multiple executors are in play
		if len(executors) > 1 {
			label := "local"
			if exec.IsRemote() {
				label = exec.HostLabel()
			}
			fmt.Fprintf(out, "# %s\n", label)
		}
		for _, line := range lines {
			fmt.Fprintln(out, line.Line)
		}
	}
	return nil
}

func attachToSession(name string) error {
	sessionName := name
	if !strings.HasPrefix(sessionName, "agent-") && !strings.HasPrefix(sessionName, "atmux-") {
		sessionName = "agent-" + sessionName
	}

	session := &tmux.Session{Name: sessionName}
	if !session.Exists() {
		return fmt.Errorf("session %s does not exist\nUse 'atmux sessions' to see active sessions", sessionName)
	}

	return session.Attach()
}

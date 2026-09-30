package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/porganisciak/agent-tmux/tui"
	"github.com/spf13/cobra"
)

var agentsCmd = &cobra.Command{
	Use:     "agents",
	Aliases: []string{"overview", "recaps"},
	Short:   "Overview of every Claude Code session: state and recap",
	Long: `Shows every Claude Code pane as a card with what it is doing and
Claude's latest recap of the session. Click a card, or select it and press
Enter, to jump to that pane.

States:
  working      a turn is running
  needs input  blocked on a permission prompt, question, or dialog
  ready        finished recently; there is a reply to read
  dormant      finished more than 30 minutes ago
  fresh        started but never prompted

Controls:
  Arrows/hjkl  Move between cards
  Enter/click  Jump to the pane
  r            Refresh now
  q/Esc        Quit`,
	RunE: runAgents,
}

var (
	agentsList    bool
	agentsJSON    bool
	agentsNoPopup bool
	agentsRemote  string
	agentsLocal   bool
)

func init() {
	rootCmd.AddCommand(agentsCmd)
	agentsCmd.Flags().BoolVarP(&agentsList, "list", "l", false, "Print the overview instead of opening the grid")
	agentsCmd.Flags().BoolVar(&agentsJSON, "json", false, "Print agent states as JSON")
	agentsCmd.Flags().BoolVar(&agentsNoPopup, "no-popup", false, "Disable popup mode (default: popup when inside tmux)")
	agentsCmd.Flags().StringVar(&agentsRemote, "remote", "", "Remote hosts to include (comma-separated aliases or hosts)")
	agentsCmd.Flags().BoolVar(&agentsLocal, "local", false, "Only this machine's agents, not saved remote hosts")
}

func runAgents(cmd *cobra.Command, args []string) error {
	executors := []tmux.TmuxExecutor{tmux.NewLocalExecutor()}
	if !agentsLocal {
		var err error
		if executors, err = buildExecutors(agentsRemote); err != nil {
			return err
		}
	}
	defer closeExecutors(executors)

	if agentsJSON || agentsList {
		agents := tmux.ListAgentPanes(executors)
		tmux.LoadTranscripts(agents)
		if agentsJSON {
			return writeAgentsJSON(cmd.OutOrStdout(), agents, time.Now())
		}
		writeAgentsList(cmd.OutOrStdout(), agents, time.Now())
		return nil
	}

	if os.Getenv("TMUX") != "" && !agentsNoPopup {
		if agentsRemote != "" {
			return launchAsPopup("agents", "--remote", agentsRemote)
		}
		return launchAsPopup("agents")
	}

	result, err := tui.RunAgents(tui.AgentsOptions{Executors: executors})
	if err != nil {
		return err
	}
	if result.Target == "" {
		return nil
	}
	if result.Executor != nil && result.Executor.IsRemote() {
		return tmux.AttachToSessionWithExecutor(result.SessionName, result.Executor)
	}
	return tmux.AttachToSession(result.Target)
}

func writeAgentsJSON(w io.Writer, agents []tmux.AgentPane, now time.Time) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(tmux.BuildAgentsPayload(agents, now))
}

func writeAgentsList(w io.Writer, agents []tmux.AgentPane, now time.Time) {
	if len(agents) == 0 {
		fmt.Fprintln(w, "No Claude Code sessions found.")
		return
	}
	for i, a := range agents {
		if i > 0 {
			fmt.Fprintln(w)
		}
		name := a.SessionName
		if a.Host != "" {
			name = "[" + a.Host + "] " + name
		}
		_, title := tmux.SplitAgentTitle(a.Pane.Title)
		fmt.Fprintf(w, "%s  %s  %s\n", tui.AgentActivityLabel(a, now), name, title)
		if text := tui.AgentSummaryText(a); text != "" {
			fmt.Fprintf(w, "  %s\n", strings.ReplaceAll(text, "\n", "\n  "))
		}
	}
}

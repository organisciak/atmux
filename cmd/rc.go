package cmd

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/spf13/cobra"
)

var (
	rcAll    bool
	rcRemote string
	rcDry    bool
)

var rcCmd = &cobra.Command{
	Use:     "rc [name]",
	Aliases: []string{"remote-control"},
	Short:   "Enable Remote Control on Claude Code panes",
	Long: `Detect panes running Claude Code and send /remote-control to enable
remote access from claude.ai/code or the Claude mobile app.

By default, shows an interactive selector to choose which panes to enable.
Use --all to enable on every detected Claude pane without prompting.

The remote control session name defaults to the tmux session name (in atmux
naming style). Override with a positional argument.

Examples:
  atmux rc                        # interactive selector
  atmux rc --all                  # enable on all Claude panes
  atmux rc "My Project"           # custom session name
  atmux rc --remote=server1       # include remote hosts
  atmux rc --remote=server1 --all # all panes on remote host`,
	RunE: runRC,
}

func init() {
	rcCmd.Flags().BoolVarP(&rcAll, "all", "a", false,
		"Send /remote-control to all detected Claude panes without prompting")
	rcCmd.Flags().StringVarP(&rcRemote, "remote", "r", "",
		"Remote host(s) or aliases (comma-separated)")
	rcCmd.Flags().BoolVar(&rcDry, "dry-run", false,
		"Show detected panes without sending commands")

	rootCmd.AddCommand(rcCmd)
}

func runRC(cmd *cobra.Command, args []string) error {
	// Build executors
	var executors []tmux.TmuxExecutor
	if rcRemote != "" {
		cfg, err := loadRemoteConfig()
		if err != nil {
			return fmt.Errorf("failed to load remote config: %w", err)
		}
		remoteHosts, err := config.ResolveRemoteHosts(cfg, rcRemote, false)
		if err != nil {
			return err
		}
		for _, rh := range remoteHosts {
			executors = append(executors, tmux.NewRemoteExecutor(
				rh.Host, rh.Port, rh.AttachMethod, rh.Alias,
			))
		}
		// Also include local
		executors = append([]tmux.TmuxExecutor{tmux.NewLocalExecutor()}, executors...)
	} else {
		executors = []tmux.TmuxExecutor{tmux.NewLocalExecutor()}
	}
	defer closeExecutors(executors)

	// Find Claude panes
	fmt.Println("Scanning for Claude Code panes...")
	panes := tmux.FindClaudePanes(executors)

	if len(panes) == 0 {
		fmt.Println("No Claude Code panes detected.")
		return nil
	}

	fmt.Printf("Found %d Claude Code pane(s)\n\n", len(panes))

	if rcDry {
		for _, p := range panes {
			fmt.Printf("  %s\n", p.Label())
		}
		return nil
	}

	// Determine which panes to target
	var selected []tmux.ClaudePane
	if rcAll || len(panes) == 1 {
		selected = panes
	} else {
		// Interactive checkbox selector
		var err error
		selected, err = runPaneSelector(panes)
		if err != nil {
			return err
		}
	}

	if len(selected) == 0 {
		fmt.Println("No panes selected.")
		return nil
	}

	// Build /remote-control command
	rcCommand := "/remote-control"
	if len(args) > 0 {
		rcCommand += " " + args[0]
	}

	// Send to each selected pane
	for _, cp := range selected {
		name := rcCommand
		// If no custom name, use session-based name in atmux style
		if len(args) == 0 {
			slug := makeSlug(cp.SessionName)
			name = rcCommand + " " + slug
		}

		fmt.Printf("  Sending to %s ... ", cp.Label())
		err := tmux.SendCommandWithMethodAndExecutor(
			cp.Pane.Target, name, tmux.SendMethodEnterDelayed, cp.Executor,
		)
		if err != nil {
			fmt.Printf("error: %v\n", err)
		} else {
			fmt.Println("ok")
		}
	}

	fmt.Printf("\nRemote Control enabled on %d pane(s).\n", len(selected))
	fmt.Println("Connect at claude.ai/code or the Claude mobile app.")
	return nil
}

// makeSlug creates a display name from a session name.
// Strips common prefixes and cleans up for human readability.
func makeSlug(sessionName string) string {
	name := sessionName
	// Strip atmux prefixes
	for _, prefix := range []string{"agent-", "atmux-"} {
		name = strings.TrimPrefix(name, prefix)
	}
	// Replace underscores with spaces, title-case
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	// Clean up whitespace
	reg := regexp.MustCompile(`\s+`)
	name = reg.ReplaceAllString(strings.TrimSpace(name), " ")
	if name == "" {
		name = filepath.Base(sessionName)
	}
	return name
}

// --- Checkbox TUI ---

type selectorModel struct {
	panes    []tmux.ClaudePane
	selected []bool
	cursor   int
	done     bool
	aborted  bool
}

func (m selectorModel) Init() tea.Cmd {
	return nil
}

func (m selectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.aborted = true
			m.done = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.panes)-1 {
				m.cursor++
			}
		case " ", "x":
			m.selected[m.cursor] = !m.selected[m.cursor]
		case "a":
			// Toggle all
			allSelected := true
			for _, s := range m.selected {
				if !s {
					allSelected = false
					break
				}
			}
			for i := range m.selected {
				m.selected[i] = !allSelected
			}
		case "enter":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

var (
	rcTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	rcSelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	rcCursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	rcHelpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func (m selectorModel) View() string {
	if m.done {
		return ""
	}

	var b strings.Builder
	b.WriteString(rcTitleStyle.Render("Select Claude panes for Remote Control"))
	b.WriteString("\n\n")

	for i, p := range m.panes {
		cursor := "  "
		if i == m.cursor {
			cursor = rcCursorStyle.Render("> ")
		}

		check := "[ ]"
		label := p.Label()
		if m.selected[i] {
			check = rcSelectedStyle.Render("[x]")
			label = rcSelectedStyle.Render(label)
		}

		b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, check, label))
	}

	b.WriteString("\n")
	b.WriteString(rcHelpStyle.Render("space/x: toggle  a: all  enter: confirm  q: cancel"))
	b.WriteString("\n")

	return b.String()
}

func runPaneSelector(panes []tmux.ClaudePane) ([]tmux.ClaudePane, error) {
	m := selectorModel{
		panes:    panes,
		selected: make([]bool, len(panes)),
		cursor:   0,
	}
	// Pre-select all by default
	for i := range m.selected {
		m.selected[i] = true
	}

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("selector error: %w", err)
	}

	final := finalModel.(selectorModel)
	if final.aborted {
		return nil, nil
	}

	var result []tmux.ClaudePane
	for i, sel := range final.selected {
		if sel {
			result = append(result, panes[i])
		}
	}
	return result, nil
}

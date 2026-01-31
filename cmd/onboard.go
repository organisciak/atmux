package cmd

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "First-time user guide for atmux",
	Long:  "Interactive guide to help new users understand atmux features and setup.",
	RunE:  runOnboard,
}

func init() {
	rootCmd.AddCommand(onboardCmd)
}

func runOnboard(cmd *cobra.Command, args []string) error {
	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		MarginBottom(1)

	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170"))

	codeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82"))

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	out := cmd.OutOrStdout()

	// Title
	fmt.Fprintln(out, titleStyle.Render("Welcome to atmux!"))
	fmt.Fprintln(out, dimStyle.Render("A tmux session manager optimized for AI coding workflows.\n"))

	// Quick Start
	fmt.Fprintln(out, sectionStyle.Render("Quick Start"))
	fmt.Fprintln(out, "  "+codeStyle.Render("atmux")+"           Start or resume a session for current directory")
	fmt.Fprintln(out, "  "+codeStyle.Render("atmux sessions")+"  Browse and attach to existing sessions")
	fmt.Fprintln(out, "  "+codeStyle.Render("atmux browse")+"    Visual session/window/pane browser")
	fmt.Fprintln(out)

	// Session Structure
	fmt.Fprintln(out, sectionStyle.Render("Session Structure"))
	fmt.Fprintln(out, "  Each session is created with:")
	fmt.Fprintln(out, "  • "+codeStyle.Render("agents")+" window — split panes for AI coding tools")
	fmt.Fprintln(out, "  • "+codeStyle.Render("diag")+" window — diagnostics and monitoring")
	fmt.Fprintln(out)

	// Project Config
	fmt.Fprintln(out, sectionStyle.Render("Project Configuration"))
	fmt.Fprintln(out, "  Create "+codeStyle.Render(".agent-tmux.conf")+" in your project root to customize:")
	fmt.Fprintln(out, dimStyle.Render(`
    # Example .agent-tmux.conf
    window dev
      pane npm run dev
      pane npm run test -- --watch

    window logs
      pane tail -f app.log
`))

	// Commands
	fmt.Fprintln(out, sectionStyle.Render("All Commands"))
	fmt.Fprintln(out, "  "+codeStyle.Render("atmux")+"              Show landing page (configurable default)")
	fmt.Fprintln(out, "  "+codeStyle.Render("atmux sessions")+"     List sessions with click-to-attach")
	fmt.Fprintln(out, "  "+codeStyle.Render("atmux browse")+"       Full session/window/pane browser")
	fmt.Fprintln(out, "  "+codeStyle.Render("atmux attach NAME")+"  Attach to a specific session")
	fmt.Fprintln(out, "  "+codeStyle.Render("atmux kill NAME")+"    Kill a session")
	fmt.Fprintln(out, "  "+codeStyle.Render("atmux list")+"         Simple session list (non-interactive)")
	fmt.Fprintln(out, "  "+codeStyle.Render("atmux init")+"         Create a sample .agent-tmux.conf")
	fmt.Fprintln(out)

	// Tips
	fmt.Fprintln(out, sectionStyle.Render("Tips"))
	fmt.Fprintln(out, "  • Run "+codeStyle.Render("atmux")+" with no arguments to see the landing page")
	fmt.Fprintln(out, "  • Set your default behavior in the landing page's Defaults section")
	fmt.Fprintln(out, "  • Use "+codeStyle.Render("atmux --reset-defaults")+" to restore landing page as default")
	fmt.Fprintln(out)

	fmt.Fprintln(out, dimStyle.Render("Run `atmux` to get started!"))

	return nil
}

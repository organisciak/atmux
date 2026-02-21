package cmd

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/tui"
	"github.com/spf13/cobra"
)

var onboardQuick bool

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Interactive setup wizard for atmux",
	Long:  "Interactive wizard to configure your AI coding agents and create global config.",
	RunE:  runOnboard,
}

func init() {
	rootCmd.AddCommand(onboardCmd)
	onboardCmd.Flags().BoolVar(&onboardQuick, "quick", false, "Show quick reference guide instead of wizard")
}

func runOnboard(cmd *cobra.Command, args []string) error {
	if !onboardQuick {
		// Run interactive wizard
		result, err := tui.RunOnboard()
		if err != nil {
			return err
		}
		if result.Completed {
			fmt.Println("\nConfiguration saved!")
		} else {
			fmt.Println("\nSetup skipped. Run 'atmux onboard' to configure later.")
		}

		// Show keybinding results
		if result.BrowseBindAdded || result.SessionsBindAdded {
			fmt.Println("\nKeybindings added to ~/.tmux.conf!")
			if result.BrowseBindAdded {
				fmt.Println("  prefix + S → atmux browse --popup (tree-style session browser)")
			}
			if result.SessionsBindAdded {
				fmt.Println("  prefix + s → atmux sessions -p (quick session list popup)")
			}
			fmt.Println("\nTo activate, run:")
			fmt.Println("  tmux source-file ~/.tmux.conf")
		} else if result.KeybindError != "" {
			fmt.Printf("\nWarning: Failed to add keybindings: %s\n", result.KeybindError)
		}

		fmt.Println("\nRun 'atmux' to start a session.")
		return nil
	}

	// Show quick reference (--quick flag)
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
	fmt.Fprintln(out, "  Each session is created with an "+codeStyle.Render("agents")+" window")
	fmt.Fprintln(out, "  containing your configured AI coding tools (claude, codex, etc.)")
	fmt.Fprintln(out)

	// Configuration
	fmt.Fprintln(out, sectionStyle.Render("Configuration"))
	fmt.Fprintln(out, "  Global: "+codeStyle.Render("~/.config/atmux/config"))
	fmt.Fprintln(out, "  Project: "+codeStyle.Render(".agent-tmux.conf")+" (overrides global)")
	fmt.Fprintln(out, dimStyle.Render(`
    # Example config
    agent:claude --dangerously-skip-permissions
    agent:codex --full-auto

    window:dev
    pane:npm run dev
    pane:npm run test -- --watch
`))

	// Commands
	fmt.Fprintln(out, sectionStyle.Render("All Commands"))
	fmt.Fprintln(out, "  "+codeStyle.Render("atmux")+"              Show landing page (configurable default)")
	fmt.Fprintln(out, "  "+codeStyle.Render("atmux sessions [NAME]")+"  List sessions or attach directly")
	fmt.Fprintln(out, "  "+codeStyle.Render("atmux browse")+"       Full session/window/pane browser")
	fmt.Fprintln(out, "  "+codeStyle.Render("atmux kill NAME")+"    Kill a session")
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

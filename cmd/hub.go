package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/porganisciak/agent-tmux/tui"
)

// hubView is a view the sessions list offers as a tab. New views belong here
// so they show up beside the list instead of being forgotten.
type hubView struct {
	name    string
	summary string
	// args run the view in the terminal the list was in, never as a popup of
	// its own: the list may already be one.
	args []string
}

var hubViews = []hubView{
	{
		name:    "Agents",
		summary: "Every Claude Code session as a card: whether it is working, waiting on you, or done, for how long, and its latest recap. Click a card to jump to it.",
		args:    []string{"agents", "--no-popup"},
	},
	{
		name:    "Tiles",
		summary: "Your most recently active Claude Code panes side by side in one window, live and typeable. They go back where they came from when you leave.",
		args:    []string{"tiles"},
	},
	{
		name:    "Live",
		summary: "A session tree on the left and the selected session's real pane on the right.",
		args:    []string{"live"},
	},
	{
		name:    "Browse",
		summary: "Every session, window and pane as a tree with previews; send commands to any pane.",
		args:    []string{"browse", "--no-popup"},
	},
	{
		name:    "Recents",
		summary: "Projects you have worked in recently, ready to reopen.",
		args:    []string{"recents", "--no-popup"},
	},
	{
		name:    "Schedule",
		summary: "Commands sent to panes on a schedule.",
		args:    []string{"schedule"},
	},
	{
		name:    "Setup",
		summary: "The setup wizard: tmux keybindings, status-bar titles and other options.",
		args:    []string{"onboard"},
	},
}

// sessionsHubViews describes the views for the sessions list, with the tmux
// key each is bound to.
func sessionsHubViews() []tui.HubView {
	keys := boundAtmuxKeys()
	views := make([]tui.HubView, 0, len(hubViews))
	for _, v := range hubViews {
		views = append(views, tui.HubView{
			Name:    v.name,
			Summary: v.summary,
			Command: "atmux " + v.args[0],
			Key:     keys[v.args[0]],
		})
	}
	return views
}

func findHubView(name string) (hubView, bool) {
	for _, v := range hubViews {
		if v.name == name {
			return v, true
		}
	}
	return hubView{}, false
}

// boundAtmuxKeys maps atmux subcommands to the tmux prefix key that runs them,
// e.g. "agents" → "prefix a". Empty outside tmux.
func boundAtmuxKeys() map[string]string {
	if os.Getenv("TMUX") == "" {
		return nil
	}
	out, err := exec.Command("tmux", "list-keys", "-T", "prefix").Output()
	if err != nil {
		return nil
	}
	return parseAtmuxKeys(string(out))
}

var atmuxCommandPattern = regexp.MustCompile(`\batmux\s+([a-z][a-z-]*)`)

// parseAtmuxKeys reads `tmux list-keys -T prefix` output. A line looks like
//
//	bind-key -T prefix a run-shell "atmux agents"
//
// with any number of flags before -T. The first binding seen for a command
// wins.
func parseAtmuxKeys(listKeys string) map[string]string {
	keys := make(map[string]string)
	for _, line := range strings.Split(listKeys, "\n") {
		fields := strings.Fields(line)
		key := ""
		for i := 0; i+2 < len(fields); i++ {
			if fields[i] == "-T" && fields[i+1] == "prefix" {
				key = fields[i+2]
				break
			}
		}
		if key == "" {
			continue
		}
		m := atmuxCommandPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if _, seen := keys[m[1]]; !seen {
			keys[m[1]] = "prefix " + key
		}
	}
	return keys
}

// runSessionsHub shows the sessions list, opens any view picked from its tabs,
// and comes back to the list when that view is closed.
func runSessionsHub(opts tui.SessionsOptions) (*tui.SessionsResult, error) {
	opts.Views = sessionsHubViews()
	for {
		result, err := tui.RunSessionsList(opts)
		if err != nil || result.OpenView == "" {
			return result, err
		}
		back, err := runHubView(result.OpenView)
		if err != nil || !back {
			return &tui.SessionsResult{}, err
		}
		opts.View = result.OpenView
	}
}

// runHubView runs a view chosen from the sessions list's tabs. It reports
// whether to show the list again: yes when the view was closed without taking
// the user anywhere, no once it has switched them to a session or pane.
func runHubView(name string) (backToList bool, err error) {
	v, ok := findHubView(name)
	if !ok {
		return false, fmt.Errorf("unknown view %q", name)
	}
	self, err := os.Executable()
	if err != nil {
		return false, err
	}
	before := clientPosition()
	c := exec.Command(self, v.args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	err = c.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// The view has said what went wrong; stay out of its way.
		return false, nil
	}
	if before == "" {
		// Outside tmux there is no way to tell, and the view has had the
		// terminal; leave it there.
		return false, err
	}
	return clientPosition() == before, err
}

// clientPosition identifies where the tmux client is looking, to notice a
// view that switched it somewhere. Empty outside tmux.
func clientPosition() string {
	if os.Getenv("TMUX") == "" {
		return ""
	}
	out, err := exec.Command("tmux", "display-message", "-p", "#{client_session}\t#{window_id}\t#{pane_id}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

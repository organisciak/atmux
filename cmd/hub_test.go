package cmd

import (
	"testing"
)

func TestParseAtmuxKeys(t *testing.T) {
	out := `bind-key    -T prefix       s                    run-shell "atmux sessions"
bind-key    -T prefix       a                    run-shell "atmux agents"
bind-key -r -T prefix       C-a                  run-shell "/usr/local/bin/atmux agents --remote vps"
bind-key    -T prefix       S                    run-shell "atmux browse --popup"
bind-key    -T prefix       c                    new-window
`
	got := parseAtmuxKeys(out)
	want := map[string]string{
		"sessions": "prefix s",
		"agents":   "prefix a", // the first binding wins
		"browse":   "prefix S",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for cmd, key := range want {
		if got[cmd] != key {
			t.Errorf("%s: got %q, want %q", cmd, got[cmd], key)
		}
	}
}

// Every tab must open something that exists, with flags it accepts.
func TestHubViewsRunRealCommands(t *testing.T) {
	seen := map[string]bool{}
	for _, v := range hubViews {
		if seen[v.name] {
			t.Errorf("view %q listed twice", v.name)
		}
		seen[v.name] = true
		if v.summary == "" {
			t.Errorf("view %q has no summary", v.name)
		}
		c, rest, err := rootCmd.Find(v.args)
		if err != nil || c == rootCmd {
			t.Errorf("view %q: %v is not a command", v.name, v.args)
			continue
		}
		if err := c.ParseFlags(rest); err != nil {
			t.Errorf("view %q: %v: %v", v.name, v.args, err)
		}
	}
}

package tmux

import (
	"os/exec"
	"strings"
	"testing"
)

func TestFindRemoteWindowMatchesTheWholeTag(t *testing.T) {
	out := "@1\t\n@4\tvps:agent-x2\n@5\tvps:agent-x\n@6\tocs:agent-x\n"
	if got := findRemoteWindow(out, "vps:agent-x"); got != "@5" {
		t.Fatalf("got %q, want @5", got)
	}
	if got := findRemoteWindow(out, "vps:other"); got != "" {
		t.Fatalf("got %q for a session with no window", got)
	}
}

func TestFindRemoteSession(t *testing.T) {
	cases := []struct {
		name, out, id, free string
	}{
		{"none yet", "$1\t\twork\n", "", "remote"},
		{"found by option, whatever its name", "$1\t\twork\n$3\t1\tmy-remotes\n", "$3", ""},
		{"user's own session named remote is left alone", "$1\t\tremote\n$2\t\tremote-2\n", "", "remote-3"},
	}
	for _, c := range cases {
		id, free := findRemoteSession(c.out)
		if id != c.id || free != c.free {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", c.name, id, free, c.id, c.free)
		}
	}
}

func TestHoldOnFailureWaitsOnlyWhenTheConnectionFails(t *testing.T) {
	out, err := exec.Command("sh", "-c", holdOnFailure("true", "vps")).Output()
	if err != nil || len(out) != 0 {
		t.Fatalf("clean exit: out %q, err %v; want silent", out, err)
	}
	cmd := exec.Command("sh", "-c", holdOnFailure("(exit 255)", "it's-a-host"))
	cmd.Stdin = strings.NewReader("\n")
	out, _ = cmd.Output()
	if !strings.Contains(string(out), "connection to it's-a-host ended with exit status 255") ||
		!strings.Contains(string(out), "Press Enter") {
		t.Fatalf("failure message = %q", out)
	}
}

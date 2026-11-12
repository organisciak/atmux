package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
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

func TestReconnectOnFailure(t *testing.T) {
	// The loop marks its pane through tmux; keep that off any real server.
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	// A clean exit closes the window without a prompt.
	out, err := exec.Command("sh", "-c", reconnectOnFailure("true", "vps")).Output()
	if err != nil || len(out) != 0 {
		t.Fatalf("clean exit: out %q, err %v; want silent", out, err)
	}

	// A failure prompts; Enter retries (here succeeding the second time) and
	// Ctrl-D (end of input) closes.
	dir := t.TempDir()
	flaky := "if [ -e " + dir + "/ok ]; then echo connected; else touch " + dir + "/ok; (exit 255); fi"
	cmd := exec.Command("sh", "-c", reconnectOnFailure(flaky, "it's-a-host"))
	cmd.Stdin = strings.NewReader("\n")
	out, _ = cmd.Output()
	got := string(out)
	if !strings.Contains(got, "connection to it's-a-host ended with exit status 255") ||
		!strings.Contains(got, "Press Enter to reconnect") || !strings.HasSuffix(got, "connected\n") {
		t.Fatalf("retry output = %q", got)
	}

	cmd = exec.Command("sh", "-c", reconnectOnFailure("(exit 255)", "vps"))
	cmd.Stdin = strings.NewReader("")
	if out, err := cmd.Output(); err != nil || strings.Count(string(out), "exit status 255") != 1 {
		t.Fatalf("Ctrl-D: out %q, err %v; want one prompt and a clean close", out, err)
	}
}

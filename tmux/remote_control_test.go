package tmux

import (
	"strings"
	"testing"
	"time"
)

func TestRemoteControlDisplayName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"agent-my-app", "my app"},
		{"agent-3d-print", "3d print"},
		{"atmux-some_project", "some project"},
		{"agent-creativity-task-archive", "creativity task archive"},
		{"plain", "plain"},
	}
	for _, c := range cases {
		if got := RemoteControlDisplayName(c.in); got != c.want {
			t.Errorf("RemoteControlDisplayName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRemoteControlDisplayName_NeverEmpty(t *testing.T) {
	// A name that is nothing but a prefix would otherwise slug to "", leaving
	// the remote session unlabelled in the app.
	if got := RemoteControlDisplayName("agent-"); got == "" {
		t.Fatal("expected a fallback name, got empty")
	}
}

func TestRemoteControlDisplayName_CollapsesWhitespace(t *testing.T) {
	if got := RemoteControlDisplayName("agent-a--b__c"); got != "a b c" {
		t.Fatalf("got %q, want %q", got, "a b c")
	}
}

func TestWaitForAgentPane_TimesOutWithoutHanging(t *testing.T) {
	// A session that never starts an agent must not block the launch forever.
	// A session that exists but only ever runs a shell: the agent never shows.
	exec := &fakeExecutor{
		responses: map[string]fakeResponse{
			"list-sessions": {output: []byte("missing:0\n")},
			"list-windows":  {output: []byte("@1:0:bash:1\n")},
			"list-panes":    {output: []byte("%1:0:MCE-HOSTNAME:zsh:1:80:24\n")},
		},
	}

	start := time.Now()
	_, err := WaitForAgentPane(exec, "missing", 400*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("timeout was not honoured, took %v", elapsed)
	}
}

func TestDefaultAgents_UsesAutoPermissionsAndContinue(t *testing.T) {
	agents := DefaultAgents()
	if len(agents) != 1 {
		t.Fatalf("expected one default agent, got %d", len(agents))
	}

	cmd := agents[0].Command
	if !strings.Contains(cmd, "--permission-mode auto") {
		t.Errorf("default should use auto permissions, got %q", cmd)
	}
	if strings.Contains(cmd, "--dangerously-skip-permissions") {
		t.Errorf("default should no longer bypass permissions, got %q", cmd)
	}
	if !strings.Contains(cmd, "--continue") {
		t.Errorf("default should resume the project's conversation, got %q", cmd)
	}
}

func TestDefaultAgents_FirstRunDropsContinue(t *testing.T) {
	// The default carries --continue, which only makes sense once a project has
	// a conversation to resume. A never-seen directory must start clean.
	stripped := stripResumeFlags(DefaultAgents()[0].Command)
	if strings.Contains(stripped, "--continue") {
		t.Fatalf("first run should not resume, got %q", stripped)
	}
	if !strings.Contains(stripped, "--permission-mode auto") {
		t.Fatalf("first run lost the permission mode: %q", stripped)
	}
}

package tmux

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/porganisciak/agent-tmux/config"
)

// newLiveSession creates a real detached tmux session for the test and removes
// it afterwards. Exercising the actual tmux option plumbing is the point: a
// wrong format string or option name only shows up against the real binary.
func newLiveSession(t *testing.T, name string) *Session {
	t.Helper()

	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
	if err := exec.Command("tmux", "new-session", "-d", "-s", name, "sleep", "60").Run(); err != nil {
		t.Skipf("could not create tmux session: %v", err)
	}
	t.Cleanup(func() {
		exec.Command("tmux", "kill-session", "-t", name).Run() //nolint:errcheck
	})

	return &Session{Name: name}
}

func sessionOption(t *testing.T, name, option string) string {
	t.Helper()
	out, err := exec.Command("tmux", "show-options", "-t", name, option).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func TestApplySessionTitle_SetsStatusRight(t *testing.T) {
	s := newLiveSession(t, "atmux-title-apply-test")

	if err := s.ApplySessionTitle(); err != nil {
		t.Fatalf("ApplySessionTitle: %v", err)
	}

	got := sessionOption(t, s.Name, "status-right")
	if !strings.Contains(got, "pane_title") {
		t.Fatalf("status-right does not show the pane title: %q", got)
	}
}

func TestClearSessionTitle_RestoresInheritedValue(t *testing.T) {
	s := newLiveSession(t, "atmux-title-clear-test")

	if err := s.ApplySessionTitle(); err != nil {
		t.Fatalf("ApplySessionTitle: %v", err)
	}
	if err := s.ClearSessionTitle(); err != nil {
		t.Fatalf("ClearSessionTitle: %v", err)
	}

	if got := sessionOption(t, s.Name, "status-right"); strings.Contains(got, "pane_title") {
		t.Fatalf("status-right should have been unset, got %q", got)
	}
}

func TestApplyConfig_HonoursSessionTitleDirective(t *testing.T) {
	s := newLiveSession(t, "atmux-title-config-test")

	if err := s.ApplyConfig(&config.Config{SessionTitle: true}); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}

	if got := sessionOption(t, s.Name, "status-right"); !strings.Contains(got, "pane_title") {
		t.Fatalf("config directive did not reach the session: %q", got)
	}
}

func TestApplyConfig_LeavesStatusBarAloneByDefault(t *testing.T) {
	// Overwriting a user's status-right uninvited would be invasive.
	s := newLiveSession(t, "atmux-title-default-test")

	if err := s.ApplyConfig(&config.Config{}); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}

	if got := sessionOption(t, s.Name, "status-right"); strings.Contains(got, "pane_title") {
		t.Fatalf("status-right was set without the directive: %q", got)
	}
}

func TestSessionTitleStatusRight_RendersAgainstRealTmux(t *testing.T) {
	s := newLiveSession(t, "atmux-title-render-test")

	// tmux must understand the format; an unsupported one renders literally.
	out, err := exec.Command("tmux", "display-message", "-p", "-t", s.Name, sessionTitleStatusRight).Output()
	if err != nil {
		t.Fatalf("display-message: %v", err)
	}
	if rendered := string(out); strings.Contains(rendered, "pane_title") {
		t.Fatalf("format was not interpreted by tmux: %q", rendered)
	}
}

package tmux

import (
	"strings"
	"testing"
)

func TestWrapRemoteCommand_DirectIsUnwrapped(t *testing.T) {
	got := wrapRemoteCommand(remoteShellDirect, "tmux", []string{"list-sessions", "-F", "#{session_name}"})
	want := "tmux 'list-sessions' '-F' '#{session_name}'"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapRemoteCommand_UnknownBehavesLikeDirect(t *testing.T) {
	// The first attempt on an unprobed host must be the cheap one.
	direct := wrapRemoteCommand(remoteShellDirect, "tmux", []string{"-V"})
	if got := wrapRemoteCommand(remoteShellUnknown, "tmux", []string{"-V"}); got != direct {
		t.Fatalf("unknown mode should try the direct form first, got %q", got)
	}
}

func TestWrapRemoteCommand_LoginShellQuotesWholeCommand(t *testing.T) {
	got := wrapRemoteCommand(remoteShellLogin, "tmux", []string{"list-sessions", "-F", "#{session_name}"})
	want := `bash -lc 'tmux '\''list-sessions'\'' '\''-F'\'' '\''#{session_name}'\'''`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapRemoteCommand_LoginShellSurvivesFormatStrings(t *testing.T) {
	// tmux format strings are full of characters a shell would otherwise eat;
	// they pass through two levels of quoting here.
	format := "#{session_activity}\t#{session_path}\t#{?session_attached, (attached),}"
	got := wrapRemoteCommand(remoteShellLogin, "tmux", []string{"list-sessions", "-F", format})

	if !strings.HasPrefix(got, "bash -lc '") {
		t.Fatalf("expected a login shell wrapper, got %q", got)
	}
	if !strings.HasSuffix(got, "'") {
		t.Fatalf("wrapper is not closed: %q", got)
	}
	// Every literal quote in the payload must have been escaped.
	if strings.Contains(got[len("bash -lc '"):len(got)-1], "'") &&
		!strings.Contains(got, `'\''`) {
		t.Fatalf("interior quotes were not escaped: %q", got)
	}
}

func TestBuildSSHInteractiveArgs_UsesLoginShellWhenNeeded(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")
	e.setShellMode(remoteShellLogin)

	args := e.buildSSHInteractiveArgs("attach-session", "-t", "work")
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "bash -lc") {
		t.Fatalf("expected a login shell for a host whose PATH lacks tmux, got %q", joined)
	}
	if !strings.Contains(joined, "attach-session") {
		t.Fatalf("attach target lost: %q", joined)
	}
}

func TestBuildSSHInteractiveArgs_DirectWhenPathIsFine(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")
	e.setShellMode(remoteShellDirect)

	args := e.buildSSHInteractiveArgs("attach-session", "-t", "work")
	joined := strings.Join(args, " ")

	if strings.Contains(joined, "bash") {
		t.Fatalf("a healthy PATH should not pay for a login shell, got %q", joined)
	}
	if !strings.HasSuffix(joined, "tmux attach-session -t work") {
		t.Fatalf("unexpected args: %q", joined)
	}
}

func TestBuildMoshArgs_UsesLoginShellWhenNeeded(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "mosh", "devbox")
	e.setShellMode(remoteShellLogin)

	joined := strings.Join(e.buildMoshArgs("attach-session", "-t", "work"), " ")
	if !strings.Contains(joined, "bash -lc") {
		t.Fatalf("mosh hits the same PATH problem, got %q", joined)
	}
}

func TestShellMode_CachedPerHost(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")
	if e.shellMode() != remoteShellUnknown {
		t.Fatal("a fresh executor should be unprobed")
	}

	e.setShellMode(remoteShellLogin)
	if e.shellMode() != remoteShellLogin {
		t.Fatal("mode must persist so the retry happens once per host")
	}
}

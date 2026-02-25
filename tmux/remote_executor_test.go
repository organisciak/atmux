package tmux

import (
	"reflect"
	"testing"
)

func TestBuildSSHInteractiveArgs_DefaultPort(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")
	got := e.buildSSHInteractiveArgs("attach-session", "-t", "mysess")
	want := []string{"-t", "-p", "22", "user@devbox", "tmux", "attach-session", "-t", "mysess"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildSSHInteractiveArgs mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildSSHInteractiveArgs_CustomPort(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 2222, "ssh", "devbox")
	got := e.buildSSHInteractiveArgs("attach-session", "-t", "work")
	want := []string{"-t", "-p", "2222", "user@devbox", "tmux", "attach-session", "-t", "work"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildSSHInteractiveArgs mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildSSHInteractiveArgs_NoTmuxArgs(t *testing.T) {
	e := NewRemoteExecutor("host", 22, "ssh", "")
	got := e.buildSSHInteractiveArgs()
	want := []string{"-t", "-p", "22", "host", "tmux"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildSSHInteractiveArgs mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildMoshArgs_DefaultPort(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "mosh", "devbox")
	got := e.buildMoshArgs("attach-session", "-t", "mysess")
	want := []string{"user@devbox", "--", "tmux", "attach-session", "-t", "mysess"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildMoshArgs mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildMoshArgs_CustomPort(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 2222, "mosh", "devbox")
	got := e.buildMoshArgs("attach-session", "-t", "mysess")
	want := []string{"--ssh=ssh -p 2222", "user@devbox", "--", "tmux", "attach-session", "-t", "mysess"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildMoshArgs mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildMoshArgs_NoTmuxArgs(t *testing.T) {
	e := NewRemoteExecutor("host", 22, "mosh", "")
	got := e.buildMoshArgs()
	want := []string{"host", "--", "tmux"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildMoshArgs mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestNewRemoteExecutor_Defaults(t *testing.T) {
	e := NewRemoteExecutor("myhost", 0, "", "")
	if e.Port != defaultSSHPort {
		t.Fatalf("expected default port %d, got %d", defaultSSHPort, e.Port)
	}
	if e.AttachMethod != "ssh" {
		t.Fatalf("expected default attach method 'ssh', got %q", e.AttachMethod)
	}
	if e.Alias != "myhost" {
		t.Fatalf("expected alias 'myhost', got %q", e.Alias)
	}
}

func TestNewRemoteExecutor_CustomValues(t *testing.T) {
	e := NewRemoteExecutor("user@box", 2222, "mosh", "devbox")
	if e.Port != 2222 {
		t.Fatalf("expected port 2222, got %d", e.Port)
	}
	if e.AttachMethod != "mosh" {
		t.Fatalf("expected attach method 'mosh', got %q", e.AttachMethod)
	}
	if e.Alias != "devbox" {
		t.Fatalf("expected alias 'devbox', got %q", e.Alias)
	}
}

func TestMoshAvailable(t *testing.T) {
	// This test verifies the function runs without error.
	// The result depends on the test environment (mosh may or may not be installed).
	_ = moshAvailable()
}

func TestInteractiveRouting_SSHMethod(t *testing.T) {
	// Verify that with attach_method=ssh, Interactive calls through the SSH path.
	// We test this indirectly by checking buildSSHInteractiveArgs is producing correct output.
	e := NewRemoteExecutor("user@host", 22, "ssh", "")
	if e.AttachMethod != "ssh" {
		t.Fatalf("expected ssh attach method, got %q", e.AttachMethod)
	}
	args := e.buildSSHInteractiveArgs("attach-session", "-t", "test")
	if args[0] != "-t" {
		t.Fatalf("expected first arg '-t' for SSH, got %q", args[0])
	}
}

func TestInteractiveRouting_MoshMethod(t *testing.T) {
	// Verify that with attach_method=mosh, the mosh args path is used.
	e := NewRemoteExecutor("user@host", 22, "mosh", "")
	if e.AttachMethod != "mosh" {
		t.Fatalf("expected mosh attach method, got %q", e.AttachMethod)
	}
	args := e.buildMoshArgs("attach-session", "-t", "test")
	if args[0] != "user@host" {
		t.Fatalf("expected first mosh arg to be host, got %q", args[0])
	}
	if args[1] != "--" {
		t.Fatalf("expected second mosh arg '--', got %q", args[1])
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"simple", "'simple'"},
		{"with space", "'with space'"},
		{"it's", "'it'\\''s'"},
		{"#{session_name}: #{session_windows} windows", "'#{session_name}: #{session_windows} windows'"},
	}
	for _, tt := range tests {
		if got := shellQuote(tt.input); got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRemoteCommand(t *testing.T) {
	got := remoteCommand("tmux", []string{"list-sessions", "-F", "#{session_name}: #{session_windows} windows"})
	want := "tmux 'list-sessions' '-F' '#{session_name}: #{session_windows} windows'"
	if got != want {
		t.Errorf("remoteCommand mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestHostLabel(t *testing.T) {
	e := NewRemoteExecutor("user@host", 22, "ssh", "my-alias")
	if got := e.HostLabel(); got != "my-alias" {
		t.Fatalf("expected HostLabel 'my-alias', got %q", got)
	}
}

func TestIsRemote(t *testing.T) {
	e := NewRemoteExecutor("host", 22, "ssh", "")
	if !e.IsRemote() {
		t.Fatal("expected IsRemote() to be true")
	}
}

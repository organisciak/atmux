package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/tmux"
)

// withProjectConfig runs the test from a temp directory containing the given
// .agent-tmux.conf, since loadRemoteConfig reads the config in the cwd.
//
// HOME is redirected too: LoadConfig merges the global config, so without this
// the results would depend on whatever hosts the developer happens to have
// configured.
func withProjectConfig(t *testing.T, contents string) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, config.DefaultConfigName), []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(original) })
}

func TestExecutorForHostLabel_ResolvesAliasToRealHost(t *testing.T) {
	withProjectConfig(t, "remote_host:me@devbox.internal\nremote_alias:devbox\nremote_port:2222\n")

	executor, err := executorForHostLabel("devbox", "")
	if err != nil {
		t.Fatalf("executorForHostLabel: %v", err)
	}

	re, ok := executor.(*tmux.RemoteExecutor)
	if !ok {
		t.Fatalf("expected a RemoteExecutor, got %T", executor)
	}
	// Using the alias as the SSH target is exactly the bug this guards against.
	if re.Host != "me@devbox.internal" {
		t.Errorf("host = %q, want me@devbox.internal", re.Host)
	}
	if re.Port != 2222 {
		t.Errorf("port = %d, want 2222 from config", re.Port)
	}
	if re.Alias != "devbox" {
		t.Errorf("alias = %q, want devbox", re.Alias)
	}
}

func TestExecutorForHostLabel_ConfiguredAttachMethodWins(t *testing.T) {
	withProjectConfig(t, "remote_host:me@devbox.internal\nremote_alias:devbox\nremote_attach:mosh\n")

	executor, err := executorForHostLabel("devbox", "ssh")
	if err != nil {
		t.Fatalf("executorForHostLabel: %v", err)
	}
	re := executor.(*tmux.RemoteExecutor)
	if re.AttachMethod != "mosh" {
		t.Fatalf("config should win over the recorded method, got %q", re.AttachMethod)
	}
}

func TestExecutorForHostLabel_HistoryFillsUnconfiguredAttachMethod(t *testing.T) {
	withProjectConfig(t, "remote_host:me@devbox.internal\nremote_alias:devbox\n")

	executor, err := executorForHostLabel("devbox", "mosh")
	if err != nil {
		t.Fatalf("executorForHostLabel: %v", err)
	}
	re := executor.(*tmux.RemoteExecutor)
	if re.AttachMethod != "mosh" {
		t.Fatalf("expected the recorded method to fill the gap, got %q", re.AttachMethod)
	}
}

func TestExecutorForHostLabel_UnknownLabelTreatedAsHostname(t *testing.T) {
	withProjectConfig(t, "")

	executor, err := executorForHostLabel("me@ad-hoc.example", "")
	if err != nil {
		t.Fatalf("executorForHostLabel: %v", err)
	}
	re := executor.(*tmux.RemoteExecutor)
	if re.Host != "me@ad-hoc.example" {
		t.Fatalf("host = %q, want the label used directly", re.Host)
	}
}

func TestExecutorForHostLabel_EmptyLabelIsAnError(t *testing.T) {
	withProjectConfig(t, "")

	if _, err := executorForHostLabel("", ""); err == nil {
		t.Fatal("expected an error for an empty host label")
	}
}

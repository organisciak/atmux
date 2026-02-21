package tmux

import (
	"testing"

	"github.com/porganisciak/agent-tmux/config"
)

func TestShellQuoteJoin_Simple(t *testing.T) {
	got := shellQuoteJoin([]string{"ssh", "-t", "-p", "22", "user@host", "tmux", "attach-session", "-t", "mysess"})
	want := "ssh -t -p 22 user@host tmux attach-session -t mysess"
	if got != want {
		t.Fatalf("shellQuoteJoin mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestShellQuoteJoin_WithSpaces(t *testing.T) {
	got := shellQuoteJoin([]string{"mosh", "--ssh=ssh -p 2222", "user@host", "--", "tmux", "attach-session", "-t", "mysess"})
	want := "mosh '--ssh=ssh -p 2222' user@host -- tmux attach-session -t mysess"
	if got != want {
		t.Fatalf("shellQuoteJoin mismatch\n got: %q\nwant: %q", got, want)
	}
}

// mockExecutor records which methods were called for testing strategy routing.
type mockExecutor struct {
	isRemote        bool
	interactiveCalled bool
	interactiveArgs   []string
}

func (m *mockExecutor) Run(args ...string) error                            { return nil }
func (m *mockExecutor) Output(args ...string) ([]byte, error)               { return nil, nil }
func (m *mockExecutor) RunWithDir(dir string, args ...string) error         { return nil }
func (m *mockExecutor) RunGeneric(command string, args ...string) ([]byte, error) { return nil, nil }
func (m *mockExecutor) HostLabel() string                                   { return "testhost" }
func (m *mockExecutor) IsRemote() bool                                      { return m.isRemote }
func (m *mockExecutor) Close() error                                        { return nil }
func (m *mockExecutor) Interactive(args ...string) error {
	m.interactiveCalled = true
	m.interactiveArgs = args
	return nil
}

func TestAttachToSessionWithStrategy_LocalIgnoresStrategy(t *testing.T) {
	// Local executor should use standard AttachToSession regardless of strategy.
	// We can't actually run tmux in tests, but we verify the function
	// takes the local path by checking it doesn't call Interactive on the executor.
	mock := &mockExecutor{isRemote: false}
	// This will fail because there's no tmux server, but it should NOT call Interactive
	_ = AttachToSessionWithStrategy("test", mock, config.AttachStrategyNewWindow)
	if mock.interactiveCalled {
		t.Fatal("expected local session to NOT call Interactive on the executor")
	}
}

func TestAttachToSessionWithStrategy_EmptyName(t *testing.T) {
	mock := &mockExecutor{isRemote: true}
	err := AttachToSessionWithStrategy("", mock, config.AttachStrategyAuto)
	if err != nil {
		t.Fatalf("expected nil error for empty name, got %v", err)
	}
	if mock.interactiveCalled {
		t.Fatal("expected no Interactive call for empty name")
	}
}

func TestAttachToSessionWithStrategy_ReplaceCallsInteractive(t *testing.T) {
	mock := &mockExecutor{isRemote: true}
	err := AttachToSessionWithStrategy("mysess", mock, config.AttachStrategyReplace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.interactiveCalled {
		t.Fatal("expected Interactive to be called for replace strategy")
	}
	if len(mock.interactiveArgs) < 3 || mock.interactiveArgs[2] != "mysess" {
		t.Fatalf("expected attach-session args with session name, got %v", mock.interactiveArgs)
	}
}

func TestAttachToSessionWithStrategy_AutoNotInsideTmux(t *testing.T) {
	// When TMUX env is not set, auto should call Interactive directly.
	t.Setenv("TMUX", "")
	mock := &mockExecutor{isRemote: true}
	err := AttachToSessionWithStrategy("mysess", mock, config.AttachStrategyAuto)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.interactiveCalled {
		t.Fatal("expected Interactive to be called when not inside tmux")
	}
}

func TestValidAttachStrategy(t *testing.T) {
	tests := []struct {
		input config.AttachStrategy
		valid bool
	}{
		{config.AttachStrategyAuto, true},
		{config.AttachStrategyReplace, true},
		{config.AttachStrategyNewWindow, true},
		{"", false},
		{"bogus", false},
	}
	for _, tc := range tests {
		got := config.ValidAttachStrategy(tc.input)
		if got != tc.valid {
			t.Errorf("ValidAttachStrategy(%q) = %v, want %v", tc.input, got, tc.valid)
		}
	}
}

func TestRemoteExecutorAttachStrategyField(t *testing.T) {
	e := NewRemoteExecutor("user@host", 22, "ssh", "myhost")
	if e.AttachStrategy != "" {
		t.Fatalf("expected empty default AttachStrategy, got %q", e.AttachStrategy)
	}
	e.AttachStrategy = "new-window"
	if e.AttachStrategy != "new-window" {
		t.Fatalf("expected 'new-window', got %q", e.AttachStrategy)
	}
}

func TestSettingsRemoteAttachStrategy(t *testing.T) {
	s := config.DefaultSettings()
	if s.RemoteAttachStrategy != "" {
		t.Fatalf("expected empty default RemoteAttachStrategy, got %q", s.RemoteAttachStrategy)
	}
}

package tmux

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestStripResumeFlags(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"claude", "claude"},
		{"claude --continue", "claude"},
		{"claude --dangerously-skip-permissions --continue", "claude --dangerously-skip-permissions"},
		{"claude --continue --dangerously-skip-permissions", "claude --dangerously-skip-permissions"},
		{"claude --resume", "claude"},
		{"codex --full-auto", "codex --full-auto"},
		// -c is intentionally left alone (collides with `bash -c "..."`).
		{"bash -c 'echo hi'", "bash -c 'echo hi'"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := stripResumeFlags(tc.in); got != tc.want {
			t.Errorf("stripResumeFlags(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseSessionLine(t *testing.T) {
	line := "agent-foo: 2 windows (created Fri Jan 30 10:00:00 2026) [80x24]"
	parsed := parseSessionLine(line)
	if parsed.Name != "agent-foo" {
		t.Fatalf("expected name agent-foo, got %q", parsed.Name)
	}
	if parsed.Line != line {
		t.Fatalf("expected line %q, got %q", line, parsed.Line)
	}
}

func TestParseSessionLineWithPath(t *testing.T) {
	// Format: activity\tsession_path\tdisplay_line
	raw := "1735000000\t/Users/me/projects/foo\tagent-foo: 2 windows (created Fri Jan 30 10:00:00 2026)"
	parsed := parseSessionLine(raw)
	if parsed.Name != "agent-foo" {
		t.Fatalf("expected name agent-foo, got %q", parsed.Name)
	}
	if parsed.WorkingDir != "/Users/me/projects/foo" {
		t.Fatalf("expected working dir, got %q", parsed.WorkingDir)
	}
	if parsed.Activity != 1735000000 {
		t.Fatalf("expected activity, got %d", parsed.Activity)
	}
	if !strings.Contains(parsed.Line, "agent-foo: 2 windows") {
		t.Fatalf("expected display line preserved, got %q", parsed.Line)
	}
}

func TestListSessionsRawWithExecutorNoServerRunning(t *testing.T) {
	executor := stubExecutor{
		outputErr: &exec.ExitError{
			Stderr: []byte("no server running on /tmp/tmux-501/default\n"),
		},
	}

	lines, err := ListSessionsRawWithExecutor(executor)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected no sessions, got %d", len(lines))
	}
}

func TestListSessionsRawWithExecutorUnexpectedError(t *testing.T) {
	expectedErr := errors.New("permission denied")
	executor := stubExecutor{outputErr: expectedErr}

	lines, err := ListSessionsRawWithExecutor(executor)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
	if lines != nil {
		t.Fatalf("expected nil sessions on error, got %v", lines)
	}
}

type stubExecutor struct {
	output    []byte
	outputErr error
}

func (s stubExecutor) Run(args ...string) error {
	return nil
}

func (s stubExecutor) Output(args ...string) ([]byte, error) {
	return s.output, s.outputErr
}

func (s stubExecutor) RunWithDir(dir string, args ...string) error {
	return nil
}

func (s stubExecutor) Interactive(args ...string) error {
	return nil
}

func (s stubExecutor) RunGeneric(command string, args ...string) ([]byte, error) {
	return nil, nil
}

func (s stubExecutor) HostLabel() string {
	return ""
}

func (s stubExecutor) IsRemote() bool {
	return false
}

func (s stubExecutor) HostState() HostState {
	return HostState{Status: HostOK}
}

func (s stubExecutor) ResetBackoff() {}

func (s stubExecutor) Close() error {
	return nil
}

func TestParseSessionLineWithAttachedFlag(t *testing.T) {
	// Format: activity\tsession_path\tsession_attached\tdisplay_line
	raw := "1735000000\t/Users/me/projects/foo\t1\tagent-foo: 2 windows (created Fri Jan 30 10:00:00 2026) (attached)"
	parsed := parseSessionLine(raw)
	if parsed.Name != "agent-foo" {
		t.Fatalf("expected name agent-foo, got %q", parsed.Name)
	}
	if parsed.WorkingDir != "/Users/me/projects/foo" {
		t.Fatalf("expected working dir, got %q", parsed.WorkingDir)
	}
	if !parsed.Attached {
		t.Fatal("expected attached to be true")
	}
	if strings.Contains(parsed.Line, "\t") {
		t.Fatalf("display line must not retain prefix fields, got %q", parsed.Line)
	}
}

func TestParseSessionLineDetached(t *testing.T) {
	raw := "1735000000\t/Users/me/projects/foo\t0\tagent-foo: 2 windows (created Fri Jan 30 10:00:00 2026)"
	if parsed := parseSessionLine(raw); parsed.Attached {
		t.Fatal("expected attached to be false for session_attached=0")
	}
}

func TestParseSessionLineOlderFormatsStillParse(t *testing.T) {
	// Output produced by an older format string must not regress.
	twoField := parseSessionLine("1735000000\tagent-foo: 2 windows (created Fri Jan 30 10:00:00 2026)")
	if twoField.Name != "agent-foo" || twoField.Activity != 1735000000 {
		t.Fatalf("two-field form mis-parsed: %+v", twoField)
	}
	if twoField.WorkingDir != "" {
		t.Fatalf("two-field form should have no working dir, got %q", twoField.WorkingDir)
	}

	bare := parseSessionLine("agent-foo: 2 windows (created Fri Jan 30 10:00:00 2026)")
	if bare.Name != "agent-foo" {
		t.Fatalf("bare form mis-parsed: %+v", bare)
	}
}

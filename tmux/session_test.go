package tmux

import (
	"errors"
	"os/exec"
	"testing"
)

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

func (s stubExecutor) Close() error {
	return nil
}

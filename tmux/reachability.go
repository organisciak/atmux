package tmux

import (
	"errors"
	"os/exec"
	"strings"
	"time"
)

// HostStatus classifies why a host's tmux data is or is not available.
// The distinction matters: a host behind a downed VPN and a host that simply
// lacks tmux both fail, but only one of them is worth retrying.
type HostStatus int

const (
	// HostUnknown means the host has not been contacted yet.
	HostUnknown HostStatus = iota
	// HostOK means the last command reached the host and tmux answered.
	HostOK
	// HostUnreachable means SSH itself failed: network down, VPN off, auth refused.
	HostUnreachable
	// HostTmuxMissing means SSH succeeded but tmux is not installed there.
	HostTmuxMissing
)

func (s HostStatus) String() string {
	switch s {
	case HostOK:
		return "ok"
	case HostUnreachable:
		return "unreachable"
	case HostTmuxMissing:
		return "tmux not installed"
	default:
		return "unknown"
	}
}

// HostState is a snapshot of a host's reachability.
type HostState struct {
	Status  HostStatus
	Err     error
	LastOK  time.Time // Zero when the host has never responded.
	RetryAt time.Time // Zero unless the host is in retry backoff.
}

// InBackoff reports whether the host is waiting out a retry delay.
func (s HostState) InBackoff() bool {
	return !s.RetryAt.IsZero() && time.Now().Before(s.RetryAt)
}

// Reason returns a short explanation suitable for display next to a host.
func (s HostState) Reason() string {
	switch s.Status {
	case HostOK:
		return ""
	case HostTmuxMissing:
		return "tmux not installed"
	case HostUnreachable:
		return "unreachable"
	default:
		return "not checked"
	}
}

const (
	// sshFailureExit is what ssh returns for its own failures, as opposed to
	// the remote command's exit status.
	sshFailureExit = 255
	// commandNotFoundExit is the shell's status for a missing command.
	commandNotFoundExit = 127

	// initialBackoff is how long a failed host waits before the next attempt.
	initialBackoff = 5 * time.Second
	// maxBackoff caps the exponential growth so a host that comes back up is
	// picked up within a couple of minutes even without an explicit refresh.
	maxBackoff = 2 * time.Minute
)

// classifyExecError maps an error from a remote command to a host status.
// Exit statuses that belong to tmux itself (such as "no server running")
// count as HostOK: the host answered, it just had nothing to report.
func classifyExecError(err error) HostStatus {
	if err == nil {
		return HostOK
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		switch exitErr.ExitCode() {
		case sshFailureExit:
			return HostUnreachable
		case commandNotFoundExit:
			return HostTmuxMissing
		}
		// Some shells report a missing command without using status 127.
		if isCommandNotFoundStderr(string(exitErr.Stderr)) {
			return HostTmuxMissing
		}
		return HostOK
	}

	// Context deadline, missing ssh binary, or a failure before exec: none of
	// these tell us the host is healthy.
	return HostUnreachable
}

func isCommandNotFoundStderr(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "command not found") ||
		strings.Contains(lower, "tmux: not found")
}

// nextBackoff returns the delay following the current one, doubling up to
// maxBackoff.
func nextBackoff(current time.Duration) time.Duration {
	if current <= 0 {
		return initialBackoff
	}
	doubled := current * 2
	if doubled > maxBackoff {
		return maxBackoff
	}
	return doubled
}

package tmux

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// exitErrorWithCode produces a real *exec.ExitError carrying the given status,
// which is what classifyExecError has to work with in production.
func exitErrorWithCode(t *testing.T, code int) *exec.ExitError {
	t.Helper()
	err := exec.Command("sh", "-c", "exit "+strconv.Itoa(code)).Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError for code %d, got %T", code, err)
	}
	return exitErr
}

func TestClassifyExecError_Nil(t *testing.T) {
	if got := classifyExecError(nil); got != HostOK {
		t.Fatalf("expected HostOK for nil error, got %v", got)
	}
}

func TestClassifyExecError_SSHFailureIsUnreachable(t *testing.T) {
	if got := classifyExecError(exitErrorWithCode(t, sshFailureExit)); got != HostUnreachable {
		t.Fatalf("expected HostUnreachable for exit 255, got %v", got)
	}
}

func TestClassifyExecError_CommandNotFoundIsTmuxMissing(t *testing.T) {
	if got := classifyExecError(exitErrorWithCode(t, commandNotFoundExit)); got != HostTmuxMissing {
		t.Fatalf("expected HostTmuxMissing for exit 127, got %v", got)
	}
}

func TestClassifyExecError_TmuxOwnFailureIsHealthy(t *testing.T) {
	// `tmux list-sessions` with no server exits 1. The host answered, so it is
	// healthy — this is the case that was previously reported as unreachable.
	if got := classifyExecError(exitErrorWithCode(t, 1)); got != HostOK {
		t.Fatalf("expected HostOK for exit 1, got %v", got)
	}
}

func TestClassifyExecError_StderrFallbackForMissingCommand(t *testing.T) {
	// Some shells report a missing command with a status other than 127.
	exitErr := exitErrorWithCode(t, 2)
	exitErr.Stderr = []byte("zsh:1: command not found: tmux")
	if got := classifyExecError(exitErr); got != HostTmuxMissing {
		t.Fatalf("expected HostTmuxMissing from stderr, got %v", got)
	}
}

func TestClassifyExecError_NonExitErrorIsUnreachable(t *testing.T) {
	if got := classifyExecError(context.DeadlineExceeded); got != HostUnreachable {
		t.Fatalf("expected HostUnreachable for a timeout, got %v", got)
	}
}

func TestNextBackoff_GrowsAndCaps(t *testing.T) {
	if got := nextBackoff(0); got != initialBackoff {
		t.Fatalf("expected first backoff %v, got %v", initialBackoff, got)
	}
	if got := nextBackoff(initialBackoff); got != 2*initialBackoff {
		t.Fatalf("expected doubling, got %v", got)
	}
	if got := nextBackoff(maxBackoff); got != maxBackoff {
		t.Fatalf("expected cap at %v, got %v", maxBackoff, got)
	}
	if got := nextBackoff(maxBackoff - time.Second); got != maxBackoff {
		t.Fatalf("expected cap when doubling overshoots, got %v", got)
	}
}

func TestHostState_InBackoff(t *testing.T) {
	if (HostState{}).InBackoff() {
		t.Fatal("zero RetryAt must not count as backoff")
	}
	if (HostState{RetryAt: time.Now().Add(-time.Second)}).InBackoff() {
		t.Fatal("expired RetryAt must not count as backoff")
	}
	if !(HostState{RetryAt: time.Now().Add(time.Minute)}).InBackoff() {
		t.Fatal("future RetryAt must count as backoff")
	}
}

func TestHostState_ReasonDistinguishesFailures(t *testing.T) {
	// The whole point of the split: a host with no tmux must not read the same
	// as a host behind a downed VPN.
	unreachable := HostState{Status: HostUnreachable}.Reason()
	missing := HostState{Status: HostTmuxMissing}.Reason()
	if unreachable == missing {
		t.Fatalf("unreachable and tmux-missing must differ, both were %q", unreachable)
	}
	if got := (HostState{Status: HostOK}).Reason(); got != "" {
		t.Fatalf("healthy host should have no reason, got %q", got)
	}
}

func TestRecordFailure_SetsBackoffAndPreservesLastOK(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")

	e.mu.Lock()
	e.markVerifiedLocked()
	e.mu.Unlock()

	lastOK := e.HostState().LastOK
	if lastOK.IsZero() {
		t.Fatal("expected LastOK to be set after success")
	}

	wantErr := errors.New("boom")
	e.mu.Lock()
	e.recordFailureLocked(HostUnreachable, wantErr)
	e.mu.Unlock()

	state := e.HostState()
	if state.Status != HostUnreachable {
		t.Fatalf("expected HostUnreachable, got %v", state.Status)
	}
	if !errors.Is(state.Err, wantErr) {
		t.Fatalf("expected stored error, got %v", state.Err)
	}
	if !state.InBackoff() {
		t.Fatal("expected host to be in backoff after a failure")
	}
	if !state.LastOK.Equal(lastOK) {
		t.Fatal("LastOK must survive a later failure so the UI can show last-seen")
	}
}

func TestRecordFailure_BackoffGrowsAcrossAttempts(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")

	e.mu.Lock()
	e.recordFailureLocked(HostUnreachable, errors.New("1"))
	first := e.backoff
	e.recordFailureLocked(HostUnreachable, errors.New("2"))
	second := e.backoff
	e.mu.Unlock()

	if second <= first {
		t.Fatalf("expected backoff to grow, got %v then %v", first, second)
	}
}

func TestEnsureControlMaster_FailsFastWhileInBackoff(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")

	wantErr := errors.New("host is down")
	e.mu.Lock()
	e.recordFailureLocked(HostUnreachable, wantErr)
	e.mu.Unlock()

	// A host in backoff must not attempt to dial: the point is that one
	// unreachable host does not cost every refresh a connect timeout.
	start := time.Now()
	err := e.ensureControlMaster()
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("expected immediate failure while in backoff, took %v", elapsed)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the stored error, got %v", err)
	}
}

func TestResetBackoff_AllowsImmediateRetry(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")

	e.mu.Lock()
	e.recordFailureLocked(HostUnreachable, errors.New("down"))
	e.mu.Unlock()

	if !e.HostState().InBackoff() {
		t.Fatal("expected backoff after failure")
	}

	e.ResetBackoff()

	if e.HostState().InBackoff() {
		t.Fatal("ResetBackoff must clear the retry delay")
	}
	if e.backoff != 0 {
		t.Fatalf("expected backoff duration reset, got %v", e.backoff)
	}
}

func TestRecordOutcome_MissingProbeDoesNotMarkHostUnhealthy(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")

	// RunGeneric probes for optional binaries (such as a remote atmux). A
	// missing one says nothing about the host.
	e.recordOutcome(exitErrorWithCode(t, commandNotFoundExit), HostOK)

	if state := e.HostState(); state.Status != HostOK {
		t.Fatalf("expected HostOK for a missing probe binary, got %v", state.Status)
	}
}

func TestRecordOutcome_MissingTmuxIsDistinctFromUnreachable(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")
	e.recordOutcome(exitErrorWithCode(t, commandNotFoundExit), HostTmuxMissing)

	if state := e.HostState(); state.Status != HostTmuxMissing {
		t.Fatalf("expected HostTmuxMissing, got %v", state.Status)
	}
}

func TestLocalExecutor_IsAlwaysHealthy(t *testing.T) {
	l := NewLocalExecutor()
	if got := l.HostState().Status; got != HostOK {
		t.Fatalf("expected local host to be HostOK, got %v", got)
	}
	l.ResetBackoff() // Must not panic.
}

func TestConnectionOpts_IncludeTimeoutAndKeepalive(t *testing.T) {
	joined := strings.Join(connectionOpts(), " ")
	for _, want := range []string{"ConnectTimeout=5", "ServerAliveInterval=15", "ServerAliveCountMax=3"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in connection opts, got %q", want, joined)
		}
	}
}

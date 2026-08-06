package tmux

import (
	"strings"
)

// remoteAtmuxMode records how (or whether) atmux can be invoked on a host.
type remoteAtmuxMode int

const (
	// remoteAtmuxUnknown means the host has not been probed yet.
	remoteAtmuxUnknown remoteAtmuxMode = iota
	// remoteAtmuxDirect means `atmux` is on the non-interactive PATH.
	remoteAtmuxDirect
	// remoteAtmuxLoginShell means atmux was only found via a login shell.
	remoteAtmuxLoginShell
	// remoteAtmuxAbsent means the host has no usable atmux; fall back to tmux.
	remoteAtmuxAbsent
)

// loginShellFallback is tried when atmux is missing from the non-interactive
// PATH. SSH runs commands in a non-login, non-interactive shell, which on many
// setups skips the profile that puts ~/bin or Homebrew on PATH — so "not found"
// there does not mean atmux is absent.
const loginShellFallback = "bash"

// remoteAtmuxArgs builds the RunGeneric arguments for invoking atmux on a host
// in the given mode.
func remoteAtmuxArgs(mode remoteAtmuxMode, args ...string) (string, []string) {
	if mode == remoteAtmuxLoginShell {
		return loginShellFallback, []string{"-lc", "atmux " + strings.Join(args, " ")}
	}
	return "atmux", args
}

// sessionsViaAtmux asks a remote atmux for its own session list, which carries
// the metadata plain `tmux list-sessions` cannot: working directories, project
// colors, and beads counts.
//
// The fetch doubles as the capability probe, so a supported host costs one
// round trip rather than a probe plus a fetch. Returns ok=false when the host
// has no usable atmux or speaks a schema this build does not understand; the
// caller then falls back to plain tmux.
func (e *RemoteExecutor) sessionsViaAtmux() ([]SessionLine, bool) {
	mode := e.atmuxMode()
	if mode == remoteAtmuxAbsent {
		return nil, false
	}

	args := []string{"sessions", "--json"}
	if e.SkipBeads {
		// Counting beads shells out once per session on the remote, which is
		// the slowest part of the payload.
		args = append(args, "--no-beads")
	}

	// An unprobed host tries the direct call first, then a login shell.
	modes := []remoteAtmuxMode{mode}
	if mode == remoteAtmuxUnknown {
		modes = []remoteAtmuxMode{remoteAtmuxDirect, remoteAtmuxLoginShell}
	}

	for _, m := range modes {
		command, cmdArgs := remoteAtmuxArgs(m, args...)
		out, err := e.RunGeneric(command, cmdArgs...)
		if err != nil {
			continue
		}

		lines, perr := ParseSessionsPayload(out, e.HostLabel())
		if perr != nil {
			// The host has atmux but we cannot read what it said. Retrying
			// through another shell will not change that.
			e.setAtmuxMode(remoteAtmuxAbsent)
			return nil, false
		}

		e.setAtmuxMode(m)
		return lines, true
	}

	e.setAtmuxMode(remoteAtmuxAbsent)
	return nil, false
}

func (e *RemoteExecutor) atmuxMode() remoteAtmuxMode {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.atmux
}

func (e *RemoteExecutor) setAtmuxMode(mode remoteAtmuxMode) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.atmux = mode
}

// RemoteAtmuxAvailable reports whether this host served its session list
// through its own atmux. It is only meaningful after a fetch.
func (e *RemoteExecutor) RemoteAtmuxAvailable() bool {
	mode := e.atmuxMode()
	return mode == remoteAtmuxDirect || mode == remoteAtmuxLoginShell
}

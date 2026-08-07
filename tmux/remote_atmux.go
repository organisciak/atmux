package tmux

// remoteAtmuxMode records whether a host can serve its own session list.
type remoteAtmuxMode int

const (
	// remoteAtmuxUnknown means the host has not been probed yet.
	remoteAtmuxUnknown remoteAtmuxMode = iota
	// remoteAtmuxAvailable means the host's atmux speaks a schema we can read.
	remoteAtmuxAvailable
	// remoteAtmuxAbsent means the host has no usable atmux; fall back to tmux.
	remoteAtmuxAbsent
)

// sessionsViaAtmux asks a remote atmux for its own session list, which carries
// the metadata plain `tmux list-sessions` cannot: working directories, project
// colors, and beads counts.
//
// The fetch doubles as the capability probe, so a supported host costs one
// round trip rather than a probe plus a fetch. Returns ok=false when the host
// has no usable atmux, runs one too old to serve JSON, or speaks a schema this
// build does not understand; the caller then falls back to plain tmux.
//
// Finding atmux on a host whose PATH lacks it is handled by RunGeneric, which
// retries through a login shell.
func (e *RemoteExecutor) sessionsViaAtmux() ([]SessionLine, bool) {
	if e.atmuxMode() == remoteAtmuxAbsent {
		return nil, false
	}

	args := []string{"sessions", "--json"}
	if e.SkipBeads {
		// Counting beads shells out once per session on the remote, which is
		// the slowest part of the payload.
		args = append(args, "--no-beads")
	}

	out, err := e.RunGeneric("atmux", args...)
	if err != nil {
		// Either atmux is absent, or it predates --json and rejected the flag.
		e.setAtmuxMode(remoteAtmuxAbsent)
		return nil, false
	}

	lines, perr := ParseSessionsPayload(out, e.HostLabel())
	if perr != nil {
		// The host has atmux but we cannot read what it said. Retrying will
		// not change that.
		e.setAtmuxMode(remoteAtmuxAbsent)
		return nil, false
	}

	e.setAtmuxMode(remoteAtmuxAvailable)
	return lines, true
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
	return e.atmuxMode() == remoteAtmuxAvailable
}

package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultSSHPort    = 22
	defaultSSHTimeout = 10 * time.Second

	// controlCheckTimeout bounds `ssh -O check`. The check talks only to the
	// local socket, so it is fast when the master is alive and fails quickly
	// when it is not.
	controlCheckTimeout = 3 * time.Second

	// connectTimeout bounds the TCP/handshake phase. A blackholed host (VPN
	// down) would otherwise hang for the full command timeout.
	connectTimeout = 5

	// serverAliveInterval and serverAliveCountMax make a master whose
	// connection has silently died tear itself down. `ssh -O check` only asks
	// the local master process whether it is running, so without these a dead
	// master reports healthy and every command through it hangs.
	serverAliveInterval = 15
	serverAliveCountMax = 3

	// aliveCheckInterval bounds how often the local `-O check` runs.
	aliveCheckInterval = 5 * time.Second
)

// connectionOpts are the SSH options shared by the control master and every
// command that rides it.
func connectionOpts() []string {
	return []string{
		"-o", "ConnectTimeout=" + strconv.Itoa(connectTimeout),
		"-o", "ServerAliveInterval=" + strconv.Itoa(serverAliveInterval),
		"-o", "ServerAliveCountMax=" + strconv.Itoa(serverAliveCountMax),
	}
}

// RemoteExecutor runs tmux commands on a remote host via SSH.
// It uses SSH ControlMaster for connection pooling.
type RemoteExecutor struct {
	Host           string // user@host or SSH config alias
	Port           int    // SSH port (default 22)
	AttachMethod   string // "ssh" or "mosh"
	Alias          string // Display alias (e.g., "devbox")
	AttachStrategy string // Per-host override: "auto", "replace", or "new-window" (empty = use global)
	SkipBeads      bool   // Ask a remote atmux to skip beads counts (they cost a shell-out per session)

	mu           sync.Mutex
	controlPath  string    // ControlMaster socket path (stable across processes)
	state        HostState // Last known reachability
	backoff      time.Duration
	lastVerified time.Time // When the control master was last confirmed alive
	atmux        remoteAtmuxMode
	shell        remoteShellMode
}

// NewRemoteExecutor creates a new RemoteExecutor for the given host.
func NewRemoteExecutor(host string, port int, attachMethod, alias string) *RemoteExecutor {
	if port <= 0 {
		port = defaultSSHPort
	}
	if attachMethod == "" {
		attachMethod = "ssh"
	}
	if alias == "" {
		alias = host
	}
	return &RemoteExecutor{
		Host:         host,
		Port:         port,
		AttachMethod: attachMethod,
		Alias:        alias,
	}
}

// ensureControlMaster resolves the persistent control socket for this host and
// starts a ControlMaster if one is not already running. The socket path is
// stable across processes, so a master opened by an earlier atmux invocation is
// reused here without a new SSH handshake.
func (e *RemoteExecutor) ensureControlMaster() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// A host in backoff fails immediately rather than burning the dial timeout
	// again. Without this, one host behind a downed VPN costs every refresh a
	// full ConnectTimeout.
	if e.state.InBackoff() {
		return e.state.Err
	}

	// Re-checking the master on every tmux call would add a subprocess per
	// command, and a tree fetch issues several per session.
	if e.state.Status == HostOK && time.Since(e.lastVerified) < aliveCheckInterval {
		return nil
	}

	if e.controlPath == "" {
		path, err := controlSocketPath(e.Host, e.Port)
		if err != nil {
			e.recordFailureLocked(HostUnreachable, err)
			return err
		}
		e.controlPath = path
	}

	// Reusing a live master is the entire point of the stable path.
	if controlMasterAlive(e.controlPath, e.Host, e.Port) {
		e.markVerifiedLocked()
		return nil
	}

	// A socket file that fails `-O check` is stale: the remote rebooted or the
	// master was killed. SSH will not clear it on its own, and leaving it in
	// place makes every later connection fail.
	removeStaleSocket(e.controlPath)

	if err := e.startControlMaster(); err != nil {
		e.recordFailureLocked(HostUnreachable, err)
		return err
	}
	e.markVerifiedLocked()
	return nil
}

// markVerifiedLocked records that the control master is confirmed alive.
// Callers must hold e.mu.
func (e *RemoteExecutor) markVerifiedLocked() {
	now := time.Now()
	e.lastVerified = now
	e.state = HostState{Status: HostOK, LastOK: now}
	e.backoff = 0
}

// recordFailureLocked stores a failure and schedules the next retry.
// Callers must hold e.mu.
func (e *RemoteExecutor) recordFailureLocked(status HostStatus, err error) {
	e.backoff = nextBackoff(e.backoff)
	e.state = HostState{
		Status:  status,
		Err:     err,
		LastOK:  e.state.LastOK, // Preserve when the host last worked.
		RetryAt: time.Now().Add(e.backoff),
	}
	e.lastVerified = time.Time{}
}

// recordOutcome classifies the result of a remote command and updates state.
// This is what distinguishes a host that is unreachable from one that answers
// fine but has no tmux installed.
func (e *RemoteExecutor) recordOutcome(err error, notFoundStatus HostStatus) {
	e.mu.Lock()
	defer e.mu.Unlock()

	status := classifyExecError(err)
	if status == HostTmuxMissing {
		status = notFoundStatus
	}
	if status != HostOK {
		e.recordFailureLocked(status, err)
		return
	}
	e.markVerifiedLocked()
}

// HostState returns the last known reachability state for this host.
func (e *RemoteExecutor) HostState() HostState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

// ResetBackoff clears any retry delay so the next call reconnects immediately.
// An explicit user refresh should always be allowed to retry.
func (e *RemoteExecutor) ResetBackoff() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.state.RetryAt = time.Time{}
	e.backoff = 0
}

// controlMasterAlive reports whether a live ControlMaster is listening on path.
func controlMasterAlive(path, host string, port int) bool {
	if path == "" || !socketExists(path) {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), controlCheckTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ssh",
		"-o", "ControlPath="+path,
		"-O", "check",
		"-p", strconv.Itoa(port),
		host,
	)
	return cmd.Run() == nil
}

// startControlMaster opens a backgrounded ControlMaster. `-f` returns once the
// connection is authenticated, so no socket polling is needed.
func (e *RemoteExecutor) startControlMaster() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultSSHTimeout)
	defer cancel()

	args := []string{
		"-f", // Background after authentication
		"-N", // No remote command
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + e.controlPath,
		"-o", "ControlPersist=" + controlPersist(),
		"-o", "StrictHostKeyChecking=accept-new",
	}
	args = append(args, connectionOpts()...)
	args = append(args, "-p", strconv.Itoa(e.Port), e.Host)

	out, err := exec.CommandContext(ctx, "ssh", args...).CombinedOutput()
	if err == nil {
		return nil
	}

	// Another atmux process may have won the race and created the master
	// between our check and our start; that is a success, not a failure.
	if controlMasterAlive(e.controlPath, e.Host, e.Port) {
		return nil
	}

	if detail := strings.TrimSpace(string(out)); detail != "" {
		return fmt.Errorf("SSH ControlMaster to %s failed: %w: %s", e.Host, err, detail)
	}
	return fmt.Errorf("SSH ControlMaster to %s failed: %w", e.Host, err)
}

// removeStaleSocket deletes a control socket that no longer has a live master.
func removeStaleSocket(path string) {
	if path == "" || !socketExists(path) {
		return
	}
	os.Remove(path) //nolint:errcheck
}

// sshArgs returns the common SSH arguments including ControlPath.
func (e *RemoteExecutor) sshArgs() []string {
	args := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=" + controlPersist(),
		"-o", "StrictHostKeyChecking=accept-new",
	}
	args = append(args, connectionOpts()...)
	args = append(args, "-p", strconv.Itoa(e.Port))

	e.mu.Lock()
	path := e.controlPath
	e.mu.Unlock()
	if path != "" {
		args = append(args, "-o", "ControlPath="+path)
	}
	return args
}

// shellQuote wraps s in single quotes for safe passage through a remote shell.
// Interior single quotes are escaped as '\” (end-quote, literal quote, re-open).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// remoteCommand builds a single shell-safe command string for SSH.
// SSH concatenates all args after the host and passes them to the remote shell,
// so each argument must be individually quoted to preserve spaces and special chars.
func remoteCommand(command string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, command)
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// execSSH runs one already-rendered command string on the host.
func (e *RemoteExecutor) execSSH(command string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultSSHTimeout)
	defer cancel()

	sshArgs := e.sshArgs()
	sshArgs = append(sshArgs, e.Host, command)

	return exec.CommandContext(ctx, "ssh", sshArgs...).Output()
}

// runRemote executes a command on the host and records what the outcome says
// about its reachability. notFoundStatus is the status to record when the
// command is genuinely missing: a missing tmux makes the host unusable, but a
// missing probe binary says nothing about the host's health.
//
// A "command not found" is retried through a login shell before it is believed.
// SSH runs a non-interactive, non-login shell, whose PATH is frequently just
// the system defaults — a Mac with Homebrew or /usr/local/bin reports tmux
// missing while it is installed and running sessions.
func (e *RemoteExecutor) runRemote(command string, args []string, notFoundStatus HostStatus) ([]byte, error) {
	if err := e.ensureControlMaster(); err != nil {
		return nil, err
	}

	mode := e.shellMode()
	out, err := e.execSSH(wrapRemoteCommand(mode, command, args))

	if err == nil {
		if mode == remoteShellUnknown {
			e.setShellMode(remoteShellDirect)
		}
		e.recordOutcome(nil, notFoundStatus)
		return out, nil
	}

	if mode == remoteShellUnknown && classifyExecError(err) == HostTmuxMissing {
		if retried, rerr := e.execSSH(wrapRemoteCommand(remoteShellLogin, command, args)); rerr == nil {
			e.setShellMode(remoteShellLogin)
			e.recordOutcome(nil, notFoundStatus)
			return retried, nil
		}
	}

	e.recordOutcome(err, notFoundStatus)
	return out, err
}

// shellMode reports how commands must be invoked on this host.
func (e *RemoteExecutor) shellMode() remoteShellMode {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.shell
}

func (e *RemoteExecutor) setShellMode(mode remoteShellMode) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.shell = mode
}

// ensureShellMode settles how commands must be invoked, probing with a cheap
// command when it is not yet known. Used before an interactive attach, which
// cannot retry once it owns the terminal.
func (e *RemoteExecutor) ensureShellMode() remoteShellMode {
	if mode := e.shellMode(); mode != remoteShellUnknown {
		return mode
	}
	e.runRemote("tmux", []string{"-V"}, HostTmuxMissing) //nolint:errcheck
	return e.shellMode()
}

func (e *RemoteExecutor) Run(args ...string) error {
	_, err := e.runRemote("tmux", args, HostTmuxMissing)
	return err
}

func (e *RemoteExecutor) Output(args ...string) ([]byte, error) {
	return e.runRemote("tmux", args, HostTmuxMissing)
}

func (e *RemoteExecutor) RunWithDir(dir string, args ...string) error {
	// Remote sessions don't use local working directories;
	// the working dir is set via tmux's -c flag in the args themselves.
	return e.Run(args...)
}

// moshAvailable checks whether the mosh binary is on PATH.
func moshAvailable() bool {
	_, err := exec.LookPath("mosh")
	return err == nil
}

func (e *RemoteExecutor) Interactive(args ...string) error {
	if e.AttachMethod == "mosh" {
		if !moshAvailable() {
			fmt.Fprintf(os.Stderr, "Warning: mosh not found on PATH. Install mosh or set attach_method=ssh in your config.\nFalling back to SSH for %s.\n", e.Host)
			return e.interactiveSSH(args...)
		}
		return e.interactiveMosh(args...)
	}
	return e.interactiveSSH(args...)
}

// buildSSHInteractiveArgs constructs the argument list for an interactive SSH
// attach. When a control socket has been resolved the attach rides the existing
// master, so attaching costs no additional handshake.
//
// shellMode decides how tmux is invoked: on a host whose non-login PATH lacks
// tmux, the attach has to go through a login shell or it fails immediately.
func (e *RemoteExecutor) buildSSHInteractiveArgs(args ...string) []string {
	sshArgs := []string{
		"-t", // Force pseudo-terminal
	}
	if e.controlPath != "" {
		sshArgs = append(sshArgs,
			"-o", "ControlMaster=auto",
			"-o", "ControlPath="+e.controlPath,
			"-o", "ControlPersist="+controlPersist(),
		)
	}
	sshArgs = append(sshArgs, "-p", strconv.Itoa(e.Port), e.Host)

	if e.shellMode() == remoteShellLogin {
		return append(sshArgs, loginShellFallback, "-lc", remoteCommand("tmux", args))
	}
	return append(sshArgs, append([]string{"tmux"}, args...)...)
}

func (e *RemoteExecutor) interactiveSSH(args ...string) error {
	// Best effort: a missing master only costs a handshake, so never block the
	// attach on control socket setup. Settling the shell mode here matters
	// more — an interactive attach cannot retry once it owns the terminal.
	e.ensureShellMode()

	sshArgs := e.buildSSHInteractiveArgs(args...)

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("interactive SSH to %s failed: %w", e.Host, err)
	}
	return nil
}

// buildMoshArgs constructs the argument list for an interactive mosh attach.
// As with SSH, a host whose non-login PATH lacks tmux needs a login shell.
func (e *RemoteExecutor) buildMoshArgs(args ...string) []string {
	moshArgs := []string{e.Host, "--"}
	if e.shellMode() == remoteShellLogin {
		moshArgs = append(moshArgs, loginShellFallback, "-lc", remoteCommand("tmux", args))
	} else {
		moshArgs = append(moshArgs, append([]string{"tmux"}, args...)...)
	}

	if e.Port != defaultSSHPort {
		moshArgs = append([]string{"--ssh=ssh -p " + strconv.Itoa(e.Port)}, moshArgs...)
	}
	return moshArgs
}

func (e *RemoteExecutor) interactiveMosh(args ...string) error {
	e.ensureShellMode()

	moshArgs := e.buildMoshArgs(args...)

	cmd := exec.Command("mosh", moshArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mosh connection to %s failed: %w", e.Host, err)
	}
	return nil
}

func (e *RemoteExecutor) RunGeneric(command string, args ...string) ([]byte, error) {
	// A missing probe binary is not a sign of an unhealthy host.
	return e.runRemote(command, args, HostOK)
}

// socketExists checks whether a Unix socket file exists at the given path.
func socketExists(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.Mode().Type()&os.ModeSocket != 0
}

func (e *RemoteExecutor) HostLabel() string {
	return e.Alias
}

func (e *RemoteExecutor) IsRemote() bool {
	return true
}

// Close is deliberately a no-op. The ControlMaster socket is shared across
// atmux invocations so that a second invocation reuses the existing connection;
// tearing it down at process exit would defeat that. Use Disconnect for
// explicit teardown.
func (e *RemoteExecutor) Close() error {
	return nil
}

// Disconnect closes this host's shared ControlMaster, ending connection reuse
// until the next connect. Safe to call when no master is running.
func (e *RemoteExecutor) Disconnect() error {
	path := e.controlPath
	if path == "" {
		resolved, err := controlSocketPath(e.Host, e.Port)
		if err != nil {
			return err
		}
		path = resolved
	}

	if !socketExists(path) {
		return nil
	}

	args := []string{
		"-o", "ControlPath=" + path,
		"-O", "exit",
		"-p", strconv.Itoa(e.Port),
		e.Host,
	}
	out, err := exec.Command("ssh", args...).CombinedOutput()

	// ssh removes the socket itself on a clean exit; clear it either way so a
	// failed teardown does not leave a stale socket behind.
	os.Remove(path) //nolint:errcheck

	if err != nil {
		if detail := strings.TrimSpace(string(out)); detail != "" {
			return fmt.Errorf("failed to close SSH ControlMaster to %s: %w: %s", e.Host, err, detail)
		}
		return fmt.Errorf("failed to close SSH ControlMaster to %s: %w", e.Host, err)
	}
	return nil
}

// ControlSocketActive reports whether a live ControlMaster is available for
// this host without opening one.
func (e *RemoteExecutor) ControlSocketActive() bool {
	path := e.controlPath
	if path == "" {
		resolved, err := controlSocketPath(e.Host, e.Port)
		if err != nil {
			return false
		}
		path = resolved
	}
	return controlMasterAlive(path, e.Host, e.Port)
}

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
)

// RemoteExecutor runs tmux commands on a remote host via SSH.
// It uses SSH ControlMaster for connection pooling.
type RemoteExecutor struct {
	Host           string // user@host or SSH config alias
	Port           int    // SSH port (default 22)
	AttachMethod   string // "ssh" or "mosh"
	Alias          string // Display alias (e.g., "devbox")
	AttachStrategy string // Per-host override: "auto", "replace", or "new-window" (empty = use global)

	controlPath string    // ControlMaster socket path (stable across processes)
	controlOnce sync.Once // Ensures ControlMaster is resolved at most once per process
	controlErr  error     // Error from ControlMaster setup
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
	e.controlOnce.Do(func() {
		path, err := controlSocketPath(e.Host, e.Port)
		if err != nil {
			e.controlErr = err
			return
		}
		e.controlPath = path

		// Reusing a live master is the entire point of the stable path.
		if e.controlMasterAlive() {
			return
		}

		// A socket file that fails `-O check` is stale: the remote rebooted or
		// the master was killed. SSH will not clear it on its own, and leaving
		// it in place makes every later connection fail.
		removeStaleSocket(e.controlPath)

		e.controlErr = e.startControlMaster()
	})
	return e.controlErr
}

// controlMasterAlive reports whether a usable ControlMaster is already
// listening on this host's socket.
func (e *RemoteExecutor) controlMasterAlive() bool {
	return controlMasterAlive(e.controlPath, e.Host, e.Port)
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
		"-p", strconv.Itoa(e.Port),
		e.Host,
	}

	out, err := exec.CommandContext(ctx, "ssh", args...).CombinedOutput()
	if err == nil {
		return nil
	}

	// Another atmux process may have won the race and created the master
	// between our check and our start; that is a success, not a failure.
	if e.controlMasterAlive() {
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
		"-p", strconv.Itoa(e.Port),
	}
	if e.controlPath != "" {
		args = append(args, "-o", "ControlPath="+e.controlPath)
	}
	return args
}

// shellQuote wraps s in single quotes for safe passage through a remote shell.
// Interior single quotes are escaped as '\'' (end-quote, literal quote, re-open).
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

func (e *RemoteExecutor) Run(args ...string) error {
	if err := e.ensureControlMaster(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultSSHTimeout)
	defer cancel()

	sshArgs := e.sshArgs()
	sshArgs = append(sshArgs, e.Host, remoteCommand("tmux", args))

	return exec.CommandContext(ctx, "ssh", sshArgs...).Run()
}

func (e *RemoteExecutor) Output(args ...string) ([]byte, error) {
	if err := e.ensureControlMaster(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultSSHTimeout)
	defer cancel()

	sshArgs := e.sshArgs()
	sshArgs = append(sshArgs, e.Host, remoteCommand("tmux", args))

	return exec.CommandContext(ctx, "ssh", sshArgs...).Output()
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
	sshArgs = append(sshArgs,
		"-p", strconv.Itoa(e.Port),
		e.Host,
		"tmux",
	)
	sshArgs = append(sshArgs, args...)
	return sshArgs
}

func (e *RemoteExecutor) interactiveSSH(args ...string) error {
	// Best effort: a missing master only costs a handshake, so never block the
	// attach on control socket setup.
	e.ensureControlMaster() //nolint:errcheck

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
func (e *RemoteExecutor) buildMoshArgs(args ...string) []string {
	moshArgs := []string{e.Host, "--", "tmux"}
	moshArgs = append(moshArgs, args...)

	if e.Port != defaultSSHPort {
		moshArgs = append([]string{"--ssh=ssh -p " + strconv.Itoa(e.Port)}, moshArgs...)
	}
	return moshArgs
}

func (e *RemoteExecutor) interactiveMosh(args ...string) error {
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
	if err := e.ensureControlMaster(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultSSHTimeout)
	defer cancel()

	sshArgs := e.sshArgs()
	sshArgs = append(sshArgs, e.Host, remoteCommand(command, args))

	return exec.CommandContext(ctx, "ssh", sshArgs...).Output()
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

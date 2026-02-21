package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	defaultSSHPort    = 22
	defaultSSHTimeout = 10 * time.Second
)

// RemoteExecutor runs tmux commands on a remote host via SSH.
// It uses SSH ControlMaster for connection pooling.
type RemoteExecutor struct {
	Host           string // user@host or SSH config alias
	Port           int    // SSH port (default 22)
	AttachMethod   string // "ssh" or "mosh"
	Alias          string // Display alias (e.g., "devbox")
	AttachStrategy string // Per-host override: "auto", "replace", or "new-window" (empty = use global)

	controlPath string    // ControlMaster socket path
	controlOnce sync.Once // Ensures ControlMaster is started at most once
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

// ensureControlMaster lazily starts an SSH ControlMaster connection.
func (e *RemoteExecutor) ensureControlMaster() error {
	e.controlOnce.Do(func() {
		// Create a temp directory for the socket
		dir, err := os.MkdirTemp("", "atmux-ssh-*")
		if err != nil {
			e.controlErr = fmt.Errorf("failed to create temp dir for SSH socket: %w", err)
			return
		}
		e.controlPath = filepath.Join(dir, "ctrl-%C")

		ctx, cancel := context.WithTimeout(context.Background(), defaultSSHTimeout)
		defer cancel()

		args := []string{
			"-o", "ControlMaster=auto",
			"-o", "ControlPath=" + e.controlPath,
			"-o", "ControlPersist=300", // Keep alive for 5 minutes
			"-o", "StrictHostKeyChecking=accept-new",
			"-p", strconv.Itoa(e.Port),
			"-N", // No remote command
			e.Host,
		}

		cmd := exec.CommandContext(ctx, "ssh", args...)
		if err := cmd.Start(); err != nil {
			e.controlErr = fmt.Errorf("failed to start SSH ControlMaster to %s: %w", e.Host, err)
			return
		}

		// Wait briefly for the control socket to appear, then let it run in background
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()

		// Give it a moment to establish
		select {
		case err := <-done:
			// Process exited immediately, which is expected with -N and ControlPersist
			if err != nil {
				e.controlErr = fmt.Errorf("SSH ControlMaster to %s failed: %w", e.Host, err)
			}
		case <-time.After(2 * time.Second):
			// Still running, which is fine — it's in the background
		}
	})
	return e.controlErr
}

// sshArgs returns the common SSH arguments including ControlPath.
func (e *RemoteExecutor) sshArgs() []string {
	args := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=300",
		"-o", "StrictHostKeyChecking=accept-new",
		"-p", strconv.Itoa(e.Port),
	}
	if e.controlPath != "" {
		args = append(args, "-o", "ControlPath="+e.controlPath)
	}
	return args
}

func (e *RemoteExecutor) Run(args ...string) error {
	if err := e.ensureControlMaster(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultSSHTimeout)
	defer cancel()

	sshArgs := e.sshArgs()
	sshArgs = append(sshArgs, e.Host, "tmux")
	sshArgs = append(sshArgs, args...)

	return exec.CommandContext(ctx, "ssh", sshArgs...).Run()
}

func (e *RemoteExecutor) Output(args ...string) ([]byte, error) {
	if err := e.ensureControlMaster(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultSSHTimeout)
	defer cancel()

	sshArgs := e.sshArgs()
	sshArgs = append(sshArgs, e.Host, "tmux")
	sshArgs = append(sshArgs, args...)

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

// buildSSHInteractiveArgs constructs the argument list for an interactive SSH attach.
func (e *RemoteExecutor) buildSSHInteractiveArgs(args ...string) []string {
	sshArgs := []string{
		"-t", // Force pseudo-terminal
		"-p", strconv.Itoa(e.Port),
		e.Host,
		"tmux",
	}
	sshArgs = append(sshArgs, args...)
	return sshArgs
}

func (e *RemoteExecutor) interactiveSSH(args ...string) error {
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
	sshArgs = append(sshArgs, e.Host, command)
	sshArgs = append(sshArgs, args...)

	return exec.CommandContext(ctx, "ssh", sshArgs...).Output()
}

func (e *RemoteExecutor) HostLabel() string {
	return e.Alias
}

func (e *RemoteExecutor) IsRemote() bool {
	return true
}

func (e *RemoteExecutor) Close() error {
	if e.controlPath == "" {
		return nil
	}

	// Send exit command to ControlMaster
	args := []string{
		"-o", "ControlPath=" + e.controlPath,
		"-O", "exit",
		e.Host,
	}
	exec.Command("ssh", args...).Run() //nolint:errcheck

	// Clean up socket directory
	dir := filepath.Dir(e.controlPath)
	os.RemoveAll(dir) //nolint:errcheck

	return nil
}

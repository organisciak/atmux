package tmux

import (
	"os"
	"os/exec"
)

// TmuxExecutor abstracts how tmux commands are executed, allowing local
// and remote (SSH) implementations.
type TmuxExecutor interface {
	// Run executes a tmux command (fire-and-forget).
	Run(args ...string) error
	// Output executes a tmux command and returns stdout.
	Output(args ...string) ([]byte, error)
	// RunWithDir executes a tmux command with a working directory set.
	RunWithDir(dir string, args ...string) error
	// Interactive runs a tmux command attached to the user's terminal.
	Interactive(args ...string) error
	// RunGeneric executes a non-tmux command (e.g., ps) and returns stdout.
	RunGeneric(command string, args ...string) ([]byte, error)
	// HostLabel returns a display label for this executor ("" for local).
	HostLabel() string
	// IsRemote returns true if this executor targets a remote host.
	IsRemote() bool
	// Close releases any resources (e.g., SSH ControlMaster sockets).
	Close() error
}

// LocalExecutor runs tmux commands on the local machine.
type LocalExecutor struct{}

// NewLocalExecutor creates a new LocalExecutor.
func NewLocalExecutor() *LocalExecutor {
	return &LocalExecutor{}
}

func (e *LocalExecutor) Run(args ...string) error {
	return exec.Command("tmux", args...).Run()
}

func (e *LocalExecutor) Output(args ...string) ([]byte, error) {
	return exec.Command("tmux", args...).Output()
}

func (e *LocalExecutor) RunWithDir(dir string, args ...string) error {
	cmd := exec.Command("tmux", args...)
	cmd.Dir = dir
	return cmd.Run()
}

func (e *LocalExecutor) Interactive(args ...string) error {
	cmd := exec.Command("tmux", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (e *LocalExecutor) RunGeneric(command string, args ...string) ([]byte, error) {
	return exec.Command(command, args...).Output()
}

func (e *LocalExecutor) HostLabel() string {
	return ""
}

func (e *LocalExecutor) IsRemote() bool {
	return false
}

func (e *LocalExecutor) Close() error {
	return nil
}

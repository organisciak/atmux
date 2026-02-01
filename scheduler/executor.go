package scheduler

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/porganisciak/agent-tmux/tmux"
)

const (
	preActionDelay = 2 * time.Second
)

// ExecutionResult contains the result of executing a job
type ExecutionResult struct {
	JobID   string
	Success bool
	Error   error
}

// ExecuteJob executes a single scheduled job
func ExecuteJob(job *ScheduledJob) error {
	// Validate target exists
	if err := ValidateTarget(job.Target); err != nil {
		return fmt.Errorf("target validation failed: %w", err)
	}

	// Execute pre-action if specified
	if job.PreAction != PreActionNone {
		preCmd := ""
		switch job.PreAction {
		case PreActionNew:
			preCmd = "/new"
		case PreActionCompact:
			preCmd = "/compact"
		}

		if preCmd != "" {
			if err := tmux.SendCommand(job.Target, preCmd); err != nil {
				return fmt.Errorf("pre-action failed: %w", err)
			}
			time.Sleep(preActionDelay)
		}
	}

	// Send main command
	if err := tmux.SendCommand(job.Target, job.Command); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}

	return nil
}

// ExecutePending executes all pending jobs
func ExecutePending() ([]ExecutionResult, error) {
	store, err := Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load schedules: %w", err)
	}

	pending := store.PendingJobs()
	results := make([]ExecutionResult, 0, len(pending))

	for _, job := range pending {
		result := ExecutionResult{
			JobID:   job.ID,
			Success: true,
		}

		// Execute the job
		if err := ExecuteJob(&job); err != nil {
			result.Success = false
			result.Error = err
			job.LastError = err.Error()
		} else {
			job.LastError = ""
		}

		// Update job state
		job.LastRun = time.Now()
		nextRun, err := NextRunAfter(job.Schedule, job.LastRun)
		if err != nil {
			// If we can't calculate next run, disable the job
			job.Enabled = false
			job.LastError = fmt.Sprintf("failed to calculate next run: %v", err)
		} else {
			job.NextRun = nextRun
		}

		// Save updated job
		if err := store.Update(job); err != nil {
			// Log but don't fail the batch
			result.Error = fmt.Errorf("failed to update job state: %w", err)
		}

		results = append(results, result)
	}

	// Save the store
	if err := store.Save(); err != nil {
		return results, fmt.Errorf("failed to save schedules: %w", err)
	}

	return results, nil
}

// ValidateTarget checks if a tmux target (session:window.pane) exists
func ValidateTarget(target string) error {
	// Parse the target
	parts := strings.Split(target, ":")
	if len(parts) < 1 {
		return fmt.Errorf("invalid target format: %s", target)
	}

	sessionName := parts[0]

	// Check if session exists
	cmd := exec.Command("tmux", "has-session", "-t", sessionName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("session does not exist: %s", sessionName)
	}

	// If there's a window.pane part, validate that too
	if len(parts) > 1 {
		// Try to get pane info
		cmd := exec.Command("tmux", "display-message", "-t", target, "-p", "#{pane_id}")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("target pane does not exist: %s", target)
		}
	}

	return nil
}

// TargetExists returns true if the target exists (non-error version of ValidateTarget)
func TargetExists(target string) bool {
	return ValidateTarget(target) == nil
}

// ListAvailableTargets returns all available tmux targets
func ListAvailableTargets() ([]string, error) {
	tree, err := tmux.FetchTree()
	if err != nil {
		return nil, err
	}

	var targets []string
	for _, sess := range tree.Sessions {
		for _, win := range sess.Windows {
			for _, pane := range win.Panes {
				targets = append(targets, pane.Target)
			}
		}
	}

	return targets, nil
}

// RunDaemon runs a daemon that checks for pending jobs at regular intervals
func RunDaemon(interval time.Duration, stop <-chan struct{}) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start
	results, err := ExecutePending()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing pending jobs: %v\n", err)
	}
	for _, r := range results {
		if r.Error != nil {
			fmt.Fprintf(os.Stderr, "Job %s failed: %v\n", r.JobID, r.Error)
		}
	}

	for {
		select {
		case <-stop:
			return nil
		case <-ticker.C:
			results, err := ExecutePending()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error executing pending jobs: %v\n", err)
				continue
			}
			for _, r := range results {
				if r.Error != nil {
					fmt.Fprintf(os.Stderr, "Job %s failed: %v\n", r.JobID, r.Error)
				}
			}
		}
	}
}

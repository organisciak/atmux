package scheduler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/porganisciak/agent-tmux/config"
)

// PreAction represents an action to perform before the main command
type PreAction string

const (
	PreActionNone    PreAction = "none"
	PreActionNew     PreAction = "new"     // Send /new, wait 2s, then command
	PreActionCompact PreAction = "compact" // Send /compact, wait 2s, then command
)

// ScheduledJob represents a scheduled command to send to a tmux pane
type ScheduledJob struct {
	ID        string    `json:"id"`
	Schedule  string    `json:"schedule"`     // Cron expression
	Target    string    `json:"target"`       // session:window.pane
	Command   string    `json:"command"`
	PreAction PreAction `json:"pre_action"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	LastRun   time.Time `json:"last_run,omitempty"`
	NextRun   time.Time `json:"next_run"`
	LastError string    `json:"last_error,omitempty"`
}

// ScheduleStore manages the collection of scheduled jobs
type ScheduleStore struct {
	Jobs []ScheduledJob `json:"jobs"`
	mu   sync.RWMutex
}

// SchedulePath returns the path to the schedules JSON file
func SchedulePath() (string, error) {
	dir, err := config.SettingsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "schedules.json"), nil
}

// Load loads the schedule store from disk
func Load() (*ScheduleStore, error) {
	path, err := SchedulePath()
	if err != nil {
		return nil, err
	}

	store := &ScheduleStore{
		Jobs: []ScheduledJob{},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, store); err != nil {
		return nil, err
	}

	return store, nil
}

// Save writes the schedule store to disk
func (s *ScheduleStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path, err := SchedulePath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Add adds a new job to the store
func (s *ScheduleStore) Add(job ScheduledJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Calculate next run time
	nextRun, err := NextRun(job.Schedule)
	if err != nil {
		return err
	}
	job.NextRun = nextRun

	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}

	if job.ID == "" {
		job.ID = GenerateID()
	}

	s.Jobs = append(s.Jobs, job)
	return nil
}

// Remove removes a job by ID
func (s *ScheduleStore) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, job := range s.Jobs {
		if job.ID == id {
			s.Jobs = append(s.Jobs[:i], s.Jobs[i+1:]...)
			return nil
		}
	}
	return &NotFoundError{ID: id}
}

// GetByID returns a job by its ID
func (s *ScheduleStore) GetByID(id string) (*ScheduledJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range s.Jobs {
		if s.Jobs[i].ID == id {
			return &s.Jobs[i], nil
		}
	}
	return nil, &NotFoundError{ID: id}
}

// Update updates an existing job
func (s *ScheduleStore) Update(job ScheduledJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.Jobs {
		if s.Jobs[i].ID == job.ID {
			s.Jobs[i] = job
			return nil
		}
	}
	return &NotFoundError{ID: job.ID}
}

// PendingJobs returns all jobs where NextRun <= now and Enabled is true
func (s *ScheduleStore) PendingJobs() []ScheduledJob {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	var pending []ScheduledJob

	for _, job := range s.Jobs {
		if job.Enabled && !job.NextRun.IsZero() && job.NextRun.Before(now) {
			pending = append(pending, job)
		}
	}

	return pending
}

// EnabledJobs returns all enabled jobs
func (s *ScheduleStore) EnabledJobs() []ScheduledJob {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var enabled []ScheduledJob
	for _, job := range s.Jobs {
		if job.Enabled {
			enabled = append(enabled, job)
		}
	}
	return enabled
}

// NotFoundError is returned when a job is not found
type NotFoundError struct {
	ID string
}

func (e *NotFoundError) Error() string {
	return "job not found: " + e.ID
}

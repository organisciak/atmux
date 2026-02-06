package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PreAction defines what happens before sending a scheduled command
type PreAction string

const (
	PreActionNone       PreAction = "none"
	PreActionCompact    PreAction = "compact"
	PreActionNewSession PreAction = "new_session"
)

// ScheduledJob represents a scheduled command
type ScheduledJob struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`      // Optional friendly name
	CronExpr  string    `json:"cron_expr"` // 5-field cron expression
	Target    string    `json:"target"`    // Tmux target (session:window.pane)
	Command   string    `json:"command"`   // Command to send
	PreAction PreAction `json:"pre_action"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	LastRunAt time.Time `json:"last_run_at,omitempty"`
}

// Schedule represents the schedule configuration
type Schedule struct {
	Jobs    []ScheduledJob `json:"jobs"`
	Version int            `json:"version"`
}

const scheduleFileName = "schedule.json"
const scheduleVersion = 1

// SchedulePath returns the path to the schedule file
func SchedulePath() (string, error) {
	dir, err := SettingsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, scheduleFileName), nil
}

// LoadSchedule loads the schedule from disk
func LoadSchedule() (*Schedule, error) {
	path, err := SchedulePath()
	if err != nil {
		return &Schedule{Version: scheduleVersion}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Schedule{Version: scheduleVersion}, nil
		}
		return &Schedule{Version: scheduleVersion}, err
	}

	var schedule Schedule
	if err := json.Unmarshal(data, &schedule); err != nil {
		return &Schedule{Version: scheduleVersion}, err
	}

	return &schedule, nil
}

// Save writes the schedule to disk
func (s *Schedule) Save() error {
	dir, err := SettingsDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path, err := SchedulePath()
	if err != nil {
		return err
	}

	s.Version = scheduleVersion
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// AddJob adds a new job to the schedule
func (s *Schedule) AddJob(job ScheduledJob) error {
	if job.ID == "" {
		job.ID = generateJobID()
	}
	job.CreatedAt = time.Now()
	job.UpdatedAt = time.Now()
	s.Jobs = append(s.Jobs, job)
	return s.Save()
}

// UpdateJob updates an existing job
func (s *Schedule) UpdateJob(job ScheduledJob) error {
	for i, j := range s.Jobs {
		if j.ID == job.ID {
			job.UpdatedAt = time.Now()
			job.CreatedAt = j.CreatedAt
			s.Jobs[i] = job
			return s.Save()
		}
	}
	return fmt.Errorf("job not found: %s", job.ID)
}

// DeleteJob removes a job from the schedule
func (s *Schedule) DeleteJob(id string) error {
	for i, j := range s.Jobs {
		if j.ID == id {
			s.Jobs = append(s.Jobs[:i], s.Jobs[i+1:]...)
			return s.Save()
		}
	}
	return fmt.Errorf("job not found: %s", id)
}

// GetJob returns a job by ID
func (s *Schedule) GetJob(id string) (*ScheduledJob, error) {
	for i, j := range s.Jobs {
		if j.ID == id {
			return &s.Jobs[i], nil
		}
	}
	return nil, fmt.Errorf("job not found: %s", id)
}

// ToggleJob toggles the enabled state of a job
func (s *Schedule) ToggleJob(id string) error {
	for i, j := range s.Jobs {
		if j.ID == id {
			s.Jobs[i].Enabled = !s.Jobs[i].Enabled
			s.Jobs[i].UpdatedAt = time.Now()
			return s.Save()
		}
	}
	return fmt.Errorf("job not found: %s", id)
}

// EnabledJobs returns only enabled jobs
func (s *Schedule) EnabledJobs() []ScheduledJob {
	var enabled []ScheduledJob
	for _, j := range s.Jobs {
		if j.Enabled {
			enabled = append(enabled, j)
		}
	}
	return enabled
}

// SortedJobs returns jobs sorted by next run time
func (s *Schedule) SortedJobs() []ScheduledJob {
	jobs := make([]ScheduledJob, len(s.Jobs))
	copy(jobs, s.Jobs)
	sort.Slice(jobs, func(i, j int) bool {
		// Enabled jobs first
		if jobs[i].Enabled != jobs[j].Enabled {
			return jobs[i].Enabled
		}
		// Then by next run time
		nextI, _ := NextRun(jobs[i].CronExpr)
		nextJ, _ := NextRun(jobs[j].CronExpr)
		return nextI.Before(nextJ)
	})
	return jobs
}

// generateJobID creates a unique job ID
func generateJobID() string {
	return fmt.Sprintf("job_%d", time.Now().UnixNano())
}

// CronField represents a cron field with its valid range
type CronField struct {
	Name string
	Min  int
	Max  int
}

var cronFields = []CronField{
	{"minute", 0, 59},
	{"hour", 0, 23},
	{"day", 1, 31},
	{"month", 1, 12},
	{"weekday", 0, 6}, // 0=Sunday
}

// Weekday names for display
var weekdayNames = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

// Month names for display
var monthNames = []string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// CronPreset represents a common schedule preset
type CronPreset struct {
	Name        string
	Description string
	Expr        string
}

// GetCronPresets returns common schedule presets
func GetCronPresets() []CronPreset {
	return []CronPreset{
		{"Every minute", "Runs every minute", "* * * * *"},
		{"Every 5 minutes", "Runs at :00, :05, :10...", "*/5 * * * *"},
		{"Every 15 minutes", "Runs at :00, :15, :30, :45", "*/15 * * * *"},
		{"Every 30 minutes", "Runs at :00 and :30", "*/30 * * * *"},
		{"Every hour", "Runs at the top of each hour", "0 * * * *"},
		{"Every 2 hours", "Runs every 2 hours at :00", "0 */2 * * *"},
		{"Every 4 hours", "Runs every 4 hours at :00", "0 */4 * * *"},
		{"Daily at midnight", "Runs at 00:00", "0 0 * * *"},
		{"Daily at 9am", "Runs at 09:00", "0 9 * * *"},
		{"Weekdays at 9am", "Mon-Fri at 09:00", "0 9 * * 1-5"},
		{"Weekly on Sunday", "Runs Sunday at 00:00", "0 0 * * 0"},
		{"Custom", "Enter custom cron expression", ""},
	}
}

// ParseCron validates and parses a cron expression
// Returns an error if the expression is invalid
func ParseCron(expr string) error {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Errorf("cron expression must have 5 fields, got %d", len(fields))
	}

	for i, field := range fields {
		if err := validateCronField(field, cronFields[i]); err != nil {
			return fmt.Errorf("invalid %s field: %w", cronFields[i].Name, err)
		}
	}

	return nil
}

// validateCronField validates a single cron field
func validateCronField(value string, field CronField) error {
	// Handle wildcards
	if value == "*" {
		return nil
	}

	// Handle step values (*/5, 1-10/2)
	if strings.Contains(value, "/") {
		parts := strings.SplitN(value, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid step format")
		}
		step, err := strconv.Atoi(parts[1])
		if err != nil || step < 1 {
			return fmt.Errorf("invalid step value: %s", parts[1])
		}
		if parts[0] != "*" {
			return validateCronField(parts[0], field)
		}
		return nil
	}

	// Handle ranges (1-5)
	if strings.Contains(value, "-") {
		parts := strings.SplitN(value, "-", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid range format")
		}
		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid range start: %s", parts[0])
		}
		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("invalid range end: %s", parts[1])
		}
		if start < field.Min || start > field.Max {
			return fmt.Errorf("range start %d out of bounds (%d-%d)", start, field.Min, field.Max)
		}
		if end < field.Min || end > field.Max {
			return fmt.Errorf("range end %d out of bounds (%d-%d)", end, field.Min, field.Max)
		}
		if start > end {
			return fmt.Errorf("range start %d greater than end %d", start, end)
		}
		return nil
	}

	// Handle lists (1,2,3)
	if strings.Contains(value, ",") {
		parts := strings.Split(value, ",")
		for _, part := range parts {
			if err := validateCronField(strings.TrimSpace(part), field); err != nil {
				return err
			}
		}
		return nil
	}

	// Simple number
	num, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("invalid number: %s", value)
	}
	if num < field.Min || num > field.Max {
		return fmt.Errorf("value %d out of bounds (%d-%d)", num, field.Min, field.Max)
	}

	return nil
}

// CronToEnglish converts a cron expression to human-readable format
func CronToEnglish(expr string) string {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return expr
	}

	minute, hour, day, month, weekday := fields[0], fields[1], fields[2], fields[3], fields[4]

	// Check for common patterns
	switch expr {
	case "* * * * *":
		return "Every minute"
	case "*/5 * * * *":
		return "Every 5 minutes"
	case "*/10 * * * *":
		return "Every 10 minutes"
	case "*/15 * * * *":
		return "Every 15 minutes"
	case "*/30 * * * *":
		return "Every 30 minutes"
	case "0 * * * *":
		return "Every hour"
	case "0 */2 * * *":
		return "Every 2 hours"
	case "0 */4 * * *":
		return "Every 4 hours"
	case "0 */6 * * *":
		return "Every 6 hours"
	case "0 0 * * *":
		return "Daily at midnight"
	case "0 0 * * 0":
		return "Weekly on Sunday at midnight"
	case "0 0 * * 1":
		return "Weekly on Monday at midnight"
	case "0 0 1 * *":
		return "Monthly on the 1st at midnight"
	}

	// Build description
	var parts []string

	// Time component
	if minute == "*" && hour == "*" {
		parts = append(parts, "Every minute")
	} else if strings.HasPrefix(minute, "*/") {
		interval := strings.TrimPrefix(minute, "*/")
		parts = append(parts, fmt.Sprintf("Every %s minutes", interval))
	} else if minute != "*" && hour == "*" {
		parts = append(parts, fmt.Sprintf("At :%s every hour", padZero(minute)))
	} else if minute != "*" && hour != "*" {
		parts = append(parts, fmt.Sprintf("At %s:%s", padZero(hour), padZero(minute)))
	} else if hour != "*" {
		if strings.HasPrefix(hour, "*/") {
			interval := strings.TrimPrefix(hour, "*/")
			parts = append(parts, fmt.Sprintf("Every %s hours", interval))
		} else {
			parts = append(parts, fmt.Sprintf("At hour %s", hour))
		}
	}

	// Day/weekday component
	if day != "*" && weekday == "*" {
		parts = append(parts, fmt.Sprintf("on day %s", day))
	} else if weekday != "*" && day == "*" {
		parts = append(parts, fmt.Sprintf("on %s", formatWeekdays(weekday)))
	}

	// Month component
	if month != "*" {
		parts = append(parts, fmt.Sprintf("in %s", formatMonths(month)))
	}

	if len(parts) == 0 {
		return expr
	}

	return strings.Join(parts, " ")
}

func padZero(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

func formatWeekdays(value string) string {
	if strings.Contains(value, "-") {
		parts := strings.SplitN(value, "-", 2)
		start, _ := strconv.Atoi(parts[0])
		end, _ := strconv.Atoi(parts[1])
		if start >= 0 && start <= 6 && end >= 0 && end <= 6 {
			return weekdayNames[start] + "-" + weekdayNames[end]
		}
	}
	if strings.Contains(value, ",") {
		var names []string
		for _, part := range strings.Split(value, ",") {
			idx, _ := strconv.Atoi(strings.TrimSpace(part))
			if idx >= 0 && idx <= 6 {
				names = append(names, weekdayNames[idx])
			}
		}
		return strings.Join(names, ", ")
	}
	idx, _ := strconv.Atoi(value)
	if idx >= 0 && idx <= 6 {
		return weekdayNames[idx]
	}
	return value
}

func formatMonths(value string) string {
	if strings.Contains(value, "-") {
		parts := strings.SplitN(value, "-", 2)
		start, _ := strconv.Atoi(parts[0])
		end, _ := strconv.Atoi(parts[1])
		if start >= 1 && start <= 12 && end >= 1 && end <= 12 {
			return monthNames[start] + "-" + monthNames[end]
		}
	}
	if strings.Contains(value, ",") {
		var names []string
		for _, part := range strings.Split(value, ",") {
			idx, _ := strconv.Atoi(strings.TrimSpace(part))
			if idx >= 1 && idx <= 12 {
				names = append(names, monthNames[idx])
			}
		}
		return strings.Join(names, ", ")
	}
	idx, _ := strconv.Atoi(value)
	if idx >= 1 && idx <= 12 {
		return monthNames[idx]
	}
	return value
}

// NextRun calculates the next run time from now for a cron expression
func NextRun(expr string) (time.Time, error) {
	return NextRunFrom(expr, time.Now())
}

// NextRunFrom calculates the next run time from a given time
func NextRunFrom(expr string, from time.Time) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("invalid cron expression")
	}

	// Start from the next minute
	next := from.Truncate(time.Minute).Add(time.Minute)

	// Try for up to 4 years to find a matching time
	endSearch := next.AddDate(4, 0, 0)

	for next.Before(endSearch) {
		if matchesCron(next, fields) {
			return next, nil
		}
		next = next.Add(time.Minute)
	}

	return time.Time{}, fmt.Errorf("no matching time found within 4 years")
}

// matchesCron checks if a time matches a cron expression
func matchesCron(t time.Time, fields []string) bool {
	minute, hour, day, month, weekday := fields[0], fields[1], fields[2], fields[3], fields[4]

	return matchField(t.Minute(), minute, 0, 59) &&
		matchField(t.Hour(), hour, 0, 23) &&
		matchField(t.Day(), day, 1, 31) &&
		matchField(int(t.Month()), month, 1, 12) &&
		matchField(int(t.Weekday()), weekday, 0, 6)
}

// matchField checks if a value matches a cron field pattern
func matchField(value int, pattern string, min, max int) bool {
	if pattern == "*" {
		return true
	}

	// Handle step values
	if strings.Contains(pattern, "/") {
		parts := strings.SplitN(pattern, "/", 2)
		step, _ := strconv.Atoi(parts[1])
		if step <= 0 {
			return false
		}
		if parts[0] == "*" {
			return value%step == 0
		}
		// Range with step
		start := min
		if strings.Contains(parts[0], "-") {
			rangeParts := strings.SplitN(parts[0], "-", 2)
			start, _ = strconv.Atoi(rangeParts[0])
		}
		return value >= start && (value-start)%step == 0
	}

	// Handle ranges
	if strings.Contains(pattern, "-") {
		parts := strings.SplitN(pattern, "-", 2)
		start, _ := strconv.Atoi(parts[0])
		end, _ := strconv.Atoi(parts[1])
		return value >= start && value <= end
	}

	// Handle lists
	if strings.Contains(pattern, ",") {
		for _, part := range strings.Split(pattern, ",") {
			partVal, _ := strconv.Atoi(strings.TrimSpace(part))
			if value == partVal {
				return true
			}
		}
		return false
	}

	// Simple number
	num, _ := strconv.Atoi(pattern)
	return value == num
}

// FormatNextRun formats the next run time relative to now
func FormatNextRun(expr string) string {
	next, err := NextRun(expr)
	if err != nil {
		return "invalid"
	}

	now := time.Now()
	diff := next.Sub(now)

	switch {
	case diff < time.Minute:
		return "< 1 min"
	case diff < time.Hour:
		return fmt.Sprintf("in %d min", int(diff.Minutes()))
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		mins := int(diff.Minutes()) % 60
		if mins > 0 {
			return fmt.Sprintf("in %dh %dm", hours, mins)
		}
		return fmt.Sprintf("in %dh", hours)
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("in %d days", int(diff.Hours()/24))
	default:
		return next.Format("Jan 2")
	}
}

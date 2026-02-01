package scheduler

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// cronParser is a shared parser that supports standard cron expressions
// Uses 5-field format: minute hour day-of-month month day-of-week
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// ParseSchedule validates a cron expression and returns the next run time
func ParseSchedule(schedule string) (time.Time, error) {
	sched, err := cronParser.Parse(schedule)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	return sched.Next(time.Now()), nil
}

// NextRun returns the next run time for a schedule
func NextRun(schedule string) (time.Time, error) {
	return ParseSchedule(schedule)
}

// NextRunAfter returns the next run time after a given time
func NextRunAfter(schedule string, after time.Time) (time.Time, error) {
	sched, err := cronParser.Parse(schedule)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	return sched.Next(after), nil
}

// CronPreset represents a common cron schedule
type CronPreset struct {
	Label      string // Human-readable label
	Expression string // Cron expression
}

// CommonPresets returns a list of common cron presets
func CommonPresets() []CronPreset {
	return []CronPreset{
		{Label: "Every morning at 9:00 AM", Expression: "0 9 * * *"},
		{Label: "Every hour", Expression: "0 * * * *"},
		{Label: "Every 30 minutes", Expression: "*/30 * * * *"},
		{Label: "Every day at midnight", Expression: "0 0 * * *"},
		{Label: "Every Monday at 6:00 AM", Expression: "0 6 * * 1"},
		{Label: "Weekdays at 9:00 AM", Expression: "0 9 * * 1-5"},
	}
}

// CronToEnglish converts a cron expression to human-readable English
func CronToEnglish(expr string) string {
	expr = strings.TrimSpace(expr)

	// Handle descriptors first
	if strings.HasPrefix(expr, "@") {
		switch expr {
		case "@yearly", "@annually":
			return "Once a year (Jan 1 at midnight)"
		case "@monthly":
			return "Once a month (1st at midnight)"
		case "@weekly":
			return "Once a week (Sunday at midnight)"
		case "@daily", "@midnight":
			return "Every day at midnight"
		case "@hourly":
			return "Every hour"
		}
		// Handle @every
		if strings.HasPrefix(expr, "@every ") {
			duration := strings.TrimPrefix(expr, "@every ")
			return "Every " + duration
		}
		return expr
	}

	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return expr // Return as-is if not valid
	}

	minute, hour, dom, month, dow := parts[0], parts[1], parts[2], parts[3], parts[4]

	// Check for common patterns
	// Every N minutes
	if strings.HasPrefix(minute, "*/") && hour == "*" && dom == "*" && month == "*" && dow == "*" {
		interval := strings.TrimPrefix(minute, "*/")
		if interval == "1" {
			return "Every minute"
		}
		return fmt.Sprintf("Every %s minutes", interval)
	}

	// Every hour at minute X
	if hour == "*" && dom == "*" && month == "*" && dow == "*" {
		if minute == "0" {
			return "Every hour"
		}
		m, _ := strconv.Atoi(minute)
		if m > 0 {
			return fmt.Sprintf("Every hour at minute %d", m)
		}
	}

	// Daily at specific time
	if dom == "*" && month == "*" && dow == "*" {
		return fmt.Sprintf("Every day at %s", formatTime(hour, minute))
	}

	// Weekdays only
	if dom == "*" && month == "*" && (dow == "1-5" || dow == "MON-FRI") {
		return fmt.Sprintf("Weekdays at %s", formatTime(hour, minute))
	}

	// Specific day of week
	if dom == "*" && month == "*" && dow != "*" {
		dayName := dowToName(dow)
		if dayName != "" {
			return fmt.Sprintf("Every %s at %s", dayName, formatTime(hour, minute))
		}
	}

	// Monthly on specific day
	if month == "*" && dow == "*" && dom != "*" {
		return fmt.Sprintf("Day %s of every month at %s", dom, formatTime(hour, minute))
	}

	// Fallback: describe each component
	return describeCron(minute, hour, dom, month, dow)
}

// formatTime formats hour and minute into a readable time string
func formatTime(hour, minute string) string {
	h, errH := strconv.Atoi(hour)
	m, errM := strconv.Atoi(minute)

	if errH != nil || errM != nil {
		// Handle wildcards or ranges
		if hour == "*" && minute == "0" {
			return "the start of every hour"
		}
		return fmt.Sprintf("%s:%s", hour, minute)
	}

	// Convert to 12-hour format
	ampm := "AM"
	if h >= 12 {
		ampm = "PM"
	}
	if h > 12 {
		h -= 12
	}
	if h == 0 {
		h = 12
	}

	if m == 0 {
		return fmt.Sprintf("%d:00 %s", h, ampm)
	}
	return fmt.Sprintf("%d:%02d %s", h, m, ampm)
}

// dowToName converts day of week number/range to name
func dowToName(dow string) string {
	days := map[string]string{
		"0": "Sunday", "7": "Sunday",
		"1": "Monday",
		"2": "Tuesday",
		"3": "Wednesday",
		"4": "Thursday",
		"5": "Friday",
		"6": "Saturday",
	}

	if name, ok := days[dow]; ok {
		return name
	}

	// Handle named days
	dowUpper := strings.ToUpper(dow)
	namedDays := map[string]string{
		"SUN": "Sunday",
		"MON": "Monday",
		"TUE": "Tuesday",
		"WED": "Wednesday",
		"THU": "Thursday",
		"FRI": "Friday",
		"SAT": "Saturday",
	}
	if name, ok := namedDays[dowUpper]; ok {
		return name
	}

	return ""
}

// describeCron provides a generic description of cron components
func describeCron(minute, hour, dom, month, dow string) string {
	var parts []string

	if minute != "*" && minute != "0" {
		parts = append(parts, fmt.Sprintf("minute %s", minute))
	}

	if hour != "*" {
		parts = append(parts, formatTime(hour, minute))
	}

	if dom != "*" {
		parts = append(parts, fmt.Sprintf("on day %s", dom))
	}

	if month != "*" {
		parts = append(parts, fmt.Sprintf("in month %s", month))
	}

	if dow != "*" {
		dayName := dowToName(dow)
		if dayName != "" {
			parts = append(parts, fmt.Sprintf("on %s", dayName))
		} else {
			parts = append(parts, fmt.Sprintf("on weekday %s", dow))
		}
	}

	if len(parts) == 0 {
		return "Every minute"
	}

	return strings.Join(parts, ", ")
}

// ValidateCronField validates a single cron field
func ValidateCronField(field string, min, max int) error {
	if field == "*" {
		return nil
	}

	// Check for step values (*/N)
	if strings.HasPrefix(field, "*/") {
		step := strings.TrimPrefix(field, "*/")
		n, err := strconv.Atoi(step)
		if err != nil {
			return fmt.Errorf("invalid step value: %s", step)
		}
		if n < 1 || n > max {
			return fmt.Errorf("step value %d out of range (1-%d)", n, max)
		}
		return nil
	}

	// Check for ranges (N-M)
	if strings.Contains(field, "-") {
		rangeMatch := regexp.MustCompile(`^(\d+)-(\d+)$`)
		matches := rangeMatch.FindStringSubmatch(field)
		if matches == nil {
			return fmt.Errorf("invalid range: %s", field)
		}
		start, _ := strconv.Atoi(matches[1])
		end, _ := strconv.Atoi(matches[2])
		if start < min || end > max || start > end {
			return fmt.Errorf("range %s out of bounds (%d-%d)", field, min, max)
		}
		return nil
	}

	// Check for lists (N,M,...)
	if strings.Contains(field, ",") {
		for _, part := range strings.Split(field, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil {
				return fmt.Errorf("invalid list value: %s", part)
			}
			if n < min || n > max {
				return fmt.Errorf("value %d out of range (%d-%d)", n, min, max)
			}
		}
		return nil
	}

	// Single value
	n, err := strconv.Atoi(field)
	if err != nil {
		return fmt.Errorf("invalid value: %s", field)
	}
	if n < min || n > max {
		return fmt.Errorf("value %d out of range (%d-%d)", n, min, max)
	}

	return nil
}

// BuildCronExpression builds a cron expression from individual fields
func BuildCronExpression(minute, hour, dom, month, dow string) string {
	return fmt.Sprintf("%s %s %s %s %s", minute, hour, dom, month, dow)
}

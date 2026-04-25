package tui

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const liveBeadsPanelRows = 10

type liveBeadsSummary struct {
	Dir      string
	HasBeads bool
	Count    int
	Issues   []liveBeadsIssue
	Err      error
}

type liveBeadsIssue struct {
	ID           string                `json:"id"`
	Title        string                `json:"title"`
	Status       string                `json:"status"`
	Priority     int                   `json:"priority"`
	IssueType    string                `json:"issue_type"`
	Dependencies []liveBeadsDependency `json:"dependencies"`
	Ready        bool                  `json:"-"`
}

type liveBeadsDependency struct {
	DependsOnID string `json:"depends_on_id"`
}

func loadLiveBeadsSummary(dir string) liveBeadsSummary {
	summary := liveBeadsSummary{Dir: dir}
	if dir == "" {
		return summary
	}

	path := filepath.Join(dir, ".beads", "issues.jsonl")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return summary
		}
		summary.HasBeads = true
		summary.Err = err
		return summary
	}
	defer file.Close()

	summary.HasBeads = true
	var issues []liveBeadsIssue
	statusByID := map[string]string{}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var issue liveBeadsIssue
		if err := json.Unmarshal([]byte(line), &issue); err != nil {
			continue
		}
		statusByID[issue.ID] = issue.Status
		if issue.Status == "open" {
			issues = append(issues, issue)
		}
	}
	if err := scanner.Err(); err != nil {
		summary.Err = err
	}

	for i := range issues {
		issues[i].Ready = true
		for _, dep := range issues[i].Dependencies {
			if dep.DependsOnID != "" && statusByID[dep.DependsOnID] == "open" {
				issues[i].Ready = false
				break
			}
		}
	}

	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Ready != issues[j].Ready {
			return issues[i].Ready
		}
		if issues[i].Priority != issues[j].Priority {
			return issues[i].Priority < issues[j].Priority
		}
		return issues[i].ID < issues[j].ID
	})

	summary.Count = len(issues)
	if len(issues) > liveBeadsPanelRows {
		summary.Issues = issues[:liveBeadsPanelRows]
	} else {
		summary.Issues = issues
	}
	return summary
}

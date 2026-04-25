package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestLoadLiveBeadsSummarySortsReadyIssuesFirst(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.Mkdir(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{"id":"a","title":"Closed blocker","status":"closed","priority":0}
{"id":"b","title":"Blocked high","status":"open","priority":0,"dependencies":[{"depends_on_id":"c"}]}
{"id":"c","title":"Ready medium","status":"open","priority":2}
{"id":"d","title":"Ready high","status":"open","priority":1}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "issues.jsonl"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	summary := loadLiveBeadsSummary(dir)
	if !summary.HasBeads {
		t.Fatal("expected beads summary")
	}
	if summary.Count != 3 {
		t.Fatalf("expected 3 open issues, got %d", summary.Count)
	}
	got := []string{summary.Issues[0].ID, summary.Issues[1].ID, summary.Issues[2].ID}
	want := []string{"d", "c", "b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected order %v, got %v", want, got)
		}
	}
	if !summary.Issues[0].Ready || summary.Issues[2].Ready {
		t.Fatalf("unexpected ready flags: %+v", summary.Issues)
	}
}

func TestWrapStatusPartsKeepsEveryShortcut(t *testing.T) {
	parts := []string{"[q]uit", "[a]ttach", "[↑↓]nav", "[⏎]expand/focus"}
	lines := wrapStatusParts(parts, 16)
	got := strings.Join(lines, " ")
	want := strings.Join(parts, " ")
	if got != want {
		t.Fatalf("expected all shortcuts preserved %q, got %q", want, got)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > 16 {
			t.Fatalf("expected line %q to fit width 16", line)
		}
	}
	if len(lines) < 2 {
		t.Fatalf("expected wrapped output, got %v", lines)
	}
}

func TestRenderBeadsIssueLinesSplitsMetadataAndTitle(t *testing.T) {
	model := LiveModel{width: 28}
	issue := liveBeadsIssue{
		ID:       "atmux-lhh",
		Title:    "Add --detach flag to skip TTY attachment",
		Priority: 1,
		Ready:    true,
	}

	lines := model.renderBeadsIssueLines(issue)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	meta := lipgloss.NewStyle().Render(lines[0])
	title := lipgloss.NewStyle().Render(lines[1])
	if !strings.Contains(meta, "P1") || !strings.Contains(meta, "ready") || !strings.Contains(meta, "atmux-lhh") {
		t.Fatalf("metadata line missing fields: %q", meta)
	}
	if !strings.HasPrefix(title, "    ") {
		t.Fatalf("expected indented title line, got %q", title)
	}
	if strings.Contains(meta, "detach") {
		t.Fatalf("metadata line should not include title: %q", meta)
	}
	if lipgloss.Width(title) > model.width {
		t.Fatalf("title line width %d exceeds model width %d", lipgloss.Width(title), model.width)
	}
}

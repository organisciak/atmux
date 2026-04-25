package tui

import (
	"os"
	"path/filepath"
	"testing"
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

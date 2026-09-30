package tmux

import "testing"

func TestParsePaneLineKeepsColonsInTitle(t *testing.T) {
	pane, ok := parsePaneLine("work:0", "%12\t1\t✳ Fix: login bug\t2.1.283\t1\t120\t40")
	if !ok {
		t.Fatal("line with a colon in the title was rejected")
	}
	if pane.Title != "✳ Fix: login bug" || pane.Command != "2.1.283" {
		t.Fatalf("title/command = %q/%q", pane.Title, pane.Command)
	}
	if pane.ID != "%12" || pane.Index != 1 || !pane.Active || pane.Width != 120 || pane.Height != 40 {
		t.Fatalf("unexpected pane %+v", pane)
	}
	if pane.Target != "work:0.1" {
		t.Fatalf("target = %q", pane.Target)
	}
	if !isClaudePane(pane) {
		t.Fatal("pane should still be detected as Claude")
	}
}

func TestParsePaneLineRejectsShortLines(t *testing.T) {
	if _, ok := parsePaneLine("work:0", "%12\t1\ttitle"); ok {
		t.Fatal("short line accepted")
	}
}
